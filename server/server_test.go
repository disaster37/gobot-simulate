package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/cloudwego/hertz/pkg/common/ut"
	simulate "github.com/disaster37/gobot-simulate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newServerSim(t *testing.T, opts ...simulate.Option) *simulate.Simulator {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	options := []simulate.Option{
		simulate.WithAddr("127.0.0.1"),
		simulate.WithPort(0),
	}
	options = append(options, opts...)
	sim := simulate.New(options...)
	require.NoError(t, sim.Start(ctx))
	t.Cleanup(func() {
		_ = sim.Stop(context.Background())
		cancel()
	})
	return sim
}

func populateBoard(t *testing.T, sim *simulate.Simulator) {
	t.Helper()
	sim.Adaptor("dfp",
		simulate.WithAdaptorName("fat"),
		simulate.WithPlatform("raspberry-pi"),
		simulate.WithInputPullup("11"),
		simulate.WithInitialValue("distance", float64(0)),
	)
	_, err := sim.RegisterDevice("dfp", simulate.DeviceSpec{
		ID: "relay_drum", Label: "Drum", Kind: simulate.DeviceKindRelay, Pin: "3",
	})
	require.NoError(t, err)
	_, err = sim.RegisterDevice("dfp", simulate.DeviceSpec{
		ID: "button_start", Label: "Start", Kind: simulate.DeviceKindButton, Pin: "11", DefaultState: simulate.IntPtr(1),
	})
	require.NoError(t, err)
	_, err = sim.RegisterDevice("dfp", simulate.DeviceSpec{
		ID: "distance", Label: "Distance", Kind: simulate.DeviceKindValue, ValueName: "distance",
	})
	require.NoError(t, err)
	_, err = sim.RegisterDevice("dfp", simulate.DeviceSpec{
		ID: "ack", Label: "Acknowledge", Kind: simulate.DeviceKindFunction, FunctionName: "acknoledgeRebooted",
	})
	require.NoError(t, err)
}

func doRequest(sim *simulate.Simulator, method, path string, body []byte, headers ...ut.Header) *ut.ResponseRecorder {
	var payload *ut.Body
	if body != nil {
		payload = &ut.Body{Body: bytes.NewReader(body), Len: len(body)}
		headers = append(headers, ut.Header{Key: "Content-Type", Value: "application/json"})
	}
	return ut.PerformRequest(sim.Hertz().Engine, method, path, payload, headers...)
}

func decodeMap(t *testing.T, body []byte) map[string]interface{} {
	t.Helper()
	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &payload))
	return payload
}

