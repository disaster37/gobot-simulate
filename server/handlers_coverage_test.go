package server_test

import (
	"net/http"
	"testing"

	"github.com/cloudwego/hertz/pkg/common/ut"
	simulate "github.com/disaster37/gobot-simulate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmptyBoardsAndDevicesLists(t *testing.T) {
	sim := newServerSim(t)

	boards := doRequest(sim, http.MethodGet, "/api/v1/boards", nil)
	require.Equal(t, http.StatusOK, boards.Code)
	assert.Contains(t, boards.Body.String(), `"boards":[]`)

	// A board with no devices returns an empty array, not null.
	sim.Adaptor("dfp")
	devices := doRequest(sim, http.MethodGet, "/api/v1/boards/dfp/devices", nil)
	require.Equal(t, http.StatusOK, devices.Code)
	assert.Contains(t, devices.Body.String(), `"devices":[]`)
}

func TestValueEndpointsOnUnknownBoard(t *testing.T) {
	sim := newServerSim(t)

	get := doRequest(sim, http.MethodGet, "/api/v1/boards/unknown/values/distance", nil)
	assert.Equal(t, http.StatusNotFound, get.Code)
	assert.Contains(t, get.Body.String(), "BOARD_NOT_FOUND")

	put := doRequest(sim, http.MethodPut, "/api/v1/boards/unknown/values/distance", []byte(`{"value":1}`))
	assert.Equal(t, http.StatusNotFound, put.Code)
}

func TestSetDeviceByteNoPinAndServo(t *testing.T) {
	sim := newServerSim(t)
	sim.Adaptor("dfp")
	_, err := sim.RegisterDevice("dfp", simulate.DeviceSpec{ID: "pwm_nopin", Kind: simulate.DeviceKindPWM})
	require.NoError(t, err)
	_, err = sim.RegisterDevice("dfp", simulate.DeviceSpec{ID: "servo", Kind: simulate.DeviceKindServo, Pin: "9"})
	require.NoError(t, err)

	noPin := doRequest(sim, http.MethodPatch, "/api/v1/boards/dfp/devices/pwm_nopin", []byte(`{"byteValue":5}`))
	assert.Equal(t, http.StatusBadRequest, noPin.Code)
	assert.Contains(t, noPin.Body.String(), "INVALID_VALUE")

	servo := doRequest(sim, http.MethodPatch, "/api/v1/boards/dfp/devices/servo", []byte(`{"byteValue":90}`))
	require.Equal(t, http.StatusOK, servo.Code, servo.Body.String())
}

func TestHandlePinByteAndAnalogOnUnknownBoard(t *testing.T) {
	sim := newServerSim(t)

	bytePin := doRequest(sim, http.MethodPost, "/api/v1/boards/unknown/pins/5", []byte(`{"byteValue":1}`))
	assert.Equal(t, http.StatusNotFound, bytePin.Code)

	analogPin := doRequest(sim, http.MethodPost, "/api/v1/boards/unknown/pins/A0", []byte(`{"analogValue":1}`))
	assert.Equal(t, http.StatusNotFound, analogPin.Code)
}

func TestFunctionExecuteFalseWithoutRegisteredFunction(t *testing.T) {
	sim := newServerSim(t)
	sim.Adaptor("dfp")
	_, err := sim.RegisterDevice("dfp", simulate.DeviceSpec{ID: "function_ack", Kind: simulate.DeviceKindFunction, FunctionName: "ack"})
	require.NoError(t, err)

	response := doRequest(sim, http.MethodPost, "/api/v1/boards/dfp/functions/ack", []byte(`{"execute":false}`))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Contains(t, response.Body.String(), `"callCount":0`)
	assert.Contains(t, response.Body.String(), `"result":0`)
}

func TestBodyLimitValidContentLengthPasses(t *testing.T) {
	sim := newServerSim(t)
	sim.Adaptor("dfp")

	response := doRequest(sim, http.MethodPost, "/api/v1/boards/dfp/fault", []byte(`{"message":"ok"}`),
		ut.Header{Key: "Content-Length", Value: "100"})
	assert.Equal(t, http.StatusOK, response.Code, response.Body.String())
}
