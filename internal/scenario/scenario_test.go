package scenario

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/disaster37/gobot-simulate/internal/clock"
	"github.com/disaster37/gobot-simulate/internal/state"
	"github.com/disaster37/gobot-simulate/internal/trace"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type call struct {
	method string
	board  string
	target string
	value  interface{}
}

type fakeDispatcher struct {
	mu    sync.Mutex
	calls []call
}

func (f *fakeDispatcher) record(method, board, target string, value interface{}) {
	f.mu.Lock()
	f.calls = append(f.calls, call{method: method, board: board, target: target, value: value})
	f.mu.Unlock()
}

func (f *fakeDispatcher) snapshot() []call {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]call, len(f.calls))
	copy(out, f.calls)
	return out
}

func (f *fakeDispatcher) ScenarioSetPin(_ context.Context, boardID, pin string, value int) error {
	f.record("setPin", boardID, pin, value)
	return nil
}

func (f *fakeDispatcher) ScenarioSetValue(_ context.Context, boardID, name string, value interface{}) error {
	f.record("setValue", boardID, name, value)
	return nil
}

func (f *fakeDispatcher) ScenarioPress(_ context.Context, boardID, deviceID string) error {
	f.record("press", boardID, deviceID, nil)
	return nil
}

func (f *fakeDispatcher) ScenarioRelease(_ context.Context, boardID, deviceID string) error {
	f.record("release", boardID, deviceID, nil)
	return nil
}

func (f *fakeDispatcher) ScenarioCommand(_ context.Context, boardID, deviceID, action string, value *int) error {
	f.record("command", boardID, deviceID, action)
	return nil
}

func (f *fakeDispatcher) ScenarioSetDeviceFault(_ context.Context, boardID, deviceID, fault string) error {
	f.record("setFault", boardID, deviceID, fault)
	return nil
}

func (f *fakeDispatcher) ScenarioClearDeviceFault(_ context.Context, boardID, deviceID string) error {
	f.record("clearFault", boardID, deviceID, nil)
	return nil
}

func (f *fakeDispatcher) ScenarioEmitStep(_ context.Context, boardID, name, status string, detail map[string]interface{}) error {
	f.record("step", boardID, name, status)
	return nil
}

func boolPtr(v bool) *bool { return &v }

func newEngine(t *testing.T, rules []Rule, dispatch Dispatcher, clk clock.Clock) *Engine {
	t.Helper()
	engine := New(rules, dispatch, clk)
	for _, rule := range rules {
		require.NoError(t, rule.Validate(rule.ID))
	}
	return engine
}

