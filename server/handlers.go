package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/disaster37/gobot-simulate/internal/state"
)

type errorEnvelope struct {
	Error apiError `json:"error"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	BoardID string `json:"boardId,omitempty"`
}

type boardsResponse struct {
	Boards []state.BoardState `json:"boards"`
}

type devicesResponse struct {
	Devices []state.DeviceState `json:"devices"`
}

type healthResponse struct {
	Status        string `json:"status"`
	Version       string `json:"version"`
	UptimeSeconds int64  `json:"uptimeSeconds"`
}

type devicePatchRequest struct {
	Active    *bool    `json:"active"`
	Value     *float64 `json:"value"`
	ByteValue *byte    `json:"byteValue"`
	Angle     *byte    `json:"angle"`
}

type pinPatchRequest struct {
	DigitalValue *int           `json:"digitalValue"`
	ByteValue    *byte          `json:"byteValue"`
	AnalogValue  *int           `json:"analogValue"`
	Mode         *state.PinMode `json:"mode"`
}

func (s *Server) handleHealth(_ context.Context, c *app.RequestContext) {
	snap := s.backend.Snapshot()
	c.JSON(http.StatusOK, healthResponse{
		Status:        "ok",
		Version:       s.cfg.Version,
		UptimeSeconds: snap.Meta.UptimeSeconds,
	})
}

func (s *Server) handleSnapshot(_ context.Context, c *app.RequestContext) {
	c.JSON(http.StatusOK, s.backend.Snapshot())
}

func (s *Server) handleBoards(_ context.Context, c *app.RequestContext) {
	boards := s.backend.Boards()
	if boards == nil {
		boards = []state.BoardState{}
	}
	c.JSON(http.StatusOK, boardsResponse{Boards: boards})
}

func (s *Server) handleBoard(_ context.Context, c *app.RequestContext) {
	boardID := c.Param("board")
	if !validatePath(c, "board", boardID) {
		return
	}
	board, err := s.backend.Board(boardID)
	if err != nil {
		writeBackendError(c, boardID, err)
		return
	}
	c.JSON(http.StatusOK, board)
}

func (s *Server) handleDevices(_ context.Context, c *app.RequestContext) {
	boardID := c.Param("board")
	if !validatePath(c, "board", boardID) {
		return
	}
	devices, err := s.backend.Devices(boardID)
	if err != nil {
		writeBackendError(c, boardID, err)
		return
	}
	if devices == nil {
		devices = []state.DeviceState{}
	}
	c.JSON(http.StatusOK, devicesResponse{Devices: devices})
}

func (s *Server) handleDevice(_ context.Context, c *app.RequestContext) {
	boardID := c.Param("board")
	deviceID := c.Param("device")
	if !validatePath(c, "board", boardID) || !validatePath(c, "device", deviceID) {
		return
	}
	device, err := s.backend.Device(boardID, deviceID)
	if err != nil {
		writeBackendError(c, boardID, err)
		return
	}
	c.JSON(http.StatusOK, device)
}

func (s *Server) handlePatchDevice(_ context.Context, c *app.RequestContext) {
	boardID := c.Param("board")
	deviceID := c.Param("device")
	if !validatePath(c, "board", boardID) || !validatePath(c, "device", deviceID) {
		return
	}
	var req devicePatchRequest
	if err := decodeJSON(c, &req); err != nil {
		writeError(c, boardID, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}
	if req.Active == nil && req.Value == nil && req.ByteValue == nil && req.Angle == nil {
		writeBackendError(c, boardID, state.ErrInvalidValue)
		return
	}

	device, err := s.backend.Device(boardID, deviceID)
	if err != nil {
		writeBackendError(c, boardID, err)
		return
	}
	if req.Active != nil {
		if err = s.setDeviceActive(boardID, device, *req.Active); err != nil {
			writeBackendError(c, boardID, err)
			return
		}
	}
	if req.Value != nil {
		if err = s.setDeviceValue(boardID, device, *req.Value); err != nil {
			writeBackendError(c, boardID, err)
			return
		}
	}
	if req.ByteValue != nil {
		if err = s.setDeviceByte(boardID, device, *req.ByteValue); err != nil {
			writeBackendError(c, boardID, err)
			return
		}
	}
	if req.Angle != nil {
		if err = s.setDeviceAngle(boardID, device, *req.Angle); err != nil {
			writeBackendError(c, boardID, err)
			return
		}
	}

	updated, err := s.backend.Device(boardID, deviceID)
	if err != nil {
		writeBackendError(c, boardID, err)
		return
	}
	c.JSON(http.StatusOK, updated)
}

func (s *Server) setDeviceActive(boardID string, device state.DeviceState, active bool) error {
	switch device.Kind {
	case state.DeviceKindRelay, state.DeviceKindLED, state.DeviceKindButton, state.DeviceKindCaptor:
		if device.Active == active {
			return nil
		}
		return s.backend.ToggleDevice(boardID, device.ID)
	default:
		return state.ErrReadOnly
	}
}

func (s *Server) setDeviceValue(boardID string, device state.DeviceState, value float64) error {
	switch device.Kind {
	case state.DeviceKindValue:
		return s.backend.SetValue(boardID, device.ValueName, value, state.SourceUI)
	case state.DeviceKindAnalog, state.DeviceKindSensor:
		if len(device.Pins) == 0 {
			return state.ErrInvalidValue
		}
		return s.backend.SetAnalog(boardID, device.Pins[0], int(value), state.SourceUI)
	case state.DeviceKindPWM:
		if len(device.Pins) == 0 || value < 0 || value > 255 {
			return state.ErrInvalidValue
		}
		return s.backend.SetPWM(boardID, device.Pins[0], byte(value), state.SourceUI)
	case state.DeviceKindServo:
		if len(device.Pins) == 0 || value < 0 || value > 180 {
			return state.ErrInvalidValue
		}
		return s.backend.SetServo(boardID, device.Pins[0], byte(value), state.SourceUI)
	default:
		return state.ErrReadOnly
	}
}

func (s *Server) setDeviceByte(boardID string, device state.DeviceState, value byte) error {
	if len(device.Pins) == 0 {
		return state.ErrInvalidValue
	}
	switch device.Kind {
	case state.DeviceKindPWM:
		return s.backend.SetPWM(boardID, device.Pins[0], value, state.SourceUI)
	case state.DeviceKindServo:
		return s.backend.SetServo(boardID, device.Pins[0], value, state.SourceUI)
	case state.DeviceKindAnalog, state.DeviceKindSensor:
		return s.backend.SetAnalog(boardID, device.Pins[0], int(value), state.SourceUI)
	default:
		return state.ErrReadOnly
	}
}

func (s *Server) setDeviceAngle(boardID string, device state.DeviceState, angle byte) error {
	if device.Kind != state.DeviceKindServo || len(device.Pins) == 0 {
		return state.ErrReadOnly
	}
	return s.backend.SetServo(boardID, device.Pins[0], angle, state.SourceUI)
}

func (s *Server) handlePress(_ context.Context, c *app.RequestContext) {
	boardID := c.Param("board")
	deviceID := c.Param("device")
	if !validatePath(c, "board", boardID) || !validatePath(c, "device", deviceID) {
		return
	}
	var req struct {
		HoldMs *int `json:"holdMs"`
	}
	if err := decodeOptionalJSON(c, &req); err != nil {
		writeError(c, boardID, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}
	holdMs := 0
	if req.HoldMs != nil {
		holdMs = *req.HoldMs
		if holdMs < 0 || holdMs > 60000 {
			writeBackendError(c, boardID, state.ErrInvalidValue)
			return
		}
	}
	if err := s.backend.PressButton(boardID, deviceID); err != nil {
		writeBackendError(c, boardID, err)
		return
	}
	if holdMs > 0 {
		go func() {
			time.Sleep(time.Duration(holdMs) * time.Millisecond)
			_ = s.backend.ReleaseButton(boardID, deviceID)
		}()
	}
	s.writeDevice(c, boardID, deviceID)
}

func (s *Server) handleRelease(_ context.Context, c *app.RequestContext) {
	boardID := c.Param("board")
	deviceID := c.Param("device")
	if !validatePath(c, "board", boardID) || !validatePath(c, "device", deviceID) {
		return
	}
	if err := decodeOptionalJSON(c, &struct{}{}); err != nil && !errors.Is(err, io.EOF) {
		writeError(c, boardID, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}
	if err := s.backend.ReleaseButton(boardID, deviceID); err != nil {
		writeBackendError(c, boardID, err)
		return
	}
	s.writeDevice(c, boardID, deviceID)
}

func (s *Server) handlePin(_ context.Context, c *app.RequestContext) {
	boardID := c.Param("board")
	pin := c.Param("pin")
	if !validatePath(c, "board", boardID) || !validatePath(c, "pin", pin) {
		return
	}
	var req pinPatchRequest
	if err := decodeJSON(c, &req); err != nil {
		writeError(c, boardID, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}

	applied := false
	if req.Mode != nil {
		if !req.Mode.Valid() {
			writeBackendError(c, boardID, state.ErrInvalidPinMode)
			return
		}
		if err := s.backend.BindPin(boardID, pin, "", *req.Mode); err != nil {
			writeBackendError(c, boardID, err)
			return
		}
		applied = true
	}
	if req.DigitalValue != nil {
		if *req.DigitalValue != 0 && *req.DigitalValue != 1 {
			writeBackendError(c, boardID, state.ErrInvalidValue)
			return
		}
		if err := s.backend.SetDigital(boardID, pin, *req.DigitalValue, state.SourceUI); err != nil {
			writeBackendError(c, boardID, err)
			return
		}
		applied = true
	}
	if req.ByteValue != nil {
		if err := s.backend.SetPWM(boardID, pin, *req.ByteValue, state.SourceUI); err != nil {
			writeBackendError(c, boardID, err)
			return
		}
		applied = true
	}
	if req.AnalogValue != nil {
		if err := s.backend.SetAnalog(boardID, pin, *req.AnalogValue, state.SourceUI); err != nil {
			writeBackendError(c, boardID, err)
			return
		}
		applied = true
	}
	if !applied {
		writeBackendError(c, boardID, state.ErrInvalidValue)
		return
	}

	pinState, err := s.backend.Pin(boardID, pin)
	if err != nil {
		writeBackendError(c, boardID, err)
		return
	}
	c.JSON(http.StatusOK, pinState)
}

func (s *Server) handleGetValue(_ context.Context, c *app.RequestContext) {
	boardID := c.Param("board")
	name := c.Param("name")
	if !validatePath(c, "board", boardID) || !validatePath(c, "value", name) {
		return
	}
	values, err := s.backend.Values(boardID)
	if err != nil {
		writeBackendError(c, boardID, err)
		return
	}
	value, ok := values[name]
	if !ok {
		writeError(c, boardID, http.StatusNotFound, "VALUE_NOT_FOUND", "simulate: value not found: "+name)
		return
	}
	c.JSON(http.StatusOK, value)
}

func (s *Server) handleSetValue(_ context.Context, c *app.RequestContext) {
	boardID := c.Param("board")
	name := c.Param("name")
	if !validatePath(c, "board", boardID) || !validatePath(c, "value", name) {
		return
	}
	body, err := c.Body()
	if err != nil {
		writeError(c, boardID, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}
	var payload map[string]json.RawMessage
	if err = json.Unmarshal(body, &payload); err != nil {
		writeError(c, boardID, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}
	raw, ok := payload["value"]
	if !ok {
		writeBackendError(c, boardID, state.ErrInvalidValue)
		return
	}
	var value interface{}
	if err = json.Unmarshal(raw, &value); err != nil {
		writeError(c, boardID, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}
	if err = s.backend.SetValue(boardID, name, value, state.SourceUI); err != nil {
		writeBackendError(c, boardID, err)
		return
	}
	values, err := s.backend.Values(boardID)
	if err != nil {
		writeBackendError(c, boardID, err)
		return
	}
	c.JSON(http.StatusOK, values[name])
}

func (s *Server) handleFunction(_ context.Context, c *app.RequestContext) {
	boardID := c.Param("board")
	name := c.Param("name")
	if !validatePath(c, "board", boardID) || !validatePath(c, "function", name) {
		return
	}
	var req struct {
		Parameters string `json:"parameters"`
		ReturnCode *int   `json:"returnCode"`
		Execute    *bool  `json:"execute"`
	}
	if err := decodeOptionalJSON(c, &req); err != nil && !errors.Is(err, io.EOF) {
		writeError(c, boardID, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}
	if req.ReturnCode != nil {
		if err := s.backend.SetFunctionCode(boardID, name, *req.ReturnCode, state.SourceUI); err != nil {
			writeBackendError(c, boardID, err)
			return
		}
	}
	execute := true
	if req.Execute != nil {
		execute = *req.Execute
	}
	result := 0
	if execute {
		value, err := s.backend.CallFunction(boardID, name, req.Parameters, state.SourceUI)
		if err != nil {
			writeBackendError(c, boardID, err)
			return
		}
		result = value
	}
	board, err := s.backend.Board(boardID)
	if err != nil {
		writeBackendError(c, boardID, err)
		return
	}
	fn, ok := board.Functions[name]
	if !ok {
		fn = state.ExtraFunction{Name: name}
	}
	c.JSON(http.StatusOK, struct {
		Function state.ExtraFunction `json:"function"`
		Result   int                 `json:"result"`
	}{Function: fn, Result: result})
}

func (s *Server) handleSetFault(_ context.Context, c *app.RequestContext) {
	boardID := c.Param("board")
	if !validatePath(c, "board", boardID) {
		return
	}
	var req struct {
		Message string `json:"message"`
	}
	if err := decodeJSON(c, &req); err != nil {
		writeError(c, boardID, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}
	req.Message = strings.TrimSpace(req.Message)
	if req.Message == "" || len(req.Message) > 1024 {
		writeBackendError(c, boardID, state.ErrInvalidValue)
		return
	}
	if err := s.backend.SetFault(boardID, errors.New(req.Message)); err != nil {
		writeBackendError(c, boardID, err)
		return
	}
	s.writeFault(c, boardID)
}

func (s *Server) handleClearFault(_ context.Context, c *app.RequestContext) {
	boardID := c.Param("board")
	if !validatePath(c, "board", boardID) {
		return
	}
	if err := s.backend.ClearFault(boardID); err != nil {
		writeBackendError(c, boardID, err)
		return
	}
	s.writeFault(c, boardID)
}

func (s *Server) writeDevice(c *app.RequestContext, boardID, deviceID string) {
	device, err := s.backend.Device(boardID, deviceID)
	if err != nil {
		writeBackendError(c, boardID, err)
		return
	}
	c.JSON(http.StatusOK, device)
}

func (s *Server) writeFault(c *app.RequestContext, boardID string) {
	board, err := s.backend.Board(boardID)
	if err != nil {
		writeBackendError(c, boardID, err)
		return
	}
	c.JSON(http.StatusOK, board.Fault)
}

func validatePath(c *app.RequestContext, kind, value string) bool {
	if err := state.ValidateIdentifier(kind, value); err != nil {
		writeBackendError(c, "", err)
		return false
	}
	return true
}

func decodeJSON(c *app.RequestContext, dst interface{}) error {
	body, err := c.Body()
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return io.EOF
	}
	return json.Unmarshal(body, dst)
}

func decodeOptionalJSON(c *app.RequestContext, dst interface{}) error {
	err := decodeJSON(c, dst)
	if errors.Is(err, io.EOF) {
		return nil
	}
	return err
}

func writeBackendError(c *app.RequestContext, boardID string, err error) {
	status, code := classifyError(err)
	writeError(c, boardID, status, code, err.Error())
}

func classifyError(err error) (int, string) {
	switch {
	case errors.Is(err, state.ErrBoardNotFound):
		return http.StatusNotFound, "BOARD_NOT_FOUND"
	case errors.Is(err, state.ErrDeviceNotFound):
		return http.StatusNotFound, "DEVICE_NOT_FOUND"
	case errors.Is(err, state.ErrPinNotFound):
		return http.StatusNotFound, "PIN_NOT_FOUND"
	case errors.Is(err, state.ErrInvalidID):
		return http.StatusBadRequest, "INVALID_IDENTIFIER"
	case errors.Is(err, state.ErrInvalidPinMode):
		return http.StatusUnprocessableEntity, "INVALID_PIN_MODE"
	case errors.Is(err, state.ErrReadOnly):
		return http.StatusUnprocessableEntity, "READ_ONLY"
	case errors.Is(err, state.ErrInvalidValue):
		return http.StatusBadRequest, "INVALID_VALUE"
	case errors.Is(err, state.ErrClientCap):
		return http.StatusTooManyRequests, "TOO_MANY_CLIENTS"
	default:
		return http.StatusInternalServerError, "INTERNAL"
	}
}

func writeError(c *app.RequestContext, boardID string, status int, code, message string) {
	c.JSON(status, errorEnvelope{Error: apiError{Code: code, Message: message, BoardID: boardID}})
}

// parseEventID parses an SSE Last-Event-ID value.
func parseEventID(raw string) uint64 {
	if raw == "" {
		return 0
	}
	value, err := strconv.ParseUint(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return 0
	}
	return value
}