func TestRESTEndpoints(t *testing.T) {
	sim := newServerSim(t)
	populateBoard(t, sim)

	health := doRequest(sim, http.MethodGet, "/api/v1/health", nil)
	require.Equal(t, http.StatusOK, health.Code)
	healthPayload := decodeMap(t, health.Body.Bytes())
	assert.Equal(t, "ok", healthPayload["status"])

	snapshot := doRequest(sim, http.MethodGet, "/api/v1/snapshot", nil)
	require.Equal(t, http.StatusOK, snapshot.Code)
	var snap simulate.Snapshot
	require.NoError(t, json.Unmarshal(snapshot.Body.Bytes(), &snap))
	require.Len(t, snap.Boards, 1)
	assert.Equal(t, "dfp", snap.Boards[0].ID)
	assert.Equal(t, float64(99), snap.Boards[0].Values["test"].Value)

	boards := doRequest(sim, http.MethodGet, "/api/v1/boards", nil)
	require.Equal(t, http.StatusOK, boards.Code)
	assert.Contains(t, boards.Body.String(), `"boards"`)

	board := doRequest(sim, http.MethodGet, "/api/v1/boards/dfp", nil)
	require.Equal(t, http.StatusOK, board.Code)
	assert.Contains(t, board.Body.String(), `"name":"fat"`)

	devices := doRequest(sim, http.MethodGet, "/api/v1/boards/dfp/devices", nil)
	require.Equal(t, http.StatusOK, devices.Code)
	assert.Contains(t, devices.Body.String(), `"relay_drum"`)

	device := doRequest(sim, http.MethodGet, "/api/v1/boards/dfp/devices/relay_drum", nil)
	require.Equal(t, http.StatusOK, device.Code)

	patch := doRequest(sim, http.MethodPatch, "/api/v1/boards/dfp/devices/relay_drum", []byte(`{"active":true}`))
	require.Equal(t, http.StatusOK, patch.Code, patch.Body.String())
	var patched simulate.DeviceState
	require.NoError(t, json.Unmarshal(patch.Body.Bytes(), &patched))
	assert.True(t, patched.Active)
	pin, err := sim.Pin("dfp", "3")
	require.NoError(t, err)
	assert.Equal(t, 1, pin.DigitalValue)

	press := doRequest(sim, http.MethodPost, "/api/v1/boards/dfp/devices/button_start/press", []byte(`{}`))
	require.Equal(t, http.StatusOK, press.Code, press.Body.String())
	pin, err = sim.Pin("dfp", "11")
	require.NoError(t, err)
	assert.Equal(t, 0, pin.DigitalValue)

	release := doRequest(sim, http.MethodPost, "/api/v1/boards/dfp/devices/button_start/release", []byte(`{}`))
	require.Equal(t, http.StatusOK, release.Code, release.Body.String())
	pin, err = sim.Pin("dfp", "11")
	require.NoError(t, err)
	assert.Equal(t, 1, pin.DigitalValue)

	pinResponse := doRequest(sim, http.MethodPost, "/api/v1/boards/dfp/pins/3", []byte(`{"digitalValue":0}`))
	require.Equal(t, http.StatusOK, pinResponse.Code, pinResponse.Body.String())
	var updatedPin simulate.PinState
	require.NoError(t, json.Unmarshal(pinResponse.Body.Bytes(), &updatedPin))
	assert.Equal(t, 0, updatedPin.DigitalValue)

	value := doRequest(sim, http.MethodGet, "/api/v1/boards/dfp/values/distance", nil)
	require.Equal(t, http.StatusOK, value.Code)
	assert.Contains(t, value.Body.String(), `"value":0`)

	setValue := doRequest(sim, http.MethodPut, "/api/v1/boards/dfp/values/distance", []byte(`{"value":42.5}`))
	require.Equal(t, http.StatusOK, setValue.Code, setValue.Body.String())
	assert.Contains(t, setValue.Body.String(), `42.5`)

	fn := doRequest(sim, http.MethodPost, "/api/v1/boards/dfp/functions/acknoledgeRebooted", []byte(`{"parameters":"1"}`))
	require.Equal(t, http.StatusOK, fn.Code, fn.Body.String())
	assert.Contains(t, fn.Body.String(), `"result":0`)
	assert.Contains(t, fn.Body.String(), `"callCount":1`)

	fault := doRequest(sim, http.MethodPost, "/api/v1/boards/dfp/fault", []byte(`{"message":"i2c timeout"}`))
	require.Equal(t, http.StatusOK, fault.Code, fault.Body.String())
	assert.Contains(t, fault.Body.String(), `"active":true`)

	clear := doRequest(sim, http.MethodDelete, "/api/v1/boards/dfp/fault", nil)
	require.Equal(t, http.StatusOK, clear.Code)
	assert.Contains(t, clear.Body.String(), `"active":false`)
}

func TestRESTErrors(t *testing.T) {
	sim := newServerSim(t)
	populateBoard(t, sim)

	missing := doRequest(sim, http.MethodGet, "/api/v1/boards/unknown", nil)
	assert.Equal(t, http.StatusNotFound, missing.Code)
	assert.Contains(t, missing.Body.String(), `"BOARD_NOT_FOUND"`)

	invalid := doRequest(sim, http.MethodGet, "/api/v1/boards/dfp/devices/bad@id", nil)
	assert.Equal(t, http.StatusBadRequest, invalid.Code)
	assert.Contains(t, invalid.Body.String(), `"INVALID_IDENTIFIER"`)

	missingDevice := doRequest(sim, http.MethodGet, "/api/v1/boards/dfp/devices/nope", nil)
	assert.Equal(t, http.StatusNotFound, missingDevice.Code)
	assert.Contains(t, missingDevice.Body.String(), `"DEVICE_NOT_FOUND"`)

	badMode := doRequest(sim, http.MethodPost, "/api/v1/boards/dfp/pins/3", []byte(`{"mode":"teleport"}`))
	assert.Equal(t, http.StatusUnprocessableEntity, badMode.Code)
	assert.Contains(t, badMode.Body.String(), `"INVALID_PIN_MODE"`)

	badJSON := doRequest(sim, http.MethodPatch, "/api/v1/boards/dfp/devices/relay_drum", []byte(`{`))
	assert.Equal(t, http.StatusBadRequest, badJSON.Code)
	assert.Contains(t, badJSON.Body.String(), `"INVALID_JSON"`)
}

