package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/gin-gonic/gin"

	"github.com/0funct0ry/vessel/internal/auth"
	"github.com/0funct0ry/vessel/internal/dockerapi"
	"github.com/0funct0ry/vessel/internal/store"
)

const execIdleTimeout = 15 * time.Minute

type execControl struct {
	Type    string `json:"type"`
	Message string `json:"message,omitempty"`
	Cols    int    `json:"cols,omitempty"`
	Rows    int    `json:"rows,omitempty"`
}

func (s *server) handleContainerExec(c *gin.Context) {
	if !sameOrigin(c.Request) {
		c.AbortWithStatusJSON(http.StatusForbidden, errorEnvelope{Error: errorBody{Code: "invalid_origin", Message: "WebSocket origin must match this host"}})
		return
	}
	var username string
	if s.authOn {
		claims, ok := s.tickets.Consume(c.Query("ticket"))
		if !ok {
			authFailure(c, "invalid_token")
			return
		}
		if !auth.Allows(claims.Role, store.RoleOperator) {
			c.AbortWithStatusJSON(http.StatusForbidden, errorEnvelope{Error: errorBody{Code: "forbidden_role", Message: "insufficient role", Required: store.RoleOperator, Actual: claims.Role}})
			return
		}
		username = claims.Name
	} else {
		username = "auth-off"
	}

	command := strings.TrimSpace(c.DefaultQuery("cmd", "/bin/sh"))
	if command == "" {
		command = "/bin/sh"
	}
	argv := strings.Fields(command)
	if len(argv) == 0 {
		Fail(c, invalidQuery("cmd must not be empty"))
		return
	}
	tty := c.DefaultQuery("tty", "1") != "0"
	user := c.Query("user")

	execID, effective, fallback, err := s.createExec(c.Request.Context(), c.Param("id"), argv, user, tty)
	if err != nil {
		Fail(c, forResource("container", c.Param("id"), err))
		return
	}
	session, err := s.docker.StartExec(c.Request.Context(), execID, tty)
	if err != nil {
		Fail(c, err)
		return
	}
	defer session.Close()

	ws, err := websocket.Accept(c.Writer, c.Request, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	defer func() { _ = ws.CloseNow() }()
	s.logger.Info("console session started", "user", username, "container", c.Param("id"), "cmd", effective)
	if fallback != "" {
		_ = writeExecControl(c.Request.Context(), ws, "using "+fallback)
	}
	s.proxyExec(c.Request.Context(), ws, session, execID, c.Param("id"))
}

func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return false
	}
	u, err := url.Parse(origin)
	return err == nil && u.Host != "" && strings.EqualFold(u.Host, r.Host)
}

func (s *server) createExec(ctx context.Context, container string, argv []string, user string, tty bool) (id, effective, fallback string, err error) {
	commands := [][]string{argv}
	for _, shell := range []string{"/bin/bash", "/bin/sh", "/bin/ash"} {
		if argv[0] != shell {
			commands = append(commands, []string{shell})
		}
	}
	for i, cmd := range commands {
		id, err = s.docker.CreateExec(ctx, container, dockerapi.ExecOptions{Cmd: cmd, User: user, TTY: tty})
		if err == nil {
			effective = strings.Join(cmd, " ")
			if i > 0 {
				fallback = cmd[0]
			}
			return id, effective, fallback, nil
		}
		if !strings.Contains(strings.ToLower(err.Error()), "no such file") {
			return "", "", "", err
		}
	}
	return "", "", "", err
}

func (s *server) proxyExec(parent context.Context, ws *websocket.Conn, session dockerapi.ExecSession, execID, container string) {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	activity := make(chan struct{}, 1)
	touch := func() {
		select {
		case activity <- struct{}{}:
		default:
		}
	}
	done := make(chan string, 2)
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := session.Read(buf)
			if n > 0 {
				touch()
				if werr := ws.Write(ctx, websocket.MessageText, buf[:n]); werr != nil {
					done <- "client disconnected"
					return
				}
			}
			if err != nil {
				if errors.Is(err, io.EOF) {
					done <- s.execEndReason(ctx, container)
				} else {
					done <- "console stream closed"
				}
				return
			}
		}
	}()
	go func() {
		for {
			typ, data, err := ws.Read(ctx)
			if err != nil {
				done <- "client disconnected"
				return
			}
			if typ != websocket.MessageText {
				continue
			}
			touch()
			var control execControl
			if json.Unmarshal(data, &control) == nil && control.Type == "resize" && control.Cols > 0 && control.Rows > 0 {
				if err := s.docker.ResizeExec(ctx, execID, control.Cols, control.Rows); err != nil {
					done <- "resize failed"
					return
				}
				continue
			}
			if _, err := session.Write(data); err != nil {
				done <- "console stream closed"
				return
			}
		}
	}()
	timer := time.NewTimer(execIdleTimeout)
	defer timer.Stop()
	for {
		select {
		case reason := <-done:
			if reason != "client disconnected" {
				_ = writeExecControl(context.Background(), ws, reason)
				_ = ws.Close(websocket.StatusNormalClosure, reason)
			}
			return
		case <-activity:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(execIdleTimeout)
		case <-timer.C:
			_ = writeExecControl(context.Background(), ws, "session closed after 15 minutes of inactivity")
			_ = ws.Close(websocket.StatusPolicyViolation, "idle timeout")
			return
		case <-ctx.Done():
			return
		}
	}
}

func (s *server) execEndReason(ctx context.Context, container string) string {
	detail, err := s.docker.InspectContainer(ctx, container)
	if err == nil && detail.State != "running" {
		return "container exited, session closed"
	}
	return "console session closed"
}

func writeExecControl(ctx context.Context, ws *websocket.Conn, message string) error {
	b, _ := json.Marshal(execControl{Type: "info", Message: message})
	return ws.Write(ctx, websocket.MessageText, b)
}
