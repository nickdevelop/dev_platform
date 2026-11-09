// Gzip response compression for the platform's HTTP service.
//
// The workbench transfers a few response shapes whose size is dominated by
// text: full session history JSON (tens to hundreds of KB), the settings and
// project payloads, and the built frontend bundles. None of them were
// compressed before, so the browser paid for every byte even though it always
// advertises gzip.
//
// The middleware is deliberately conservative. It decides at the first body
// write, after Gin's renderer has set the final Content-Type, and skips
// everything where re-encoding would be wrong or wasteful:
//
//   - Server-Sent Events (the stream must stay unbuffered frame by frame),
//   - WebSocket upgrades (the response writer is hijacked),
//   - Range/partial responses (206 has byte-range semantics),
//   - empty or already encoded bodies,
//   - images, fonts and other binary payloads,
//   - tiny files with a known Content-Length.
//
// It sets Vary so shared caches key on Accept-Encoding, and implements Unwrap
// so http.ResponseController keeps reaching the underlying connection (the SSE
// handlers clear the server write deadline that way).
package http

import (
	"compress/gzip"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

// minCompressBytes is the smallest known body worth compressing. A gzip stream
// adds ~20 bytes of framing, so tiny assets are returned as-is. Dynamic JSON
// usually has no Content-Length yet; it is compressed without buffering it to
// find out whether it is small.
const minCompressBytes = 1024

// gzipWriterPool reuses compressors across requests. gzip.Writer is not
// goroutine-safe, but each request owns one instance for its whole lifetime.
var gzipWriterPool = sync.Pool{
	New: func() any { return gzip.NewWriter(io.Discard) },
}

// Compress returns the response compression middleware. It must be installed
// before the handlers (and before Recovery) so every write goes through the
// wrapper, while AccessLog still observes the original writer's status after
// the wrapper flushes.
func Compress() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !acceptsGzip(c.GetHeader("Accept-Encoding")) || isUpgradeRequest(c.Request) || c.GetHeader("Range") != "" {
			c.Next()
			return
		}

		original := c.Writer
		writer := &compressWriter{
			ResponseWriter: original,
			status:         http.StatusOK,
		}
		c.Writer = writer
		defer func() {
			writer.close()
			c.Writer = original
		}()
		c.Next()
	}
}

// compressWriter buffers only the decision, never the body. WriteHeader records
// the status; the first body write picks identity or gzip and sends headers once.
type compressWriter struct {
	gin.ResponseWriter
	status      int
	decided     bool
	compress    bool
	wroteHeader bool
	gz          *gzip.Writer
}

// WriteHeader records the status without sending it: the content type may not
// be known until the renderer writes the body.
func (w *compressWriter) WriteHeader(code int) {
	if code <= 0 || w.wroteHeader {
		return
	}
	w.status = code
}

// Status reports the status recorded for this response, so middleware that
// inspects it before the body is written does not see a stale 200.
func (w *compressWriter) Status() int {
	return w.status
}

// WriteHeaderNow flushes headers without a body. A handler that flushes before
// writing (notably SSE) is answering in identity encoding by definition.
func (w *compressWriter) WriteHeaderNow() {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(w.status)
	w.ResponseWriter.WriteHeaderNow()
}

func (w *compressWriter) Write(p []byte) (int, error) {
	w.writeHeader()
	if w.compress {
		return w.gz.Write(p)
	}
	return w.ResponseWriter.Write(p)
}

func (w *compressWriter) WriteString(s string) (int, error) {
	w.writeHeader()
	if w.compress {
		return w.gz.Write([]byte(s))
	}
	return w.ResponseWriter.WriteString(s)
}

// Flush keeps streaming semantics: an active gzip stream is flushed before the
// underlying writer, so SSE is never wrapped here (it takes the identity path).
func (w *compressWriter) Flush() {
	if !w.wroteHeader {
		w.WriteHeaderNow()
	}
	if w.compress && w.gz != nil {
		_ = w.gz.Flush()
	}
	w.ResponseWriter.Flush()
}

// Unwrap lets http.ResponseController reach the real connection. The SSE
// handlers depend on this to clear the server's absolute write deadline.
func (w *compressWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *compressWriter) writeHeader() {
	if w.wroteHeader {
		return
	}
	if !w.decided {
		w.decide()
	}
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(w.status)
	w.ResponseWriter.WriteHeaderNow()
}

// close finalizes the response. A handler that wrote no body still gets its
// status flushed; an open gzip stream gets its trailer and returns to the pool.
func (w *compressWriter) close() {
	if !w.wroteHeader {
		w.WriteHeaderNow()
	}
	if w.gz != nil {
		_ = w.gz.Close()
		w.gz.Reset(io.Discard)
		gzipWriterPool.Put(w.gz)
		w.gz = nil
	}
}

