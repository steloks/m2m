package device

import (
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/steloks/m2m/internal/event"
)

type Device struct {
	ID          string
	LastSeenAt  time.Time
	ArmState    string
	ArmMode     *string
	OpenSensors []int
}

type eventPayload struct {
	Mode     string `json:"mode"`
	SensorID int    `json:"sensor_id"`
}

// Apply updates the device's state with a new event. Events must be applied in
// the order they arrive.
func (device *Device) Apply(received event.Event) error {
	var fields eventPayload
	if err := json.Unmarshal(received.Payload, &fields); err != nil {
		return fmt.Errorf("decode %s payload: %w", received.Type, err)
	}

	device.LastSeenAt = received.ReceivedAt
	switch received.Type {
	case "armed":
		device.ArmState = "armed"
		device.ArmMode = &fields.Mode
	case "disarmed":
		device.ArmState = "disarmed"
		device.ArmMode = nil
	case "sensor_opened":
		index, open := slices.BinarySearch(device.OpenSensors, fields.SensorID)
		if !open {
			device.OpenSensors = slices.Insert(device.OpenSensors, index, fields.SensorID)
		}
	case "sensor_closed":
		device.OpenSensors = slices.DeleteFunc(device.OpenSensors, func(sensorID int) bool {
			return sensorID == fields.SensorID
		})
	}
	return nil
}
