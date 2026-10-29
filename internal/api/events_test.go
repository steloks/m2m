package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/steloks/m2m/internal/store"
)

const eventBody = `{"event_id":"7c1e9a52-3b0f-4d8e-9a61-2f4b8c0d1e77","type":"sensor_opened","occurred_at":"2026-09-29T14:02:00Z","payload":{"sensor_id":4}}`

// eventWith returns eventBody with some fields replaced by raw JSON values.
// An empty value removes the field.
func eventWith(changes map[string]string) string {
	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal([]byte(eventBody), &fields); err != nil {
		panic(err)
	}
	for name, value := range changes {
		if value == "" {
			delete(fields, name)
			continue
		}
		fields[name] = json.RawMessage(value)
	}
	body, err := json.Marshal(fields)
	if err != nil {
		panic(err)
	}
	return string(body)
}

func postEvent(t *testing.T, server *Server, deviceID, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/devices/"+deviceID+"/events", strings.NewReader(body))
	request.Header.Set(tokenHeader, testToken)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	return recorder
}

// These requests are rejected before the store is used, so the server runs
// without a database. A nil store would panic if it were reached.
func TestRecordEventRejectsBadRequests(t *testing.T) {
	const (
		badBody      = "body must be a JSON object with fields of the right type"
		badPayload   = "payload must be a JSON object with fields of the right type"
		badTimestamp = "occurred_at must be an ISO 8601 timestamp with a time zone, e.g. 2026-09-29T14:02:00Z"
		badType      = "type must be one of armed, disarmed, sensor_opened, sensor_closed, heartbeat"
		badSensor    = "payload.sensor_id must be between 1 and 64"
	)
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "empty body", body: ``, want: badBody},
		{name: "malformed json", body: `{"event_id":`, want: badBody},
		{name: "array body", body: `[]`, want: badBody},
		{name: "body too large", body: `{"event_id":"` + strings.Repeat("a", maxBodyBytes) + `"}`, want: badBody},

		{name: "missing event_id", body: eventWith(map[string]string{"event_id": ""}), want: "event_id is required"},
		{name: "numeric event_id", body: eventWith(map[string]string{"event_id": `42`}), want: badBody},
		{name: "short event_id", body: eventWith(map[string]string{"event_id": `"a1"`}), want: "event_id must be a UUID"},
		{name: "event_id without hyphens", body: eventWith(map[string]string{"event_id": `"7c1e9a523b0f4d8e9a612f4b8c0d1e77"`}), want: "event_id must be a UUID"},

		{name: "missing type", body: eventWith(map[string]string{"type": ""}), want: "type is required"},
		{name: "unknown type", body: eventWith(map[string]string{"type": `"exploded"`}), want: badType},
		{name: "type in wrong case", body: eventWith(map[string]string{"type": `"Armed"`}), want: badType},

		{name: "missing occurred_at", body: eventWith(map[string]string{"occurred_at": ""}), want: "occurred_at is required"},
		{name: "occurred_at without time zone", body: eventWith(map[string]string{"occurred_at": `"2026-09-29T14:02:00"`}), want: badTimestamp},
		{name: "occurred_at date only", body: eventWith(map[string]string{"occurred_at": `"2026-09-29"`}), want: badTimestamp},
		{name: "occurred_at as text", body: eventWith(map[string]string{"occurred_at": `"yesterday"`}), want: badTimestamp},

		{name: "missing payload", body: eventWith(map[string]string{"payload": ""}), want: "payload is required"},
		{name: "null payload", body: eventWith(map[string]string{"payload": `null`}), want: "payload is required"},
		{name: "payload is an array", body: eventWith(map[string]string{"payload": `[]`}), want: badPayload},
		{name: "payload is a string", body: eventWith(map[string]string{"payload": `"{}"`}), want: badPayload},

		{name: "armed without mode", body: eventWith(map[string]string{"type": `"armed"`, "payload": `{"user_id":3}`}), want: "payload.mode is required"},
		{name: "armed with unknown mode", body: eventWith(map[string]string{"type": `"armed"`, "payload": `{"mode":"night"}`}), want: "payload.mode must be one of away, stay"},
		{name: "armed with numeric mode", body: eventWith(map[string]string{"type": `"armed"`, "payload": `{"mode":1}`}), want: badPayload},

		{name: "sensor without sensor_id", body: eventWith(map[string]string{"payload": `{}`}), want: "payload.sensor_id is required"},
		{name: "sensor_id zero", body: eventWith(map[string]string{"payload": `{"sensor_id":0}`}), want: badSensor},
		{name: "sensor_id 65", body: eventWith(map[string]string{"payload": `{"sensor_id":65}`}), want: badSensor},
		{name: "negative sensor_id", body: eventWith(map[string]string{"payload": `{"sensor_id":-4}`}), want: badSensor},
		{name: "sensor_id as string", body: eventWith(map[string]string{"payload": `{"sensor_id":"4"}`}), want: badPayload},
		{name: "fractional sensor_id", body: eventWith(map[string]string{"payload": `{"sensor_id":4.5}`}), want: badPayload},
		{name: "sensor_closed out of range", body: eventWith(map[string]string{"type": `"sensor_closed"`, "payload": `{"sensor_id":65}`}), want: badSensor},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := postEvent(t, NewServer(testToken, testOfflineThreshold, nil), "panel-0042", testCase.body)
			assertError(t, recorder, http.StatusBadRequest, testCase.want)
		})
	}
}

