package server_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	simulate "github.com/disaster37/gobot-simulate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gobot.io/x/gobot/v2/drivers/gpio"
)

const hardwareTopology = `schema: "1"
boards:
  - id: dfp
    name: fat
    pins:
      - pin: "3"
        mode: output
      - pin: "7"
        mode: output
      - pin: "11"
        mode: input
        pull: up
    devices:
      - id: relay_drum
        kind: relay
        pin: "3"
        wiring:
          drive: active-high
          contact: no
          loadLabel: drum
      - id: led
        kind: led
        pin: "7"
      - id: button_start
        kind: button
        pin: "11"
        defaultState: 1
        wiring:
          contact: no
          mechanism: momentary
          pull: up
    chips:
      - id: oled
        bus: i2c-1
        address: 60
        model: ssd1306
    values:
      distance: 12
    functions:
      ack:
        returnCode: 0
scenarios:
  - id: manual
    name: Manual scenario
    trigger:
      type: manual
    actions:
      - type: emitStep
        name: scenario-ran
        status: ok
`

func hardwareSim(t *testing.T) *simulate.Simulator {
	t.Helper()
	sim := newServerSim(t)
	require.NoError(t, sim.LoadTopology(strings.NewReader(hardwareTopology)))
	return sim
}

func TestTraceWaveformAndStepEndpoints(t *testing.T) {
	sim := hardwareSim(t)

	response := doRequest(sim, http.MethodPost, "/api/v1/boards/dfp/pins/3", []byte(`{"digitalValue":1}`))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())

	trace := doRequest(sim, http.MethodGet, "/api/v1/boards/dfp/trace", nil)
	require.Equal(t, http.StatusOK, trace.Code)
	var records struct {
		Records []map[string]interface{} `json:"records"`
	}
	require.NoError(t, json.Unmarshal(trace.Body.Bytes(), &records))
	require.NotEmpty(t, records.Records)
	assert.Equal(t, "pin-write", records.Records[0]["kind"])

	filtered := doRequest(sim, http.MethodGet, "/api/v1/boards/dfp/trace?kinds=pin-write&target=3", nil)
	require.Equal(t, http.StatusOK, filtered.Code)
	require.NoError(t, json.Unmarshal(filtered.Body.Bytes(), &records))
	require.NotEmpty(t, records.Records)

	badKind := doRequest(sim, http.MethodGet, "/api/v1/boards/dfp/trace?kinds=bogus", nil)
	assert.Equal(t, http.StatusBadRequest, badKind.Code)

	waveform := doRequest(sim, http.MethodGet, "/api/v1/boards/dfp/waveform?pins=3&maxPoints=10", nil)
	require.Equal(t, http.StatusOK, waveform.Code)
	var series struct {
		Series []map[string]interface{} `json:"series"`
	}
	require.NoError(t, json.Unmarshal(waveform.Body.Bytes(), &series))
	require.Len(t, series.Series, 1)
	assert.Equal(t, "3", series.Series[0]["pin"])

	step := doRequest(sim, http.MethodPost, "/api/v1/boards/dfp/steps", []byte(`{"name":"wash","status":"running","detail":{"cycle":1}}`))
	require.Equal(t, http.StatusCreated, step.Code, step.Body.String())

	invalidStep := doRequest(sim, http.MethodPost, "/api/v1/boards/dfp/steps", []byte(`{"name":""}`))
	assert.Equal(t, http.StatusBadRequest, invalidStep.Code)
}

