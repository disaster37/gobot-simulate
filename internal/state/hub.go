package state

import (
	"sync"
	"sync/atomic"
)

type subscriber struct {
	id uint64
	ch chan Event
}

// Hub is a non-blocking publish/subscribe fan-out for simulator events.
type Hub struct {
	mu          sync.RWMutex
	nextID      uint64
	subscribers map[<-chan Event]*subscriber
	dropped     atomic.Uint64
}

// NewHub creates an empty event hub.
func NewHub() *Hub {
	return &Hub{subscribers: make(map[<-chan Event]*subscriber)}
}

// Subscribe registers a new subscriber. Events are dropped for slow
// subscribers instead of blocking state mutations.
func (h *Hub) Subscribe() <-chan Event {
	ch := make(chan Event, 256)
	var recv <-chan Event = ch

	h.mu.Lock()
	h.nextID++
	h.subscribers[recv] = &subscriber{id: h.nextID, ch: ch}
	h.mu.Unlock()

	return recv
}

// Unsubscribe removes a subscriber without closing its channel. Closing would
// race with concurrent Publish callers; dropping the map entry is sufficient
// because the channel becomes unreachable after removal.
func (h *Hub) Unsubscribe(ch <-chan Event) {
	h.mu.Lock()
	delete(h.subscribers, ch)
	h.mu.Unlock()
}

// Publish delivers evt to every subscriber without blocking.
func (h *Hub) Publish(evt Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, sub := range h.subscribers {
		select {
		case sub.ch <- evt:
		default:
			h.dropped.Add(1)
		}
	}
}

// Dropped returns the number of events dropped due to slow subscribers.
func (h *Hub) Dropped() uint64 {
	return h.dropped.Load()
}

// Count returns the number of active subscribers.
func (h *Hub) Count() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.subscribers)
}
