package device

import (
	"slices"
	"testing"
	"time"

	"github.com/steloks/m2m/internal/event"
)

var receivedAt = time.Date(2026, 9, 29, 14, 2, 0, 0, time.UTC)

func TestApply(t *testing.T) {
	away, stay := "away", "stay"

	tests := []struct {
		name      string
		start     Device
		eventType string
		payload   string
		want      Device
	}{
		{
			name:      "armed away",
			start:     Device{ArmState: "disarmed"},
			eventType: "armed",
			payload:   `{"mode":"away","user_id":3}`,
			want:      Device{ArmState: "armed", ArmMode: &away},
		},
		{
			name:      "armed again changes mode",
			start:     Device{ArmState: "armed", ArmMode: &away},
			eventType: "armed",
			payload:   `{"mode":"stay"}`,
			want:      Device{ArmState: "armed", ArmMode: &stay},
		},
		{
			name:      "disarmed clears mode",
			start:     Device{ArmState: "armed", ArmMode: &away},
			eventType: "disarmed",
			payload:   `{"user_id":3}`,
			want:      Device{ArmState: "disarmed"},
		},
		{
			name:      "sensor opened keeps sensors sorted",
			start:     Device{ArmState: "disarmed", OpenSensors: []int{2, 9}},
			eventType: "sensor_opened",
			payload:   `{"sensor_id":4}`,
			want:      Device{ArmState: "disarmed", OpenSensors: []int{2, 4, 9}},
		},
		{
			name:      "sensor opened twice is listed once",
			start:     Device{ArmState: "disarmed", OpenSensors: []int{4}},
			eventType: "sensor_opened",
			payload:   `{"sensor_id":4}`,
			want:      Device{ArmState: "disarmed", OpenSensors: []int{4}},
		},
		{
			name:      "sensor closed",
			start:     Device{ArmState: "disarmed", OpenSensors: []int{2, 4, 9}},
			eventType: "sensor_closed",
			payload:   `{"sensor_id":4}`,
			want:      Device{ArmState: "disarmed", OpenSensors: []int{2, 9}},
		},
		{
			name:      "closing a sensor that is not open",
			start:     Device{ArmState: "disarmed", OpenSensors: []int{2}},
			eventType: "sensor_closed",
			payload:   `{"sensor_id":4}`,
			want:      Device{ArmState: "disarmed", OpenSensors: []int{2}},
		},
		{
			name:      "heartbeat changes only last seen",
			start:     Device{ArmState: "armed", ArmMode: &away, OpenSensors: []int{4}},
			eventType: "heartbeat",
			payload:   `{"battery_pct":87,"rssi_dbm":-71}`,
			want:      Device{ArmState: "armed", ArmMode: &away, OpenSensors: []int{4}},
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			device := testCase.start
			device.LastSeenAt = receivedAt.Add(-time.Hour)
			if err := device.Apply(event.Event{Type: testCase.eventType, Payload: []byte(testCase.payload), ReceivedAt: receivedAt}); err != nil {
				t.Fatalf("Apply: %v", err)
			}

			if !device.LastSeenAt.Equal(receivedAt) {
				t.Errorf("LastSeenAt = %v, want %v", device.LastSeenAt, receivedAt)
			}
			if device.ArmState != testCase.want.ArmState {
				t.Errorf("ArmState = %q, want %q", device.ArmState, testCase.want.ArmState)
			}
			if gotMode, wantMode := modeString(device.ArmMode), modeString(testCase.want.ArmMode); gotMode != wantMode {
				t.Errorf("ArmMode = %s, want %s", gotMode, wantMode)
			}
			if !slices.Equal(device.OpenSensors, testCase.want.OpenSensors) {
				t.Errorf("OpenSensors = %v, want %v", device.OpenSensors, testCase.want.OpenSensors)
			}
		})
	}
}

func TestApplyRejectsInvalidPayload(t *testing.T) {
	device := Device{ArmState: "disarmed"}
	if err := device.Apply(event.Event{Type: "armed", Payload: []byte(`not json`), ReceivedAt: receivedAt}); err == nil {
		t.Fatal("Apply returned no error")
	}
}

func modeString(mode *string) string {
	if mode == nil {
		return "null"
	}
	return *mode
}
