package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/disaster37/gobot-simulate/boardprofile"
	"github.com/disaster37/gobot-simulate/internal/bus"
	"github.com/disaster37/gobot-simulate/internal/scenario"
	"github.com/disaster37/gobot-simulate/internal/state"
	"github.com/disaster37/gobot-simulate/internal/trace"
)

// maxCommandValue bounds a device command's numeric argument. It must cover
// the packed RGB triplet (0xFFFFFF) used by the setrgb action.
const maxCommandValue = 0xFFFFFF

func (s *Server) handleCommand(ctx context.Context, c *app.RequestContext) {
	boardID := c.Param("board")
	deviceID := c.Param("device")
	if !validatePath(c, "board", boardID) || !validatePath(c, "device", deviceID) {
		return
	}
	var req struct {
		Action string `json:"action"`
		Value  *int   `json:"value"`
	}
	if err := decodeJSON(c, &req); err != nil {
		writeError(c, boardID, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}
	req.Action = strings.TrimSpace(req.Action)
	if req.Action == "" || len(req.Action) > 64 {
		writeBackendError(c, boardID, state.ErrInvalidValue)
		return
	}
	// The RGB LED command packs three 8-bit channels into a single integer
	// (red<<16 | green<<8 | blue), so the upper bound must cover 0xFFFFFF.
	if req.Value != nil && (*req.Value < -maxCommandValue || *req.Value > maxCommandValue) {
		writeBackendError(c, boardID, state.ErrInvalidValue)
		return
	}
	if err := s.backend.CommandDevice(boardID, deviceID, req.Action, req.Value); err != nil {
		writeBackendError(c, boardID, err)
		return
	}
	s.writeDevice(c, boardID, deviceID)
	_ = ctx
}

func (s *Server) handleSetDeviceFault(_ context.Context, c *app.RequestContext) {
	boardID := c.Param("board")
	deviceID := c.Param("device")
	if !validatePath(c, "board", boardID) || !validatePath(c, "device", deviceID) {
		return
	}
	var req struct {
		Fault string `json:"fault"`
	}
	if err := decodeJSON(c, &req); err != nil {
		writeError(c, boardID, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}
	updated, err := s.backend.SetDeviceFault(boardID, deviceID, strings.TrimSpace(req.Fault))
	if err != nil {
		writeBackendError(c, boardID, err)
		return
	}
	c.JSON(http.StatusOK, updated)
}

func (s *Server) handleClearDeviceFault(_ context.Context, c *app.RequestContext) {
	boardID := c.Param("board")
	deviceID := c.Param("device")
	if !validatePath(c, "board", boardID) || !validatePath(c, "device", deviceID) {
		return
	}
	var req struct {
		Fault string `json:"fault"`
	}
	if err := decodeOptionalJSON(c, &req); err != nil {
		writeError(c, boardID, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}
	fault := strings.TrimSpace(req.Fault)
	if fault == "" {
		fault = strings.TrimSpace(c.Query("fault"))
	}
	var (
		updated state.DeviceState
		err     error
	)
	if fault == "" {
		updated, err = s.backend.ClearDeviceFaults(boardID, deviceID)
	} else {
		updated, err = s.backend.ClearDeviceFault(boardID, deviceID, fault)
	}
	if err != nil {
		writeBackendError(c, boardID, err)
		return
	}
	c.JSON(http.StatusOK, updated)
}

func (s *Server) handleTrace(_ context.Context, c *app.RequestContext) {
	boardID := c.Param("board")
	if !validatePath(c, "board", boardID) {
		return
	}
	since := uint64(0)
	if raw := strings.TrimSpace(c.Query("since")); raw != "" {
		value, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			writeBackendError(c, boardID, state.ErrInvalidValue)
			return
		}
		since = value
	}
	var kinds []string
	if raw := strings.TrimSpace(c.Query("kinds")); raw != "" {
		kinds = strings.Split(raw, ",")
		if len(kinds) > len(trace.Kinds()) {
			writeBackendError(c, boardID, state.ErrInvalidValue)
			return
		}
	}
	records, err := s.backend.Trace(boardID, since, kinds, strings.TrimSpace(c.Query("target")))
	if err != nil {
		writeBackendError(c, boardID, err)
		return
	}
	if len(records) > 5000 {
		records = records[len(records)-5000:]
	}
	c.JSON(http.StatusOK, struct {
		Records []trace.Record `json:"records"`
	}{Records: records})
}

func (s *Server) handleWaveform(_ context.Context, c *app.RequestContext) {
	boardID := c.Param("board")
	if !validatePath(c, "board", boardID) {
		return
	}
	var pins []string
	if raw := strings.TrimSpace(c.Query("pins")); raw != "" {
		pins = strings.Split(raw, ",")
		if len(pins) > 32 {
			writeBackendError(c, boardID, state.ErrInvalidValue)
			return
		}
		for _, pin := range pins {
			if !validatePath(c, "pin", strings.TrimSpace(pin)) {
				return
			}
		}
	}
	from, err := parseTimeQuery(c.Query("from"))
	if err != nil {
		writeBackendError(c, boardID, state.ErrInvalidValue)
		return
	}
	to, err := parseTimeQuery(c.Query("to"))
	if err != nil {
		writeBackendError(c, boardID, state.ErrInvalidValue)
		return
	}
	maxPoints := 500
	if raw := strings.TrimSpace(c.Query("maxPoints")); raw != "" {
		value, parseErr := strconv.Atoi(raw)
		if parseErr != nil || value < 2 || value > 10000 {
			writeBackendError(c, boardID, state.ErrInvalidValue)
			return
		}
		maxPoints = value
	}
	series, err := s.backend.Waveform(boardID, pins, from, to, maxPoints)
	if err != nil {
		writeBackendError(c, boardID, err)
		return
	}
	c.JSON(http.StatusOK, struct {
		Series []trace.Series `json:"series"`
	}{Series: series})
}

func parseTimeQuery(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, nil
	}
	value, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}, err
	}
	return value, nil
}

