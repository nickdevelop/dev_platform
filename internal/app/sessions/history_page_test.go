package sessions

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"devplatform/internal/core/agents"
)

func historyPageSession(t *testing.T) (*testEnv, Session) {
	t.Helper()
	env := newTestEnv(t)
	return env, createNativeSession(t, env, "native-page")
}

func TestHistoryPagePassesPagingOptions(t *testing.T) {
	env, session := historyPageSession(t)

	env.fake.mu.Lock()
	env.fake.historyResult = agents.HistoryResult{
		Events:     historyEvents(time.Now().UTC(), "older"),
		HasMore:    true,
		NextCursor: "12",
	}
	env.fake.mu.Unlock()

	history, err := env.svc.HistoryPage(context.Background(), env.projectID, session.ID, HistoryOptions{
		Limit:         200,
		Before:        "34",
		IncludeImages: false,
	})
	if err != nil {
		t.Fatalf("HistoryPage: %v", err)
	}
	if !history.HasMore || history.NextCursor != "12" {
		t.Fatalf("HasMore/NextCursor = %v/%q, want true/12", history.HasMore, history.NextCursor)
	}

	env.fake.mu.Lock()
	defer env.fake.mu.Unlock()
	request := env.fake.historyReqs[len(env.fake.historyReqs)-1]
	if request.Limit != 200 || request.Before != "34" {
		t.Errorf("adapter request = %+v, want limit=200 before=34", request)
	}
}

func TestHistoryPageRejectsCursorWithoutLimit(t *testing.T) {
	env, session := historyPageSession(t)

	if _, err := env.svc.HistoryPage(context.Background(), env.projectID, session.ID, HistoryOptions{Before: "12"}); err == nil {
		t.Fatal("HistoryPage accepted a cursor without a limit")
	}
}

func TestHistoryHandlerParsesPagingQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	env, session := historyPageSession(t)
	router := gin.New()
	env.svc.RegisterRoutes(router)

	env.fake.mu.Lock()
	env.fake.historyResult = agents.HistoryResult{
		Events:     historyEvents(time.Now().UTC(), "page"),
		HasMore:    true,
		NextCursor: "40",
	}
	env.fake.mu.Unlock()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf(
		"/projects/%d/sessions/%s/history?limit=200&before=60&includeImages=false",
		env.projectID, session.ID,
	), nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Events     []agents.NormalizedEvent `json:"events"`
		Truncated  bool                     `json:"truncated"`
		HasMore    bool                     `json:"hasMore"`
		NextCursor string                   `json:"nextCursor"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(payload.Events) != 1 || !payload.HasMore || payload.NextCursor != "40" {
		t.Fatalf("payload = %+v, want one event with hasMore and cursor 40", payload)
	}

	env.fake.mu.Lock()
	defer env.fake.mu.Unlock()
	request := env.fake.historyReqs[len(env.fake.historyReqs)-1]
	if request.Limit != 200 || request.Before != "60" {
		t.Errorf("adapter request = %+v, want limit=200 before=60", request)
	}
}

func TestHistoryHandlerRejectsInvalidPaging(t *testing.T) {
	gin.SetMode(gin.TestMode)
	env, session := historyPageSession(t)
	router := gin.New()
	env.svc.RegisterRoutes(router)

	for _, query := range []string{
		"?limit=abc",
		"?limit=-1",
		"?before=12",
		"?limit=10&before=" + strings.Repeat("a", 64),
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf(
			"/projects/%d/sessions/%s/history%s", env.projectID, session.ID, query,
		), nil)
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("query %q status = %d, want 400", query, rec.Code)
		}
	}
}

func TestHistoryHandlerMapsInvalidCursor(t *testing.T) {
	gin.SetMode(gin.TestMode)
	env, session := historyPageSession(t)
	router := gin.New()
	env.svc.RegisterRoutes(router)

	env.fake.mu.Lock()
	env.fake.historyErr = agents.ErrInvalidHistoryCursor
	env.fake.mu.Unlock()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf(
		"/projects/%d/sessions/%s/history?limit=10&before=99",
		env.projectID, session.ID,
	), nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var payload map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload["error"] != "invalid_history_cursor" {
		t.Errorf("error = %q, want invalid_history_cursor", payload["error"])
	}
}

func TestHistoryPageRewritesImagesToURLs(t *testing.T) {
	env, session := historyPageSession(t)
	env.fake.mu.Lock()
	env.fake.historyResult = agents.HistoryResult{Events: []agents.NormalizedEvent{{
		Type:      agents.EventMessage,
		Timestamp: time.Now().UTC(),
		Message: &agents.MessagePayload{
			Role:  "user",
			Text:  "看图",
			Final: true,
			Images: []agents.ImageAttachment{{
				ID:        "msg_1:1",
				Name:      "chart.png",
				MediaType: "image/png",
				Data:      "aGVsbG8=",
			}},
		},
	}}}
	env.fake.mu.Unlock()

	history, err := env.svc.HistoryPage(context.Background(), env.projectID, session.ID, HistoryOptions{
		Limit:         20,
		IncludeImages: false,
	})
	if err != nil {
		t.Fatalf("HistoryPage: %v", err)
	}
	image := history.Events[0].Message.Images[0]
	if image.Data != "" {
		t.Errorf("Data = %q, want it stripped for lazy loading", image.Data)
	}
	if !strings.Contains(image.URL, "/history/images/") || !strings.Contains(image.URL, "msg_1:1") {
		t.Errorf("URL = %q, want the history image endpoint", image.URL)
	}
}

func TestHistoryPageKeepsUnsupportedImageFormatsInline(t *testing.T) {
	env, session := historyPageSession(t)
	env.fake.mu.Lock()
	env.fake.historyResult = agents.HistoryResult{Events: []agents.NormalizedEvent{{
		Type:      agents.EventMessage,
		Timestamp: time.Now().UTC(),
		Message: &agents.MessagePayload{
			Role: "user", Text: "看图", Final: true,
			Images: []agents.ImageAttachment{{ID: "svg", MediaType: "image/svg+xml", Data: "PHN2Zy8+"}},
		},
	}}}
	env.fake.mu.Unlock()

	history, err := env.svc.HistoryPage(context.Background(), env.projectID, session.ID, HistoryOptions{
		IncludeImages: false,
	})
	if err != nil {
		t.Fatalf("HistoryPage: %v", err)
	}
	image := history.Events[0].Message.Images[0]
	if image.Data == "" || image.URL != "" {
		t.Errorf("image = %+v, want unsupported format kept inline", image)
	}
}

func TestHistoryPageKeepsInlineImagesWhenRequested(t *testing.T) {
	env, session := historyPageSession(t)
	env.fake.mu.Lock()
	env.fake.historyResult = agents.HistoryResult{Events: []agents.NormalizedEvent{{
		Type:      agents.EventMessage,
		Timestamp: time.Now().UTC(),
		Message: &agents.MessagePayload{
			Role: "user", Text: "看图", Final: true,
			Images: []agents.ImageAttachment{{ID: "i1", MediaType: "image/png", Data: "aGVsbG8="}},
		},
	}}}
	env.fake.mu.Unlock()

	history, err := env.svc.HistoryPage(context.Background(), env.projectID, session.ID, HistoryOptions{
		IncludeImages: true,
	})
	if err != nil {
		t.Fatalf("HistoryPage: %v", err)
	}
	image := history.Events[0].Message.Images[0]
	if image.Data != "aGVsbG8=" || image.URL != "" {
		t.Errorf("image = %+v, want inline data and no URL", image)
	}
}

func TestHistoryHandlerServesImage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	env, session := historyPageSession(t)
	router := gin.New()
	env.svc.RegisterRoutes(router)

	env.fake.mu.Lock()
	env.fake.historyImage = agents.ImageAttachment{
		ID: "msg_1:1", MediaType: "image/png", Data: "cG5nLWJ5dGVz",
	}
	env.fake.mu.Unlock()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf(
		"/projects/%d/sessions/%s/history/images/msg_1:1", env.projectID, session.ID,
	), nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "image/png" {
		t.Errorf("Content-Type = %q, want image/png", got)
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
	}
	if got := rec.Body.String(); got != "png-bytes" {
		t.Errorf("body = %q, want decoded image bytes", got)
	}

	env.fake.mu.Lock()
	defer env.fake.mu.Unlock()
	if len(env.fake.historyImageReqs) != 1 {
		t.Fatalf("adapter saw %d image requests, want 1", len(env.fake.historyImageReqs))
	}
	request := env.fake.historyImageReqs[0]
	if request.Directory != env.projectDir || request.SessionID != "native-page" || request.ImageID != "msg_1:1" {
		t.Errorf("image request = %+v", request)
	}
}

func TestHistoryHandlerMapsImageErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	env, session := historyPageSession(t)
	router := gin.New()
	env.svc.RegisterRoutes(router)

	env.fake.mu.Lock()
	env.fake.historyImageErr = agents.ErrHistoryImageNotFound
	env.fake.mu.Unlock()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf(
		"/projects/%d/sessions/%s/history/images/missing", env.projectID, session.ID,
	), nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}

	env.fake.mu.Lock()
	env.fake.historyImageErr = nil
	env.fake.historyImage = agents.ImageAttachment{ID: "i", MediaType: "image/svg+xml", Data: "PHN2Zy8+"}
	env.fake.mu.Unlock()

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, fmt.Sprintf(
		"/projects/%d/sessions/%s/history/images/i", env.projectID, session.ID,
	), nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("svg status = %d, want 415", rec.Code)
	}
}
