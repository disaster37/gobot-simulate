package simulate

import (
	"context"

	"github.com/disaster37/gobot-simulate/internal/scenario"
)

func (s *Simulator) startRuntime(ctx context.Context) {
	s.runtimeMu.Lock()
	if s.running {
		s.runtimeMu.Unlock()
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	runtimeCtx, cancel := context.WithCancel(ctx)
	s.runtimeCtx = runtimeCtx
	s.runtimeCancel = cancel
	s.running = true
	s.runtimeMu.Unlock()

	s.runtimeOnce.Do(func() {
		if scenarios := s.currentScenarios(); scenarios != nil {
			scenarios.Start(runtimeCtx)
		}
		go s.pumpEvents(runtimeCtx)
	})
}

func (s *Simulator) currentScenarios() *scenario.Engine {
	s.scenarioMu.RLock()
	defer s.scenarioMu.RUnlock()
	return s.scenarios
}

func (s *Simulator) pumpEvents(ctx context.Context) {
	events := s.store.Subscribe()
	defer s.store.Unsubscribe(events)
	traceCh := s.traces.Subscribe()
	defer s.traces.Unsubscribe(traceCh)

	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-events:
			if !ok {
				return
			}
			if scenarios := s.currentScenarios(); scenarios != nil {
				scenarios.HandleEvent(evt)
			}
		case rec, ok := <-traceCh:
			if !ok {
				return
			}
			if scenarios := s.currentScenarios(); scenarios != nil {
				scenarios.HandleTrace(rec)
			}
		}
	}
}

func (s *Simulator) isRunning() bool {
	s.runtimeMu.Lock()
	defer s.runtimeMu.Unlock()
	return s.running
}

func (s *Simulator) runtimeContext() context.Context {
	s.runtimeMu.Lock()
	defer s.runtimeMu.Unlock()
	if s.runtimeCtx == nil {
		return context.Background()
	}
	return s.runtimeCtx
}

// Scenarios returns the configured automation rules.
func (s *Simulator) Scenarios() []scenario.Rule {
	scenarios := s.currentScenarios()
	if scenarios == nil {
		return []scenario.Rule{}
	}
	return scenarios.Rules()
}

// SetScenarioEnabled enables or disables one scenario rule.
func (s *Simulator) SetScenarioEnabled(id string, enabled bool) error {
	scenarios := s.currentScenarios()
	if scenarios == nil {
		return scenario.ErrRuleNotFound
	}
	return scenarios.SetEnabled(id, enabled)
}

// TriggerScenario manually fires a scenario rule.
func (s *Simulator) TriggerScenario(ctx context.Context, id string) error {
	scenarios := s.currentScenarios()
	if scenarios == nil {
		return scenario.ErrRuleNotFound
	}
	return scenarios.Trigger(ctx, id)
}

// ScenarioDropped returns the dropped-action counter.
func (s *Simulator) ScenarioDropped() uint64 {
	scenarios := s.currentScenarios()
	if scenarios == nil {
		return 0
	}
	return scenarios.Dropped()
}
