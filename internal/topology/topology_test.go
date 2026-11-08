package topology

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const validTopology = `schema: "1"
boards:
  - id: dfp
    name: DFP
    profile: raspi
    latency: 5ms
    pins:
      - pin: "3"
        label: drum
        mode: output
      - pin: "11"
        mode: input
        pull: up
    devices:
      - id: relay_drum
        kind: relay
        pin: "3"
        wiring:
          drive: active-high
          contact: no
          loadLabel: drum
      - id: button_start
        kind: button
        pin: "11"
        defaultState: 1
    chips:
      - id: oled
        bus: i2c-1
        address: 60
        model: ssd1306
    values:
      distance: 12
    functions:
      ack:
        returnCode: 2
scenarios:
  - id: start
    trigger:
      type: manual
    actions:
      - type: emitStep
        name: running
`

func TestParseValidTopology(t *testing.T) {
	loaded, err := Load(strings.NewReader(validTopology))
	require.NoError(t, err)
	require.Len(t, loaded.Spec.Boards, 1)
	board := loaded.Spec.Boards[0]
	assert.Equal(t, "dfp", board.ID)
	assert.Equal(t, "raspi", board.Profile)
	assert.Len(t, board.Pins, 2)
	assert.Len(t, board.Devices, 2)
	assert.Equal(t, 60, board.Chips[0].Address)
	assert.Equal(t, 12, board.Values["distance"])
	assert.Equal(t, 2, board.Functions["ack"].ReturnCode)
	require.Len(t, loaded.Spec.Scenarios, 1)
	assert.NotEmpty(t, loaded.Source)
}

func TestParseDefaultsSchema(t *testing.T) {
	loaded, err := Load(strings.NewReader(`boards:
  - id: b
`))
	require.NoError(t, err)
	assert.Equal(t, SchemaVersion, loaded.Spec.Schema)
}

func TestValidationErrorsArePathQualified(t *testing.T) {
	_, err := Load(strings.NewReader(`schema: "1"
boards:
  - id: dfp
    pins:
      - pin: "11"
        mode: bogus
    devices:
      - id: relay
        kind: relay
        pin: "3"
        wiring:
          contact: nf
`))
	require.Error(t, err)
	message := err.Error()
	assert.Contains(t, message, "boards[0].pins[0].mode: unknown value")
	assert.Contains(t, message, "boards[0].devices[0].wiring.contact: unknown value")
}

func TestValidationRejectsUnknownFields(t *testing.T) {
	_, err := Load(strings.NewReader(`schema: "1"
boards:
  - id: dfp
    mystery: true
`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "field mystery not found")
}

func TestValidationRejectsDuplicatesAndMissingIdentifiers(t *testing.T) {
	_, err := Load(strings.NewReader(`schema: "1"
boards:
  - id: dfp
    devices:
      - id: relay
        kind: relay
      - id: relay
        kind: relay
`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate device")
}

func TestValidationRequiresValueAndFunctionNames(t *testing.T) {
	_, err := Load(strings.NewReader(`schema: "1"
boards:
  - id: dfp
    devices:
      - id: v
        kind: value
      - id: f
        kind: function
`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "valueName: required")
	assert.Contains(t, err.Error(), "functionName: required")
}

func TestValidationRejectsUnknownScenarioAndChip(t *testing.T) {
	_, err := Load(strings.NewReader(`schema: "1"
boards:
  - id: dfp
    chips:
      - id: c
        model: ssd1306
scenarios:
  - id: rule
    trigger:
      type: teleport
    actions:
      - type: emitStep
`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boards[0].chips[0].bus: required")
	assert.Contains(t, err.Error(), "scenarios[0].trigger.type: unknown value")
}

func TestSchemaVersionRejected(t *testing.T) {
	_, err := Load(strings.NewReader(`schema: "2"
boards:
  - id: dfp
`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported version")
}

func TestLoadEmptyDocument(t *testing.T) {
	_, err := Load(strings.NewReader(""))
	require.Error(t, err)
}

func TestLoadBoundsSize(t *testing.T) {
	large := strings.Repeat("x", MaxTopologySize+1)
	_, err := Load(strings.NewReader(large))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds")
}

func TestLoadFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "topology.yaml")
	require.NoError(t, os.WriteFile(path, []byte(validTopology), 0o600))
	loaded, err := LoadFile(path)
	require.NoError(t, err)
	assert.Equal(t, path, loaded.Path)
	assert.Equal(t, "dfp", loaded.Spec.Boards[0].ID)

	_, err = LoadFile(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is a directory")

	_, err = LoadFile(filepath.Join(dir, "missing.yaml"))
	require.Error(t, err)
}

func TestDevicePinsDeduplicates(t *testing.T) {
	device := Device{Pin: "3", Pins: []string{"3", "4"}}
	assert.Equal(t, []string{"3", "4"}, devicePins(device))
}