func TestAuthMiddleware(t *testing.T) {
	sim := newServerSim(t, simulate.WithAuthToken("secret"))
	populateBoard(t, sim)

	health := doRequest(sim, http.MethodGet, "/api/v1/health", nil)
	assert.Equal(t, http.StatusOK, health.Code)

	unauthorized := doRequest(sim, http.MethodGet, "/api/v1/snapshot", nil)
	assert.Equal(t, http.StatusUnauthorized, unauthorized.Code)
	assert.Contains(t, unauthorized.Body.String(), `"UNAUTHORIZED"`)

	wrong := doRequest(sim, http.MethodGet, "/api/v1/snapshot", nil, ut.Header{Key: "Authorization", Value: "Bearer nope"})
	assert.Equal(t, http.StatusUnauthorized, wrong.Code)

	authorized := doRequest(sim, http.MethodGet, "/api/v1/snapshot", nil, ut.Header{Key: "Authorization", Value: "Bearer secret"})
	assert.Equal(t, http.StatusOK, authorized.Code)
}

func TestCORSAllowlist(t *testing.T) {
	sim := newServerSim(t, simulate.WithCORSOrigins("http://localhost:3000"))
	populateBoard(t, sim)

	allowed := doRequest(sim, http.MethodGet, "/api/v1/boards", nil, ut.Header{Key: "Origin", Value: "http://localhost:3000"})
	require.Equal(t, http.StatusOK, allowed.Code)
	assert.Equal(t, "http://localhost:3000", allowed.Result().Header.Get("Access-Control-Allow-Origin"))

	denied := doRequest(sim, http.MethodGet, "/api/v1/boards", nil, ut.Header{Key: "Origin", Value: "http://evil.test"})
	require.Equal(t, http.StatusOK, denied.Code)
	assert.Empty(t, denied.Result().Header.Get("Access-Control-Allow-Origin"))

	preflight := doRequest(sim, http.MethodOptions, "/api/v1/boards", nil, ut.Header{Key: "Origin", Value: "http://localhost:3000"})
	assert.Equal(t, http.StatusNoContent, preflight.Code)

	badPreflight := doRequest(sim, http.MethodOptions, "/api/v1/boards", nil, ut.Header{Key: "Origin", Value: "http://evil.test"})
	assert.Equal(t, http.StatusForbidden, badPreflight.Code)
}

func TestBodyLimit(t *testing.T) {
	sim := newServerSim(t)
	populateBoard(t, sim)

	large := bytes.Repeat([]byte("a"), 70*1024)
	response := doRequest(sim, http.MethodPost, "/api/v1/boards/dfp/fault", large,
		ut.Header{Key: "Content-Length", Value: "70000"})
	assert.Equal(t, http.StatusRequestEntityTooLarge, response.Code)
	assert.Contains(t, response.Body.String(), `"PAYLOAD_TOO_LARGE"`)
}

