package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequireToken(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler := requireToken(testToken, next)

	tests := []struct {
		name   string
		header []string // nil means the header is not sent
		want   int
	}{
		{name: "missing header", header: nil, want: http.StatusUnauthorized},
		{name: "empty token", header: []string{""}, want: http.StatusUnauthorized},
		{name: "wrong token", header: []string{"nope"}, want: http.StatusUnauthorized},
		{name: "token prefix", header: []string{"test"}, want: http.StatusUnauthorized},
		{name: "correct token", header: []string{testToken}, want: http.StatusNoContent},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/devices/panel-1", nil)
			if tt.header != nil {
				req.Header.Set(tokenHeader, tt.header[0])
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if tt.want == http.StatusUnauthorized {
				assertError(t, rec, http.StatusUnauthorized, "missing or invalid X-Device-Token")
				return
			}
			if rec.Code != tt.want {
				t.Errorf("status = %d, want %d", rec.Code, tt.want)
			}
		})
	}
}

func TestServerChecksTokenBeforeRouting(t *testing.T) {
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/nope", nil)
	rec := httptest.NewRecorder()
	NewServer(testToken).ServeHTTP(rec, req)

	assertError(t, rec, http.StatusUnauthorized, "missing or invalid X-Device-Token")
}
