package sessions

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestSendAndInterruptAreGenericRoutes proves a turn is sent and interrupted
// through the generic session routes, and that the opencode-prefixed aliases
// for the same operations are gone.
func TestSendAndInterruptAreGenericRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	env := newTestEnv(t)
	router := gin.New()
	env.svc.RegisterRoutes(router)
	env.svc.RegisterOpencodeRoutes(router)

	generic := fmt.Sprintf("/projects/%d/sessions/missing/", env.projectID)
	for _, suffix := range []string{"messages", "interrupt"} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, generic+suffix, strings.NewReader(`{"text":"hi"}`)))
		if rec.Code == http.StatusNotFound && !strings.Contains(rec.Body.String(), "session_not_found") {
			t.Errorf("generic %s route is not registered: %d %s", suffix, rec.Code, rec.Body.String())
		}
	}

	for _, suffix := range []string{"prompt", "interrupt"} {
		path := fmt.Sprintf("/projects/%d/opencode/sessions/missing/%s", env.projectID, suffix)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`)))
		if rec.Code != http.StatusNotFound {
			t.Errorf("legacy opencode %s route = %d, want 404 (the generic route is the only one)", suffix, rec.Code)
		}
	}
}
