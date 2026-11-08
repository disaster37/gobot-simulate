package bus

// This file contains register-accurate models for the environment sensors
// whose calibration registers have well-defined datasheet vectors. The seeded
// values are the same vectors used by gobot's own driver tests.

func registersFromBytes(start uint8, data ...byte) map[uint8]uint32 {
	out := make(map[uint8]uint32, len(data))
	reg := start
	for _, value := range data {
		out[reg] = uint32(value)
		reg++
	}
	return out
}

func seededRegisterModel(model, name string, registers map[uint8]uint32, fields ...FieldDescriptor) *RegisterModel {
	return NewRegisterModel(ChipDescriptor{
		Model:     model,
		Name:      name,
		Transport: "i2c",
		Widget:    "sensor",
		Fields:    fields,
	}, WithRegisters(registers))
}

func bmp280CommonRegisters() map[uint8]uint32 {
	regs := registersFromBytes(0x88,
		126, 109, 214, 102, 50, 0, 54, 149, 220, 213, 208, 11,
		64, 30, 166, 255, 249, 255, 172, 38, 10, 216, 189, 16,
	)
	for reg, value := range registersFromBytes(0xFA, 128, 243, 0) {
		regs[reg] = value
	}
	for reg, value := range registersFromBytes(0xF7, 77, 23, 48) {
		regs[reg] = value
	}
	return regs
}

// NewBME280Model returns a BME280 with datasheet calibration and sample data.
func NewBME280Model() *RegisterModel {
	regs := bmp280CommonRegisters()
	for reg, value := range registersFromBytes(0xA1, 75) {
		regs[reg] = value
	}
	for reg, value := range registersFromBytes(0xE1, 112, 1, 0, 19, 1, 0, 30) {
		regs[reg] = value
	}
	for reg, value := range registersFromBytes(0xFA, 129, 0, 0) {
		regs[reg] = value
	}
	for reg, value := range registersFromBytes(0xFD, 111, 83) {
		regs[reg] = value
	}
	return seededRegisterModel("bme280", "BME280 environment sensor", regs,
		FieldDescriptor{Name: "temperature", Unit: "degC", Widget: "number"},
		FieldDescriptor{Name: "humidity", Unit: "%", Widget: "number"},
		FieldDescriptor{Name: "pressure", Unit: "Pa", Widget: "number"},
	)
}

// NewBMP280Model returns a BMP280 with datasheet calibration and sample data.
func NewBMP280Model() *RegisterModel {
	return seededRegisterModel("bmp280", "BMP280 pressure sensor", bmp280CommonRegisters(),
		FieldDescriptor{Name: "temperature", Unit: "degC", Widget: "number"},
		FieldDescriptor{Name: "pressure", Unit: "Pa", Widget: "number"},
	)
}

// NewBMP180Model returns a BMP180 with the datasheet example vectors.
func NewBMP180Model() *RegisterModel {
	regs := registersFromBytes(0xAA,
		1, 0x98, // AC1 408
		0xFF, 0xB8, // AC2 -72
		0xC7, 0xD1, // AC3 -14383
		0x7F, 0xE5, // AC4 32741
		0x7F, 0xF5, // AC5 32757
		0x5A, 0x71, // AC6 23153
		0x18, 0x2E, // B1 6190
		0x00, 0x04, // B2 4
		0x80, 0x00, // MB -32768
		0xDD, 0xF9, // MC -8711
		0x0B, 0x34, // MD 2868
	)
	for reg, value := range registersFromBytes(0xF6, 0x6C, 0xFA) {
		regs[reg] = value
	}
	return seededRegisterModel("bmp180", "BMP180 pressure sensor", regs,
		FieldDescriptor{Name: "temperature", Unit: "degC", Widget: "number"},
		FieldDescriptor{Name: "pressure", Unit: "Pa", Widget: "number"},
	)
}

// NewBMP388Model returns a BMP388 with the datasheet calibration vector.
func NewBMP388Model() *RegisterModel {
	regs := registersFromBytes(0x00, 0x50)
	for reg, value := range registersFromBytes(0x31,
		36, 107, 156, 73, 246, 104, 255, 189, 245, 35, 0, 151,
		101, 184, 122, 243, 246, 211, 64, 14, 196, 0, 0, 0,
	) {
		regs[reg] = value
	}
	for reg, value := range registersFromBytes(0x07, 0, 28, 127) {
		regs[reg] = value
	}
	for reg, value := range registersFromBytes(0x04, 0, 66, 113) {
		regs[reg] = value
	}
	return seededRegisterModel("bmp388", "BMP388 pressure sensor", regs,
		FieldDescriptor{Name: "temperature", Unit: "degC", Widget: "number"},
		FieldDescriptor{Name: "pressure", Unit: "Pa", Widget: "number"},
	)
}

// NewMPL115A2Model returns an MPL115A2 with the datasheet example data.
func NewMPL115A2Model() *RegisterModel {
	regs := registersFromBytes(0x04,
		0x3E, 0xCE, // A0
		0xB3, 0xF9, // B1
		0xC4, 0xB5, // B2
		0x0C, 0xF2, // C12
	)
	for reg, value := range registersFromBytes(0x00, 0x66, 0x80, 0x7E, 0xC0) {
		regs[reg] = value
	}
	return seededRegisterModel("mpl115a2", "MPL115A2 pressure sensor", regs,
		FieldDescriptor{Name: "temperature", Unit: "degC", Widget: "number"},
		FieldDescriptor{Name: "pressure", Unit: "kPa", Widget: "number"},
	)
}

func init() {
	Register("bme280", func(ChipConfig) (ChipModel, error) { return NewBME280Model(), nil })
	Register("bmp280", func(ChipConfig) (ChipModel, error) { return NewBMP280Model(), nil })
	Register("bmp180", func(ChipConfig) (ChipModel, error) { return NewBMP180Model(), nil })
	Register("bmp388", func(ChipConfig) (ChipModel, error) { return NewBMP388Model(), nil })
	Register("mpl115a2", func(ChipConfig) (ChipModel, error) { return NewMPL115A2Model(), nil })
}
