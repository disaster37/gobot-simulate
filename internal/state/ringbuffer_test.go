package state

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRingBufferWrapAndReplay(t *testing.T) {
	ring := NewRingBuffer(3)
	for seq := uint64(1); seq <= 5; seq++ {
		ring.Append(Event{Seq: seq})
	}

	assert.Equal(t, 3, ring.Len())
	assert.Equal(t, uint64(5), ring.LastSeq())

	missed, ok := ring.Since(4)
	require.True(t, ok)
	require.Len(t, missed, 1)
	assert.Equal(t, uint64(5), missed[0].Seq)

	missed, ok = ring.Since(2)
	require.True(t, ok)
	require.Len(t, missed, 3)
	assert.Equal(t, []uint64{3, 4, 5}, []uint64{missed[0].Seq, missed[1].Seq, missed[2].Seq})

	_, ok = ring.Since(1)
	assert.False(t, ok, "sequence older than the buffer must request a snapshot")

	missed, ok = ring.Since(5)
	assert.True(t, ok)
	assert.Empty(t, missed)

	_, ok = ring.Since(0)
	assert.True(t, ok)
}

func TestRingBufferEmpty(t *testing.T) {
	ring := NewRingBuffer(2)
	assert.Equal(t, uint64(0), ring.LastSeq())
	_, ok := ring.Since(10)
	assert.False(t, ok)
	_, ok = ring.Since(0)
	assert.True(t, ok)
}

func TestHubUnsubscribeNeverClosesChannel(t *testing.T) {
	hub := NewHub()
	ch := hub.Subscribe()
	assert.Equal(t, 1, hub.Count())

	hub.Unsubscribe(ch)
	assert.Equal(t, 0, hub.Count())

	hub.Publish(Event{Seq: 1})
	select {
	case evt := <-ch:
		t.Fatalf("unexpected event after unsubscribe: %+v", evt)
	default:
	}
}

func TestHubDropsWhenSubscriberIsFull(t *testing.T) {
	hub := NewHub()
	ch := hub.Subscribe()
	defer hub.Unsubscribe(ch)

	for i := 0; i < 300; i++ {
		hub.Publish(Event{Seq: uint64(i + 1)})
	}
	assert.Greater(t, hub.Dropped(), uint64(0))
}
