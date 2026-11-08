// Package trace implements a bounded, independently locked I/O trace used by
// the timeline, waveform and SSE trace stream. Keeping it separate from the
// state event hub prevents trace volume from slowing down state fan-out.
package trace

import (
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/disaster37/gobot-simulate/internal/clock"
	"github.com/disaster37/gobot-simulate/internal/state"
)

// Kind classifies a trace record.
type Kind string

const (
	KindPinRead      Kind = "pin-read"
	KindPinWrite     Kind = "pin-write"
	KindPWM          Kind = "pwm"
	KindServo        Kind = "servo"
	KindAnalog       Kind = "analog"
	KindBusTxn       Kind = "bus-txn"
	KindValueRead    Kind = "value-read"
	KindValueWrite   Kind = "value-write"
	KindFunctionCall Kind = "function-call"
	KindDriverEvent  Kind = "driver-event"
	KindFault        Kind = "fault"
	KindStep         Kind = "step"
)

// Kinds returns every trace kind.
func Kinds() []Kind {
	return []Kind{
		KindPinRead, KindPinWrite, KindPWM, KindServo, KindAnalog,
		KindBusTxn, KindValueRead, KindValueWrite, KindFunctionCall,
		KindDriverEvent, KindFault, KindStep,
	}
}

// Valid reports whether k is a known trace kind.
func (k Kind) Valid() bool {
	for _, candidate := range Kinds() {
		if k == candidate {
			return true
		}
	}
	return false
}

// Record is one bounded trace entry.
type Record struct {
	Seq        uint64                 `json:"seq"`
	Timestamp  time.Time              `json:"timestamp"`
	BoardID    string                 `json:"boardId,omitempty"`
	Kind       Kind                   `json:"kind"`
	Target     string                 `json:"target,omitempty"`
	Operation  string                 `json:"operation,omitempty"`
	Value      interface{}            `json:"value,omitempty"`
	Source     state.EventSource      `json:"source,omitempty"`
	DurationNs int64                  `json:"durationNs,omitempty"`
	Error      string                 `json:"error,omitempty"`
	Detail     map[string]interface{} `json:"detail,omitempty"`
}

// Point is one waveform sample.
type Point struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
}

// Series is the downsampled step series for one pin.
type Series struct {
	Pin         string  `json:"pin"`
	Points      []Point `json:"points"`
	Downsampled bool    `json:"downsampled"`
}

// Store is a bounded trace ring with its own lock and subscriber fan-out.
type Store struct {
	mu   sync.RWMutex
	buf  []Record
	size int
	head int
	n    int
	seq  atomic.Uint64
	clk  clock.Clock

	subMu sync.RWMutex
	subs  map[<-chan Record]chan Record
}

// New creates a trace store holding at most size records.
func New(size int, clk clock.Clock) *Store {
	if size < 1 {
		size = 4096
	}
	if clk == nil {
		clk = clock.RealClock{}
	}
	return &Store{
		buf:  make([]Record, size),
		size: size,
		clk:  clk,
		subs: make(map[<-chan Record]chan Record),
	}
}

// Append records one trace entry. It never blocks event production.
func (s *Store) Append(rec Record) Record {
	rec.Seq = s.seq.Add(1)
	if rec.Timestamp.IsZero() {
		rec.Timestamp = s.clk.Now()
	}
	if rec.Detail != nil {
		detail := make(map[string]interface{}, len(rec.Detail))
		for k, v := range rec.Detail {
			detail[k] = v
		}
		rec.Detail = detail
	}

	s.mu.Lock()
	if s.n < s.size {
		s.buf[(s.head+s.n)%s.size] = rec
		s.n++
	} else {
		s.buf[s.head] = rec
		s.head = (s.head + 1) % s.size
	}
	s.mu.Unlock()

	s.subMu.RLock()
	for _, ch := range s.subs {
		select {
		case ch <- rec:
		default:
		}
	}
	s.subMu.RUnlock()
	return rec
}

// Query returns records newer than since, filtered by board, kind and target.
func (s *Store) Query(boardID string, since uint64, kinds []Kind, target string) []Record {
	allowed := make(map[Kind]struct{}, len(kinds))
	for _, kind := range kinds {
		allowed[kind] = struct{}{}
	}

	s.mu.RLock()
	out := make([]Record, 0, s.n)
	for i := 0; i < s.n; i++ {
		rec := s.buf[(s.head+i)%s.size]
		if rec.Seq <= since {
			continue
		}
		if boardID != "" && rec.BoardID != boardID {
			continue
		}
		if target != "" && rec.Target != target {
			continue
		}
		if len(allowed) > 0 {
			if _, ok := allowed[rec.Kind]; !ok {
				continue
			}
		}
		out = append(out, rec)
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Seq < out[j].Seq })
	return out
}

