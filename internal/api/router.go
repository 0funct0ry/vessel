// Package api implements the Vessel HTTP API (Gin router and handlers).
package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/0funct0ry/vessel/internal/version"
)

// NewRouter builds the Gin engine. In M1 this only registers the no-auth
// health/version routes from SPEC §5.1; the rest of the surface arrives with
// later milestones.
func NewRouter() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	v1 := r.Group("/api/v1")
	v1.GET("/health", handleHealth)
	v1.GET("/version", handleVersion)

	return r
}

func handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func handleVersion(c *gin.Context) {
	c.JSON(http.StatusOK, version.Get())
}
