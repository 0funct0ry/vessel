package api

import (
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/gin-gonic/gin"
)

// basePathPlaceholder is templated into index.html's meta tag at build time
// (see web/index.html) and swapped for the real --base-path value here.
const basePathPlaceholder = "__VESSEL_BASE_PATH__"

const notBuiltPage = `<!doctype html>
<html>
<head><meta charset="utf-8"><title>Vessel</title></head>
<body style="font:14px system-ui;padding:40px;color:#12303F">
<h1>UI not built</h1>
<p>UI not built — run <code>make build</code>.</p>
</body>
</html>`

// mountStatic serves the embedded frontend (or, if it was not built into
// this binary, a page saying so) for every path not claimed by basePath +
// "/api/". Unknown non-API GET paths fall back to index.html so client-side
// routing works.
func mountStatic(r *gin.Engine, basePath string, dist fs.FS, distErr error) {
	effectiveBase := basePath
	if effectiveBase == "" {
		effectiveBase = "/"
	}

	if distErr == nil && dist == nil {
		distErr = fs.ErrNotExist
	}
	if distErr != nil {
		r.NoRoute(func(c *gin.Context) {
			if strings.HasPrefix(c.Request.URL.Path, basePath+"/api/") {
				c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "not_found", "message": "not found"}})
				return
			}
			c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(notBuiltPage))
		})
		return
	}

	index, err := fs.ReadFile(dist, "index.html")
	if err != nil {
		r.NoRoute(func(c *gin.Context) {
			c.Data(http.StatusInternalServerError, "text/html; charset=utf-8", []byte(notBuiltPage))
		})
		return
	}
	templatedIndex := []byte(strings.ReplaceAll(string(index), basePathPlaceholder, effectiveBase))

	serveIndex := func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Data(http.StatusOK, "text/html; charset=utf-8", templatedIndex)
	}

	r.NoRoute(func(c *gin.Context) {
		reqPath := c.Request.URL.Path
		if strings.HasPrefix(reqPath, basePath+"/api/") {
			c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "not_found", "message": "not found"}})
			return
		}
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			c.Status(http.StatusMethodNotAllowed)
			return
		}

		rel := strings.TrimPrefix(reqPath, basePath)
		rel = strings.TrimPrefix(rel, "/")
		if rel == "" || rel == "index.html" {
			serveIndex(c)
			return
		}

		f, err := dist.Open(path.Clean(rel))
		if err != nil {
			// Unknown path: fall back to index.html for client-side routing.
			serveIndex(c)
			return
		}
		_ = f.Close()

		c.Header("Cache-Control", "public, max-age=31536000, immutable")
		http.ServeFileFS(c.Writer, c.Request, dist, rel)
	})
}
