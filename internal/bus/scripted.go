package bus

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"sync"
)

// ScriptedModel is a chip model whose responses are configured by command
// bytes. It covers command-oriented sensors (SHT3x/SHT2x/...) without
// hard-coding every datasheet formula in the simulator, while still speaking
// the real driver protocol.
type ScriptedModel struct {
	mu        sync.RWMutex
	desc      ChipDescriptor
	regs      map[uint8]uint32
	pointer   uint8
	responses map[string][]byte
	last      string
}

// NewScriptedModel creates a command-scripted model.
func NewScriptedModel(desc ChipDescriptor, options map[string]interface{}) *ScriptedModel {
	model := &ScriptedModel{
		desc:      desc,
		regs:      make(map[uint8]uint32),
		responses: parseResponses(options),
	}
	if defaults, ok := options["defaults"].(map[string]interface{}); ok {
		for key, value := range defaults {
			parsed, err := strconv.ParseUint(strings.TrimPrefix(key, "0x"), 16, 8)
			if err != nil {
				continue
			}
			model.regs[uint8(parsed)] = uint32(intValue(value)) //nolint:gosec // intentional width conversion
		}
	}
	return model
}

func parseResponses(options map[string]interface{}) map[string][]byte {
	out := make(map[string][]byte)
	raw, ok := options["responses"].(map[string]interface{})
	if !ok {
		return out
	}
	for command, value := range raw {
		key := strings.ToLower(strings.ReplaceAll(strings.TrimPrefix(command, "0x"), " ", ""))
		switch data := value.(type) {
		case []interface{}:
			bytes := make([]byte, 0, len(data))
			for _, item := range data {
				bytes = append(bytes, byte(intValue(item))) //nolint:gosec // intentional width conversion
			}
			out[key] = bytes
		case string:
			decoded, err := hex.DecodeString(strings.ReplaceAll(data, " ", ""))
			if err == nil {
				out[key] = decoded
			}
		}
	}
	return out
}

// Describe implements ChipModel.
func (m *ScriptedModel) Describe() ChipDescriptor { return m.desc }

// Transfer implements ChipModel.
func (m *ScriptedModel) Transfer(txn Transaction) (Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	previous := m.last
	if len(txn.Write) > 0 {
		m.pointer = txn.Write[0]
		for _, value := range txn.Write[1:] {
			m.regs[m.pointer] = uint32(value)
			m.pointer++
		}
		// A read transaction also carries a register pointer; keep the last
		// command so command-keyed responses still match.
		if txn.ReadLen == 0 {
			m.last = hex.EncodeToString(txn.Write)
		}
	}
	data := make([]byte, 0, txn.ReadLen)
	if response, ok := m.responses[previous]; ok && txn.ReadLen > 0 {
		data = append(data, response...)
		if len(data) > txn.ReadLen {
			data = data[:txn.ReadLen]
		}
		for len(data) < txn.ReadLen {
			data = append(data, 0)
		}
	} else {
		reg := m.pointer
		for i := 0; i < txn.ReadLen; i++ {
			data = append(data, uint8(m.regs[reg])) //nolint:gosec // intentional width conversion
			reg++
		}
		m.pointer = reg
	}
	return Response{Data: data, Values: m.valuesLocked()}, nil
}

// Set implements ChipModel.
func (m *ScriptedModel) Set(field string, value interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	switch {
	case field == "command" || field == "lastCommand":
		m.last = fmt.Sprint(value)
		return nil
	case strings.HasPrefix(field, "response:"):
		command := strings.ToLower(strings.TrimPrefix(field, "response:"))
		switch data := value.(type) {
		case []interface{}:
			bytes := make([]byte, 0, len(data))
			for _, item := range data {
				bytes = append(bytes, byte(intValue(item))) //nolint:gosec // intentional width conversion
			}
			m.responses[command] = bytes
			return nil
		case string:
			decoded, err := hex.DecodeString(strings.ReplaceAll(data, " ", ""))
			if err != nil {
				return err
			}
			m.responses[command] = decoded
			return nil
		}
	}
	return fmt.Errorf("simulate: scripted model has no settable field %q", field)
}

// Values implements ValuesProvider.
func (m *ScriptedModel) Values() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.valuesLocked()
}

func (m *ScriptedModel) valuesLocked() map[string]interface{} {
	out := make(map[string]interface{}, len(m.regs)+2)
	for reg, value := range m.regs {
		out[fmt.Sprintf("reg%d", reg)] = value
	}
	out["lastCommand"] = m.last
	out["pointer"] = int(m.pointer)
	return out
}

func init() {
	Register("scripted", func(cfg ChipConfig) (ChipModel, error) {
		desc := ChipDescriptor{
			Model:     "scripted",
			Name:      "Command-scripted chip",
			Transport: cfg.Transport,
			Address:   cfg.Address,
			Widget:    "register-map",
		}
		if cfg.Transport == "" {
			desc.Transport = "i2c"
		}
		return NewScriptedModel(desc, cfg.Options), nil
	})
}
