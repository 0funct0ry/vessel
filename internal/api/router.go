// Package api implements the Vessel HTTP API (Gin router and handlers).
package api

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/0funct0ry/vessel/internal/auth"
	"github.com/0funct0ry/vessel/internal/store"
	"github.com/0funct0ry/vessel/internal/version"
)

// Config supplies the runtime dependencies and routing options for the API.
type Config struct {
	Docker      DockerClient
	ReadOnly    bool
	BasePath    string
	Logger      *slog.Logger
	Store       store.Store
	AuthEnabled bool
	Tokens      *auth.Tokens
}

type server struct {
	docker   DockerClient
	stats    *statsHub
	store    store.Store
	tokens   *auth.Tokens
	tickets  *auth.Tickets
	throttle *auth.Throttle
	basePath string
}

// NewRouter builds the Gin engine and registers the v1 HTTP API.
func NewRouter(cfg Config) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	r := gin.New()
	r.Use(requestIDMiddleware())
	r.Use(accessLogMiddleware(logger))
	r.Use(recoveryMiddleware(logger))
	r.Use(readOnlyMiddleware(cfg.ReadOnly))

	basePath := normalizeBasePath(cfg.BasePath)
	s := &server{docker: cfg.Docker, stats: newStatsHub(cfg.Docker), store: cfg.Store, tokens: cfg.Tokens, tickets: auth.NewTickets(), throttle: auth.NewThrottle(), basePath: basePath}
	v1 := r.Group(basePath + "/api/v1")
	v1.GET("/health", handleHealth)
	v1.GET("/version", handleVersion)
	v1.POST("/auth/login", s.handleLogin)
	if cfg.AuthEnabled {
		v1.Use(s.authMiddleware())
	}
	v1.Use(s.roleMiddleware())
	v1.POST("/auth/logout", s.handleLogout)
	v1.GET("/auth/me", s.handleMe)
	v1.POST("/auth/ws-ticket", s.handleWSTicket)
	v1.GET("/host", s.handleHost)
	v1.GET("/containers", s.handleContainers)
	v1.GET("/containers/:id", s.handleContainer)
	v1.GET("/containers/:id/logs", s.handleContainerLogs)
	v1.GET("/containers/:id/stats", s.handleContainerStats)
	v1.GET("/containers/:id/top", s.handleContainerTop)
	v1.POST("/containers/:id/:action", s.handleContainerLifecycle)
	v1.DELETE("/containers/:id", s.handleContainerRemove)
	v1.GET("/images", s.handleImages)
	v1.GET("/images/:id", s.handleImage)
	v1.POST("/images/pull", s.handleImagePull)
	v1.POST("/images/:id/tag", s.handleImageTag)
	v1.DELETE("/images/:id", s.handleImageRemove)
	v1.GET("/volumes", s.handleVolumes)
	v1.POST("/volumes", s.handleVolumeCreate)
	v1.GET("/volumes/:name", s.handleVolume)
	v1.DELETE("/volumes/:name", s.handleVolumeRemove)
	v1.GET("/networks", s.handleNetworks)
	v1.POST("/networks", s.handleNetworkCreate)
	v1.GET("/networks/:id", s.handleNetwork)
	v1.DELETE("/networks/:id", s.handleNetworkRemove)
	v1.POST("/networks/:id/connect", s.handleNetworkConnect)
	v1.POST("/networks/:id/disconnect", s.handleNetworkDisconnect)
	v1.POST("/prune/:kind", s.handlePrune)

	return r
}

func normalizeBasePath(basePath string) string {
	basePath = strings.TrimSpace(basePath)
	if basePath == "" || basePath == "/" {
		return ""
	}
	return "/" + strings.Trim(basePath, "/")
}

func handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func handleVersion(c *gin.Context) {
	c.JSON(http.StatusOK, version.Get())
}