func TestBusChipTopologyAndScenarioEndpoints(t *testing.T) {
	sim := hardwareSim(t)

	buses := doRequest(sim, http.MethodGet, "/api/v1/boards/dfp/buses", nil)
	require.Equal(t, http.StatusOK, buses.Code)
	assert.Contains(t, buses.Body.String(), `"i2c-1"`)

	chips := doRequest(sim, http.MethodGet, "/api/v1/boards/dfp/chips", nil)
	require.Equal(t, http.StatusOK, chips.Code)
	assert.Contains(t, chips.Body.String(), `"oled"`)

	chip := doRequest(sim, http.MethodGet, "/api/v1/chips/oled", nil)
	require.Equal(t, http.StatusOK, chip.Code)
	payload := decodeMap(t, chip.Body.Bytes())
	assert.Contains(t, payload, "descriptor")
	assert.Contains(t, payload, "display")

	patch := doRequest(sim, http.MethodPatch, "/api/v1/chips/oled", []byte(`{"field":"text","value":"hello"}`))
	require.Equal(t, http.StatusOK, patch.Code, patch.Body.String())
	assert.Contains(t, patch.Body.String(), `"pixels"`)

	raw := doRequest(sim, http.MethodPost, "/api/v1/chips/oled/raw", []byte(`{"write":[64,255],"readLen":0}`))
	require.Equal(t, http.StatusOK, raw.Code, raw.Body.String())

	tooLarge := doRequest(sim, http.MethodPost, "/api/v1/chips/oled/raw", []byte(`{"write":[1,2,3],"readLen":9999}`))
	assert.Equal(t, http.StatusBadRequest, tooLarge.Code)

	topology := doRequest(sim, http.MethodGet, "/api/v1/topology", nil)
	require.Equal(t, http.StatusOK, topology.Code)
	assert.Contains(t, topology.Body.String(), `"dfp"`)

	source := doRequest(sim, http.MethodGet, "/api/v1/topology/source", nil)
	require.Equal(t, http.StatusOK, source.Code)
	assert.Contains(t, source.Body.String(), "schema")

	scenarios := doRequest(sim, http.MethodGet, "/api/v1/scenarios", nil)
	require.Equal(t, http.StatusOK, scenarios.Code)
	assert.Contains(t, scenarios.Body.String(), `"manual"`)

	disable := doRequest(sim, http.MethodPatch, "/api/v1/scenarios/manual", []byte(`{"enabled":false}`))
	require.Equal(t, http.StatusOK, disable.Code, disable.Body.String())
	assert.Contains(t, disable.Body.String(), `"enabled":false`)

	triggerDisabled := doRequest(sim, http.MethodPost, "/api/v1/scenarios/manual/trigger", nil)
	assert.Equal(t, http.StatusConflict, triggerDisabled.Code)

	enable := doRequest(sim, http.MethodPatch, "/api/v1/scenarios/manual", []byte(`{"enabled":true}`))
	require.Equal(t, http.StatusOK, enable.Code)
	trigger := doRequest(sim, http.MethodPost, "/api/v1/scenarios/manual/trigger", nil)
	require.Equal(t, http.StatusAccepted, trigger.Code, trigger.Body.String())

	missing := doRequest(sim, http.MethodPatch, "/api/v1/scenarios/nope", []byte(`{"enabled":true}`))
	assert.Equal(t, http.StatusNotFound, missing.Code)
}

func TestDeviceFaultAndCommandEndpoints(t *testing.T) {
	sim := hardwareSim(t)
	adaptor := sim.Adaptor("dfp")
	driver := gpio.NewRelayDriver(adaptor, "3")
	require.NoError(t, driver.Start())
	driver.SetName("relay_drum")
	require.NoError(t, sim.AttachDevices("dfp", driver))

	command := doRequest(sim, http.MethodPost, "/api/v1/boards/dfp/devices/relay_drum/command", []byte(`{"action":"on"}`))
	require.Equal(t, http.StatusOK, command.Code, command.Body.String())
	assert.True(t, driver.State())
	assert.Contains(t, command.Body.String(), `"active":true`)

	off := doRequest(sim, http.MethodPost, "/api/v1/boards/dfp/devices/relay_drum/command", []byte(`{"action":"off"}`))
	require.Equal(t, http.StatusOK, off.Code, off.Body.String())
	assert.False(t, driver.State())

	unknownAction := doRequest(sim, http.MethodPost, "/api/v1/boards/dfp/devices/relay_drum/command", []byte(`{"action":"teleport"}`))
	assert.Equal(t, http.StatusUnprocessableEntity, unknownAction.Code)

	fault := doRequest(sim, http.MethodPost, "/api/v1/boards/dfp/devices/relay_drum/fault", []byte(`{"fault":"welded-contact"}`))
	require.Equal(t, http.StatusOK, fault.Code, fault.Body.String())
	assert.Contains(t, fault.Body.String(), "welded-contact")
	read, err := sim.DigitalRead("dfp", "3")
	require.NoError(t, err)
	assert.Equal(t, 1, read, "welded contact must be observable by the driver")

	badFault := doRequest(sim, http.MethodPost, "/api/v1/boards/dfp/devices/relay_drum/fault", []byte(`{"fault":"melted"}`))
	assert.Equal(t, http.StatusBadRequest, badFault.Code)

	clear := doRequest(sim, http.MethodDelete, "/api/v1/boards/dfp/devices/relay_drum/fault", nil)
	require.Equal(t, http.StatusOK, clear.Code, clear.Body.String())
	assert.NotContains(t, clear.Body.String(), "welded-contact")

	// No attached driver for the LED means commands are rejected.
	noDriver := doRequest(sim, http.MethodPost, "/api/v1/boards/dfp/devices/led/command", []byte(`{"action":"on"}`))
	assert.Equal(t, http.StatusUnprocessableEntity, noDriver.Code)
}

func TestWiringWarningsExposed(t *testing.T) {
	sim := newServerSim(t)
	sim.Adaptor("dfp")
	_, err := sim.RegisterDevice("dfp", simulate.DeviceSpec{
		ID: "button", Kind: simulate.DeviceKindButton, Pin: "11", DefaultState: simulate.IntPtr(0),
		Wiring: simulate.Wiring{Contact: simulate.ContactNO, Pull: simulate.PullUp, Mechanism: simulate.MechanismMomentary},
	})
	require.NoError(t, err)
	response := doRequest(sim, http.MethodGet, "/api/v1/boards/dfp/devices/button", nil)
	require.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), "WIRING_MISMATCH")
}
