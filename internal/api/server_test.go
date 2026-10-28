package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

const testToken = "test-token"

func TestUnknownRouteReturnsJSONError(t *testing.T) {
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/nope", nil)
	req.Header.Set(tokenHeader, testToken)
	rec := httptest.NewRecorder()
	NewServer(testToken, nil).ServeHTTP(rec, req)

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

func TestTestRoute(t *testing.T) {
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/test", nil)
	req.Header.Set(tokenHeader, testToken)
	rec := httptest.NewRecorder()
	NewServer(testToken, nil).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if want := `{"status":"ok"}` + "\n"; rec.Body.String() != want {
		t.Errorf("body = %q, want %q", rec.Body.String(), want)
	}
}