// Waveform builds downsampled step series for each requested pin.
func (s *Store) Waveform(boardID string, pins []string, from, to time.Time, maxPoints int) []Series {
	if maxPoints < 2 {
		maxPoints = 2
	}
	if maxPoints > 10000 {
		maxPoints = 10000
	}
	if len(pins) == 0 {
		pins = s.pins(boardID)
	}
	out := make([]Series, 0, len(pins))
	for _, pin := range pins {
		points := s.pinPoints(boardID, pin, from, to)
		series := Series{Pin: pin, Points: points}
		if len(points) > maxPoints {
			series.Points = downsample(points, maxPoints)
			series.Downsampled = true
		}
		out = append(out, series)
	}
	return out
}

func (s *Store) pins(boardID string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	seen := make(map[string]struct{})
	var out []string
	for i := 0; i < s.n; i++ {
		rec := s.buf[(s.head+i)%s.size]
		if rec.BoardID != boardID || rec.Target == "" {
			continue
		}
		switch rec.Kind {
		case KindPinRead, KindPinWrite, KindPWM, KindServo, KindAnalog:
		default:
			continue
		}
		if _, ok := seen[rec.Target]; ok {
			continue
		}
		seen[rec.Target] = struct{}{}
		out = append(out, rec.Target)
	}
	sort.Strings(out)
	return out
}

func (s *Store) pinPoints(boardID, pin string, from, to time.Time) []Point {
	s.mu.RLock()
	defer s.mu.RUnlock()
	points := make([]Point, 0, s.n)
	for i := 0; i < s.n; i++ {
		rec := s.buf[(s.head+i)%s.size]
		if rec.BoardID != boardID || rec.Target != pin {
			continue
		}
		switch rec.Kind {
		case KindPinRead, KindPinWrite, KindPWM, KindServo, KindAnalog:
		default:
			continue
		}
		if !from.IsZero() && rec.Timestamp.Before(from) {
			continue
		}
		if !to.IsZero() && rec.Timestamp.After(to) {
			continue
		}
		value, ok := numericValue(rec.Value)
		if !ok {
			continue
		}
		points = append(points, Point{Timestamp: rec.Timestamp, Value: value})
	}
	return points
}

func numericValue(value interface{}) (float64, bool) {
	switch v := value.(type) {
	case int:
		return float64(v), true
	case int8:
		return float64(v), true
	case int16:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	case uint:
		return float64(v), true
	case uint8:
		return float64(v), true
	case uint16:
		return float64(v), true
	case uint32:
		return float64(v), true
	case uint64:
		return float64(v), true
	case float32:
		return float64(v), true
	case float64:
		return v, true
	case bool:
		if v {
			return 1, true
		}
		return 0, true
	default:
		return 0, false
	}
}

// downsample keeps step transitions and the first/last sample, bounded by
// maxPoints.
func downsample(points []Point, maxPoints int) []Point {
	if len(points) <= maxPoints {
		return points
	}
	keep := make(map[int]struct{}, maxPoints)
	keep[0] = struct{}{}
	keep[len(points)-1] = struct{}{}
	for i := 1; i < len(points) && len(keep) < maxPoints; i++ {
		if points[i].Value != points[i-1].Value {
			keep[i] = struct{}{}
		}
	}
	if len(keep) > maxPoints {
		out := make([]Point, 0, maxPoints)
		step := float64(len(points)-1) / float64(maxPoints-1)
		seen := make(map[int]struct{}, maxPoints)
		for i := 0; i < maxPoints; i++ {
			idx := int(float64(i) * step)
			if idx >= len(points) {
				idx = len(points) - 1
			}
			if _, ok := seen[idx]; ok {
				continue
			}
			seen[idx] = struct{}{}
			out = append(out, points[idx])
		}
		return out
	}
	out := make([]Point, 0, len(keep))
	for i, point := range points {
		if _, ok := keep[i]; ok {
			out = append(out, point)
		}
	}
	return out
}

// Subscribe returns a new trace channel.
func (s *Store) Subscribe() <-chan Record {
	ch := make(chan Record, 256)
	var recv <-chan Record = ch
	s.subMu.Lock()
	s.subs[recv] = ch
	s.subMu.Unlock()
	return recv
}

// Unsubscribe removes a trace channel.
func (s *Store) Unsubscribe(ch <-chan Record) {
	s.subMu.Lock()
	delete(s.subs, ch)
	s.subMu.Unlock()
}

// Len returns the number of retained records.
func (s *Store) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.n
}