// newTestStore connects to TEST_DATABASE_URL, a database with db/schema.sql
// applied (docker compose up -d db). Without it the test is skipped.
func newTestStore(t *testing.T) (*store.Store, *pgx.Conn) {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	testStore, err := store.New(t.Context(), databaseURL)
	if err != nil {
		t.Fatalf("connect store: %v", err)
	}
	t.Cleanup(testStore.Close)

	conn, err := pgx.Connect(t.Context(), databaseURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(context.Background()) })
	return testStore, conn
}

// newDeviceID returns a device id no other test uses, and deletes its rows
// when the test ends.
func newDeviceID(t *testing.T, conn *pgx.Conn) string {
	t.Helper()
	deviceID := fmt.Sprintf("test-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = conn.Exec(ctx, `DELETE FROM events WHERE device_id = $1`, deviceID)
		_, _ = conn.Exec(ctx, `DELETE FROM devices WHERE device_id = $1`, deviceID)
	})
	return deviceID
}

func countEvents(t *testing.T, conn *pgx.Conn, deviceID string) int {
	t.Helper()
	var count int
	if err := conn.QueryRow(t.Context(), `SELECT count(*) FROM events WHERE device_id = $1`, deviceID).Scan(&count); err != nil {
		t.Fatalf("count events: %v", err)
	}
	return count
}

func TestRecordEventCreatesEvent(t *testing.T) {
	testStore, conn := newTestStore(t)
	deviceID := newDeviceID(t, conn)

	recorder := postEvent(t, NewServer(testToken, testOfflineThreshold, testStore), deviceID, eventBody)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body %s", recorder.Code, http.StatusCreated, recorder.Body)
	}
	var got map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	want := map[string]any{
		"device_id":   deviceID,
		"event_id":    "7c1e9a52-3b0f-4d8e-9a61-2f4b8c0d1e77",
		"type":        "sensor_opened",
		"occurred_at": "2026-09-29T14:02:00Z",
		"payload":     map[string]any{"sensor_id": float64(4)},
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("response = %v, want %v", got, want)
	}
	if count := countEvents(t, conn, deviceID); count != 1 {
		t.Errorf("stored events = %d, want 1", count)
	}

	var devices int
	if err := conn.QueryRow(t.Context(), `SELECT count(*) FROM devices WHERE device_id = $1`, deviceID).Scan(&devices); err != nil {
		t.Fatalf("count devices: %v", err)
	}
	if devices != 1 {
		t.Errorf("devices = %d, want 1", devices)
	}
}

