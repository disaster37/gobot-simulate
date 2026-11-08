// Package scenario implements the declarative rule-based automation engine.
package scenario

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/disaster37/gobot-simulate/internal/clock"
	"github.com/disaster37/gobot-simulate/internal/state"
	"github.com/disaster37/gobot-simulate/internal/trace"
)

// TriggerType enumerates supported rule triggers.
type TriggerType string

const (
	TriggerDeviceChanged  TriggerType = "device.changed"
	TriggerPinChanged     TriggerType = "pin.changed"
	TriggerValueChanged   TriggerType = "value.changed"
	TriggerFunctionCalled TriggerType = "function.called"
	TriggerBusWritten     TriggerType = "bus.written"
	TriggerTimerEvery     TriggerType = "timer.every"
	TriggerDelayAfter     TriggerType = "delay.after"
	TriggerBoardStarted   TriggerType = "board.started"
	TriggerManual         TriggerType = "manual"
)

// ActionType enumerates supported rule actions.
type ActionType string

const (
	ActionSetPin     ActionType = "setPin"
	ActionSetValue   ActionType = "setValue"
	ActionPress      ActionType = "press"
	ActionRelease    ActionType = "release"
	ActionCommand    ActionType = "command"
	ActionRamp       ActionType = "ramp"
	ActionSetFault   ActionType = "setFault"
	ActionClearFault ActionType = "clearFault"
	ActionEmitStep   ActionType = "emitStep"
)

// Trigger describes when a rule fires.
type Trigger struct {
	Type    TriggerType `json:"type" yaml:"type"`
	Device  string      `json:"device,omitempty" yaml:"device,omitempty"`
	Pin     string      `json:"pin,omitempty" yaml:"pin,omitempty"`
	Name    string      `json:"name,omitempty" yaml:"name,omitempty"`
	Bus     string      `json:"bus,omitempty" yaml:"bus,omitempty"`
	Address string      `json:"address,omitempty" yaml:"address,omitempty"`
	Every   string      `json:"every,omitempty" yaml:"every,omitempty"`
	After   string      `json:"after,omitempty" yaml:"after,omitempty"`
}

// Condition filters a trigger. Operators: eq, ne, gt, gte, lt, lte, truthy, falsy.
type Condition struct {
	Type     string      `json:"type" yaml:"type"`
	Target   string      `json:"target" yaml:"target"`
	Operator string      `json:"operator,omitempty" yaml:"operator,omitempty"`
	Value    interface{} `json:"value,omitempty" yaml:"value,omitempty"`
}

// Action describes what a rule does.
type Action struct {
	Type       ActionType             `json:"type" yaml:"type"`
	Target     string                 `json:"target,omitempty" yaml:"target,omitempty"`
	Pin        string                 `json:"pin,omitempty" yaml:"pin,omitempty"`
	Name       string                 `json:"name,omitempty" yaml:"name,omitempty"`
	Command    string                 `json:"command,omitempty" yaml:"command,omitempty"`
	Value      interface{}            `json:"value,omitempty" yaml:"value,omitempty"`
	From       *float64               `json:"from,omitempty" yaml:"from,omitempty"`
	To         *float64               `json:"to,omitempty" yaml:"to,omitempty"`
	DurationMs int                    `json:"durationMs,omitempty" yaml:"durationMs,omitempty"`
	StepMs     int                    `json:"stepMs,omitempty" yaml:"stepMs,omitempty"`
	DelayMs    int                    `json:"delayMs,omitempty" yaml:"delayMs,omitempty"`
	Fault      string                 `json:"fault,omitempty" yaml:"fault,omitempty"`
	Status     string                 `json:"status,omitempty" yaml:"status,omitempty"`
	Detail     map[string]interface{} `json:"detail,omitempty" yaml:"detail,omitempty"`
}

