// Package clock provides a small abstraction over wall-clock time so state
// transitions can be tested deterministically.
package clock

import (
	"sync"
	"time"
)

// Clock is the subset of time operations used by the simulator.
type Clock interface {
	Now() time.Time
	Since(t time.Time) time.Duration
	After(d time.Duration) <-chan time.Time
}

// RealClock delegates to the standard library.
type RealClock struct{}

// Now returns the current wall-clock time.
func (RealClock) Now() time.Time { return time.Now() }

// Since returns the elapsed time since t.
func (RealClock) Since(t time.Time) time.Duration { return time.Since(t) }

// After waits for the duration to elapse and then sends the current time.
func (RealClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

type waiter struct {
	at time.Time
	ch chan time.Time
}

// MockClock is a manually advanced clock for deterministic tests.
type MockClock struct {
	mu      sync.Mutex
	now     time.Time
	waiters []waiter
}

// NewMockClock returns a MockClock initialised to t.
func NewMockClock(t time.Time) *MockClock {
	return &MockClock{now: t}
}

// Now returns the mock clock's current time.
func (m *MockClock) Now() time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.now
}

// Since returns the simulated elapsed time since t.
func (m *MockClock) Since(t time.Time) time.Duration {
	return m.Now().Sub(t)
}

// After registers a waiter fired by Advance.
func (m *MockClock) After(d time.Duration) <-chan time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()

	ch := make(chan time.Time, 1)
	m.waiters = append(m.waiters, waiter{at: m.now.Add(d), ch: ch})
	return ch
}

// Waiters returns the number of pending timers. Tests use this to avoid
// advancing the clock before a timer goroutine has registered itself.
func (m *MockClock) Waiters() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.waiters)
}

// Advance moves the mock clock forward and fires due waiters.
func (m *MockClock) Advance(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.now = m.now.Add(d)
	remaining := m.waiters[:0]
	for _, w := range m.waiters {
		if !w.at.After(m.now) {
			select {
			case w.ch <- m.now:
			default:
			}
			continue
		}
		remaining = append(remaining, w)
	}
	m.waiters = remaining
}
