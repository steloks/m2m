package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const (
	testToken            = "test-token"
	testOfflineThreshold = 5 * time.Minute
)

func TestUnknownRouteReturnsJSONError(t *testing.T) {
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/nope", nil)
	req.Header.Set(tokenHeader, testToken)
	rec := httptest.NewRecorder()
	NewServer(testToken, testOfflineThreshold, nil).ServeHTTP(rec, req)

	assertError(t, rec, http.StatusNotFound, "not found")
}

func assertError(t *testing.T, rec *httptest.ResponseRecorder, status int, message string) {
	t.Helper()
	if rec.Code != status {
		t.Errorf("status = %d, want %d", rec.Code, status)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
	want := `{"error":true,"message":"` + message + `"}` + "\n"
	if got := rec.Body.String(); got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}
