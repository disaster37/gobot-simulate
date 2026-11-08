package bus

import (
	"fmt"
	"sort"
	"sync"
)

// RegisterModel is a generic register-map chip. It understands the common I2C
// "first write byte selects the register pointer" protocol and serves
// consecutive register bytes for reads. Model-specific chips embed it and
// override Transfer or hooks where a datasheet differs.
type RegisterModel struct {
	mu       sync.RWMutex
	desc     ChipDescriptor
	pointer  uint8
	regs     map[uint8]uint32
	valuesFn func(*RegisterModel) map[string]interface{}
	setters  map[string]func(*RegisterModel, interface{}) error
	onWrite  func(*RegisterModel, uint8, uint32)
	onRead   func(*RegisterModel, uint8) uint32
}

// RegisterOption configures a RegisterModel.
type RegisterOption func(*RegisterModel)

// WithRegisters seeds register values.
func WithRegisters(values map[uint8]uint32) RegisterOption {
	return func(m *RegisterModel) {
		for reg, value := range values {
			m.regs[reg] = value
		}
	}
}

// WithRegisterDescriptor replaces the descriptor.
func WithRegisterDescriptor(desc ChipDescriptor) RegisterOption {
	return func(m *RegisterModel) { m.desc = desc }
}

// WithValuesFunc installs derived field values.
func WithValuesFunc(fn func(*RegisterModel) map[string]interface{}) RegisterOption {
	return func(m *RegisterModel) { m.valuesFn = fn }
}

// WithSetter installs a field setter.
func WithSetter(field string, fn func(*RegisterModel, interface{}) error) RegisterOption {
	return func(m *RegisterModel) { m.setters[field] = fn }
}

// WithWriteHook installs a register write hook.
func WithWriteHook(fn func(*RegisterModel, uint8, uint32)) RegisterOption {
	return func(m *RegisterModel) { m.onWrite = fn }
}

// WithReadHook installs a register read hook.
func WithReadHook(fn func(*RegisterModel, uint8) uint32) RegisterOption {
	return func(m *RegisterModel) { m.onRead = fn }
}

// NewRegisterModel creates a generic register chip.
func NewRegisterModel(desc ChipDescriptor, opts ...RegisterOption) *RegisterModel {
	m := &RegisterModel{
		desc:    desc,
		regs:    make(map[uint8]uint32),
		setters: make(map[string]func(*RegisterModel, interface{}) error),
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// Describe implements ChipModel.
func (m *RegisterModel) Describe() ChipDescriptor { return m.desc }

// Pointer returns the current register pointer.
func (m *RegisterModel) Pointer() uint8 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.pointer
}

// Register reads one register value.
func (m *RegisterModel) Register(reg uint8) uint32 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value := m.regs[reg]
	if m.onRead != nil {
		value = m.onRead(m, reg)
	}
	return value
}

// SetRegister writes one register value.
func (m *RegisterModel) SetRegister(reg uint8, value uint32) {
	m.mu.Lock()
	m.regs[reg] = value
	hook := m.onWrite
	m.mu.Unlock()
	if hook != nil {
		hook(m, reg, value)
	}
}

// Transfer implements ChipModel.
func (m *RegisterModel) Transfer(txn Transaction) (Response, error) {
	if txn.Transport == "spi" {
		return m.transferSPI(txn)
	}
	m.mu.Lock()
	if len(txn.Write) > 0 {
		m.pointer = txn.Write[0]
		reg := txn.Write[0]
		for _, b := range txn.Write[1:] {
			m.writeLocked(reg, uint32(b))
			reg++
		}
	}
	data := make([]byte, 0, txn.ReadLen)
	reg := m.pointer
	for i := 0; i < txn.ReadLen; i++ {
		value := m.readLocked(reg)
		data = append(data, uint8(value)) //nolint:gosec // intentional width conversion
		reg++
	}
	m.pointer = reg
	m.mu.Unlock()
	return Response{Data: data, Values: m.Values()}, nil
}

func (m *RegisterModel) transferSPI(txn Transaction) (Response, error) {
	data := make([]byte, len(txn.Write))
	for i, tx := range txn.Write {
		if i == 0 {
			m.mu.Lock()
			m.pointer = tx
			m.mu.Unlock()
			continue
		}
		m.mu.Lock()
		m.writeLocked(m.pointer, uint32(tx))
		m.pointer++
		value := m.readLocked(m.pointer)
		m.mu.Unlock()
		data[i] = uint8(value) //nolint:gosec // intentional width conversion
	}
	return Response{Data: data, Values: m.Values()}, nil
}

func (m *RegisterModel) writeLocked(reg uint8, value uint32) {
	m.regs[reg] = value
	if m.onWrite != nil {
		m.onWrite(m, reg, value)
	}
}

func (m *RegisterModel) readLocked(reg uint8) uint32 {
	if m.onRead != nil {
		return m.onRead(m, reg)
	}
	return m.regs[reg]
}

// Set implements ChipModel.
func (m *RegisterModel) Set(field string, value interface{}) error {
	m.mu.Lock()
	setter, ok := m.setters[field]
	m.mu.Unlock()
	if ok {
		return setter(m, value)
	}
	return fmt.Errorf("%w: %s has no settable field %q", ErrChipNotFound, m.desc.Model, field)
}

// Values implements ValuesProvider.
func (m *RegisterModel) Values() map[string]interface{} {
	m.mu.RLock()
	values := make(map[string]interface{}, len(m.regs)+1)
	keys := make([]int, 0, len(m.regs))
	for reg := range m.regs {
		keys = append(keys, int(reg))
	}
	sort.Ints(keys)
	for _, key := range keys {
		values[fmt.Sprintf("reg%d", key)] = m.regs[uint8(key)] //nolint:gosec // intentional width conversion
	}
	values["pointer"] = int(m.pointer)
	fn := m.valuesFn
	m.mu.RUnlock()
	if fn != nil {
		for key, value := range fn(m) {
			values[key] = value
		}
	}
	return values
}

// GenericConfig returns a descriptor and options for the generic fallback.
func GenericConfig(options map[string]interface{}) (ChipDescriptor, []RegisterOption) {
	desc := ChipDescriptor{
		Model:     "generic",
		Name:      "Generic register-map chip",
		Transport: "i2c",
		Widget:    "register-map",
	}
	opts := make([]RegisterOption, 0, 2)
	if defaults, ok := options["defaults"].(map[string]interface{}); ok {
		regs := make(map[uint8]uint32, len(defaults))
		for key, value := range defaults {
			var reg int64
			if _, err := fmt.Sscanf(key, "%x", &reg); err != nil {
				if _, err := fmt.Sscanf(key, "%d", &reg); err != nil {
					continue
				}
			}
			regs[uint8(reg)] = uint32(intValue(value)) //nolint:gosec // intentional width conversion
		}
		opts = append(opts, WithRegisters(regs))
	}
	return desc, opts
}

func init() {
	Register("generic", func(cfg ChipConfig) (ChipModel, error) {
		desc, opts := GenericConfig(cfg.Options)
		if cfg.Address != 0 {
			desc.Address = cfg.Address
		}
		return NewRegisterModel(desc, opts...), nil
	})
	Register("register-map", func(cfg ChipConfig) (ChipModel, error) {
		desc, opts := GenericConfig(cfg.Options)
		desc.Model = "register-map"
		return NewRegisterModel(desc, opts...), nil
	})
}
