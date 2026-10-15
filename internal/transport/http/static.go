// Production hosting of the built web UI. The Go service serves the Vite
// build output from disk so the frontend and the API share one origin in
// production: no CORS, cookies always first-party, and one process to run.
//
// Serving rules:
//   - /api routes always take precedence; unknown /api paths are JSON 404s,
//     never the SPA entry (so API clients get API errors),
//   - hashed Vite bundles under /assets are served with an immutable cache
//     lifetime; a missing asset is a 404, not HTML, so broken references
//     are visible,
//   - any other unknown path serves an exact file when one exists (favicon,
//     images) and falls back to the SPA entry otherwise, so client-side
//     routes like /login survive a hard refresh,
//   - file access is confined to the configured web directory; traversal
//     attempts cannot escape it.
package http

import (
	"io"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/gin-gonic/gin"
)

// init corrects the media type Go's built-in table gives .woff2 (font/woff).
// The built-in font slices are served from /assets, and a face served under the
// wrong type is a face a browser may refuse. The registration cannot fail for a
// fixed extension and a valid type, so its error is not worth branching on.
func init() {
	_ = mime.AddExtensionType(".woff2", "font/woff2")
}

const (
	// indexCacheControl makes browsers revalidate the SPA entry so a
	// redeploy is picked up on the next load.
	indexCacheControl = "no-cache"
	// assetCacheControl lets browsers keep hashed asset bundles forever;
	// their names change with every build.
	assetCacheControl = "public, max-age=31536000, immutable"
)

// serveStatic mounts the built frontend from webDir onto the router. It is
// called by NewRouter when Dependencies.WebDir is set, after every /api
// route is registered, so static serving can never shadow the API.
func serveStatic(router *gin.Engine, webDir string) {
	webFS := os.DirFS(webDir)
	fileServer := http.FileServerFS(webFS)

	// Vite emits content-hashed bundles under assets/. The URL path maps
	// directly onto the web directory layout (webFS/assets/...), so the
	// file server consumes the request path unmodified. Only files are
	// served: a directory request (notably /assets/ itself) is a 404 rather
	// than an http.FileServer directory listing that would expose build
	// filenames.
	assets := router.Group("/assets", cacheControl(assetCacheControl))
	assets.GET("/*filepath", gin.WrapH(onlyFiles(webFS, fileServer)))

	// Everything no registered route claimed.
	router.NoRoute(func(c *gin.Context) {
		if isAPIPath(c.Request.URL.Path) {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			c.AbortWithStatusJSON(http.StatusMethodNotAllowed, gin.H{"error": "method_not_allowed"})
			return
		}

		// Serve an exact file when it exists inside the web directory. The
		// leading "/" in Clean pins the result inside the root, so a
		// traversal like /../secret resolves to a harmless in-root name.
		name := strings.TrimPrefix(path.Clean("/"+c.Request.URL.Path), "/")
		if name != "" && name != "." {
			if info, err := fs.Stat(webFS, name); err == nil && !info.IsDir() {
				fileServer.ServeHTTP(c.Writer, c.Request)
				return
			}
		}
		serveIndex(webFS)(c)
	})
}

// onlyFiles wraps an fs-backed handler so a request that resolves to a
// directory is a 404 instead of a directory listing. It is used for the
// hashed-asset subtree, which contains only files.
func onlyFiles(webFS fs.FS, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if name != "" && name != "." {
			if info, err := fs.Stat(webFS, name); err == nil && info.IsDir() {
				http.NotFound(w, r)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// serveIndex writes the SPA entry (index.html), the fallback target for all
// client-side routes.
func serveIndex(webFS fs.FS) gin.HandlerFunc {
	return func(c *gin.Context) {
		file, err := webFS.Open("index.html")
		if err != nil {
			// The configured web directory has no built UI; say so instead
			// of serving an empty shell.
			c.String(http.StatusNotFound, "web UI assets not found; build the frontend or point -web-dir at a built UI")
			return
		}
		defer file.Close()

		c.Header("Cache-Control", indexCacheControl)
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.Status(http.StatusOK)
		_, _ = io.Copy(c.Writer, file)
	}
}

// cacheControl stamps every response it wraps with the given Cache-Control.
func cacheControl(value string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", value)
		c.Next()
	}
}

// isAPIPath reports whether a request path belongs to the API surface.
func isAPIPath(p string) bool {
	return p == "/api" || strings.HasPrefix(p, "/api/")
}
