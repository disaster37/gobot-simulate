package server_test

import (
	"net/http"
	"testing"

	simulate "github.com/disaster37/gobot-simulate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupRichBoard(t *testing.T) *simulate.Simulator {
	t.Helper()
	sim := newServerSim(t)
	sim.Adaptor("dfp", simulate.WithAdaptorName("fat"), simulate.WithInputPullup("11"))
	for _, spec := range []simulate.DeviceSpec{
		{ID: "relay_drum", Kind: simulate.DeviceKindRelay, Pin: "3"},
		{ID: "led", Kind: simulate.DeviceKindLED, Pin: "7"},
		{ID: "button_start", Kind: simulate.DeviceKindButton, Pin: "11", DefaultState: simulate.IntPtr(1)},
		{ID: "captor", Kind: simulate.DeviceKindCaptor, Pin: "12", DefaultState: simulate.IntPtr(1)},
		{ID: "pwm_fan", Kind: simulate.DeviceKindPWM, Pin: "5"},
		{ID: "servo_valve", Kind: simulate.DeviceKindServo, Pin: "9"},
		{ID: "sensor_level", Kind: simulate.DeviceKindSensor, Pin: "A0"},
		{ID: "value_distance", Kind: simulate.DeviceKindValue, ValueName: "distance"},
		{ID: "function_ack", Kind: simulate.DeviceKindFunction, FunctionName: "ack"},
	} {
		_, err := sim.RegisterDevice("dfp", spec)
		require.NoError(t, err)
	}
	return sim
}

func TestPatchDeviceErrorBranches(t *testing.T) {
	sim := setupRichBoard(t)

	tests := []struct {
		name       string
		path       string
		body       string
		wantStatus int
		wantCode   string
	}{
		{"no fields", "/api/v1/boards/dfp/devices/relay_drum", `{}`, http.StatusBadRequest, "INVALID_VALUE"},
		{"relay active read only", "/api/v1/boards/dfp/devices/relay_drum", `{"active":false}`, http.StatusUnprocessableEntity, "READ_ONLY"},
		{"pwm value over range", "/api/v1/boards/dfp/devices/pwm_fan", `{"value":300}`, http.StatusBadRequest, "INVALID_VALUE"},
		{"pwm value ok", "/api/v1/boards/dfp/devices/pwm_fan", `{"value":100}`, http.StatusOK, ""},
		{"servo value over range", "/api/v1/boards/dfp/devices/servo_valve", `{"value":200}`, http.StatusBadRequest, "INVALID_VALUE"},
		{"servo value ok", "/api/v1/boards/dfp/devices/servo_valve", `{"value":90}`, http.StatusOK, ""},
		{"function value read only", "/api/v1/boards/dfp/devices/function_ack", `{"value":1}`, http.StatusUnprocessableEntity, "READ_ONLY"},
		{"sensor byte ok", "/api/v1/boards/dfp/devices/sensor_level", `{"byteValue":5}`, http.StatusOK, ""},
		{"relay byte read only", "/api/v1/boards/dfp/devices/relay_drum", `{"byteValue":5}`, http.StatusUnprocessableEntity, "READ_ONLY"},
		{"relay angle read only", "/api/v1/boards/dfp/devices/relay_drum", `{"angle":90}`, http.StatusUnprocessableEntity, "READ_ONLY"},
		{"servo angle over range", "/api/v1/boards/dfp/devices/servo_valve", `{"angle":200}`, http.StatusBadRequest, "INVALID_VALUE"},
		{"led active read only", "/api/v1/boards/dfp/devices/led", `{"active":true}`, http.StatusUnprocessableEntity, "READ_ONLY"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := doRequest(sim, http.MethodPatch, tt.path, []byte(tt.body))
			assert.Equal(t, tt.wantStatus, response.Code, response.Body.String())
			if tt.wantCode != "" {
				assert.Contains(t, response.Body.String(), tt.wantCode)
			}
		})
	}
}

func TestPressReleaseErrorBranches(t *testing.T) {
	sim := setupRichBoard(t)

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
		wantCode   string
	}{
		{"press relay readonly", http.MethodPost, "/api/v1/boards/dfp/devices/relay_drum/press", `{}`, http.StatusUnprocessableEntity, "READ_ONLY"},
		{"press unknown", http.MethodPost, "/api/v1/boards/dfp/devices/nope/press", `{}`, http.StatusNotFound, "DEVICE_NOT_FOUND"},
		{"press invalid json", http.MethodPost, "/api/v1/boards/dfp/devices/button_start/press", `{`, http.StatusBadRequest, "INVALID_JSON"},
		{"press hold too long", http.MethodPost, "/api/v1/boards/dfp/devices/button_start/press", `{"holdMs":60001}`, http.StatusBadRequest, "INVALID_VALUE"},
		{"release relay readonly", http.MethodPost, "/api/v1/boards/dfp/devices/relay_drum/release", `{}`, http.StatusUnprocessableEntity, "READ_ONLY"},
		{"release unknown", http.MethodPost, "/api/v1/boards/dfp/devices/nope/release", `{}`, http.StatusNotFound, "DEVICE_NOT_FOUND"},
		{"release invalid json", http.MethodPost, "/api/v1/boards/dfp/devices/button_start/release", `{`, http.StatusBadRequest, "INVALID_JSON"},
		{"release ok", http.MethodPost, "/api/v1/boards/dfp/devices/button_start/release", `{}`, http.StatusOK, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := doRequest(sim, tt.method, tt.path, []byte(tt.body))
			assert.Equal(t, tt.wantStatus, response.Code, response.Body.String())
			if tt.wantCode != "" {
				assert.Contains(t, response.Body.String(), tt.wantCode)
			}
		})
	}
}

