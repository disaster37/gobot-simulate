package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/sse"
	"github.com/disaster37/gobot-simulate/internal/state"
)

const sseHeartbeatInterval = 15 * time.Second

func (s *Server) handleEvents(ctx context.Context, c *app.RequestContext) {
	if s.cfg.MaxClients > 0 && s.clients.Load() >= int64(s.cfg.MaxClients) {
		writeBackendError(c, "", state.ErrClientCap)
		return
	}
	s.clients.Add(1)
	defer s.clients.Add(-1)

	lastID := parseEventID(string(c.GetHeader("Last-Event-ID")))
	if lastID == 0 {
		lastID = parseEventID(c.Query("lastEventId"))
	}

	events, missed, replayOK := s.backend.SubscribeWithReplay(lastID)
	defer s.backend.Unsubscribe(events)

	c.SetStatusCode(http.StatusOK)
	writer := sse.NewWriter(c)
	defer func() { _ = writer.Close() }()
	c.Header("Cache-Control", "no-cache, no-transform")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	if err := writer.Write(&sse.Event{Retry: 2 * time.Second}); err != nil {
		return
	}

	lastSent := uint64(0)
	if lastID > 0 && replayOK {
		lastSent = lastID
		for i := range missed {
			if err := writeEvent(writer, missed[i]); err != nil {
				return
			}
			lastSent = missed[i].Seq
		}
	} else {
		snapshot := s.backend.Snapshot()
		data, err := json.Marshal(snapshot)
		if err != nil {
			return
		}
		if err := writer.Write(&sse.Event{
			ID:   strconv.FormatUint(snapshot.Meta.CurrentSeq, 10),
			Type: string(state.EventSnapshot),
			Data: data,
		}); err != nil {
			return
		}
		lastSent = snapshot.Meta.CurrentSeq
	}

	s.mu.Lock()
	stopCh := s.stopCh
	s.mu.Unlock()
	heartbeat := time.NewTicker(sseHeartbeatInterval)
	defer heartbeat.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-stopCh:
			return
		case evt, ok := <-events:
			if !ok {
				return
			}
			if evt.Seq <= lastSent {
				continue
			}
			if err := writeEvent(writer, evt); err != nil {
				return
			}
			lastSent = evt.Seq
		case <-heartbeat.C:
			data, err := json.Marshal(struct {
				Timestamp string `json:"timestamp"`
			}{Timestamp: time.Now().UTC().Format(time.RFC3339)})
			if err != nil {
				return
			}
			if err := writer.Write(&sse.Event{Type: string(state.EventHeartbeat), Data: data}); err != nil {
				return
			}
		}
	}
}

func writeEvent(writer *sse.Writer, evt state.Event) error {
	data, err := json.Marshal(evt)
	if err != nil {
		return err
	}
	return writer.Write(&sse.Event{
		ID:   strconv.FormatUint(evt.Seq, 10),
		Type: string(evt.Type),
		Data: data,
	})
}