func TestValidateRules(t *testing.T) {
	valid := Rule{
		ID:      "rule",
		Trigger: Trigger{Type: TriggerDeviceChanged, Device: "relay"},
		Actions: []Action{{Type: ActionSetPin, Pin: "3", Value: 1}},
	}
	require.NoError(t, valid.Validate("scenarios[0]"))

	tests := []struct {
		name string
		rule Rule
		want string
	}{
		{"missing id", Rule{Trigger: Trigger{Type: TriggerManual}, Actions: []Action{{Type: ActionEmitStep, Name: "x"}}}, "id"},
		{"bad trigger", Rule{ID: "r", Trigger: Trigger{Type: "bogus"}, Actions: []Action{{Type: ActionEmitStep, Name: "x"}}}, "trigger.type"},
		{"bad timer", Rule{ID: "r", Trigger: Trigger{Type: TriggerTimerEvery, Every: "nope"}, Actions: []Action{{Type: ActionEmitStep, Name: "x"}}}, "trigger.every"},
		{"bad delay", Rule{ID: "r", Trigger: Trigger{Type: TriggerDelayAfter, After: "nope"}, Actions: []Action{{Type: ActionEmitStep, Name: "x"}}}, "trigger.after"},
		{"no actions", Rule{ID: "r", Trigger: Trigger{Type: TriggerManual}}, "actions"},
		{"bad action", Rule{ID: "r", Trigger: Trigger{Type: TriggerManual}, Actions: []Action{{Type: "bogus"}}}, "actions[0].type"},
		{"setPin missing", Rule{ID: "r", Trigger: Trigger{Type: TriggerManual}, Actions: []Action{{Type: ActionSetPin}}}, "pin"},
		{"command missing target", Rule{ID: "r", Trigger: Trigger{Type: TriggerManual}, Actions: []Action{{Type: ActionCommand}}}, "target"},
		{"step missing name", Rule{ID: "r", Trigger: Trigger{Type: TriggerManual}, Actions: []Action{{Type: ActionEmitStep}}}, "name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.rule.Validate("scenarios[0]")
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestDeviceChangedTriggersAction(t *testing.T) {
	dispatch := &fakeDispatcher{}
	engine := newEngine(t, []Rule{{
		ID:      "relay-on",
		Board:   "dfp",
		Trigger: Trigger{Type: TriggerDeviceChanged, Device: "relay"},
		Actions: []Action{{Type: ActionSetPin, Pin: "3", Value: 1}},
	}}, dispatch, clock.NewMockClock(time.Unix(0, 0)))

	engine.HandleEvent(state.Event{
		Type:     state.EventDeviceUpdated,
		Source:   state.SourceHardwareCode,
		BoardID:  "dfp",
		DeviceID: "relay",
	})
	require.Len(t, dispatch.snapshot(), 1)
	assert.Equal(t, "setPin", dispatch.snapshot()[0].method)
	assert.Equal(t, "3", dispatch.snapshot()[0].target)
	assert.Equal(t, 1, dispatch.snapshot()[0].value)
}

func TestConditionsFilterEvents(t *testing.T) {
	dispatch := &fakeDispatcher{}
	engine := newEngine(t, []Rule{{
		ID:         "high",
		Board:      "dfp",
		Trigger:    Trigger{Type: TriggerPinChanged, Pin: "A0"},
		Conditions: []Condition{{Type: "newValue", Operator: "gte", Value: 500}},
		Actions:    []Action{{Type: ActionSetValue, Name: "level", Value: 1}},
	}}, dispatch, clock.NewMockClock(time.Unix(0, 0)))

	engine.HandleEvent(state.Event{Type: state.EventPinUpdated, Source: state.SourceHardwareCode, BoardID: "dfp", Pin: "A0", NewValue: 100})
	assert.Empty(t, dispatch.snapshot())
	engine.HandleEvent(state.Event{Type: state.EventPinUpdated, Source: state.SourceHardwareCode, BoardID: "dfp", Pin: "A0", NewValue: 700})
	require.Len(t, dispatch.snapshot(), 1)
}

func TestFunctionAndBoardStartedTriggers(t *testing.T) {
	dispatch := &fakeDispatcher{}
	engine := newEngine(t, []Rule{
		{
			ID:      "on-function",
			Board:   "dfp",
			Trigger: Trigger{Type: TriggerFunctionCalled, Name: "ack"},
			Actions: []Action{{Type: ActionEmitStep, Name: "acked"}},
		},
		{
			ID:      "on-start",
			Board:   "dfp",
			Trigger: Trigger{Type: TriggerBoardStarted},
			Actions: []Action{{Type: ActionPress, Target: "button_start"}},
		},
	}, dispatch, clock.NewMockClock(time.Unix(0, 0)))

	engine.HandleEvent(state.Event{Type: state.EventFunctionCalled, Source: state.SourceHardwareCode, BoardID: "dfp", ValueName: "ack", NewValue: 0})
	engine.HandleEvent(state.Event{Type: state.EventBoardStarted, Source: state.SourceSimulation, BoardID: "dfp"})
	calls := dispatch.snapshot()
	require.Len(t, calls, 2)
	assert.Equal(t, "step", calls[0].method)
	assert.Equal(t, "press", calls[1].method)
}

func TestManualTriggerAndDisabledRule(t *testing.T) {
	dispatch := &fakeDispatcher{}
	engine := newEngine(t, []Rule{
		{ID: "manual", Trigger: Trigger{Type: TriggerManual}, Actions: []Action{{Type: ActionEmitStep, Name: "ran"}}},
		{ID: "off", Enabled: boolPtr(false), Trigger: Trigger{Type: TriggerManual}, Actions: []Action{{Type: ActionEmitStep, Name: "never"}}},
	}, dispatch, clock.NewMockClock(time.Unix(0, 0)))

	require.NoError(t, engine.Trigger(context.Background(), "manual"))
	assert.Len(t, dispatch.snapshot(), 1)
	require.ErrorIs(t, engine.Trigger(context.Background(), "off"), ErrRuleDisabled)
	require.ErrorIs(t, engine.Trigger(context.Background(), "missing"), ErrRuleNotFound)

	require.NoError(t, engine.SetEnabled("off", true))
	require.NoError(t, engine.Trigger(context.Background(), "off"))
	assert.Len(t, dispatch.snapshot(), 2)
}

func TestManualOverrideSuppressesRuleUntilNextTrigger(t *testing.T) {
	dispatch := &fakeDispatcher{}
	engine := newEngine(t, []Rule{{
		ID:      "pin",
		Board:   "dfp",
		Trigger: Trigger{Type: TriggerPinChanged, Pin: "3"},
		Actions: []Action{{Type: ActionSetPin, Pin: "3", Value: 1}},
	}}, dispatch, clock.NewMockClock(time.Unix(0, 0)))

	// The UI write that produced the manual override is itself dropped.
	engine.HandleEvent(state.Event{Type: state.EventPinUpdated, Source: state.SourceUI, BoardID: "dfp", Pin: "3"})
	assert.Empty(t, dispatch.snapshot())
	assert.Equal(t, uint64(1), engine.Dropped())

	// The next non-manual trigger clears the override and lets the rule run.
	engine.HandleEvent(state.Event{Type: state.EventPinUpdated, Source: state.SourceSimulation, BoardID: "dfp", Pin: "3"})
	require.Len(t, dispatch.snapshot(), 1)
}

func TestBusWrittenTrigger(t *testing.T) {
	dispatch := &fakeDispatcher{}
	engine := newEngine(t, []Rule{{
		ID:      "bus",
		Board:   "dfp",
		Trigger: Trigger{Type: TriggerBusWritten, Bus: "i2c-1/0x3c"},
		Actions: []Action{{Type: ActionEmitStep, Name: "oled"}},
	}}, dispatch, clock.NewMockClock(time.Unix(0, 0)))

	engine.HandleTrace(trace.Record{BoardID: "dfp", Kind: trace.KindBusTxn, Target: "i2c-1/0x20"})
	assert.Empty(t, dispatch.snapshot())
	engine.HandleTrace(trace.Record{BoardID: "dfp", Kind: trace.KindBusTxn, Target: "i2c-1/0x3c"})
	assert.Len(t, dispatch.snapshot(), 1)
}

func TestAllActionTypes(t *testing.T) {
	dispatch := &fakeDispatcher{}
	rule := Rule{
		ID:      "actions",
		Board:   "dfp",
		Trigger: Trigger{Type: TriggerManual},
		Actions: []Action{
			{Type: ActionSetPin, Pin: "3", Value: 1},
			{Type: ActionSetValue, Name: "distance", Value: 42},
			{Type: ActionPress, Target: "button"},
			{Type: ActionRelease, Target: "button"},
			{Type: ActionCommand, Target: "relay", Command: "on"},
			{Type: ActionSetFault, Target: "relay", Fault: "welded-contact"},
			{Type: ActionClearFault, Target: "relay"},
			{Type: ActionEmitStep, Name: "done", Status: "ok", Detail: map[string]interface{}{"x": 1}},
		},
	}
	engine := newEngine(t, []Rule{rule}, dispatch, clock.NewMockClock(time.Unix(0, 0)))
	require.NoError(t, engine.Trigger(context.Background(), "actions"))
	calls := dispatch.snapshot()
	require.Len(t, calls, 8)
	assert.Equal(t, []string{"setPin", "setValue", "press", "release", "command", "setFault", "clearFault", "step"}, methods(calls))
}

func methods(calls []call) []string {
	out := make([]string, len(calls))
	for i, c := range calls {
		out[i] = c.method
	}
	return out
}

func TestTimerEveryFiresWithMockClock(t *testing.T) {
	dispatch := &fakeDispatcher{}
	clk := clock.NewMockClock(time.Unix(0, 0))
	engine := newEngine(t, []Rule{{
		ID:      "tick",
		Trigger: Trigger{Type: TriggerTimerEvery, Every: "100ms"},
		Actions: []Action{{Type: ActionEmitStep, Name: "tick"}},
	}}, dispatch, clk)

	engine.Start(context.Background())
	defer engine.Stop()
	require.Eventually(t, func() bool { return clk.Waiters() > 0 }, time.Second, time.Millisecond)
	clk.Advance(100 * time.Millisecond)
	require.Eventually(t, func() bool { return len(dispatch.snapshot()) == 1 }, time.Second, time.Millisecond)
	clk.Advance(100 * time.Millisecond)
	require.Eventually(t, func() bool { return len(dispatch.snapshot()) == 2 }, time.Second, time.Millisecond)
}

func TestDelayAfterFiresOnce(t *testing.T) {
	dispatch := &fakeDispatcher{}
	clk := clock.NewMockClock(time.Unix(0, 0))
	engine := newEngine(t, []Rule{{
		ID:      "delayed",
		Trigger: Trigger{Type: TriggerDelayAfter, After: "50ms"},
		Actions: []Action{{Type: ActionEmitStep, Name: "late"}},
	}}, dispatch, clk)
	engine.Start(context.Background())
	defer engine.Stop()
	require.Eventually(t, func() bool { return clk.Waiters() > 0 }, time.Second, time.Millisecond)
	clk.Advance(50 * time.Millisecond)
	require.Eventually(t, func() bool { return len(dispatch.snapshot()) == 1 }, time.Second, time.Millisecond)
	clk.Advance(time.Second)
	time.Sleep(5 * time.Millisecond)
	assert.Len(t, dispatch.snapshot(), 1)
}

func TestActionDelayAndRamp(t *testing.T) {
	dispatch := &fakeDispatcher{}
	clk := clock.NewMockClock(time.Unix(0, 0))
	engine := newEngine(t, []Rule{{
		ID:      "ramp",
		Board:   "dfp",
		Trigger: Trigger{Type: TriggerManual},
		Actions: []Action{
			{Type: ActionSetPin, Pin: "3", Value: 1, DelayMs: 10},
			{Type: ActionRamp, Pin: "5", From: float64Ptr(0), To: float64Ptr(100), DurationMs: 30, StepMs: 10},
		},
	}}, dispatch, clk)

	done := make(chan error, 1)
	go func() { done <- engine.Trigger(context.Background(), "ramp") }()
	require.Eventually(t, func() bool { return engine.active.Load() > 0 }, time.Second, time.Millisecond)
	for i := 0; i < 10 && engine.active.Load() > 0; i++ {
		clk.Advance(10 * time.Millisecond)
		time.Sleep(time.Millisecond)
	}
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("ramp did not finish")
	}
	calls := dispatch.snapshot()
	require.NotEmpty(t, calls)
	assert.Equal(t, "setPin", calls[0].method)
	assert.Equal(t, "5", calls[len(calls)-1].target)
	assert.Equal(t, 100, calls[len(calls)-1].value)
}

func float64Ptr(v float64) *float64 { return &v }

func TestStopCancelsEventTriggeredDelayedActions(t *testing.T) {
	dispatch := &fakeDispatcher{}
	clk := clock.NewMockClock(time.Unix(0, 0))
	engine := newEngine(t, []Rule{{
		ID:      "delayed",
		Board:   "dfp",
		Trigger: Trigger{Type: TriggerPinChanged, Pin: "3"},
		Actions: []Action{{Type: ActionSetPin, Pin: "5", Value: 1, DelayMs: 100}},
	}}, dispatch, clk)
	engine.Start(context.Background())

	handled := make(chan struct{})
	go func() {
		defer close(handled)
		engine.HandleEvent(state.Event{Type: state.EventPinUpdated, Source: state.SourceHardwareCode, BoardID: "dfp", Pin: "3"})
	}()

	// Wait until the delayed action has registered its timer, then stop the
	// engine before the timer fires.
	require.Eventually(t, func() bool { return clk.Waiters() > 0 }, time.Second, time.Millisecond)
	engine.Stop()
	clk.Advance(100 * time.Millisecond)

	select {
	case <-handled:
	case <-time.After(time.Second):
		t.Fatal("HandleEvent did not return after Stop")
	}
	assert.Empty(t, dispatch.snapshot(), "Stop must cancel in-flight delayed actions")
}

func TestLoopGuardDropsExcessExecutions(t *testing.T) {
	dispatch := &fakeDispatcher{}
	clk := clock.NewMockClock(time.Unix(0, 0))
	engine := newEngine(t, []Rule{{
		ID:      "loop",
		Trigger: Trigger{Type: TriggerManual},
		Actions: []Action{{Type: ActionEmitStep, Name: "x"}},
	}}, dispatch, clk)
	rule := engine.Rules()[0]
	for i := 0; i < maxRuleExecutionsPerWindow+5; i++ {
		engine.execute(context.Background(), rule, 0)
	}
	assert.Len(t, dispatch.snapshot(), maxRuleExecutionsPerWindow)
	assert.Equal(t, uint64(5), engine.Dropped())
	clk.Advance(ruleBudgetWindow)
	engine.execute(context.Background(), rule, 0)
	assert.Len(t, dispatch.snapshot(), maxRuleExecutionsPerWindow+1)
}