func TestHandlePinErrorBranches(t *testing.T) {
	sim := setupRichBoard(t)

	badDigital := doRequest(sim, http.MethodPost, "/api/v1/boards/dfp/pins/3", []byte(`{"digitalValue":256}`))
	assert.Equal(t, http.StatusBadRequest, badDigital.Code)
	assert.Contains(t, badDigital.Body.String(), "INVALID_VALUE")

	badBoard := doRequest(sim, http.MethodPost, "/api/v1/boards/unknown/pins/3", []byte(`{"digitalValue":0}`))
	assert.Equal(t, http.StatusNotFound, badBoard.Code)
	assert.Contains(t, badBoard.Body.String(), "BOARD_NOT_FOUND")

	badPin := doRequest(sim, http.MethodPost, "/api/v1/boards/dfp/pins/bad@pin", []byte(`{"digitalValue":0}`))
	assert.Equal(t, http.StatusBadRequest, badPin.Code)
	assert.Contains(t, badPin.Body.String(), "INVALID_IDENTIFIER")
}

func TestSetValueMalformedJSON(t *testing.T) {
	sim := setupRichBoard(t)

	array := doRequest(sim, http.MethodPut, "/api/v1/boards/dfp/values/distance", []byte(`[1,2,3]`))
	assert.Equal(t, http.StatusBadRequest, array.Code)
	assert.Contains(t, array.Body.String(), "INVALID_JSON")

	badName := doRequest(sim, http.MethodPut, "/api/v1/boards/dfp/values/bad@name", []byte(`{"value":1}`))
	assert.Equal(t, http.StatusBadRequest, badName.Code)
	assert.Contains(t, badName.Body.String(), "INVALID_IDENTIFIER")
}

func TestHandleFunctionBranches(t *testing.T) {
	sim := setupRichBoard(t)

	badJSON := doRequest(sim, http.MethodPost, "/api/v1/boards/dfp/functions/ack", []byte(`{`))
	assert.Equal(t, http.StatusBadRequest, badJSON.Code)
	assert.Contains(t, badJSON.Body.String(), "INVALID_JSON")

	badBoard := doRequest(sim, http.MethodPost, "/api/v1/boards/unknown/functions/ack", []byte(`{}`))
	assert.Equal(t, http.StatusNotFound, badBoard.Code)
	assert.Contains(t, badBoard.Body.String(), "BOARD_NOT_FOUND")
}

func TestFaultBranches(t *testing.T) {
	sim := setupRichBoard(t)

	tooLong := doRequest(sim, http.MethodPost, "/api/v1/boards/dfp/fault", []byte(`{"message":"`+longString(1025)+`"}`))
	assert.Equal(t, http.StatusBadRequest, tooLong.Code)
	assert.Contains(t, tooLong.Body.String(), "INVALID_VALUE")

	badJSON := doRequest(sim, http.MethodPost, "/api/v1/boards/dfp/fault", []byte(`{`))
	assert.Equal(t, http.StatusBadRequest, badJSON.Code)

	badBoard := doRequest(sim, http.MethodPost, "/api/v1/boards/unknown/fault", []byte(`{"message":"x"}`))
	assert.Equal(t, http.StatusNotFound, badBoard.Code)

	clearNoFault := doRequest(sim, http.MethodDelete, "/api/v1/boards/dfp/fault", nil)
	assert.Equal(t, http.StatusOK, clearNoFault.Code)
	assert.Contains(t, clearNoFault.Body.String(), `"active":false`)

	clearBadBoard := doRequest(sim, http.MethodDelete, "/api/v1/boards/unknown/fault", nil)
	assert.Equal(t, http.StatusNotFound, clearBadBoard.Code)
}

func TestDevicesAndDeviceNotFound(t *testing.T) {
	sim := setupRichBoard(t)

	devicesMissing := doRequest(sim, http.MethodGet, "/api/v1/boards/unknown/devices", nil)
	assert.Equal(t, http.StatusNotFound, devicesMissing.Code)
	assert.Contains(t, devicesMissing.Body.String(), "BOARD_NOT_FOUND")
}

func TestStaticAssetServing(t *testing.T) {
	sim := newServerSim(t)

	index := doRequest(sim, http.MethodGet, "/index.html", nil)
	require.Equal(t, http.StatusOK, index.Code)
	assert.Contains(t, index.Body.String(), "Gobot Simulator")

	asset := doRequest(sim, http.MethodGet, "/_nuxt/builds/latest.json", nil)
	require.Equal(t, http.StatusOK, asset.Code)
	assert.Equal(t, "public, max-age=31536000, immutable", asset.Result().Header.Get("Cache-Control"))
}

func longString(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'a'
	}
	return string(b)
}