// Rule is a complete declarative automation rule.
type Rule struct {
	ID         string      `json:"id" yaml:"id"`
	Name       string      `json:"name,omitempty" yaml:"name,omitempty"`
	Enabled    *bool       `json:"enabled,omitempty" yaml:"enabled,omitempty"`
	Board      string      `json:"board,omitempty" yaml:"board,omitempty"`
	Trigger    Trigger     `json:"trigger" yaml:"trigger"`
	Conditions []Condition `json:"conditions,omitempty" yaml:"conditions,omitempty"`
	Actions    []Action    `json:"actions" yaml:"actions"`
}

// IsEnabled reports whether the rule is enabled (default true).
func (r Rule) IsEnabled() bool {
	return r.Enabled == nil || *r.Enabled
}

// Validate returns path-qualified validation errors.
func (r Rule) Validate(path string) error {
	var errs []error
	if strings.TrimSpace(r.ID) == "" {
		errs = append(errs, fmt.Errorf("%s.id: must not be empty", path))
	}
	if !r.Trigger.Type.Valid() {
		errs = append(errs, fmt.Errorf("%s.trigger.type: unknown value %q", path, r.Trigger.Type))
	}
	if r.Trigger.Type == TriggerTimerEvery {
		if _, err := time.ParseDuration(r.Trigger.Every); err != nil {
			errs = append(errs, fmt.Errorf("%s.trigger.every: invalid duration %q", path, r.Trigger.Every))
		}
	}
	if r.Trigger.Type == TriggerDelayAfter {
		if _, err := time.ParseDuration(r.Trigger.After); err != nil {
			errs = append(errs, fmt.Errorf("%s.trigger.after: invalid duration %q", path, r.Trigger.After))
		}
	}
	if len(r.Actions) == 0 {
		errs = append(errs, fmt.Errorf("%s.actions: must not be empty", path))
	}
	for i, action := range r.Actions {
		errs = append(errs, action.validate(fmt.Sprintf("%s.actions[%d]", path, i))...)
	}
	return errors.Join(errs...)
}

// Valid reports whether t is a known trigger type.
func (t TriggerType) Valid() bool {
	switch t {
	case TriggerDeviceChanged, TriggerPinChanged, TriggerValueChanged,
		TriggerFunctionCalled, TriggerBusWritten, TriggerTimerEvery,
		TriggerDelayAfter, TriggerBoardStarted, TriggerManual:
		return true
	default:
		return false
	}
}

// Valid reports whether a is a known action type.
func (a ActionType) Valid() bool {
	switch a {
	case ActionSetPin, ActionSetValue, ActionPress, ActionRelease, ActionCommand,
		ActionRamp, ActionSetFault, ActionClearFault, ActionEmitStep:
		return true
	default:
		return false
	}
}

func (a Action) validate(path string) []error {
	var errs []error
	if !a.Type.Valid() {
		errs = append(errs, fmt.Errorf("%s.type: unknown value %q", path, a.Type))
		return errs
	}
	if a.DurationMs < 0 || a.StepMs < 0 || a.DelayMs < 0 {
		errs = append(errs, fmt.Errorf("%s: durations must not be negative", path))
	}
	switch a.Type {
	case ActionSetPin, ActionRamp:
		if a.Pin == "" && a.Target == "" {
			errs = append(errs, fmt.Errorf("%s.pin: required for %s", path, a.Type))
		}
	case ActionSetValue:
		if a.Name == "" && a.Target == "" {
			errs = append(errs, fmt.Errorf("%s.name: required for setValue", path))
		}
	case ActionPress, ActionRelease, ActionCommand, ActionSetFault, ActionClearFault:
		if a.Target == "" {
			errs = append(errs, fmt.Errorf("%s.target: required for %s", path, a.Type))
		}
	case ActionEmitStep:
		if a.Name == "" {
			errs = append(errs, fmt.Errorf("%s.name: required for emitStep", path))
		}
	}
	return errs
}

