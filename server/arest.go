package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/disaster37/gobot-simulate/internal/state"
)

// handleAREST emulates the aREST HTTP firmware API used by gobot-arest.
func (s *Server) handleAREST(_ context.Context, c *app.RequestContext) {
	boardID := c.Param("board")
	status, body := ARESTResponse(s.backend, boardID, string(c.Method()), c.Param("action"), c.Query("params"))
	c.Data(status, "application/json; charset=utf-8", body)
}

// ARESTResponse implements the aREST contract for one request and returns the
// HTTP status plus a JSON body. It is shared by the HTTP surface and the
// serial transport so both stay byte-for-byte consistent.
func ARESTResponse(backend Backend, boardID, method, rawAction, params string) (int, []byte) {
	if err := state.ValidateIdentifier("board", boardID); err != nil {
		return encodeError(http.StatusBadRequest, "INVALID_IDENTIFIER", err.Error(), boardID)
	}
	if err := backend.BoardFault(boardID); err != nil {
		return encodeError(http.StatusInternalServerError, "BOARD_FAULT", err.Error(), boardID)
	}
	if latency := backend.BoardLatency(boardID); latency > 0 && latency <= 30*time.Second {
		time.Sleep(latency)
	}
	action := "/" + strings.TrimLeft(strings.TrimSpace(rawAction), "/")
	method = strings.ToUpper(strings.TrimSpace(method))

	switch {
	case action == "/" && method == http.MethodGet:
		values, err := backend.Values(boardID)
		if err != nil {
			return encodeBackendError(err, boardID)
		}
		variables := make(map[string]interface{}, len(values))
		for name, value := range values {
			if isLegacyTestValue(name, value.Value) {
				continue
			}
			variables[name] = value.Value
		}
		return encodeJSON(http.StatusOK, map[string]interface{}{"variables": variables})
	case action == "/" && method == http.MethodPost:
		return encodeJSON(http.StatusOK, map[string]interface{}{"return_value": 1})
	case action == "/id":
		return encodeJSON(http.StatusOK, map[string]interface{}{"id": boardID, "name": boardID})
	}

	segments := strings.Split(strings.TrimPrefix(action, "/"), "/")
	switch {
	case len(segments) == 3 && segments[0] == "mode":
		if method != http.MethodPost {
			return encodeError(http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "POST required", boardID)
		}
		if err := backend.BindPinMode(boardID, segments[1], segments[2]); err != nil {
			return encodeBackendError(err, boardID)
		}
		return encodeJSON(http.StatusOK, map[string]interface{}{"return_value": 1})
	case len(segments) == 3 && segments[0] == "analog":
		return encodeError(http.StatusNotFound, "NOT_FOUND", "analog pin route not supported", boardID)
	case len(segments) == 3 && segments[0] == "digital":
		value, err := strconv.Atoi(segments[2])
		if err != nil {
			return encodeBackendError(state.ErrInvalidValue, boardID)
		}
		if err := backend.SetDigital(boardID, segments[1], value, state.SourceHardwareCode); err != nil {
			return encodeBackendError(err, boardID)
		}
		return encodeJSON(http.StatusOK, map[string]interface{}{"return_value": 1})
	case len(segments) == 2 && segments[0] == "digital":
		if method != http.MethodGet {
			return encodeError(http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "GET required", boardID)
		}
		value, err := backend.DigitalRead(boardID, segments[1])
		if err != nil {
			return encodeBackendError(err, boardID)
		}
		return encodeJSON(http.StatusOK, map[string]interface{}{"return_value": value})
	}

	name := strings.TrimPrefix(action, "/")
	if name == "" || strings.Contains(name, "/") {
		return encodeError(http.StatusNotFound, "NOT_FOUND", "resource not found", boardID)
	}
	switch method {
	case http.MethodGet:
		values, err := backend.Values(boardID)
		if err != nil {
			return encodeBackendError(err, boardID)
		}
		value, ok := values[name]
		if !ok || isLegacyTestValue(name, value.Value) {
			return encodeError(http.StatusNotFound, "VARIABLE_NOT_FOUND", "Variable not found", boardID)
		}
		return encodeJSON(http.StatusOK, map[string]interface{}{name: value.Value})
	case http.MethodPost:
		result, ok, err := callFunction(backend, boardID, name, params)
		if err != nil {
			return encodeBackendError(err, boardID)
		}
		if !ok {
			return encodeError(http.StatusNotFound, "FUNCTION_NOT_FOUND", "Function not found", boardID)
		}
		return encodeJSON(http.StatusOK, map[string]interface{}{"return_value": result})
	default:
		return encodeError(http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", boardID)
	}
}

func encodeJSON(status int, payload interface{}) (int, []byte) {
	data, err := json.Marshal(payload)
	if err != nil {
		return encodeError(http.StatusInternalServerError, "INTERNAL", "internal server error", "")
	}
	return status, data
}

func encodeError(status int, code, message, boardID string) (int, []byte) {
	return encodeJSON(status, errorEnvelope{Error: apiError{Code: code, Message: message, BoardID: boardID}})
}

func encodeBackendError(err error, boardID string) (int, []byte) {
	status, code := classifyError(err)
	return encodeError(status, code, err.Error(), boardID)
}

func callFunction(backend Backend, boardID, name, params string) (int, bool, error) {
	board, err := backend.Board(boardID)
	if err != nil {
		return 0, false, err
	}
	if _, ok := board.Functions[name]; !ok {
		return 0, false, nil
	}
	result, err := backend.CallFunction(boardID, name, params, state.SourceHardwareCode)
	if err != nil {
		return 0, false, err
	}
	return result, true, nil
}

func isLegacyTestValue(name string, value interface{}) bool {
	if name != "test" {
		return false
	}
	switch v := value.(type) {
	case int:
		return v == 99
	case int64:
		return v == 99
	case float64:
		return v == 99
	default:
		return false
	}
}

// handleParticle emulates the Particle cloud REST API used by the Particle
// adaptor (APIServer can be pointed at the simulator base URL).
func (s *Server) handleParticle(_ context.Context, c *app.RequestContext) {
	boardID := c.Param("board")
	if !validatePath(c, "board", boardID) {
		return
	}
	if err := s.honourBoard(boardID); err != nil {
		writeError(c, boardID, http.StatusInternalServerError, "BOARD_FAULT", err.Error())
		return
	}
	action := "/" + strings.TrimLeft(strings.TrimSpace(c.Param("action")), "/")
	if action == "/" {
		c.JSON(http.StatusOK, map[string]interface{}{"id": boardID, "connected": true})
		return
	}
	params := splitParams(c.PostForm("params"))
	switch action {
	case "/digitalread":
		if len(params) < 1 {
			writeBackendError(c, boardID, state.ErrInvalidValue)
			return
		}
		value, err := s.backend.DigitalRead(boardID, params[0])
		if err != nil {
			writeBackendError(c, boardID, err)
			return
		}
		c.JSON(http.StatusOK, map[string]interface{}{"return_value": value})
	case "/digitalwrite":
		if len(params) < 2 {
			writeBackendError(c, boardID, state.ErrInvalidValue)
			return
		}
		level := 0
		if strings.EqualFold(params[1], "HIGH") || params[1] == "1" {
			level = 1
		}
		if err := s.backend.SetDigital(boardID, params[0], level, state.SourceHardwareCode); err != nil {
			writeBackendError(c, boardID, err)
			return
		}
		c.JSON(http.StatusOK, map[string]interface{}{"return_value": 1})
	case "/analogread":
		if len(params) < 1 {
			writeBackendError(c, boardID, state.ErrInvalidValue)
			return
		}
		value, err := s.backend.AnalogRead(boardID, params[0])
		if err != nil {
			writeBackendError(c, boardID, err)
			return
		}
		c.JSON(http.StatusOK, map[string]interface{}{"return_value": value})
	case "/analogwrite":
		if len(params) < 2 {
			writeBackendError(c, boardID, state.ErrInvalidValue)
			return
		}
		level, err := strconv.Atoi(params[1])
		if err != nil || level < 0 || level > 255 {
			writeBackendError(c, boardID, state.ErrInvalidValue)
			return
		}
		if err := s.backend.SetAnalog(boardID, params[0], level, state.SourceHardwareCode); err != nil {
			writeBackendError(c, boardID, err)
			return
		}
		c.JSON(http.StatusOK, map[string]interface{}{"return_value": 1})
	case "/servoset":
		if len(params) < 2 {
			writeBackendError(c, boardID, state.ErrInvalidValue)
			return
		}
		angle, err := strconv.Atoi(params[1])
		if err != nil || angle < 0 || angle > 180 {
			writeBackendError(c, boardID, state.ErrInvalidValue)
			return
		}
		if err := s.backend.SetServo(boardID, params[0], byte(angle), state.SourceHardwareCode); err != nil {
			writeBackendError(c, boardID, err)
			return
		}
		c.JSON(http.StatusOK, map[string]interface{}{"return_value": 1})
	default:
		writeError(c, boardID, http.StatusNotFound, "NOT_FOUND", fmt.Sprintf("particle function %s not found", action))
	}
}

func (s *Server) honourBoard(boardID string) error {
	if err := s.backend.BoardFault(boardID); err != nil {
		return err
	}
	if latency := s.backend.BoardLatency(boardID); latency > 0 && latency <= 30*time.Second {
		time.Sleep(latency)
	}
	return nil
}

func splitParams(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}
