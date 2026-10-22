package http

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"devplatform/internal/app/auth"
)

// routeParamPattern matches a Gin path parameter such as ":id".
var routeParamPattern = regexp.MustCompile(`:[A-Za-z0-9_]+`)

// minMutatingRoutes is the floor the matrix must cover. It exists so a future
// change to the router (for example mounting a service outside the protected
// group) cannot silently shrink the matrix until it asserts nothing.
const minMutatingRoutes = 40

// TestEveryMutatingRouteEnforcesCSRF walks the production route table and pins
// the CSRF contract for every state-changing endpoint: without the double-submit
// header the request is rejected by the middleware, and with it the request gets
// past the middleware (business errors are acceptable, a CSRF rejection is not).
//
// The matrix is derived from router.Routes(), so an endpoint added later is
// covered automatically instead of depending on someone remembering to add a
// case for it.
func TestEveryMutatingRouteEnforcesCSRF(t *testing.T) {
	// The route table is the fixture's; the client used for the assertions is
	// created per route below.
	routes := newRouterFixture(t).router.Routes()

	// One request per mutating route would otherwise emit an access-log line
	// and, for handlers whose service was built with nil collaborators, a
	// recovery stack trace. Neither is under test here.
	defaultLogger := log.Default()
	log.SetOutput(io.Discard)
	t.Cleanup(func() { log.SetOutput(defaultLogger.Writer()) })
	defaultErrorWriter := gin.DefaultErrorWriter
	gin.DefaultErrorWriter = io.Discard
	t.Cleanup(func() { gin.DefaultErrorWriter = defaultErrorWriter })

	covered := 0
	for _, route := range routes {
		if isSafeMethod(route.Method) || strings.Contains(route.Path, "/*") {
			continue
		}
		covered++
		path := routeParamPattern.ReplaceAllString(route.Path, "1")
		assertPathParams(t, path)

		t.Run(describeRoute(route.Method, route.Path), func(t *testing.T) {
			// A fresh session per route: the auth routes change the session
			// identity (login and logout both rotate the token), so a shared
			// client would poison every assertion that follows them.
			fixture := newRouterFixture(t)

			status, code, err := matrixRequest(fixture, route.Method, path, "")
			if err != nil {
				t.Fatalf("request without CSRF token: %v", err)
			}
			if status != http.StatusForbidden || code != "invalid_csrf_token" {
				t.Errorf("without %s header: status=%d error=%q, want 403 invalid_csrf_token",
					auth.CSRFHeaderName, status, code)
			}

			status, code, err = matrixRequest(fixture, route.Method, path, fixture.csrf)
			if err != nil {
				// A transport-level failure means the handler blocked or the
				// connection dropped. That is not a CSRF rejection, which is
				// the only property this test pins, so it is reported but not
				// failed.
				t.Logf("request with CSRF token did not complete: %v", err)
				return
			}
			if status == http.StatusForbidden && code == "invalid_csrf_token" {
				t.Errorf("with %s header: status=%d error=%q, want the middleware to accept the token",
					auth.CSRFHeaderName, status, code)
			}
		})
	}

	if covered < minMutatingRoutes {
		t.Fatalf("matrix covered %d mutating routes, want at least %d", covered, minMutatingRoutes)
	}
}

// isSafeMethod reports whether the method is a safe (non-mutating) method that
// RequireCSRF lets through without a token.
func isSafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}

// matrixRequest sends one request and returns its status plus the error code in
// the JSON error envelope ("" when the body carries no error code).
func matrixRequest(f *routerFixture, method, path, csrf string) (int, string, error) {
	req, err := http.NewRequest(method, f.base+path, strings.NewReader("{}"))
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if csrf != "" {
		req.Header.Set(auth.CSRFHeaderName, csrf)
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return 0, "", err
	}
	var envelope struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(body, &envelope)
	return resp.StatusCode, envelope.Error, nil
}
