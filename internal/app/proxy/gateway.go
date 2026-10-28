package proxy

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// RegisterGateway mounts the transparent proxy gateway onto the Gin engine.
// It uses a single wildcard route /proxy/*any so all subpaths and methods are captured.
// It is public and completely unauthenticated, allowing direct demo sharing.
func RegisterGateway(router *gin.Engine, service *Service) {
	router.Any("/proxy/*any", createGatewayHandler(service))
}

func createGatewayHandler(service *Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := strings.TrimPrefix(c.Param("any"), "/")
		if raw == "" {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "proxy_alias_required"})
			return
		}

		parts := strings.SplitN(raw, "/", 2)
		alias := strings.ToLower(parts[0])

		// If accessed without trailing slash (e.g. /proxy/demo), redirect to /proxy/demo/
		// so relative links in HTML/JS resolve correctly.
		if len(parts) == 1 && !strings.HasSuffix(c.Request.URL.Path, "/") {
			target := "/proxy/" + alias + "/"
			if c.Request.URL.RawQuery != "" {
				target += "?" + c.Request.URL.RawQuery
			}
			c.Redirect(http.StatusMovedPermanently, target)
			return
		}

		p, err := service.GetByAlias(alias)
		if err != nil || !p.Enabled {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "proxy_not_found_or_disabled"})
			return
		}

		subpath := "/"
		if len(parts) == 2 {
			subpath = "/" + parts[1]
		}

		switch p.Type {
		case TypeDirectory:
			serveDirectory(c, p)
		case TypePort:
			servePort(c, p, subpath)
		default:
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "unsupported_proxy_type"})
		}
	}
}

func serveDirectory(c *gin.Context, p ServiceProxy) {
	prefix := "/proxy/" + p.Alias
	fs := http.StripPrefix(prefix, http.FileServer(http.Dir(p.Target)))
	fs.ServeHTTP(c.Writer, c.Request)
}

func servePort(c *gin.Context, p ServiceProxy, subpath string) {
	// If the request is a WebSocket upgrade, handle it with raw TCP hijacking
	if isWebSocketRequest(c.Request) {
		handleWebSocketProxy(c, p, subpath)
		return
	}

	targetURL, err := url.Parse(fmt.Sprintf("http://127.0.0.1:%s", p.Target))
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "invalid_proxy_target"})
		return
	}

	prefix := "/proxy/" + p.Alias
	revProxy := httputil.NewSingleHostReverseProxy(targetURL)

	originalDirector := revProxy.Director
	revProxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.URL.Path = subpath
		req.Host = targetURL.Host

		// Set forwarding headers
		req.Header.Set("X-Forwarded-Prefix", prefix)
		if clientIP, _, err := net.SplitHostPort(req.RemoteAddr); err == nil {
			if prior := req.Header.Get("X-Forwarded-For"); prior != "" {
				clientIP = prior + ", " + clientIP
			}
			req.Header.Set("X-Forwarded-For", clientIP)
		}
	}

	revProxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, proxyErr error) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(fmt.Sprintf(`{"error":"bad_gateway","details":%q}`, proxyErr.Error())))
	}

	revProxy.ServeHTTP(c.Writer, c.Request)
}

func isWebSocketRequest(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket") &&
		strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade")
}

func handleWebSocketProxy(c *gin.Context, p ServiceProxy, subpath string) {
	backendAddr := fmt.Sprintf("127.0.0.1:%s", p.Target)
	backendConn, err := net.DialTimeout("tcp", backendAddr, 5*time.Second)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadGateway, gin.H{"error": "bad_gateway", "details": err.Error()})
		return
	}
	defer backendConn.Close()

	hijacker, ok := c.Writer.(http.Hijacker)
	if !ok {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "hijacking_not_supported"})
		return
	}

	clientConn, clientBuf, err := hijacker.Hijack()
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "hijack_failed", "details": err.Error()})
		return
	}
	defer clientConn.Close()

	c.Request.URL.Path = subpath

	if err := c.Request.Write(backendConn); err != nil {
		return
	}

	// Bi-directional pipe
	errCh := make(chan error, 2)
	go func() {
		if clientBuf.Reader.Buffered() > 0 {
			buffered := make([]byte, clientBuf.Reader.Buffered())
			_, _ = clientBuf.Reader.Read(buffered)
			_, _ = backendConn.Write(buffered)
		}
		_, err := io.Copy(backendConn, clientConn)
		errCh <- err
	}()

	go func() {
		_, err := io.Copy(clientConn, backendConn)
		errCh <- err
	}()

	<-errCh
}