// Dispatcher is implemented by the simulator. Implementations must not hold
// board locks while invoking drivers (driver-aware commands do the dispatch).
type Dispatcher interface {
	ScenarioSetPin(ctx context.Context, boardID, pin string, value int) error
	ScenarioSetValue(ctx context.Context, boardID, name string, value interface{}) error
	ScenarioPress(ctx context.Context, boardID, deviceID string) error
	ScenarioRelease(ctx context.Context, boardID, deviceID string) error
	ScenarioCommand(ctx context.Context, boardID, deviceID, action string, value *int) error
	ScenarioSetDeviceFault(ctx context.Context, boardID, deviceID, fault string) error
	ScenarioClearDeviceFault(ctx context.Context, boardID, deviceID string) error
	ScenarioEmitStep(ctx context.Context, boardID, name, status string, detail map[string]interface{}) error
}

const (
	defaultCycleDepthLimit = 8
	// maxRuleExecutionsPerWindow bounds a rule's firing rate so event/action
	// feedback loops cannot spin forever.
	maxRuleExecutionsPerWindow = 100
	ruleBudgetWindow           = time.Second
)

// Engine evaluates rules against state and trace events.
type Engine struct {
	mu        sync.RWMutex
	rules     map[string]*Rule
	order     []string
	dispatch  Dispatcher
	clk       clock.Clock
	manual    map[string]struct{}
	budget    map[string]*ruleBudget
	enabled   bool
	dropped   atomic.Uint64
	active    atomic.Int64
	ctx       context.Context
	cancel    context.CancelFunc
	startOnce sync.Once
}

type ruleBudget struct {
	windowStart time.Time
	count       int
}

// New creates an engine. Rules are validated by the topology loader.
func New(rules []Rule, dispatch Dispatcher, clk clock.Clock) *Engine {
	if clk == nil {
		clk = clock.RealClock{}
	}
	e := &Engine{
		rules:    make(map[string]*Rule, len(rules)),
		dispatch: dispatch,
		clk:      clk,
		manual:   make(map[string]struct{}),
		budget:   make(map[string]*ruleBudget),
		enabled:  true,
	}
	for i := range rules {
		rule := rules[i]
		if !rule.IsEnabled() {
			disabled := false
			rule.Enabled = &disabled
		}
		e.rules[rule.ID] = &rule
		e.order = append(e.order, rule.ID)
	}
	return e
}

// Rules returns a copy of the configured rules in declaration order.
func (e *Engine) Rules() []Rule {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]Rule, 0, len(e.order))
	for _, id := range e.order {
		if rule, ok := e.rules[id]; ok {
			out = append(out, *rule)
		}
	}
	return out
}

// SetEnabled enables or disables a rule.
func (e *Engine) SetEnabled(id string, enabled bool) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	rule, ok := e.rules[id]
	if !ok {
		return fmt.Errorf("%w: %s", ErrRuleNotFound, id)
	}
	rule.Enabled = &enabled
	return nil
}

// Trigger manually fires a rule.
func (e *Engine) Trigger(ctx context.Context, id string) error {
	e.mu.RLock()
	rule, ok := e.rules[id]
	e.mu.RUnlock()
	if !ok {
		return fmt.Errorf("%w: %s", ErrRuleNotFound, id)
	}
	if !rule.IsEnabled() {
		return fmt.Errorf("%w: %s", ErrRuleDisabled, id)
	}
	e.execute(ctx, *rule, 0)
	return nil
}

// Start launches timer-driven rules.
func (e *Engine) Start(ctx context.Context) {
	e.startOnce.Do(func() {
		if ctx == nil {
			ctx = context.Background()
		}
		e.ctx, e.cancel = context.WithCancel(ctx)
		e.mu.RLock()
		rules := make([]Rule, 0, len(e.order))
		for _, id := range e.order {
			if rule, ok := e.rules[id]; ok {
				rules = append(rules, *rule)
			}
		}
		e.mu.RUnlock()
		for _, rule := range rules {
			if !rule.IsEnabled() {
				continue
			}
			switch rule.Trigger.Type {
			case TriggerTimerEvery:
				interval, err := time.ParseDuration(rule.Trigger.Every)
				if err != nil || interval <= 0 {
					continue
				}
				go e.timerLoop(rule, interval, true)
			case TriggerDelayAfter:
				delay, err := time.ParseDuration(rule.Trigger.After)
				if err != nil || delay < 0 {
					continue
				}
				go e.timerLoop(rule, delay, false)
			}
		}
	})
}