func TestRecordEventAcceptsValidEvents(t *testing.T) {
	testStore, conn := newTestStore(t)
	server := NewServer(testToken, testOfflineThreshold, testStore)

	tests := []struct {
		name    string
		changes map[string]string
	}{
		{name: "sensor opened", changes: map[string]string{}},
		{name: "sensor closed", changes: map[string]string{"type": `"sensor_closed"`}},
		{name: "lowest sensor", changes: map[string]string{"payload": `{"sensor_id":1}`}},
		{name: "highest sensor", changes: map[string]string{"payload": `{"sensor_id":64}`}},
		{name: "armed away", changes: map[string]string{"type": `"armed"`, "payload": `{"mode":"away","user_id":3}`}},
		{name: "armed stay", changes: map[string]string{"type": `"armed"`, "payload": `{"mode":"stay"}`}},
		{name: "disarmed", changes: map[string]string{"type": `"disarmed"`, "payload": `{"user_id":3}`}},
		{name: "heartbeat", changes: map[string]string{"type": `"heartbeat"`, "payload": `{"battery_pct":87,"rssi_dbm":-71}`}},
		{name: "heartbeat with empty payload", changes: map[string]string{"type": `"heartbeat"`, "payload": `{}`}},
		{name: "unknown payload fields", changes: map[string]string{"payload": `{"sensor_id":4,"firmware":"2.1"}`}},
		{name: "uppercase event_id", changes: map[string]string{"event_id": `"7C1E9A52-3B0F-4D8E-9A61-2F4B8C0D1E77"`}},
		{name: "fractional seconds", changes: map[string]string{"occurred_at": `"2026-09-29T14:02:00.250Z"`}},
		{name: "time zone offset", changes: map[string]string{"occurred_at": `"2026-09-29T17:02:00+03:00"`}},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := postEvent(t, server, newDeviceID(t, conn), eventWith(testCase.changes))
			if recorder.Code != http.StatusCreated {
				t.Errorf("status = %d, want %d, body %s", recorder.Code, http.StatusCreated, recorder.Body)
			}
		})
	}
}

func TestRecordEventDuplicateStoresOnce(t *testing.T) {
	testStore, conn := newTestStore(t)
	deviceID := newDeviceID(t, conn)
	server := NewServer(testToken, testOfflineThreshold, testStore)

	if code := postEvent(t, server, deviceID, eventBody).Code; code != http.StatusCreated {
		t.Fatalf("first status = %d, want %d", code, http.StatusCreated)
	}
	if code := postEvent(t, server, deviceID, eventBody).Code; code != http.StatusOK {
		t.Errorf("retry status = %d, want %d", code, http.StatusOK)
	}
	if count := countEvents(t, conn, deviceID); count != 1 {
		t.Errorf("stored events = %d, want 1", count)
	}
	if ingested := testutil.ToFloat64(server.metrics.eventsIngested); ingested != 1 {
		t.Errorf("events_ingested_total = %v, want 1", ingested)
	}
	if duplicates := testutil.ToFloat64(server.metrics.eventsDuplicate); duplicates != 1 {
		t.Errorf("events_duplicate_total = %v, want 1", duplicates)
	}
}

