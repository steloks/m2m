package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"
)

// The assignment names the worked example's events a1, h1, b2 and h2, which
// are not UUIDs. These stand in for them.
const (
	a1 = "a1a1a1a1-0000-4000-8000-000000000000"
	h1 = "00000000-0000-4000-8000-0000000000f1"
	b2 = "b2b2b2b2-0000-4000-8000-000000000000"
	h2 = "00000000-0000-4000-8000-0000000000f2"
)

// deviceState summarizes GET /devices/{id} as
// "<status> <arm_state>/<arm_mode> sensors=<open_sensors> last_seen=<time>".
func deviceState(t *testing.T, server *Server, deviceID string) string {
	t.Helper()
	recorder := getDevice(t, server, deviceID)
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /devices/%s status = %d, body %s", deviceID, recorder.Code, recorder.Body)
	}
	var device struct {
		Status      string  `json:"status"`
		LastSeenAt  string  `json:"last_seen_at"`
		ArmState    string  `json:"arm_state"`
		ArmMode     *string `json:"arm_mode"`
		OpenSensors []int   `json:"open_sensors"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &device); err != nil {
		t.Fatalf("decode device: %v", err)
	}
	mode := "null"
	if device.ArmMode != nil {
		mode = *device.ArmMode
	}
	return fmt.Sprintf("%s %s/%s sensors=%v last_seen=%s",
		device.Status, device.ArmState, mode, device.OpenSensors, strings.TrimPrefix(device.LastSeenAt, "2026-09-29T"))
}

// TestWorkedExample replays the worked example from the assignment, at the
// server times it gives.
func TestWorkedExample(t *testing.T) {
	testStore, conn := newTestStore(t)
	deviceID := newDeviceID(t, conn)
	server := NewServer(testToken, testOfflineThreshold, testStore)
	var serverTime time.Time
	server.now = func() time.Time { return serverTime }

	post := func(eventID, eventType, occurredAt, payload string) func() *httptest.ResponseRecorder {
		body := fmt.Sprintf(`{"event_id":%q,"type":%q,"occurred_at":"2026-09-29T%sZ","payload":%s}`,
			eventID, eventType, occurredAt, payload)
		return func() *httptest.ResponseRecorder { return postEvent(t, server, deviceID, body) }
	}
	get := func() *httptest.ResponseRecorder { return getDevice(t, server, deviceID) }
	listOffline := func() *httptest.ResponseRecorder {
		if _, listed := listedDevices(t, server, "?status=offline")[deviceID]; !listed {
			t.Errorf("%s: device is not listed as offline", serverTime.Format(time.TimeOnly))
		}
		return listDevices(t, server, "?status=offline")
	}

	steps := []struct {
		at        string
		name      string
		send      func() *httptest.ResponseRecorder
		wantCode  int
		wantState string
	}{
		{"14:00:00", "POST armed (away), a1", post(a1, "armed", "14:00:00", `{"mode":"away","user_id":3}`),
			http.StatusCreated, "online armed/away sensors=[] last_seen=14:00:00Z"},
		{"14:01:00", "POST heartbeat, h1", post(h1, "heartbeat", "14:01:00", `{"battery_pct":87,"rssi_dbm":-71}`),
			http.StatusCreated, "online armed/away sensors=[] last_seen=14:01:00Z"},
		{"14:02:00", "POST sensor_opened 4, b2", post(b2, "sensor_opened", "14:02:00", `{"sensor_id":4}`),
			http.StatusCreated, "online armed/away sensors=[4] last_seen=14:02:00Z"},
		{"14:02:05", "POST sensor_opened 4, b2 (retry)", post(b2, "sensor_opened", "14:02:00", `{"sensor_id":4}`),
			http.StatusOK, "online armed/away sensors=[4] last_seen=14:02:00Z"},
		{"14:06:30", "GET /devices/{id}", get,
			http.StatusOK, "online armed/away sensors=[4] last_seen=14:02:00Z"},
		{"14:07:30", "GET /devices/{id}", get,
			http.StatusOK, "offline armed/away sensors=[4] last_seen=14:02:00Z"},
		{"14:07:30", "GET /devices?status=offline", listOffline,
			http.StatusOK, "offline armed/away sensors=[4] last_seen=14:02:00Z"},
		{"14:08:00", "POST heartbeat, h2", post(h2, "heartbeat", "14:08:00", `{"battery_pct":86,"rssi_dbm":-70}`),
			http.StatusCreated, "online armed/away sensors=[4] last_seen=14:08:00Z"},
	}
	for _, step := range steps {
		at, err := time.Parse(time.DateTime, "2026-09-29 "+step.at)
		if err != nil {
			t.Fatal(err)
		}
		serverTime = at

		if recorder := step.send(); recorder.Code != step.wantCode {
			t.Fatalf("%s %s: status = %d, want %d, body %s", step.at, step.name, recorder.Code, step.wantCode, recorder.Body)
		}
		if state := deviceState(t, server, deviceID); state != step.wantState {
			t.Errorf("%s %s: state = %q, want %q", step.at, step.name, state, step.wantState)
		}
	}

	eventIDs, _ := walkHistory(t, server, "/devices/"+deviceID+"/events")
	if want := []string{a1, h1, b2, h2}; !slices.Equal(eventIDs, want) {
		t.Errorf("stored events = %v, want %v", eventIDs, want)
	}
}
