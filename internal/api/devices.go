package api

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/steloks/m2m/internal/device"
)

type deviceResponse struct {
	DeviceID    string  `json:"device_id"`
	Status      string  `json:"status"`
	LastSeenAt  string  `json:"last_seen_at"`
	ArmState    string  `json:"arm_state"`
	ArmMode     *string `json:"arm_mode"`
	OpenSensors []int   `json:"open_sensors"`
}

func (server *Server) handleGetDevice(writer http.ResponseWriter, request *http.Request) {
	deviceID := request.PathValue("id")

	found, err := server.store.GetDevice(request.Context(), deviceID)
	if errors.Is(err, device.ErrNotFound) {
		writeError(writer, http.StatusNotFound, "device not found")
		return
	}
	if err != nil {
		slog.Error("get device", "device_id", deviceID, "err", err)
		writeError(writer, http.StatusInternalServerError, "internal error")
		return
	}

	openSensors := found.OpenSensors
	if openSensors == nil {
		openSensors = []int{}
	}
	writeJSON(writer, http.StatusOK, deviceResponse{
		DeviceID:    found.ID,
		Status:      found.Status(server.now(), server.offlineThreshold),
		LastSeenAt:  found.LastSeenAt.UTC().Format(time.RFC3339Nano),
		ArmState:    found.ArmState,
		ArmMode:     found.ArmMode,
		OpenSensors: openSensors,
	})
}