func TestRecordEventSetsLastSeenToServerTime(t *testing.T) {
	testStore, conn := newTestStore(t)
	deviceID := newDeviceID(t, conn)
	server := NewServer(testToken, testOfflineThreshold, testStore)
	serverTime := time.Date(2026, 9, 29, 14, 2, 0, 0, time.UTC)
	server.now = func() time.Time { return serverTime }

	firstSeen := serverTime

	body := eventWith(map[string]string{"occurred_at": `"2026-09-29T09:00:00Z"`})
	if code := postEvent(t, server, deviceID, body).Code; code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", code, http.StatusCreated)
	}
	if lastSeen := readDevice(t, conn, deviceID).lastSeenAt; !lastSeen.Equal(firstSeen) {
		t.Errorf("last_seen_at = %v, want %v", lastSeen, firstSeen)
	}

	serverTime = serverTime.Add(5 * time.Second)
	if code := postEvent(t, server, deviceID, body).Code; code != http.StatusOK {
		t.Fatalf("retry status = %d, want %d", code, http.StatusOK)
	}
	if lastSeen := readDevice(t, conn, deviceID).lastSeenAt; !lastSeen.Equal(firstSeen) {
		t.Errorf("retry moved last_seen_at from %v to %v", firstSeen, lastSeen)
	}
}

func TestRecordEventDuplicateReturnsStoredEvent(t *testing.T) {
	testStore, conn := newTestStore(t)
	deviceID := newDeviceID(t, conn)
	server := NewServer(testToken, testOfflineThreshold, testStore)

	if code := postEvent(t, server, deviceID, eventBody).Code; code != http.StatusCreated {
		t.Fatalf("first status = %d, want %d", code, http.StatusCreated)
	}
	retry := eventWith(map[string]string{
		"type":        `"sensor_closed"`,
		"occurred_at": `"2026-09-29T15:00:00Z"`,
		"payload":     `{"sensor_id":9}`,
	})
	recorder := postEvent(t, server, deviceID, retry)
	if recorder.Code != http.StatusOK {
		t.Fatalf("retry status = %d, want %d, body %s", recorder.Code, http.StatusOK, recorder.Body)
	}

	var got map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	want := map[string]any{
		"device_id":   deviceID,
		"event_id":    "7c1e9a52-3b0f-4d8e-9a61-2f4b8c0d1e77",
		"type":        "sensor_opened",
		"occurred_at": "2026-09-29T14:02:00Z",
		"payload":     map[string]any{"sensor_id": float64(4)},
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("response = %v, want %v", got, want)
	}
	if sensors := readDevice(t, conn, deviceID).openSensors; fmt.Sprint(sensors) != "[4]" {
		t.Errorf("open sensors = %v, want [4]", sensors)
	}
}

type deviceRow struct {
	lastSeenAt  time.Time
	armState    string
	armMode     *string
	openSensors []int
}

func readDevice(t *testing.T, conn *pgx.Conn, deviceID string) deviceRow {
	t.Helper()
	var row deviceRow
	err := conn.QueryRow(t.Context(),
		`SELECT last_seen_at, arm_state, arm_mode, open_sensors FROM devices WHERE device_id = $1`,
		deviceID).Scan(&row.lastSeenAt, &row.armState, &row.armMode, &row.openSensors)
	if err != nil {
		t.Fatalf("read device: %v", err)
	}
	return row
}

