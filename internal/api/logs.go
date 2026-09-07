package api

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/0funct0ry/vessel/internal/dockerapi"
)

const logBufferLines = 2000

func parseLogsOptions(c *gin.Context) (dockerapi.LogsOptions, error) {
	follow, err := queryBool(c, "follow")
	if err != nil {
		return dockerapi.LogsOptions{}, invalidQuery("follow must be a boolean")
	}
	timestamps, err := queryBool(c, "timestamps")
	if err != nil {
		return dockerapi.LogsOptions{}, invalidQuery("timestamps must be a boolean")
	}
	opts := dockerapi.LogsOptions{Follow: follow, Tail: "500", Timestamps: timestamps, Stdout: true, Stderr: true}
	if raw, ok := c.GetQuery("tail"); ok {
		if raw != "all" {
			n, err := strconv.Atoi(raw)
			if err != nil || n < 0 {
				return dockerapi.LogsOptions{}, invalidQuery("tail must be a non-negative integer or all")
			}
		}
		opts.Tail = raw
	}
	for _, field := range []string{"since", "until"} {
		raw, ok := c.GetQuery(field)
		if !ok || raw == "" {
			continue
		}
		normalized, err := normalizeLogTime(raw)
		if err != nil {
			return dockerapi.LogsOptions{}, invalidQuery("%s must be RFC3339 or Unix seconds", field)
		}
		if field == "since" {
			opts.Since = normalized
		} else {
			opts.Until = normalized
		}
	}
	if stream, ok := c.GetQuery("stream"); ok && stream != "" {
		switch stream {
		case "stdout":
			opts.Stderr = false
		case "stderr":
			opts.Stdout = false
		case "both":
		default:
			return dockerapi.LogsOptions{}, invalidQuery("stream must be stdout, stderr, or both")
		}
	}
	return opts, nil
}

func normalizeLogTime(raw string) (string, error) {
	if unix, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return strconv.FormatInt(unix, 10), nil
	}
	t, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return "", err
	}
	return t.UTC().Format(time.RFC3339Nano), nil
}

func streamName(stream dockerapi.StreamType) string {
	if stream == dockerapi.StreamStderr {
		return "stderr"
	}
	return "stdout"
}

func (s *server) handleContainerLogs(c *gin.Context) {
	opts, err := parseLogsOptions(c)
	if err != nil {
		Fail(c, err)
		return
	}
	plain := !opts.Follow && strings.Contains(c.GetHeader("Accept"), "text/plain")
	if plain {
		s.handleLogDownload(c, opts)
		return
	}

	// SSE needs a timestamp even when the caller does not want timestamps in a
	// plain-text download, so always request them from Docker here.
	opts.Timestamps = true
	StreamWithOverflow(c, logBufferLines, func(dropped int) (string, any) {
		return "log", map[string]any{"stream": "vessel", "line": fmt.Sprintf("[vessel] dropped %d lines", dropped)}
	}, func(send func(string, any) error) error {
		reader, err := s.docker.LogStream(c.Request.Context(), c.Param("id"), opts)
		if err != nil {
			return forResource("container", c.Param("id"), err)
		}
		defer reader.Close()
		for {
			line, err := reader.Next()
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return err
			}
			if err := send("log", map[string]string{
				"ts": line.Time.UTC().Format(time.RFC3339Nano), "stream": streamName(line.Stream), "line": line.Text,
			}); err != nil {
				return err
			}
		}
	})
}

func (s *server) handleLogDownload(c *gin.Context, opts dockerapi.LogsOptions) {
	id := c.Param("id")
	container, err := s.docker.InspectContainer(c.Request.Context(), id)
	if err != nil {
		Fail(c, forResource("container", id, err))
		return
	}
	reader, err := s.docker.LogStream(c.Request.Context(), id, opts)
	if err != nil {
		Fail(c, forResource("container", id, err))
		return
	}
	defer reader.Close()

	name := strings.TrimPrefix(container.Name, "/")
	if name == "" {
		name = id
	}
	filename := name + "-" + time.Now().UTC().Format("20060102T150405Z") + ".log"
	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
	c.Status(http.StatusOK)
	for {
		line, err := reader.Next()
		if err == io.EOF {
			return
		}
		if err != nil {
			return
		}
		if opts.Timestamps {
			_, _ = fmt.Fprintf(c.Writer, "%s %s\n", line.Time.UTC().Format(time.RFC3339Nano), line.Text)
		} else {
			_, _ = fmt.Fprintln(c.Writer, line.Text)
		}
		c.Writer.Flush()
	}
}
