package simulate

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSimulatorInspectionAPI(t *testing.T) {
	sim := New()
	board := sim.Adaptor("dfp", WithAdaptorName("fat"))
	board.SetInputPullup("11")
	_, err := sim.RegisterDevice("dfp", DeviceSpec{ID: "relay", Kind: DeviceKindRelay, Pin: "3"})
	require.NoError(t, err)

	snapshot := sim.Snapshot()
	assert.Equal(t, Version, snapshot.Meta.Version)
	require.Len(t, snapshot.Boards, 1)

	boards := sim.Boards()
	require.Len(t, boards, 1)
	assert.Equal(t, "dfp", boards[0].ID)

	require.NoError(t, sim.SetValue("dfp", "distance", float64(42), SourceUI))
	values, err := sim.Values("dfp")
	require.NoError(t, err)
	assert.Equal(t, float64(42), values["distance"].Value)
}

func TestSimulatorMutationAPIWithValidation(t *testing.T) {
	sim := New()
	sim.Adaptor("dfp")

	require.NoError(t, sim.SetDigital("dfp", "3", 1, SourceUI))
	pin, err := sim.Pin("dfp", "3")
	require.NoError(t, err)
	assert.Equal(t, 1, pin.DigitalValue)

	require.NoError(t, sim.SetFunctionCode("dfp", "ack", 2, SourceUI))
	result, err := sim.CallFunction("dfp", "ack", "1", SourceUI)
	require.NoError(t, err)
	assert.Equal(t, 2, result)

	// Invalid identifiers are rejected at the public boundary.
	assert.ErrorIs(t, sim.SetDigital("dfp", "bad pin", 1, SourceUI), ErrInvalidID)
	_, err = sim.CallFunction("dfp", "bad name", "", SourceUI)
	assert.ErrorIs(t, err, ErrInvalidID)
	assert.ErrorIs(t, sim.PressButton("dfp", "bad id"), ErrInvalidID)
	assert.ErrorIs(t, sim.ReleaseButton("dfp", "bad id"), ErrInvalidID)
	assert.ErrorIs(t, sim.ToggleDevice("dfp", "bad id"), ErrInvalidID)
	assert.ErrorIs(t, sim.SetPWM("dfp", "bad pin", 1, SourceUI), ErrInvalidID)
	assert.ErrorIs(t, sim.SetServo("dfp", "bad pin", 90, SourceUI), ErrInvalidID)
	assert.ErrorIs(t, sim.SetAnalog("dfp", "bad pin", 1, SourceUI), ErrInvalidID)
	assert.ErrorIs(t, sim.SetValue("dfp", "bad name", 1, SourceUI), ErrInvalidID)
	assert.ErrorIs(t, sim.BindPin("dfp", "bad pin", "", PinModeOutput), ErrInvalidID)
}

func TestSimulatorSubscribeWithReplay(t *testing.T) {
	sim := New()
	board := sim.Adaptor("dfp")
	board.SetDigitalPinState("1", 1)

	seq := sim.CurrentSeq()
	require.Greater(t, seq, uint64(0))

	ch, missed, ok := sim.SubscribeWithReplay(seq - 1)
	require.True(t, ok)
	require.NotEmpty(t, missed)
	sim.Unsubscribe(ch)

	ch, missed, ok = sim.SubscribeWithReplay(0)
	assert.False(t, ok)
	assert.Empty(t, missed)
	sim.Unsubscribe(ch)
}

func TestSimulatorServeAndClose(t *testing.T) {
	sim := New(WithPort(0))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- sim.Serve(ctx) }()
	require.Eventually(t, func() bool { return sim.URL() != "" }, time.Second, 10*time.Millisecond)

	cancel()
	require.NoError(t, <-done)

	// Close is idempotent on an already-stopped server.
	require.NoError(t, sim.Close())
}

func TestSimulatorWithConfigOption(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Port = 0
	cfg.MaxClients = 5
	cfg.EventBuffer = 64
	cfg.AuthToken = "t"
	cfg.UIEnabled = false
	sim := New(WithConfig(cfg))

	assert.Equal(t, 0, sim.cfg.Port)
	assert.Equal(t, 5, sim.cfg.MaxClients)
	assert.Equal(t, 64, sim.cfg.EventBuffer)
	assert.Equal(t, "t", sim.cfg.AuthToken)
	assert.False(t, sim.cfg.UIEnabled)

	withUI := New(WithUIEnabled(true))
	assert.True(t, withUI.cfg.UIEnabled)
}

func TestSimulatorAdaptorReturnsExisting(t *testing.T) {
	sim := New()
	first := sim.Adaptor("dfp")
	second := sim.Adaptor("dfp")
	assert.Same(t, first, second)
}

func TestSimulatorRegisterBoardNil(t *testing.T) {
	sim := New()
	assert.Error(t, sim.RegisterBoard(nil))
}

func TestTestAdaptorReadValueAlias(t *testing.T) {
	board := NewMockPlateform()
	board.TestAdaptorReadValue(func(pin string) (int, error) { return 5, nil })
	value, err := board.DigitalRead("1")
	require.NoError(t, err)
	assert.Equal(t, 5, value)
}

func TestSanitizeID(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want string
	}{
		{"empty", []string{""}, "test"},
		{"valid first", []string{"dfp"}, "dfp"},
		{"normalize invalid", []string{"has space"}, "has_space"},
		{"fallback to second", []string{"", "dfp"}, "dfp"},
		{"skip empty then valid", []string{"", "", "ok"}, "ok"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, sanitizeID(tt.in...))
		})
	}
}

func TestNormalizeConfigDefaults(t *testing.T) {
	cfg := normalizeConfig(Config{})
	def := DefaultConfig()
	assert.Equal(t, def.Addr, cfg.Addr)
	assert.Equal(t, def.MaxClients, cfg.MaxClients)
	assert.Equal(t, def.EventBuffer, cfg.EventBuffer)
	assert.Equal(t, def.CORSOrigins, cfg.CORSOrigins)

	// Explicit values are preserved.
	cfg = normalizeConfig(Config{Addr: "0.0.0.0", MaxClients: 3, EventBuffer: 32, CORSOrigins: []string{"http://a.test"}})
	assert.Equal(t, "0.0.0.0", cfg.Addr)
	assert.Equal(t, 3, cfg.MaxClients)
	assert.Equal(t, 32, cfg.EventBuffer)
	assert.Equal(t, []string{"http://a.test"}, cfg.CORSOrigins)
}