func TestRecordEventUpdatesDeviceState(t *testing.T) {
	testStore, conn := newTestStore(t)
	deviceID := newDeviceID(t, conn)
	server := NewServer(testToken, testOfflineThreshold, testStore)

	events := []map[string]string{
		{"event_id": `"a1a1a1a1-0000-4000-8000-000000000001"`, "type": `"armed"`, "payload": `{"mode":"away","user_id":3}`},
		{"event_id": `"a1a1a1a1-0000-4000-8000-000000000002"`, "payload": `{"sensor_id":7}`},
		{"event_id": `"a1a1a1a1-0000-4000-8000-000000000003"`, "payload": `{"sensor_id":4}`},
		{"event_id": `"a1a1a1a1-0000-4000-8000-000000000004"`, "payload": `{"sensor_id":9}`},
		{"event_id": `"a1a1a1a1-0000-4000-8000-000000000005"`, "type": `"sensor_closed"`, "payload": `{"sensor_id":9}`},
	}
	for _, changes := range events {
		if recorder := postEvent(t, server, deviceID, eventWith(changes)); recorder.Code != http.StatusCreated {
			t.Fatalf("status = %d, want %d, body %s", recorder.Code, http.StatusCreated, recorder.Body)
		}
	}

	got := readDevice(t, conn, deviceID)
	if got.armState != "armed" || got.armMode == nil || *got.armMode != "away" {
		t.Errorf("arm = %s/%v, want armed/away", got.armState, got.armMode)
	}
	if fmt.Sprint(got.openSensors) != "[4 7]" {
		t.Errorf("open sensors = %v, want [4 7]", got.openSensors)
	}

	if code := postEvent(t, server, deviceID, eventWith(events[4])).Code; code != http.StatusOK {
		t.Fatalf("retry status = %d, want %d", code, http.StatusOK)
	}
	if afterRetry := readDevice(t, conn, deviceID); !afterRetry.lastSeenAt.Equal(got.lastSeenAt) {
		t.Errorf("retry changed last_seen_at from %v to %v", got.lastSeenAt, afterRetry.lastSeenAt)
	}

	disarm := map[string]string{"event_id": `"a1a1a1a1-0000-4000-8000-000000000006"`, "type": `"disarmed"`, "payload": `{"user_id":3}`}
	if code := postEvent(t, server, deviceID, eventWith(disarm)).Code; code != http.StatusCreated {
		t.Fatalf("disarm status = %d, want %d", code, http.StatusCreated)
	}
	if got := readDevice(t, conn, deviceID); got.armState != "disarmed" || got.armMode != nil {
		t.Errorf("arm = %s/%v, want disarmed/nil", got.armState, got.armMode)
	}
}

func TestRecordEventSameEventIDOnTwoDevices(t *testing.T) {
	testStore, conn := newTestStore(t)
	server := NewServer(testToken, testOfflineThreshold, testStore)

	for _, deviceID := range []string{newDeviceID(t, conn), newDeviceID(t, conn)} {
		if code := postEvent(t, server, deviceID, eventBody).Code; code != http.StatusCreated {
			t.Errorf("device %s: status = %d, want %d", deviceID, code, http.StatusCreated)
		}
		if count := countEvents(t, conn, deviceID); count != 1 {
			t.Errorf("device %s: stored events = %d, want 1", deviceID, count)
		}
	}
}

func getHistory(t *testing.T, server *Server, target string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, target, nil)
	request.Header.Set(tokenHeader, testToken)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	return recorder
}

type historyPage struct {
	Events []struct {
		EventID string `json:"event_id"`
	} `json:"events"`
	Next *string `json:"next"`
}

func getHistoryPage(t *testing.T, server *Server, target string) historyPage {
	t.Helper()
	recorder := getHistory(t, server, target)
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET %s status = %d, want %d, body %s", target, recorder.Code, http.StatusOK, recorder.Body)
	}
	var page historyPage
	if err := json.Unmarshal(recorder.Body.Bytes(), &page); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return page
}

// walkHistory follows next from target until the last page, and returns the
// event IDs in order and the size of each page.
func walkHistory(t *testing.T, server *Server, target string) ([]string, []int) {
	t.Helper()
	var eventIDs []string
	var pageSizes []int
	for {
		page := getHistoryPage(t, server, target)
		pageSizes = append(pageSizes, len(page.Events))
		for _, listed := range page.Events {
			eventIDs = append(eventIDs, listed.EventID)
		}
		if page.Next == nil {
			return eventIDs, pageSizes
		}
		target = *page.Next
	}
}

func historyEventID(n int) string {
	return fmt.Sprintf("00000000-0000-4000-8000-%012d", n)
}

