package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func getDevice(t *testing.T, server *Server, deviceID string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/devices/"+deviceID, nil)
	request.Header.Set(tokenHeader, testToken)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	return recorder
}

func TestGetDeviceUnknown(t *testing.T) {
	testStore, conn := newTestStore(t)
	server := NewServer(testToken, testOfflineThreshold, testStore)

	assertError(t, getDevice(t, server, newDeviceID(t, conn)), http.StatusNotFound, "device not found")
}

func TestGetDeviceDisarmedWithNoOpenSensors(t *testing.T) {
	testStore, conn := newTestStore(t)
	deviceID := newDeviceID(t, conn)
	server := NewServer(testToken, testOfflineThreshold, testStore)
	server.now = func() time.Time { return time.Date(2026, 9, 29, 14, 0, 0, 0, time.UTC) }

	heartbeat := eventWith(map[string]string{"type": `"heartbeat"`, "payload": `{}`})
	if code := postEvent(t, server, deviceID, heartbeat).Code; code != http.StatusCreated {
		t.Fatalf("post status = %d, want %d", code, http.StatusCreated)
	}

	recorder := getDevice(t, server, deviceID)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body %s", recorder.Code, http.StatusOK, recorder.Body)
	}
	want := `{"device_id":"` + deviceID + `","status":"online","last_seen_at":"2026-09-29T14:00:00Z",` +
		`"arm_state":"disarmed","arm_mode":null,"open_sensors":[]}` + "\n"
	if recorder.Body.String() != want {
		t.Errorf("body = %s, want %s", recorder.Body, want)
	}
}

func TestGetDeviceGoesOfflineAfterThreshold(t *testing.T) {
	testStore, conn := newTestStore(t)
	deviceID := newDeviceID(t, conn)
	server := NewServer(testToken, testOfflineThreshold, testStore)
	serverTime := time.Date(2026, 9, 29, 14, 2, 0, 0, time.UTC)
	server.now = func() time.Time { return serverTime }

	armed := eventWith(map[string]string{
		"event_id": `"a1a1a1a1-0000-4000-8000-000000000001"`, "type": `"armed"`, "payload": `{"mode":"away","user_id":3}`,
	})
	for _, body := range []string{armed, eventBody} {
		if code := postEvent(t, server, deviceID, body).Code; code != http.StatusCreated {
			t.Fatalf("post status = %d, want %d", code, http.StatusCreated)
		}
	}

	tests := []struct {
		at         time.Time
		wantStatus string
	}{
		{at: serverTime.Add(4*time.Minute + 30*time.Second), wantStatus: "online"},
		{at: serverTime.Add(5*time.Minute + 30*time.Second), wantStatus: "offline"},
	}
	for _, testCase := range tests {
		serverTime = testCase.at
		recorder := getDevice(t, server, deviceID)
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d, body %s", recorder.Code, http.StatusOK, recorder.Body)
		}
		want := `{"device_id":"` + deviceID + `","status":"` + testCase.wantStatus + `","last_seen_at":"2026-09-29T14:02:00Z",` +
			`"arm_state":"armed","arm_mode":"away","open_sensors":[4]}` + "\n"
		if recorder.Body.String() != want {
			t.Errorf("at %s: body = %s, want %s", testCase.at.Format(time.TimeOnly), recorder.Body, want)
		}
	}
}