func (s *Server) handleStep(_ context.Context, c *app.RequestContext) {
	boardID := c.Param("board")
	if !validatePath(c, "board", boardID) {
		return
	}
	var req struct {
		Name   string                 `json:"name"`
		Status string                 `json:"status"`
		Detail map[string]interface{} `json:"detail"`
	}
	if err := decodeJSON(c, &req); err != nil {
		writeError(c, boardID, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Status = strings.TrimSpace(req.Status)
	if req.Name == "" || len(req.Name) > 128 || len(req.Status) > 128 {
		writeBackendError(c, boardID, state.ErrInvalidValue)
		return
	}
	if err := s.backend.Step(boardID, req.Name, req.Status, req.Detail); err != nil {
		writeBackendError(c, boardID, err)
		return
	}
	c.JSON(http.StatusCreated, struct {
		Name   string `json:"name"`
		Status string `json:"status"`
	}{Name: req.Name, Status: req.Status})
}

func (s *Server) handleBuses(_ context.Context, c *app.RequestContext) {
	boardID := c.Param("board")
	if !validatePath(c, "board", boardID) {
		return
	}
	buses, err := s.backend.Buses(boardID)
	if err != nil {
		writeBackendError(c, boardID, err)
		return
	}
	c.JSON(http.StatusOK, struct {
		Buses []map[string]interface{} `json:"buses"`
	}{Buses: buses})
}

func (s *Server) handleChips(_ context.Context, c *app.RequestContext) {
	boardID := c.Param("board")
	if !validatePath(c, "board", boardID) {
		return
	}
	chips, err := s.backend.Chips(boardID)
	if err != nil {
		writeBackendError(c, boardID, err)
		return
	}
	if chips == nil {
		chips = []bus.ChipEntry{}
	}
	c.JSON(http.StatusOK, struct {
		Chips []bus.ChipEntry `json:"chips"`
	}{Chips: chips})
}

func (s *Server) handleChip(_ context.Context, c *app.RequestContext) {
	chipID := c.Param("chip")
	if !validatePath(c, "chip", chipID) {
		return
	}
	values, err := s.backend.ChipValues(chipID)
	if err != nil {
		writeBackendError(c, "", err)
		return
	}
	c.JSON(http.StatusOK, values)
}

func (s *Server) handlePatchChip(_ context.Context, c *app.RequestContext) {
	chipID := c.Param("chip")
	if !validatePath(c, "chip", chipID) {
		return
	}
	var req struct {
		Field string      `json:"field"`
		Value interface{} `json:"value"`
	}
	if err := decodeJSON(c, &req); err != nil {
		writeError(c, "", http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}
	req.Field = strings.TrimSpace(req.Field)
	if req.Field == "" || len(req.Field) > 64 {
		writeBackendError(c, "", state.ErrInvalidValue)
		return
	}
	chip, err := s.backend.FindChip(chipID)
	if err != nil {
		writeBackendError(c, "", err)
		return
	}
	if _, setErr := s.backend.SetChipField(chip.BoardID, chipID, req.Field, req.Value); setErr != nil {
		writeBackendError(c, "", setErr)
		return
	}
	values, err := s.backend.ChipValues(chipID)
	if err != nil {
		writeBackendError(c, "", err)
		return
	}
	c.JSON(http.StatusOK, values)
}

func (s *Server) handleRawChip(_ context.Context, c *app.RequestContext) {
	chipID := c.Param("chip")
	if !validatePath(c, "chip", chipID) {
		return
	}
	var req struct {
		Write   []int `json:"write"`
		ReadLen int   `json:"readLen"`
	}
	if err := decodeJSON(c, &req); err != nil {
		writeError(c, "", http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}
	if len(req.Write) > 32 || req.ReadLen < 0 || req.ReadLen > 256 {
		writeBackendError(c, "", state.ErrInvalidValue)
		return
	}
	write := make([]byte, len(req.Write))
	for i, value := range req.Write {
		if value < 0 || value > 255 {
			writeBackendError(c, "", state.ErrInvalidValue)
			return
		}
		write[i] = byte(value)
	}
	chip, err := s.backend.FindChip(chipID)
	if err != nil {
		writeBackendError(c, "", err)
		return
	}
	response, err := s.backend.RawChip(chip.BoardID, chipID, bus.Transaction{
		Transport: chip.Transport,
		Bus:       chip.BusNumber,
		Address:   chip.Address,
		Write:     write,
		ReadLen:   req.ReadLen,
	})
	if err != nil {
		writeBackendError(c, "", err)
		return
	}
	c.JSON(http.StatusOK, response)
}

func (s *Server) handleBoardProfiles(_ context.Context, c *app.RequestContext) {
	c.JSON(http.StatusOK, struct {
		Profiles []boardprofile.Profile `json:"profiles"`
	}{Profiles: boardprofile.Profiles()})
}

func (s *Server) handleTopology(_ context.Context, c *app.RequestContext) {
	spec, _ := s.backend.Topology()
	c.JSON(http.StatusOK, spec)
}

func (s *Server) handleTopologySource(_ context.Context, c *app.RequestContext) {
	_, source := s.backend.Topology()
	if len(source) == 0 {
		writeError(c, "", http.StatusNotFound, "NOT_FOUND", "no topology source loaded")
		return
	}
	contentType := "application/yaml; charset=utf-8"
	if json.Valid(source) {
		contentType = "application/json; charset=utf-8"
	}
	c.Data(http.StatusOK, contentType, source)
}

func (s *Server) handleScenarios(_ context.Context, c *app.RequestContext) {
	rules := s.backend.Scenarios()
	if rules == nil {
		rules = []scenario.Rule{}
	}
	c.JSON(http.StatusOK, struct {
		Scenarios []scenario.Rule `json:"scenarios"`
	}{Scenarios: rules})
}

func (s *Server) handlePatchScenario(_ context.Context, c *app.RequestContext) {
	id := c.Param("id")
	if !validatePath(c, "scenario", id) {
		return
	}
	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if err := decodeJSON(c, &req); err != nil {
		writeError(c, "", http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}
	if req.Enabled == nil {
		writeBackendError(c, "", state.ErrInvalidValue)
		return
	}
	if err := s.backend.SetScenarioEnabled(id, *req.Enabled); err != nil {
		writeBackendError(c, "", err)
		return
	}
	for _, rule := range s.backend.Scenarios() {
		if rule.ID == id {
			c.JSON(http.StatusOK, rule)
			return
		}
	}
	writeError(c, "", http.StatusNotFound, "SCENARIO_NOT_FOUND", "scenario not found")
}

func (s *Server) handleTriggerScenario(_ context.Context, c *app.RequestContext) {
	id := c.Param("id")
	if !validatePath(c, "scenario", id) {
		return
	}
	if err := s.backend.TriggerScenario(context.Background(), id); err != nil {
		writeBackendError(c, "", err)
		return
	}
	c.JSON(http.StatusAccepted, struct {
		ID        string `json:"id"`
		Triggered bool   `json:"triggered"`
	}{ID: id, Triggered: true})
}