func (w *compressWriter) decide() {
	w.decided = true
	if w.status < http.StatusOK || w.status >= http.StatusMultipleChoices ||
		w.status == http.StatusNoContent || w.status == http.StatusResetContent ||
		w.status == http.StatusPartialContent {
		return
	}

	header := w.Header()
	if encoding := header.Get("Content-Encoding"); encoding != "" && !strings.EqualFold(encoding, "identity") {
		return
	}
	// Compressing a response with a strong validator would invalidate it; the
	// platform does not set ETags today, but a proxied upstream might.
	if header.Get("ETag") != "" || hasNoTransform(header) {
		return
	}
	if !compressibleContentType(header.Get("Content-Type")) {
		return
	}
	if raw := header.Get("Content-Length"); raw != "" {
		size, err := strconv.ParseInt(raw, 10, 64)
		if err == nil && size < minCompressBytes {
			return
		}
	}

	w.compress = true
	header.Set("Content-Encoding", "gzip")
	header.Del("Content-Length")
	// A compressed resource is not byte-addressable with the same offsets.
	header.Del("Accept-Ranges")
	addVary(header, "Accept-Encoding")

	w.gz = gzipWriterPool.Get().(*gzip.Writer)
	w.gz.Reset(w.ResponseWriter)
}

// acceptsGzip parses Accept-Encoding, honoring q=0 for gzip and the wildcard.
// An explicit gzip entry wins over "*" as RFC 9110 requires.
func acceptsGzip(header string) bool {
	if header == "" {
		return false
	}
	explicit, wildcard := false, false
	explicitQ, wildcardQ := 0.0, 0.0
	for _, part := range strings.Split(header, ",") {
		fields := strings.Split(part, ";")
		coding := strings.ToLower(strings.TrimSpace(fields[0]))
		q := 1.0
		for _, parameter := range fields[1:] {
			keyValue := strings.SplitN(strings.TrimSpace(parameter), "=", 2)
			if len(keyValue) != 2 || !strings.EqualFold(strings.TrimSpace(keyValue[0]), "q") {
				continue
			}
			if parsed, err := strconv.ParseFloat(strings.TrimSpace(keyValue[1]), 64); err == nil {
				q = parsed
			}
		}
		switch coding {
		case "gzip":
			explicit, explicitQ = true, q
		case "*":
			wildcard, wildcardQ = true, q
		}
	}
	if explicit {
		return explicitQ > 0
	}
	return wildcard && wildcardQ > 0
}

// isUpgradeRequest reports a protocol switch. Compressing the handshake would
// wrap the ResponseWriter that Upgrade hijacks.
func isUpgradeRequest(request *http.Request) bool {
	if request.Header.Get("Upgrade") != "" {
		return true
	}
	for _, value := range request.Header.Values("Connection") {
		for _, token := range strings.Split(value, ",") {
			if strings.EqualFold(strings.TrimSpace(token), "upgrade") {
				return true
			}
		}
	}
	return false
}

// compressibleContentType accepts textual representations. SSE is excluded
// explicitly: it is text, but it must not be buffered by a compressor.
func compressibleContentType(value string) bool {
	mediaType, _, err := mime.ParseMediaType(value)
	if err != nil || mediaType == "" {
		return false
	}
	if mediaType == "text/event-stream" {
		return false
	}
	if strings.HasPrefix(mediaType, "text/") {
		return true
	}
	if strings.HasSuffix(mediaType, "+json") || strings.HasSuffix(mediaType, "+xml") {
		return true
	}
	switch mediaType {
	case "application/json", "application/javascript", "application/xml",
		"application/xhtml+xml", "application/x-ndjson", "image/svg+xml",
		"application/wasm":
		return true
	}
	return false
}

// hasNoTransform honors an explicit intermediary prohibition.
func hasNoTransform(header http.Header) bool {
	for _, value := range header.Values("Cache-Control") {
		for _, directive := range strings.Split(value, ",") {
			if strings.EqualFold(strings.TrimSpace(directive), "no-transform") {
				return true
			}
		}
	}
	return false
}

// addVary adds Accept-Encoding once, preserving any directives already present.
func addVary(header http.Header, value string) {
	for _, existing := range header.Values("Vary") {
		for _, item := range strings.Split(existing, ",") {
			item = strings.TrimSpace(item)
			if item == "*" || strings.EqualFold(item, value) {
				return
			}
		}
	}
	header.Add("Vary", value)
}
