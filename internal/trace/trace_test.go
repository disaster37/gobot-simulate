package trace

import (
	"sync"
	"testing"
	"time"

	"github.com/disaster37/gobot-simulate/internal/clock"
	"github.com/disaster37/gobot-simulate/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKindValidation(t *testing.T) {
	for _, kind := range Kinds() {
		assert.True(t, kind.Valid(), "kind %s", kind)
	}
	assert.False(t, Kind("bogus").Valid())
}

func TestStoreAppendQueryAndBoundedRing(t *testing.T) {
	clk := clock.NewMockClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	store := New(3, clk)
	for i := 0; i < 5; i++ {
		clk.Advance(time.Millisecond)
		store.Append(Record{BoardID: "dfp", Kind: KindPinWrite, Target: "3", Value: i})
	}
	assert.Equal(t, 3, store.Len())
	records := store.Query("dfp", 0, nil, "")
	require.Len(t, records, 3)
	assert.Equal(t, 2, records[0].Value)
	assert.Equal(t, 4, records[2].Value)
	assert.Equal(t, uint64(3), records[0].Seq)

	filtered := store.Query("dfp", records[0].Seq, []Kind{KindPinWrite}, "3")
	require.Len(t, filtered, 2)

	assert.Empty(t, store.Query("other", 0, nil, ""))
	assert.Empty(t, store.Query("dfp", 0, nil, "9"))
}

func TestStoreAppendClonesDetail(t *testing.T) {
	store := New(4, nil)
	detail := map[string]interface{}{"a": 1}
	rec := store.Append(Record{Kind: KindStep, Detail: detail})
	detail["a"] = 2
	assert.Equal(t, 1, rec.Detail["a"])
	assert.Equal(t, 1, store.Query("", 0, []Kind{KindStep}, "")[0].Detail["a"])
}

func TestStoreSubscribeDropsWhenFull(t *testing.T) {
	store := New(2048, nil)
	ch := store.Subscribe()
	defer store.Unsubscribe(ch)
	for i := 0; i < 300; i++ {
		store.Append(Record{Kind: KindPinRead, Target: "1", Value: i})
	}
	// The channel must never block producers; at least one record is buffered.
	assert.NotEmpty(t, ch)
}

func TestWaveformDownsamplesAndKeepsTransitions(t *testing.T) {
	clk := clock.NewMockClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	store := New(1024, clk)
	for i := 0; i < 100; i++ {
		clk.Advance(time.Millisecond)
		value := i % 2
		store.Append(Record{
			BoardID: "dfp", Kind: KindPinWrite, Target: "3", Value: value,
			Timestamp: clk.Now(),
		})
	}
	series := store.Waveform("dfp", []string{"3"}, time.Time{}, time.Time{}, 10)
	require.Len(t, series, 1)
	assert.True(t, series[0].Downsampled)
	assert.LessOrEqual(t, len(series[0].Points), 10)
	assert.Equal(t, "3", series[0].Pin)
	// Transitions are preserved: the first and last samples have distinct values.
	assert.NotEqual(t, series[0].Points[0].Value, series[0].Points[len(series[0].Points)-1].Value)
}

func TestWaveformDiscoversPinsAndFiltersTime(t *testing.T) {
	clk := clock.NewMockClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	store := New(64, clk)
	store.Append(Record{BoardID: "dfp", Kind: KindPinWrite, Target: "3", Value: 1, Timestamp: clk.Now()})
	clk.Advance(time.Second)
	store.Append(Record{BoardID: "dfp", Kind: KindAnalog, Target: "A0", Value: 512, Timestamp: clk.Now()})
	store.Append(Record{BoardID: "dfp", Kind: KindStep, Target: "start", Value: 1, Timestamp: clk.Now()})

	series := store.Waveform("dfp", nil, time.Time{}, time.Time{}, 100)
	require.Len(t, series, 2)
	assert.Equal(t, "3", series[0].Pin)
	assert.Equal(t, "A0", series[1].Pin)

	from := clk.Now().Add(-500 * time.Millisecond)
	series = store.Waveform("dfp", []string{"3"}, from, time.Time{}, 100)
	require.Len(t, series, 1)
	assert.Empty(t, series[0].Points)
}

func TestNumericValue(t *testing.T) {
	tests := []struct {
		value interface{}
		want  float64
		ok    bool
	}{
		{1, 1, true},
		{int64(2), 2, true},
		{uint8(3), 3, true},
		{float32(4), 4, true},
		{true, 1, true},
		{false, 0, true},
		{"x", 0, false},
		{nil, 0, false},
	}
	for _, tt := range tests {
		got, ok := numericValue(tt.value)
		assert.Equal(t, tt.ok, ok)
		if tt.ok {
			assert.Equal(t, tt.want, got)
		}
	}
}

func TestConcurrentAppendAndQuery(t *testing.T) {
	store := New(128, nil)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				store.Append(Record{BoardID: "dfp", Kind: KindPinRead, Target: "1", Value: i, Source: state.SourceHardwareCode})
				_ = store.Query("dfp", 0, nil, "")
			}
		}(i)
	}
	wg.Wait()
	assert.Equal(t, 128, store.Len())
}
