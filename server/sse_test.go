package server_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	simulate "github.com/disaster37/gobot-simulate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type sseFrame struct {
	ID    string
	Event string
	Data  string
	Retry string
}

func readFrame(t *testing.T, scanner *bufio.Scanner) sseFrame {
	t.Helper()
	var frame sseFrame
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if frame != (sseFrame{}) {
				return frame
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		key, value, found := strings.Cut(line, ": ")
		if !found {
			key, value, _ = strings.Cut(line, ":")
		}
		switch key {
		case "id":
			frame.ID = value
		case "event":
			frame.Event = value
		case "data":
			frame.Data = value
		case "retry":
			frame.Retry = value
		}
	}
	t.Fatalf("event stream ended while reading a frame: %v", scanner.Err())
	return frame
}

func readEvent(t *testing.T, scanner *bufio.Scanner, eventType string) sseFrame {
	t.Helper()
	for {
		frame := readFrame(t, scanner)
		if frame.Event == eventType {
			return frame
		}
	}
}

func openSSE(t *testing.T, url string, headers map[string]string) (*http.Response, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	require.NoError(t, err)
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	response, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.StatusCode)
	return response, cancel
}

func httpGet(t *testing.T, url string) (*http.Response, error) {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	require.NoError(t, err)
	return http.DefaultClient.Do(req)
}

func TestSSEInitialSnapshotAndLiveEvent(t *testing.T) {
	sim := newServerSim(t)
	populateBoard(t, sim)

	response, cancel := openSSE(t, sim.URL()+"/api/v1/events", nil)
	defer cancel()
	defer func() { _ = response.Body.Close() }()
	scanner := bufio.NewScanner(response.Body)

	retry := readFrame(t, scanner)
	assert.Equal(t, "2000", retry.Retry)

	snapshot := readFrame(t, scanner)
	assert.Equal(t, "snapshot", snapshot.Event)
	assert.NotEmpty(t, snapshot.ID)
	var snap simulate.Snapshot
	require.NoError(t, json.Unmarshal([]byte(snapshot.Data), &snap))
	require.Len(t, snap.Boards, 1)
	assert.Equal(t, "dfp", snap.Boards[0].ID)

	require.NoError(t, sim.SetDigital("dfp", "3", 1, simulate.SourceUI))
	frame := readEvent(t, scanner, "pin.updated")
	assert.NotEmpty(t, frame.ID)
	var event simulate.Event
	require.NoError(t, json.Unmarshal([]byte(frame.Data), &event))
	assert.Equal(t, "3", event.Pin)
	assert.Equal(t, float64(1), event.NewValue)

	pin, err := sim.Pin("dfp", "3")
	require.NoError(t, err)
	assert.Equal(t, 1, pin.DigitalValue)
}

func TestSSELastEventIDReplay(t *testing.T) {
	sim := newServerSim(t)
	populateBoard(t, sim)

	require.NoError(t, sim.SetDigital("dfp", "3", 1, simulate.SourceUI))
	target := sim.CurrentSeq()
	require.Greater(t, target, uint64(0))

	response, cancel := openSSE(t, sim.URL()+"/api/v1/events", map[string]string{
		"Last-Event-ID": formatUint(target - 1),
	})
	defer cancel()
	defer func() { _ = response.Body.Close() }()
	scanner := bufio.NewScanner(response.Body)

	retry := readFrame(t, scanner)
	assert.Equal(t, "2000", retry.Retry)

	replayed := readFrame(t, scanner)
	assert.NotEqual(t, "snapshot", replayed.Event)
	assert.Equal(t, formatUint(target), replayed.ID)
}

func TestSSEMaxClients(t *testing.T) {
	sim := newServerSim(t, simulate.WithMaxClients(1))
	populateBoard(t, sim)

	first, firstCancel := openSSE(t, sim.URL()+"/api/v1/events", nil)
	defer firstCancel()
	defer func() { _ = first.Body.Close() }()
	require.Eventually(t, func() bool { return sim.ClientCount() == 1 }, time.Second, 5*time.Millisecond)

	response, err := httpGet(t, sim.URL()+"/api/v1/events")
	require.NoError(t, err)
	defer func() { _ = response.Body.Close() }()
	assert.Equal(t, http.StatusTooManyRequests, response.StatusCode)

	var payload map[string]interface{}
	require.NoError(t, json.NewDecoder(response.Body).Decode(&payload))
	errorPayload, ok := payload["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "TOO_MANY_CLIENTS", errorPayload["code"])
}

func TestSSEQueryToken(t *testing.T) {
	sim := newServerSim(t, simulate.WithAuthToken("secret"))
	populateBoard(t, sim)

	unauthorized, err := httpGet(t, sim.URL()+"/api/v1/events")
	require.NoError(t, err)
	_ = unauthorized.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, unauthorized.StatusCode)

	response, cancel := openSSE(t, sim.URL()+"/api/v1/events?token=secret", nil)
	defer cancel()
	defer func() { _ = response.Body.Close() }()
	scanner := bufio.NewScanner(response.Body)
	assert.Equal(t, "2000", readFrame(t, scanner).Retry)
	assert.Equal(t, "snapshot", readFrame(t, scanner).Event)
}

func formatUint(value uint64) string {
	return strconv.FormatUint(value, 10)
}