// postHistory posts a heartbeat for each occurred_at, with event IDs 1, 2, ...
func postHistory(t *testing.T, server *Server, deviceID string, occurredAts ...string) {
	t.Helper()
	for index, occurredAt := range occurredAts {
		body := eventWith(map[string]string{
			"event_id":    `"` + historyEventID(index+1) + `"`,
			"type":        `"heartbeat"`,
			"occurred_at": `"` + occurredAt + `"`,
			"payload":     `{"battery_pct":87}`,
		})
		if recorder := postEvent(t, server, deviceID, body); recorder.Code != http.StatusCreated {
			t.Fatalf("post status = %d, want %d, body %s", recorder.Code, http.StatusCreated, recorder.Body)
		}
	}
}

func TestHistoryRejectsBadQueries(t *testing.T) {
	const (
		badLimit  = "limit must be an integer between 1 and 500"
		badSince  = "since must be an ISO 8601 timestamp with a time zone, e.g. 2026-09-29T14:00:00Z"
		badCursor = "cursor is invalid"
	)
	tests := []struct {
		query string
		want  string
	}{
		{query: "limit=0", want: badLimit},
		{query: "limit=-1", want: badLimit},
		{query: "limit=501", want: badLimit},
		{query: "limit=ten", want: badLimit},
		{query: "limit=", want: badLimit},
		{query: "since=yesterday", want: badSince},
		{query: "since=2026-09-29", want: badSince},
		{query: "since=2026-09-29T14:00:00", want: badSince},
		{query: "since=", want: badSince},
		{query: "cursor=not*base64", want: badCursor},
		{query: "cursor=" + base64.RawURLEncoding.EncodeToString([]byte("2026-09-29T14:00:00Z")), want: badCursor},
		{query: "cursor=" + base64.RawURLEncoding.EncodeToString([]byte("2026-09-29T14:00:00Z,x")), want: badCursor},
		{query: "cursor=" + base64.RawURLEncoding.EncodeToString([]byte("today,3")), want: badCursor},
		{query: "since=2026-09-29T14:00:00Z&cursor=abc", want: "since and cursor cannot be used together"},
	}
	for _, testCase := range tests {
		t.Run(testCase.query, func(t *testing.T) {
			recorder := getHistory(t, NewServer(testToken, testOfflineThreshold, nil), "/devices/panel-0042/events?"+testCase.query)
			assertError(t, recorder, http.StatusBadRequest, testCase.want)
		})
	}
}

func TestHistoryUnknownDevice(t *testing.T) {
	testStore, conn := newTestStore(t)
	server := NewServer(testToken, testOfflineThreshold, testStore)

	assertError(t, getHistory(t, server, "/devices/"+newDeviceID(t, conn)+"/events"), http.StatusNotFound, "device not found")
}