func TestStaticSPAAndAPIFallback(t *testing.T) {
	sim := newServerSim(t)
	populateBoard(t, sim)

	index := doRequest(sim, http.MethodGet, "/", nil)
	require.Equal(t, http.StatusOK, index.Code)
	assert.Contains(t, index.Body.String(), "Gobot Simulator")

	history := doRequest(sim, http.MethodGet, "/boards/dfp", nil)
	require.Equal(t, http.StatusOK, history.Code)
	assert.Contains(t, history.Body.String(), "Gobot Simulator")

	apiMissing := doRequest(sim, http.MethodGet, "/api/v1/not-found", nil)
	assert.Equal(t, http.StatusNotFound, apiMissing.Code)
	assert.Contains(t, apiMissing.Body.String(), `"NOT_FOUND"`)
	assert.NotContains(t, apiMissing.Body.String(), "<html")

	traversal := doRequest(sim, http.MethodGet, "/..%2F..%2Fetc%2Fpasswd", nil)
	assert.NotEqual(t, http.StatusOK, traversal.Code)
	assert.NotContains(t, traversal.Body.String(), "root:")
}

func TestUIDisabled(t *testing.T) {
	sim := newServerSim(t, simulate.WithUIEnabled(false))
	response := doRequest(sim, http.MethodGet, "/", nil)
	assert.Equal(t, http.StatusNotFound, response.Code)
	assert.Contains(t, response.Body.String(), `"NOT_FOUND"`)
}

func TestSecurityHeaders(t *testing.T) {
	sim := newServerSim(t)
	response := doRequest(sim, http.MethodGet, "/api/v1/health", nil)
	require.Equal(t, http.StatusOK, response.Code)
	result := response.Result()
	headers := &result.Header
	assert.Equal(t, "nosniff", headers.Get("X-Content-Type-Options"))
	assert.Equal(t, "SAMEORIGIN", headers.Get("X-Frame-Options"))
	assert.Equal(t, "strict-origin-when-cross-origin", headers.Get("Referrer-Policy"))
	csp := headers.Get("Content-Security-Policy")
	assert.Contains(t, csp, "default-src 'self'")
	assert.Contains(t, csp, "script-src 'self' 'sha256-")
	assert.NotContains(t, csp, "script-src 'self' 'unsafe-inline'")
	assert.Contains(t, csp, "style-src 'self' 'unsafe-inline'")
}

