package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

// pingStore is a Store whose only working method is Ping.
type pingStore struct {
	Store
	err error
}

func (store pingStore) Ping(context.Context) error {
	return store.err
}

func getWithoutToken(t *testing.T, server *Server, target string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, target, nil)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	return recorder
}

func TestHealth(t *testing.T) {
	recorder := getWithoutToken(t, NewServer(testToken, testOfflineThreshold, pingStore{}), "/health")
	if recorder.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if want := `{"status":"ok"}` + "\n"; recorder.Body.String() != want {
		t.Errorf("body = %q, want %q", recorder.Body, want)
	}
}

func TestHealthWithoutDatabase(t *testing.T) {
	server := NewServer(testToken, testOfflineThreshold, pingStore{err: errors.New("connection refused")})
	assertError(t, getWithoutToken(t, server, "/health"), http.StatusServiceUnavailable, "database unavailable")
}

func TestMetrics(t *testing.T) {
	server := NewServer(testToken, testOfflineThreshold, nil)
	postEvent(t, server, "panel-0042", `{}`)

	recorder := getWithoutToken(t, server, "/metrics")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	for _, want := range []string{
		"events_ingested_total 0",
		"events_duplicate_total 0",
		`http_request_duration_seconds_count{route="POST /devices/{id}/events",status="400"} 1`,
	} {
		if !strings.Contains(recorder.Body.String(), want) {
			t.Errorf("metrics do not contain %q", want)
		}
	}
}

// Clients choose the method and the path, so neither may become a label value.
func TestMetricsSeriesDoNotGrowWithClientInput(t *testing.T) {
	server := NewServer(testToken, testOfflineThreshold, nil)
	for index := range 20 {
		request := httptest.NewRequestWithContext(t.Context(), fmt.Sprintf("FOO%d", index), fmt.Sprintf("/devices/panel-%d", index), nil)
		server.ServeHTTP(httptest.NewRecorder(), request)
	}

	if series := testutil.CollectAndCount(server.metrics.requestDuration); series != 1 {
		t.Errorf("request duration series = %d, want 1", series)
	}
}

func TestDeviceAPIStillNeedsToken(t *testing.T) {
	recorder := getWithoutToken(t, NewServer(testToken, testOfflineThreshold, nil), "/devices/panel-0042")
	assertError(t, recorder, http.StatusUnauthorized, "missing or invalid X-Device-Token")
}
