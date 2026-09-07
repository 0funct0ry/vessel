package api

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
)

const requestIDKey = "request_id"

var requestIDFallback atomic.Uint64

func requestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Request-ID")
		if id == "" {
			id = newRequestID()
		}
		c.Set(requestIDKey, id)
		c.Header("X-Request-ID", id)
		c.Next()
	}
}

func newRequestID() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err == nil {
		return hex.EncodeToString(bytes[:])
	}
	return "fallback-" + time.Now().UTC().Format("20060102T150405.000000000") + "-" +
		strconv.FormatUint(requestIDFallback.Add(1), 10)
}

func accessLogMiddleware(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		started := time.Now()
		c.Next()
		logger.InfoContext(c.Request.Context(), "http request",
			"request_id", c.GetString(requestIDKey),
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"duration", time.Since(started),
		)
	}
}

func recoveryMiddleware(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if recovered := recover(); recovered != nil {
				logger.ErrorContext(c.Request.Context(), "panic serving HTTP request",
					"request_id", c.GetString(requestIDKey), "panic", recovered, "stack", string(debug.Stack()))
				Fail(c, errInternal)
			}
		}()
		c.Next()
	}
}

func readOnlyMiddleware(enabled bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if enabled && c.Request.Method != http.MethodGet {
			c.AbortWithStatusJSON(http.StatusForbidden, errorEnvelope{Error: errorBody{
				Code: "read_only", Message: "Vessel is running in read-only mode",
			}})
			return
		}
		c.Next()
	}
}

type internalError struct{}

func (internalError) Error() string { return "internal server error" }

var errInternal error = internalError{}
