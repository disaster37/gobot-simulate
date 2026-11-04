package state

import "sync"

// RingBuffer is a bounded, thread-safe buffer of events used to replay events
// that a Server-Sent Events client missed while disconnected.
type RingBuffer struct {
	mu    sync.RWMutex
	buf   []Event
	size  int
	start int
	count int
}

// NewRingBuffer creates a ring buffer holding at most size events.
func NewRingBuffer(size int) *RingBuffer {
	if size < 1 {
		size = 1
	}
	return &RingBuffer{buf: make([]Event, size), size: size}
}

// Append adds an event, overwriting the oldest entry when the buffer is full.
func (r *RingBuffer) Append(evt Event) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.count < r.size {
		r.buf[(r.start+r.count)%r.size] = evt
		r.count++
		return
	}
	r.buf[r.start] = evt
	r.start = (r.start + 1) % r.size
}

// Since returns all buffered events with Seq greater than seq. The boolean is
// false when seq is older than the oldest retained event, meaning a full
// snapshot must be sent instead.
func (r *RingBuffer) Since(seq uint64) ([]Event, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if seq == 0 {
		return nil, true
	}
	if r.count == 0 {
		return nil, false
	}

	oldest := r.buf[r.start].Seq
	newest := r.buf[(r.start+r.count-1)%r.size].Seq
	if seq >= newest {
		return nil, true
	}
	if seq+1 < oldest {
		return nil, false
	}

	out := make([]Event, 0, r.count)
	for i := 0; i < r.count; i++ {
		evt := r.buf[(r.start+i)%r.size]
		if evt.Seq > seq {
			out = append(out, evt)
		}
	}
	return out, true
}

// Len returns the number of buffered events.
func (r *RingBuffer) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.count
}

// LastSeq returns the sequence number of the newest buffered event, or 0.
func (r *RingBuffer) LastSeq() uint64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.count == 0 {
		return 0
	}
	return r.buf[(r.start+r.count-1)%r.size].Seq
}
