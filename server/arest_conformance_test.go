package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	extra "github.com/disaster37/gobot-arest/v2/drivers/extra"
	arest "github.com/disaster37/gobot-arest/v2/plateforms/arest"
	arestClient "github.com/disaster37/gobot-arest/v2/plateforms/arest/client"
	simulate "github.com/disaster37/gobot-simulate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestARESTHTTPConformance(t *testing.T) {
	sim := newServerSim(t)
	sim.Adaptor("dfp", simulate.WithAdaptorName("dfp"), simulate.WithInputPullup("11"))
	_, err := sim.RegisterDevice("dfp", simulate.DeviceSpec{
		ID: "value_isRebooted", Name: "isRebooted", Kind: simulate.DeviceKindValue, ValueName: "isRebooted",
	})
	require.NoError(t, err)
	_, err = sim.RegisterDevice("dfp", simulate.DeviceSpec{
		ID: "function_ack", Name: "acknoledgeRebooted", Kind: simulate.DeviceKindFunction, FunctionName: "acknoledgeRebooted",
	})
	require.NoError(t, err)
	require.NoError(t, sim.SetValue("dfp", "isRebooted", false, simulate.SourceSimulation))
	require.NoError(t, sim.SetFunctionCode("dfp", "acknoledgeRebooted", 0, simulate.SourceSimulation))

	adaptor := arest.NewHTTPAdaptor(sim.ARESTBaseURL("dfp"), "dfp", 2*time.Second)
	require.NoError(t, adaptor.Connect())
	t.Cleanup(func() { _ = adaptor.Finalize() })

	// Digital write then read back through the real gobot-arest client.
	require.NoError(t, adaptor.DigitalWrite("3", 1))
	pin, err := sim.Pin("dfp", "3")
	require.NoError(t, err)
	assert.Equal(t, 1, pin.DigitalValue)
	assert.Equal(t, simulate.PinModeOutput, pin.Mode, "aREST /mode must be recorded")

	require.NoError(t, adaptor.Board.SetPinMode(context.Background(), 11, arestClient.ModeInputPullup))
	value, err := adaptor.DigitalRead("11")
	require.NoError(t, err)
	assert.Equal(t, 1, value, "pull-up input reads high")

	require.NoError(t, sim.SetDigital("dfp", "11", 0, simulate.SourceSimulation))
	value, err = adaptor.DigitalRead("11")
	require.NoError(t, err)
	assert.Equal(t, 0, value)

	// Named value surface.
	rebooted, err := adaptor.ValueRead("isRebooted")
	require.NoError(t, err)
	assert.Equal(t, false, rebooted)

	values, err := adaptor.ValuesRead()
	require.NoError(t, err)
	assert.Contains(t, values, "isRebooted")
	assert.NotContains(t, values, "test", "legacy {\"test\":99} must not leak into the aREST surface")

	// Function call return code.
	code, err := adaptor.FunctionCall("acknoledgeRebooted", "now")
	require.NoError(t, err)
	assert.Zero(t, code)

	// Missing variable must surface the documented client error.
	_, err = adaptor.ValueRead("definitely_missing")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	// Extra value and function drivers work against the emulated surface.
	valueDriver := extra.NewValueDriver(adaptor, "isRebooted", 10*time.Millisecond)
	assert.Equal(t, "isRebooted", valueDriver.ValueName())
	functionDriver := extra.NewFunctionDriver(adaptor, "acknoledgeRebooted", "x")
	require.NoError(t, functionDriver.Call())

	// Board faults surface as HTTP errors to the client.
	require.NoError(t, sim.SetFault("dfp", errors.New("hardware offline")))
	_, err = adaptor.ValueRead("isRebooted")
	require.Error(t, err, "board fault must surface as a client error")
	require.NoError(t, sim.ClearFault("dfp"))
}

func TestARESTRootHasNoLegacyTestValue(t *testing.T) {
	sim := newServerSim(t)
	sim.Adaptor("dfp", simulate.WithAdaptorName("dfp"))
	require.NoError(t, sim.SetValue("dfp", "distance", float64(42), simulate.SourceSimulation))

	response := doRequest(sim, http.MethodGet, "/arest/dfp/", nil)
	require.Equal(t, http.StatusOK, response.Code)
	payload := decodeMap(t, response.Body.Bytes())
	variables, ok := payload["variables"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, float64(42), variables["distance"])
	assert.NotContains(t, variables, "test")
	raw, err := json.Marshal(payload)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), `"test":99`)
}