// Stop cancels timers and in-flight delayed actions.
func (e *Engine) Stop() {
	if e.cancel != nil {
		e.cancel()
	}
}

func (e *Engine) timerLoop(rule Rule, d time.Duration, repeat bool) {
	if e.ctx == nil {
		return
	}
	for {
		select {
		case <-e.ctx.Done():
			return
		case <-e.clk.After(d):
			if e.ctx.Err() != nil {
				return
			}
			e.execute(e.ctx, rule, 0)
			if !repeat {
				return
			}
		}
	}
}

// HandleEvent evaluates rules for a state event.
func (e *Engine) HandleEvent(evt state.Event) {
	e.mu.RLock()
	ruleList := make([]Rule, 0, len(e.order))
	for _, id := range e.order {
		if rule, ok := e.rules[id]; ok {
			ruleList = append(ruleList, *rule)
		}
	}
	e.mu.RUnlock()

	target := eventTarget(evt)
	if evt.Source == state.SourceUI {
		e.MarkManual(evt.BoardID, target)
	} else if target != "" {
		e.clearManual(evt.BoardID, target)
	}
	for _, rule := range ruleList {
		if !rule.IsEnabled() || !e.matchesEvent(rule, evt, target) {
			continue
		}
		if !e.conditionsMatch(rule, evt) {
			continue
		}
		if rule.Board == "" {
			rule.Board = evt.BoardID
		}
		e.execute(context.Background(), rule, 0)
	}
}

// HandleTrace evaluates rules for a bus operation trace record.
func (e *Engine) HandleTrace(rec trace.Record) {
	if rec.Kind != trace.KindBusTxn {
		return
	}
	e.mu.RLock()
	ruleList := make([]Rule, 0, len(e.order))
	for _, id := range e.order {
		if rule, ok := e.rules[id]; ok {
			ruleList = append(ruleList, *rule)
		}
	}
	e.mu.RUnlock()
	for _, rule := range ruleList {
		if !rule.IsEnabled() || rule.Trigger.Type != TriggerBusWritten {
			continue
		}
		if rule.Board != "" && rule.Board != rec.BoardID {
			continue
		}
		if rule.Trigger.Bus != "" && rule.Trigger.Bus != rec.Target {
			continue
		}
		if rule.Board == "" {
			rule.Board = rec.BoardID
		}
		e.execute(context.Background(), rule, 0)
	}
}

// MarkManual records a manual override for a board target so rules acting on
// that target are suppressed until the next non-manual trigger clears it.
func (e *Engine) MarkManual(boardID, target string) {
	if target == "" {
		return
	}
	e.mu.Lock()
	e.manual[manualKey(boardID, target)] = struct{}{}
	e.mu.Unlock()
}

func (e *Engine) clearManual(boardID, target string) {
	if target == "" {
		return
	}
	e.mu.Lock()
	delete(e.manual, manualKey(boardID, target))
	e.mu.Unlock()
}

func (e *Engine) isManual(boardID, target string) bool {
	if target == "" {
		return false
	}
	e.mu.RLock()
	_, ok := e.manual[manualKey(boardID, target)]
	e.mu.RUnlock()
	return ok
}

func manualKey(boardID, target string) string { return boardID + "\x00" + target }

// Dropped returns the number of actions dropped by depth limits or manual
// overrides.
func (e *Engine) Dropped() uint64 { return e.dropped.Load() }

