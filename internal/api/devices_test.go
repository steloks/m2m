package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
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

func listDevices(t *testing.T, server *Server, rawQuery string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/devices"+rawQuery, nil)
	request.Header.Set(tokenHeader, testToken)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	return recorder
}

// listedDevices returns the listed devices by ID, as "status last_seen_at".
func listedDevices(t *testing.T, server *Server, rawQuery string) map[string]string {
	t.Helper()
	recorder := listDevices(t, server, rawQuery)
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /devices%s status = %d, want %d, body %s", rawQuery, recorder.Code, http.StatusOK, recorder.Body)
	}
	var response struct {
		Devices []struct {
			DeviceID   string `json:"device_id"`
			Status     string `json:"status"`
			LastSeenAt string `json:"last_seen_at"`
		} `json:"devices"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	listed := map[string]string{}
	for _, summary := range response.Devices {
		listed[summary.DeviceID] = summary.Status + " " + summary.LastSeenAt
	}
	return listed
}

func TestListDevicesRejectsUnknownStatus(t *testing.T) {
	for _, rawQuery := range []string{"?status=sleeping", "?status=", "?status=Offline"} {
		t.Run(rawQuery, func(t *testing.T) {
			recorder := listDevices(t, NewServer(testToken, testOfflineThreshold, nil), rawQuery)
			assertError(t, recorder, http.StatusBadRequest, "status must be one of online, offline")
		})
	}
}

func TestListDevicesByStatus(t *testing.T) {
	testStore, conn := newTestStore(t)
	server := NewServer(testToken, testOfflineThreshold, testStore)
	var serverTime time.Time
	server.now = func() time.Time { return serverTime }

	day := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	silent, atThreshold, recent := newDeviceID(t, conn), newDeviceID(t, conn), newDeviceID(t, conn)
	for deviceID, seenAt := range map[string]time.Duration{
		silent:      14 * time.Hour,
		atThreshold: 14*time.Hour + 2*time.Minute + 30*time.Second,
		recent:      14*time.Hour + 6*time.Minute,
	} {
		serverTime = day.Add(seenAt)
		if code := postEvent(t, server, deviceID, eventBody).Code; code != http.StatusCreated {
			t.Fatalf("post status = %d, want %d", code, http.StatusCreated)
		}
	}
	serverTime = day.Add(14*time.Hour + 7*time.Minute + 30*time.Second)

	want := map[string]string{
		silent:      "offline 2026-09-29T14:00:00Z",
		atThreshold: "online 2026-09-29T14:02:30Z",
		recent:      "online 2026-09-29T14:06:00Z",
	}
	tests := []struct {
		rawQuery string
		want     []string
	}{
		{rawQuery: "?status=offline", want: []string{silent}},
		{rawQuery: "?status=online", want: []string{atThreshold, recent}},
		{rawQuery: "", want: []string{silent, atThreshold, recent}},
	}
	for _, testCase := range tests {
		t.Run(testCase.rawQuery, func(t *testing.T) {
			listed := listedDevices(t, server, testCase.rawQuery)
			for _, deviceID := range []string{silent, atThreshold, recent} {
				got, ok := listed[deviceID]
				wantListed := slices.Contains(testCase.want, deviceID)
				if ok != wantListed {
					t.Errorf("%s listed = %t, want %t", deviceID, ok, wantListed)
				}
				if ok && got != want[deviceID] {
					t.Errorf("%s = %q, want %q", deviceID, got, want[deviceID])
				}
			}
		})
	}
}

func TestListDevicesNoMatches(t *testing.T) {
	testStore, _ := newTestStore(t)
	server := NewServer(testToken, testOfflineThreshold, testStore)
	server.now = func() time.Time { return time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC) }

	recorder := listDevices(t, server, "?status=offline")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if want := `{"devices":[]}` + "\n"; recorder.Body.String() != want {
		t.Errorf("body = %s, want %s", recorder.Body, want)
	}
}