func TestRESTDeviceAndPinVariants(t *testing.T) {
	sim := newServerSim(t)
	sim.Adaptor("dfp", simulate.WithAdaptorName("fat"))
	for _, spec := range []simulate.DeviceSpec{
		{ID: "value_distance", Kind: simulate.DeviceKindValue, ValueName: "distance"},
		{ID: "pwm_fan", Kind: simulate.DeviceKindPWM, Pin: "5"},
		{ID: "servo_valve", Kind: simulate.DeviceKindServo, Pin: "9"},
		{ID: "sensor_level", Kind: simulate.DeviceKindSensor, Pin: "A0"},
		{ID: "button_start", Kind: simulate.DeviceKindButton, Pin: "11", DefaultState: simulate.IntPtr(1)},
	} {
		_, err := sim.RegisterDevice("dfp", spec)
		require.NoError(t, err)
	}

	patchValue := doRequest(sim, http.MethodPatch, "/api/v1/boards/dfp/devices/value_distance", []byte(`{"value":42.5}`))
	require.Equal(t, http.StatusOK, patchValue.Code, patchValue.Body.String())
	values, err := sim.Values("dfp")
	require.NoError(t, err)
	assert.Equal(t, 42.5, values["distance"].Value)

	patchPWM := doRequest(sim, http.MethodPatch, "/api/v1/boards/dfp/devices/pwm_fan", []byte(`{"byteValue":200}`))
	require.Equal(t, http.StatusOK, patchPWM.Code, patchPWM.Body.String())
	pin, err := sim.Pin("dfp", "5")
	require.NoError(t, err)
	assert.Equal(t, byte(200), pin.ByteValue)

	patchServo := doRequest(sim, http.MethodPatch, "/api/v1/boards/dfp/devices/servo_valve", []byte(`{"angle":90}`))
	require.Equal(t, http.StatusOK, patchServo.Code, patchServo.Body.String())

	patchSensor := doRequest(sim, http.MethodPatch, "/api/v1/boards/dfp/devices/sensor_level", []byte(`{"value":512}`))
	require.Equal(t, http.StatusOK, patchSensor.Code, patchSensor.Body.String())

	patchReadOnly := doRequest(sim, http.MethodPatch, "/api/v1/boards/dfp/devices/value_distance", []byte(`{"active":true}`))
	require.Equal(t, http.StatusUnprocessableEntity, patchReadOnly.Code)
	assert.Contains(t, patchReadOnly.Body.String(), `"READ_ONLY"`)

	pinByte := doRequest(sim, http.MethodPost, "/api/v1/boards/dfp/pins/5", []byte(`{"byteValue":12}`))
	require.Equal(t, http.StatusOK, pinByte.Code, pinByte.Body.String())

	pinDigitalMax := doRequest(sim, http.MethodPost, "/api/v1/boards/dfp/pins/3", []byte(`{"digitalValue":255}`))
	require.Equal(t, http.StatusOK, pinDigitalMax.Code, pinDigitalMax.Body.String())

	pinDigitalOverflow := doRequest(sim, http.MethodPost, "/api/v1/boards/dfp/pins/3", []byte(`{"digitalValue":256}`))
	assert.Equal(t, http.StatusBadRequest, pinDigitalOverflow.Code)
	assert.Contains(t, pinDigitalOverflow.Body.String(), `"INVALID_VALUE"`)

	pinAnalog := doRequest(sim, http.MethodPost, "/api/v1/boards/dfp/pins/A0", []byte(`{"analogValue":512,"mode":"analog"}`))
	require.Equal(t, http.StatusOK, pinAnalog.Code, pinAnalog.Body.String())

	pinMode := doRequest(sim, http.MethodPost, "/api/v1/boards/dfp/pins/7", []byte(`{"mode":"input-pullup"}`))
	require.Equal(t, http.StatusOK, pinMode.Code, pinMode.Body.String())
	pin, err = sim.Pin("dfp", "7")
	require.NoError(t, err)
	assert.Equal(t, simulate.PinModeInputPullup, pin.Mode)
	assert.True(t, pin.Pullup)

	emptyPin := doRequest(sim, http.MethodPost, "/api/v1/boards/dfp/pins/7", []byte(`{}`))
	assert.Equal(t, http.StatusBadRequest, emptyPin.Code)

	pulse := doRequest(sim, http.MethodPost, "/api/v1/boards/dfp/devices/button_start/press", []byte(`{"holdMs":20}`))
	require.Equal(t, http.StatusOK, pulse.Code, pulse.Body.String())
	assert.Eventually(t, func() bool {
		pin, err := sim.Pin("dfp", "11")
		return err == nil && pin.DigitalValue == 1
	}, time.Second, 5*time.Millisecond)

	badHold := doRequest(sim, http.MethodPost, "/api/v1/boards/dfp/devices/button_start/press", []byte(`{"holdMs":-1}`))
	assert.Equal(t, http.StatusBadRequest, badHold.Code)

	function := doRequest(sim, http.MethodPost, "/api/v1/boards/dfp/functions/ack", []byte(`{"returnCode":2,"execute":false}`))
	require.Equal(t, http.StatusOK, function.Code, function.Body.String())
	assert.Contains(t, function.Body.String(), `"returnCode":2`)

	missingValue := doRequest(sim, http.MethodGet, "/api/v1/boards/dfp/values/nope", nil)
	assert.Equal(t, http.StatusNotFound, missingValue.Code)
	assert.Contains(t, missingValue.Body.String(), `"VALUE_NOT_FOUND"`)

	missingValueBody := doRequest(sim, http.MethodPut, "/api/v1/boards/dfp/values/distance", []byte(`{}`))
	assert.Equal(t, http.StatusBadRequest, missingValueBody.Code)

	emptyFault := doRequest(sim, http.MethodPost, "/api/v1/boards/dfp/fault", []byte(`{"message":"  "}`))
	assert.Equal(t, http.StatusBadRequest, emptyFault.Code)

	badLength := doRequest(sim, http.MethodPost, "/api/v1/boards/dfp/fault", []byte(`{}`),
		ut.Header{Key: "Content-Length", Value: "nope"})
	assert.Equal(t, http.StatusBadRequest, badLength.Code)
}