func (e *Engine) matchesEvent(rule Rule, evt state.Event, target string) bool {
	if rule.Board != "" && rule.Board != evt.BoardID {
		return false
	}
	t := rule.Trigger
	switch t.Type {
	case TriggerDeviceChanged:
		return evt.Type == state.EventDeviceUpdated && (t.Device == "" || t.Device == evt.DeviceID || t.Device == target)
	case TriggerPinChanged:
		return evt.Type == state.EventPinUpdated && (t.Pin == "" || t.Pin == evt.Pin || t.Pin == target)
	case TriggerValueChanged:
		return evt.Type == state.EventValueUpdated && (t.Name == "" || t.Name == evt.ValueName || t.Name == target)
	case TriggerFunctionCalled:
		return evt.Type == state.EventFunctionCalled && (t.Name == "" || t.Name == evt.ValueName || t.Name == target)
	case TriggerBoardStarted:
		return evt.Type == state.EventBoardStarted
	default:
		return false
	}
}

func (e *Engine) conditionsMatch(rule Rule, evt state.Event) bool {
	for _, cond := range rule.Conditions {
		actual := conditionValue(cond, evt)
		if !compare(cond.Operator, actual, cond.Value) {
			return false
		}
	}
	return true
}

func conditionValue(cond Condition, evt state.Event) interface{} {
	switch cond.Type {
	case "value", "newValue":
		return evt.NewValue
	case "oldValue":
		return evt.OldValue
	case "device":
		if evt.Device != nil {
			return evt.Device.Active
		}
	case "pin":
		if evt.PinState != nil {
			return evt.PinState.DigitalValue
		}
	case "source":
		return string(evt.Source)
	}
	return evt.NewValue
}

func compare(operator string, actual, expected interface{}) bool {
	switch operator {
	case "truthy", "":
		return truthy(actual)
	case "falsy":
		return !truthy(actual)
	case "eq":
		return fmt.Sprint(actual) == fmt.Sprint(expected)
	case "ne":
		return fmt.Sprint(actual) != fmt.Sprint(expected)
	case "gt", "gte", "lt", "lte":
		a, aok := number(actual)
		b, bok := number(expected)
		if !aok || !bok {
			return false
		}
		switch operator {
		case "gt":
			return a > b
		case "gte":
			return a >= b
		case "lt":
			return a < b
		default:
			return a <= b
		}
	default:
		return false
	}
}

func number(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case float32:
		return float64(n), true
	case float64:
		return n, true
	case byte:
		return float64(n), true
	default:
		return 0, false
	}
}

func truthy(v interface{}) bool {
	switch value := v.(type) {
	case bool:
		return value
	case string:
		return value != ""
	case int:
		return value != 0
	case int64:
		return value != 0
	case float64:
		return value != 0
	case nil:
		return false
	default:
		return true
	}
}

func (e *Engine) execute(ctx context.Context, rule Rule, depth int) {
	if ctx == nil {
		ctx = context.Background()
	}
	if depth >= defaultCycleDepthLimit || !e.allow(rule.ID) {
		e.dropped.Add(1)
		return
	}
	e.active.Add(1)
	defer e.active.Add(-1)

	for _, action := range rule.Actions {
		if err := e.wait(ctx, action.DelayMs); err != nil {
			return
		}
		e.apply(ctx, rule, action, depth)
	}
}

// allow enforces a sliding execution budget per rule so a rule whose actions
// feed its own trigger cannot loop forever.
func (e *Engine) allow(ruleID string) bool {
	now := e.clk.Now()
	e.mu.Lock()
	defer e.mu.Unlock()
	budget, ok := e.budget[ruleID]
	if !ok || now.Sub(budget.windowStart) >= ruleBudgetWindow {
		e.budget[ruleID] = &ruleBudget{windowStart: now, count: 1}
		return true
	}
	if budget.count >= maxRuleExecutionsPerWindow {
		return false
	}
	budget.count++
	return true
}

func (e *Engine) wait(ctx context.Context, delayMs int) error {
	if delayMs <= 0 {
		return ctx.Err()
	}
	timer := e.clk.After(time.Duration(delayMs) * time.Millisecond)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer:
		return nil
	}
}