func TestHistoryPageBody(t *testing.T) {
	testStore, conn := newTestStore(t)
	deviceID := newDeviceID(t, conn)
	server := NewServer(testToken, testOfflineThreshold, testStore)
	postHistory(t, server, deviceID, "2026-09-29T14:00:00Z", "2026-09-29T14:01:00Z")

	recorder := getHistory(t, server, "/devices/"+deviceID+"/events?limit=1")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body %s", recorder.Code, http.StatusOK, recorder.Body)
	}
	var page struct {
		Events []map[string]any `json:"events"`
		Next   *string          `json:"next"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &page); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	want := []map[string]any{{
		"event_id":    historyEventID(1),
		"type":        "heartbeat",
		"occurred_at": "2026-09-29T14:00:00Z",
		"payload":     map[string]any{"battery_pct": float64(87)},
	}}
	if fmt.Sprint(page.Events) != fmt.Sprint(want) {
		t.Errorf("events = %v, want %v", page.Events, want)
	}
	if page.Next == nil || !strings.HasPrefix(*page.Next, "/devices/"+deviceID+"/events?cursor=") {
		t.Errorf("next = %v, want a URL for the next page", page.Next)
	}
}

func TestHistoryWalksEveryEventOnce(t *testing.T) {
	testStore, conn := newTestStore(t)
	deviceID := newDeviceID(t, conn)
	server := NewServer(testToken, testOfflineThreshold, testStore)
	// Events 2 and 3 occurred at the same time, and event 5 occurred before
	// event 4 although it arrived after it.
	postHistory(t, server, deviceID,
		"2026-09-29T14:00:00Z",
		"2026-09-29T14:01:00Z",
		"2026-09-29T14:01:00Z",
		"2026-09-29T14:03:00Z",
		"2026-09-29T14:02:00Z",
	)

	tests := []struct {
		name          string
		query         string
		wantEvents    []int
		wantPageSizes []int
	}{
		{name: "pages of 2", query: "?limit=2", wantEvents: []int{1, 2, 3, 5, 4}, wantPageSizes: []int{2, 2, 1}},
		{name: "page boundary between equal times", query: "?limit=1", wantEvents: []int{1, 2, 3, 5, 4}, wantPageSizes: []int{1, 1, 1, 1, 1}},
		{name: "one page", query: "", wantEvents: []int{1, 2, 3, 5, 4}, wantPageSizes: []int{5}},
		{name: "exactly one full page", query: "?limit=5", wantEvents: []int{1, 2, 3, 5, 4}, wantPageSizes: []int{5}},
		{name: "since includes events at that time", query: "?since=2026-09-29T14:01:00Z&limit=2", wantEvents: []int{2, 3, 5, 4}, wantPageSizes: []int{2, 2}},
		{name: "since between events", query: "?since=2026-09-29T14:01:30Z", wantEvents: []int{5, 4}, wantPageSizes: []int{2}},
		{name: "since with offset", query: "?since=2026-09-29T17:01:30%2B03:00", wantEvents: []int{5, 4}, wantPageSizes: []int{2}},
		{name: "since after every event", query: "?since=2026-09-30T00:00:00Z", wantEvents: nil, wantPageSizes: []int{0}},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			eventIDs, pageSizes := walkHistory(t, server, "/devices/"+deviceID+"/events"+testCase.query)
			var wantIDs []string
			for _, n := range testCase.wantEvents {
				wantIDs = append(wantIDs, historyEventID(n))
			}
			if !slices.Equal(eventIDs, wantIDs) {
				t.Errorf("events = %v, want %v", eventIDs, wantIDs)
			}
			if !slices.Equal(pageSizes, testCase.wantPageSizes) {
				t.Errorf("page sizes = %v, want %v", pageSizes, testCase.wantPageSizes)
			}
		})
	}
}

func TestHistoryDefaultLimit(t *testing.T) {
	testStore, conn := newTestStore(t)
	deviceID := newDeviceID(t, conn)
	server := NewServer(testToken, testOfflineThreshold, testStore)
	occurredAts := make([]string, defaultHistoryLimit+1)
	for index := range occurredAts {
		occurredAts[index] = fmt.Sprintf("2026-09-29T14:%02d:00Z", index)
	}
	postHistory(t, server, deviceID, occurredAts...)

	_, pageSizes := walkHistory(t, server, "/devices/"+deviceID+"/events")
	if want := []int{defaultHistoryLimit, 1}; !slices.Equal(pageSizes, want) {
		t.Errorf("page sizes = %v, want %v", pageSizes, want)
	}
}

func TestHistoryEmptyPage(t *testing.T) {
	testStore, conn := newTestStore(t)
	deviceID := newDeviceID(t, conn)
	server := NewServer(testToken, testOfflineThreshold, testStore)
	postHistory(t, server, deviceID, "2026-09-29T14:00:00Z")

	recorder := getHistory(t, server, "/devices/"+deviceID+"/events?since=2026-09-30T00:00:00Z")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if want := `{"events":[],"next":null}` + "\n"; recorder.Body.String() != want {
		t.Errorf("body = %s, want %s", recorder.Body, want)
	}
}