func (e *Engine) apply(ctx context.Context, rule Rule, action Action, depth int) {
	boardID := rule.Board
	if e.isManual(boardID, actionTarget(action)) {
		e.dropped.Add(1)
		return
	}
	if e.dispatch == nil {
		e.dropped.Add(1)
		return
	}
	switch action.Type {
	case ActionSetPin:
		pin := firstNonEmpty(action.Pin, action.Target)
		if err := e.dispatch.ScenarioSetPin(ctx, boardID, pin, intValue(action.Value)); err != nil {
			e.dropped.Add(1)
		}
	case ActionSetValue:
		name := firstNonEmpty(action.Name, action.Target)
		if err := e.dispatch.ScenarioSetValue(ctx, boardID, name, action.Value); err != nil {
			e.dropped.Add(1)
		}
	case ActionPress:
		if err := e.dispatch.ScenarioPress(ctx, boardID, action.Target); err != nil {
			e.dropped.Add(1)
		}
	case ActionRelease:
		if err := e.dispatch.ScenarioRelease(ctx, boardID, action.Target); err != nil {
			e.dropped.Add(1)
		}
	case ActionCommand:
		var value *int
		if action.Value != nil {
			v := intValue(action.Value)
			value = &v
		}
		command := firstNonEmpty(action.Command, action.Name, "toggle")
		if err := e.dispatch.ScenarioCommand(ctx, boardID, action.Target, command, value); err != nil {
			e.dropped.Add(1)
		}
	case ActionRamp:
		e.ramp(ctx, boardID, action, depth)
	case ActionSetFault:
		if err := e.dispatch.ScenarioSetDeviceFault(ctx, boardID, action.Target, action.Fault); err != nil {
			e.dropped.Add(1)
		}
	case ActionClearFault:
		if err := e.dispatch.ScenarioClearDeviceFault(ctx, boardID, action.Target); err != nil {
			e.dropped.Add(1)
		}
	case ActionEmitStep:
		if err := e.dispatch.ScenarioEmitStep(ctx, boardID, action.Name, action.Status, action.Detail); err != nil {
			e.dropped.Add(1)
		}
	}
}

func (e *Engine) ramp(ctx context.Context, boardID string, action Action, depth int) {
	from := 0.0
	to := 0.0
	if action.From != nil {
		from = *action.From
	}
	if action.To != nil {
		to = *action.To
	}
	duration := action.DurationMs
	if duration < 0 {
		duration = 0
	}
	stepMs := action.StepMs
	if stepMs <= 0 {
		stepMs = 10
	}
	steps := duration / stepMs
	if steps < 1 {
		steps = 1
	}
	pin := firstNonEmpty(action.Pin, action.Target)
	for i := 1; i <= steps; i++ {
		if err := e.wait(ctx, stepMs); err != nil {
			return
		}
		value := from + (to-from)*float64(i)/float64(steps)
		if err := e.dispatch.ScenarioSetPin(ctx, boardID, pin, int(value)); err != nil {
			e.dropped.Add(1)
			return
		}
	}
	_ = depth
}

func actionTarget(action Action) string {
	switch action.Type {
	case ActionSetPin, ActionRamp:
		return firstNonEmpty(action.Pin, action.Target, action.Name)
	case ActionSetValue:
		return firstNonEmpty(action.Name, action.Target)
	default:
		return firstNonEmpty(action.Target, action.Name)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func intValue(v interface{}) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case float32:
		return int(n)
	case byte:
		return int(n)
	case bool:
		if n {
			return 1
		}
		return 0
	default:
		return 0
	}
}

func eventTarget(evt state.Event) string {
	switch {
	case evt.DeviceID != "":
		return evt.DeviceID
	case evt.Pin != "":
		return evt.Pin
	case evt.ValueName != "":
		return evt.ValueName
	default:
		return ""
	}
}

// ErrRuleNotFound is returned for an unknown scenario rule.
var ErrRuleNotFound = errors.New("simulate: scenario not found")

// ErrRuleDisabled is returned when manually triggering a disabled rule.
var ErrRuleDisabled = errors.New("simulate: scenario disabled")
