package conformance

import (
	"fmt"
	"strings"
	"time"

	simulate "github.com/disaster37/gobot-simulate"
	"github.com/disaster37/gobot-simulate/internal/bus"
	"github.com/disaster37/gobot-simulate/internal/topology"
	"github.com/sigurn/crc8"
	"gobot.io/x/gobot/v2/drivers/aio"
	"gobot.io/x/gobot/v2/drivers/ble"
	bleMicrobit "gobot.io/x/gobot/v2/drivers/ble/microbit"
	"gobot.io/x/gobot/v2/drivers/gpio"
	"gobot.io/x/gobot/v2/drivers/i2c"
	"gobot.io/x/gobot/v2/drivers/onewire"
	gobotSPI "gobot.io/x/gobot/v2/drivers/spi"
	gobotFirmata "gobot.io/x/gobot/v2/platforms/firmata"
)

var cases = []Case{
	{Model: "gpio-relay", Driver: "*gpio.RelayDriver", Summary: "On/Off/Toggle, pin state and welded-contact fault", Run: caseRelay},
	{Model: "gpio-led", Driver: "*gpio.LedDriver", Summary: "On/Off/Toggle and brightness state", Run: caseLED},
	{Model: "gpio-button", Driver: "*gpio.ButtonDriver", Summary: "polled press/release and default state", Run: caseButton},
	{Model: "gpio-servo", Driver: "*gpio.ServoDriver", Summary: "Move writes the servo angle", Run: caseServo},
	{Model: "gpio-motor", Driver: "*gpio.MotorDriver", Summary: "digital and analog speed control", Run: caseMotor},
	{Model: "gpio-direct-pin", Driver: "*gpio.DirectPinDriver", Summary: "raw digital read/write", Run: caseDirectPin},
	{Model: "gpio-buzzer", Driver: "*gpio.BuzzerDriver", Summary: "On/Off and state", Run: caseBuzzer},
	{Model: "gpio-rgb-led", Driver: "*gpio.RgbLedDriver", Summary: "SetRGB writes three PWM pins", Run: caseRGBLED},
	{Model: "gpio-pir", Driver: "*gpio.PIRMotionDriver", Summary: "motion pin read", Run: casePIR},
	{Model: "gpio-stepper", Driver: "*gpio.StepperDriver", Summary: "step and direction sequences", Run: caseStepper},
	{Model: "gpio-easy-driver", Driver: "*gpio.EasyDriver", Summary: "enable, disable and sleep", Run: caseEasyDriver},
	{Model: "gpio-hcsr04", Driver: "*gpio.HCSR04Driver", Summary: "trigger/echo pulse distance", Run: caseHCSR04},
	{Model: "gpio-grove", Driver: "*gpio.GroveButton/Led/Relay/Buzzer", Summary: "Grove wrappers drive the same pins", Run: caseGroveGPIO},
	{Model: "aio-analog-actuator", Driver: "*aio.AnalogActuatorDriver", Summary: "Write/WriteRaw round-trip", Run: caseAnalogActuator},
	{Model: "aio-temperature", Driver: "*aio.TemperatureSensorDriver", Summary: "raw and scaled temperature read", Run: caseTemperature},
	{Model: "aio-grove", Driver: "*aio.GroveRotary/Light/Sound/Piezo", Summary: "Grove analog sensors read the shared pin", Run: caseGroveAIO},
	{Model: "bme280", Driver: "*i2c.BME280Driver", Summary: "calibrated temperature, pressure and humidity", Run: caseBME280},
	{Model: "bmp280", Driver: "*i2c.BMP280Driver", Summary: "calibrated temperature, pressure and altitude", Run: caseBMP280},
	{Model: "bmp180", Driver: "*i2c.BMP180Driver", Summary: "datasheet example temperature and pressure", Run: caseBMP180},
	{Model: "bmp388", Driver: "*i2c.BMP388Driver", Summary: "calibrated temperature and pressure", Run: caseBMP388},
	{Model: "mpl115a2", Driver: "*i2c.MPL115A2Driver", Summary: "coefficient and conversion read", Run: caseMPL115A2},
	{Model: "sht3x", Driver: "*i2c.SHT3xDriver", Summary: "command/CRC sample response", Run: caseSHT3x},
	{Model: "sht2x", Driver: "*i2c.SHT2xDriver", Summary: "command/CRC temperature and humidity", Run: caseSHT2x},
	{Model: "pcf8591", Driver: "*i2c.PCF8591Driver", Summary: "analog channel read and DAC write", Run: casePCF8591},
	{Model: "pca953x", Driver: "*i2c.PCA953xDriver", Summary: "GPIO and PWM register round-trip", Run: casePCA953x},
	{Model: "pca9501", Driver: "*i2c.PCA9501Driver", Summary: "GPIO and EEPROM round-trip", Run: casePCA9501},
	{Model: "ina3221", Driver: "*i2c.INA3221Driver", Summary: "bus/shunt voltage and current", Run: caseINA3221},
	{Model: "adxl345", Driver: "*i2c.ADXL345Driver", Summary: "acceleration registers", Run: caseADXL345},
	{Model: "l3gd20h", Driver: "*i2c.L3GD20HDriver", Summary: "gyroscope registers", Run: caseL3GD20H},
	{Model: "mma7660", Driver: "*i2c.MMA7660Driver", Summary: "accelerometer registers", Run: caseMMA7660},
	{Model: "mpu6050", Driver: "*i2c.MPU6050Driver", Summary: "acceleration and gyro data", Run: caseMPU6050},
	{Model: "hmc5883l", Driver: "*i2c.HMC5883LDriver", Summary: "magnetometer registers", Run: caseHMC5883L},
	{Model: "hmc6352", Driver: "*i2c.HMC6352Driver", Summary: "heading command response", Run: caseHMC6352},
	{Model: "drv2605l", Driver: "*i2c.DRV2605LDriver", Summary: "haptic mode and waveform sequence", Run: caseDRV2605L},
	{Model: "lidarlite", Driver: "*i2c.LIDARLiteDriver", Summary: "distance command response", Run: caseLIDARLite},
	{Model: "tsl2561", Driver: "*i2c.TSL2561Driver", Summary: "broadband and IR channel read", Run: caseTSL2561},
	{Model: "jhd1313m1", Driver: "*i2c.JHD1313M1Driver", Summary: "character buffer and RGB backlight", Run: caseJHD1313M1},
	{Model: "ssd1306-spi", Driver: "*spi.SSD1306Driver", Summary: "SPI command stream and framebuffer", Run: caseSSD1306SPI},
	{Model: "ble-microbit", Driver: "*ble/microbit.LED/IOPin/Button/Temperature", Summary: "GATT LED matrix, pin config and notifications", Run: caseMicrobit},
	{Model: "aio-analog-sensor", Driver: "*aio.AnalogSensorDriver", Summary: "analog value round-trip and scaling", Run: caseAnalogSensor},
	{Model: "ds18b20", Driver: "*onewire.DS18B20Driver", Summary: "temperature, resolution and conversion delay", Run: caseDS18B20},
	{Model: "generic", Driver: "*i2c.Driver", Summary: "raw register read/write round-trip", Run: caseGenericI2C},
	{Model: "mcp23017", Driver: "*i2c.MCP23017Driver", Summary: "GPA/GPB direction and IO round-trip", Run: caseMCP23017},
	{Model: "pca9685", Driver: "*i2c.PCA9685Driver", Summary: "PWM channel and frequency registers", Run: casePCA9685},
	{Model: "ads1115", Driver: "*i2c.ADS1115Driver", Summary: "single-ended conversion result", Run: caseADS1115},
	{Model: "bh1750", Driver: "*i2c.BH1750Driver", Summary: "lux measurement", Run: caseBH1750},
	{Model: "ssd1306", Driver: "*i2c.SSD1306Driver", Summary: "command stream and framebuffer", Run: caseSSD1306},
	{Model: "ble-generic", Driver: "*ble.BatteryDriver/*ble.DeviceInformationDriver", Summary: "GATT reads for battery and device information services", Run: caseBLE},
	{Model: "firmata", Driver: "*firmata.Adaptor", Summary: "protocol handshake, digital and analog IO", Run: caseFirmata},
	{Model: "arest-http", Driver: "*arest.NewHTTPAdaptor", Summary: "HTTP digital, mode, value and function contract", Run: caseARESTHTTP},
	{Model: "arest-serial", Driver: "*arest.NewSerialAdaptor", Summary: "serial line protocol digital, value and function contract", Run: caseARESTSerial},
	{Model: "particle", Driver: "*particle.Adaptor", Summary: "cloud REST digital and analog emulation", Run: caseParticle},
}

var unsupported = []UnsupportedCase{
	{Driver: "aio.ThermalZoneDriver", Rationale: "reads host sysfs thermal zones; host-dependent by design"},
	{Driver: "gpio.HD44780", Rationale: "4/8-bit parallel display timing requires a pin-level protocol decoder; I2C JHD1313M1 and SSD1306 displays are supported"},
	{Driver: "gpio.MAX7219/TM1638/AIP1640", Rationale: "bit-banged shift-register timing requires a pin-level protocol decoder; the generic display buffer remains available"},
	{Driver: "i2c./spi.MFRC522", Rationale: "RFID tag/anticollision state machine not modelled; generic register-map fallback available"},
	{Driver: "i2c.CCS811", Rationale: "firmware-mode application start and gas-baseline calibration state machine not modelled; generic register-map fallback available"},
	{Driver: "i2c.GrovePi", Rationale: "GrovePi is an ATmega firmware protocol with per-command framing; generic register-map fallback available"},
	{Driver: "i2c.YL40", Rationale: "capacitive soil-moisture conversion is not characterised by gobot; generic register-map fallback available"},
	{Driver: "i2c.BlinkM", Rationale: "ASCII command protocol not modelled; generic register-map fallback available"},
	{Driver: "i2c.Adafruit1109", Rationale: "two MCP23017 expanders plus keypad scanning not modelled; use the JHD1313M1 display model"},
	{Driver: "i2c.Wiichuck", Rationale: "encrypted controller handshake not modelled; generic register-map fallback available"},
	{Driver: "i2c.PCF8583", Rationale: "clock/counter calendar semantics are host-time dependent; generic register-map fallback available"},
}

func caseRelay(env *Env) error {
	if _, err := env.Sim.RegisterDevice(env.Board, simulate.DeviceSpec{
		ID: "relay", Name: "relay", Kind: simulate.DeviceKindRelay, Pin: "3",
	}); err != nil {
		return err
	}
	driver := gpio.NewRelayDriver(env.Adaptor, "3")
	if err := driver.Start(); err != nil {
		return err
	}
	if err := env.Sim.AttachDevices(env.Board, driver); err != nil {
		return err
	}
	if err := driver.On(); err != nil {
		return err
	}
	if !driver.State() {
		return fmt.Errorf("relay State() is false after On")
	}
	pin, err := env.Sim.Pin(env.Board, "3")
	if err != nil {
		return err
	}
	if pin.DigitalValue != 1 {
		return fmt.Errorf("relay pin = %d, want 1", pin.DigitalValue)
	}
	if _, faultErr := env.Sim.SetDeviceFault(env.Board, "relay", "welded-contact"); faultErr != nil {
		return err
	}
	read, err := env.Sim.DigitalRead(env.Board, "3")
	if err != nil {
		return err
	}
	if read != 0 {
		return fmt.Errorf("welded contact read = %d, want 0 (disagrees with On)", read)
	}
	if _, err := env.Sim.ClearDeviceFaults(env.Board, "relay"); err != nil {
		return err
	}
	if err := driver.Off(); err != nil {
		return err
	}
	if driver.State() {
		return fmt.Errorf("relay State() is true after Off")
	}
	return nil
}

func caseLED(env *Env) error {
	driver := gpio.NewLedDriver(env.Adaptor, "7")
	if err := driver.Start(); err != nil {
		return err
	}
	if err := driver.On(); err != nil {
		return err
	}
	if !driver.State() {
		return fmt.Errorf("led State() false after On")
	}
	if err := driver.Toggle(); err != nil {
		return err
	}
	if driver.State() {
		return fmt.Errorf("led State() true after Toggle from On")
	}
	return nil
}

func caseButton(env *Env) error {
	if _, err := env.Sim.RegisterDevice(env.Board, simulate.DeviceSpec{
		ID: "button", Kind: simulate.DeviceKindButton, Pin: "11", DefaultState: simulate.IntPtr(1),
	}); err != nil {
		return err
	}
	driver := gpio.NewButtonDriver(env.Adaptor, "11", gpio.WithButtonDefaultState(1), gpio.WithButtonPollInterval(5*time.Millisecond))
	if err := driver.Start(); err != nil {
		return err
	}
	defer func() { _ = driver.Halt() }()
	if err := env.Sim.PressButton(env.Board, "button"); err != nil {
		return err
	}
	deadline := time.Now().Add(time.Second)
	for !driver.Active() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if !driver.Active() {
		return fmt.Errorf("button did not become active after press")
	}
	if err := env.Sim.ReleaseButton(env.Board, "button"); err != nil {
		return err
	}
	deadline = time.Now().Add(time.Second)
	for driver.Active() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if driver.Active() {
		return fmt.Errorf("button remained active after release")
	}
	return nil
}

func caseServo(env *Env) error {
	driver := gpio.NewServoDriver(env.Adaptor, "9")
	if err := driver.Start(); err != nil {
		return err
	}
	if err := driver.Move(90); err != nil {
		return err
	}
	if driver.Angle() != 90 {
		return fmt.Errorf("servo angle = %d, want 90", driver.Angle())
	}
	pin, err := env.Sim.Pin(env.Board, "9")
	if err != nil {
		return err
	}
	if pin.ByteValue != 90 {
		return fmt.Errorf("servo pin byte = %d, want 90", pin.ByteValue)
	}
	return nil
}

func caseMotor(env *Env) error {
	driver := gpio.NewMotorDriver(env.Adaptor, "5")
	if err := driver.Start(); err != nil {
		return err
	}
	if err := driver.SetSpeed(128); err != nil {
		return err
	}
	if driver.Speed() != 128 {
		return fmt.Errorf("motor speed = %d, want 128", driver.Speed())
	}
	return nil
}

func caseDirectPin(env *Env) error {
	driver := gpio.NewDirectPinDriver(env.Adaptor, "6")
	if err := driver.Start(); err != nil {
		return err
	}
	if err := driver.DigitalWrite(1); err != nil {
		return err
	}
	value, err := driver.DigitalRead()
	if err != nil {
		return err
	}
	if value != 1 {
		return fmt.Errorf("direct pin read = %d, want 1", value)
	}
	return nil
}

func caseBuzzer(env *Env) error {
	driver := gpio.NewBuzzerDriver(env.Adaptor, "8")
	if err := driver.Start(); err != nil {
		return err
	}
	if err := driver.On(); err != nil {
		return err
	}
	if !driver.State() {
		return fmt.Errorf("buzzer State() false after On")
	}
	if err := driver.Off(); err != nil {
		return err
	}
	if driver.State() {
		return fmt.Errorf("buzzer State() true after Off")
	}
	return nil
}

func caseAnalogSensor(env *Env) error {
	if err := env.Sim.BindPin(env.Board, "A0", "level", simulate.PinModeAnalog); err != nil {
		return err
	}
	if err := env.Sim.SetAnalog(env.Board, "A0", 512, simulate.SourceSimulation); err != nil {
		return err
	}
	driver := aio.NewAnalogSensorDriver(env.Adaptor, "A0")
	if err := driver.Start(); err != nil {
		return err
	}
	raw, err := driver.ReadRaw()
	if err != nil {
		return err
	}
	if raw != 512 {
		return fmt.Errorf("analog raw = %d, want 512", raw)
	}
	return nil
}

func caseDS18B20(env *Env) error {
	driver := onewire.NewDS18B20Driver(env.Adaptor, 1)
	if err := driver.Start(); err != nil {
		return err
	}
	sensor := env.Adaptor.BusManager().OneWire().DS18B20(1)
	if err := sensor.Set("temperature", 21.5); err != nil {
		return err
	}
	value, err := driver.Temperature()
	if err != nil {
		return err
	}
	if value < 21.4 || value > 21.6 {
		return fmt.Errorf("temperature = %v, want 21.5", value)
	}
	resolution, err := driver.Resolution()
	if err != nil {
		return err
	}
	if resolution != 12 {
		return fmt.Errorf("resolution = %d, want 12", resolution)
	}
	return nil
}

func caseGenericI2C(env *Env) error {
	driver := i2c.NewDriver(env.Adaptor, "generic", 0x10)
	if err := driver.Start(); err != nil {
		return err
	}
	if err := driver.Write("1", 0x42); err != nil {
		return err
	}
	value, err := driver.Read("1")
	if err != nil {
		return err
	}
	if value != 0x42 {
		return fmt.Errorf("generic register = 0x%02x, want 0x42", value)
	}
	return nil
}

func caseMCP23017(env *Env) error {
	driver := i2c.NewMCP23017Driver(env.Adaptor)
	if err := driver.Start(); err != nil {
		return err
	}
	if err := driver.SetPinMode(0, "A", 0); err != nil {
		return err
	}
	if err := driver.WriteGPIO(0, "A", 1); err != nil {
		return err
	}
	value, err := driver.ReadGPIO(0, "A")
	if err != nil {
		return err
	}
	if value != 1 {
		return fmt.Errorf("mcp23017 GPA0 = %d, want 1", value)
	}
	return nil
}

func casePCA9685(env *Env) error {
	driver := i2c.NewPCA9685Driver(env.Adaptor)
	if err := driver.Start(); err != nil {
		return err
	}
	if err := driver.SetPWMFreq(50); err != nil {
		return err
	}
	if err := driver.SetPWM(0, 0, 2048); err != nil {
		return err
	}
	values, err := env.Sim.ChipValues("pca")
	if err != nil {
		return err
	}
	raw, ok := values["values"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("pca values missing")
	}
	if got, ok := raw["channel0"].(int); !ok || got != 2048 {
		return fmt.Errorf("pca channel0 = %v, want 2048", raw["channel0"])
	}
	return nil
}

func caseADS1115(env *Env) error {
	driver := i2c.NewADS1115Driver(env.Adaptor)
	if err := driver.Start(); err != nil {
		return err
	}
	voltage, err := driver.ReadWithDefaults(0)
	if err != nil {
		return err
	}
	if voltage < 0.5 || voltage > 1.5 {
		return fmt.Errorf("ads1115 voltage = %v, want ~1.0", voltage)
	}
	return nil
}

func caseBH1750(env *Env) error {
	driver := i2c.NewBH1750Driver(env.Adaptor)
	if err := driver.Start(); err != nil {
		return err
	}
	chip, err := env.Adaptor.BusManager().ChipModel("bh")
	if err != nil {
		return err
	}
	if setErr := chip.Set("lux", 500.0); setErr != nil {
		return err
	}
	lux, err := driver.Lux()
	if err != nil {
		return err
	}
	if lux < 400 || lux > 600 {
		return fmt.Errorf("lux = %d, want ~500", lux)
	}
	return nil
}

func caseSSD1306(env *Env) error {
	driver := i2c.NewSSD1306Driver(env.Adaptor)
	if err := driver.Start(); err != nil {
		return err
	}
	driver.Clear()
	for x := 0; x < 32; x++ {
		driver.Set(x, 0, 1)
	}
	if err := driver.Display(); err != nil {
		return err
	}
	model, err := env.Adaptor.BusManager().ChipModel("oled")
	if err != nil {
		return err
	}
	display, ok := model.(bus.Display)
	if !ok {
		return fmt.Errorf("ssd1306 has no display snapshot")
	}
	snapshot := display.Snapshot()
	found := false
	for _, b := range snapshot.Pixels {
		if b != 0 {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("ssd1306 framebuffer is empty after Display()")
	}
	return nil
}

func caseBLE(env *Env) error {
	battery := ble.NewBatteryDriver(env.Adaptor)
	if err := battery.Start(); err != nil {
		return err
	}
	level, err := battery.GetBatteryLevel()
	if err != nil {
		return err
	}
	if level != 100 {
		return fmt.Errorf("battery level = %d, want 100", level)
	}
	info := ble.NewDeviceInformationDriver(env.Adaptor)
	if startErr := info.Start(); startErr != nil {
		return startErr
	}
	model, err := info.GetModelNumber()
	if err != nil {
		return err
	}
	if model == "" {
		return fmt.Errorf("empty BLE model number")
	}
	access := ble.NewGenericAccessDriver(env.Adaptor)
	if startErr := access.Start(); startErr != nil {
		return startErr
	}
	name, err := access.GetDeviceName()
	if err != nil {
		return err
	}
	if name == "" {
		return fmt.Errorf("empty BLE device name")
	}
	return nil
}

func caseFirmata(env *Env) error {
	pipe, err := env.Sim.FirmataPipe(env.Board)
	if err != nil {
		return err
	}
	adaptor := gobotFirmata.NewAdaptor(pipe)
	if connectErr := adaptor.Connect(); connectErr != nil {
		return connectErr
	}
	defer func() { _ = adaptor.Finalize() }()
	if analogErr := env.Sim.SetAnalog(env.Board, "14", 512, simulate.SourceSimulation); analogErr != nil {
		return analogErr
	}
	if writeErr := adaptor.DigitalWrite("13", 1); writeErr != nil {
		return writeErr
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		pin, pinErr := env.Sim.Pin(env.Board, "13")
		if pinErr == nil && pin.DigitalValue == 1 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	pin, err := env.Sim.Pin(env.Board, "13")
	if err != nil {
		return err
	}
	if pin.DigitalValue != 1 {
		return fmt.Errorf("firmata digital pin = %d, want 1", pin.DigitalValue)
	}
	value, err := adaptor.AnalogRead("0")
	if err != nil {
		return err
	}
	if value != 512 {
		return fmt.Errorf("firmata analog read = %d, want 512", value)
	}
	return nil
}

func caseRGBLED(env *Env) error {
	driver := gpio.NewRgbLedDriver(env.Adaptor, "1", "2", "4")
	if err := driver.Start(); err != nil {
		return err
	}
	if err := driver.SetRGB(10, 20, 30); err != nil {
		return err
	}
	for pin, want := range map[string]byte{"1": 10, "2": 20, "4": 30} {
		value, err := env.Sim.Pin(env.Board, pin)
		if err != nil {
			return err
		}
		if value.ByteValue != want {
			return fmt.Errorf("rgb pin %s = %d, want %d", pin, value.ByteValue, want)
		}
	}
	return nil
}

func casePIR(env *Env) error {
	if _, err := env.Sim.RegisterDevice(env.Board, simulate.DeviceSpec{
		ID: "pir", Kind: simulate.DeviceKindPIR, Pin: "17",
	}); err != nil {
		return err
	}
	driver := gpio.NewPIRMotionDriver(env.Adaptor, "17")
	if err := driver.Start(); err != nil {
		return err
	}
	if err := env.Sim.SetDigital(env.Board, "17", 1, simulate.SourceSimulation); err != nil {
		return err
	}
	deadline := time.Now().Add(time.Second)
	for !driver.Active() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !driver.Active() {
		return fmt.Errorf("pir did not become active")
	}
	return nil
}

func caseStepper(env *Env) error {
	driver := gpio.NewStepperDriver(env.Adaptor, [4]string{"21", "22", "23", "24"}, gpio.StepperModes.DualPhaseStepping, 200)
	if err := driver.Start(); err != nil {
		return err
	}
	if err := driver.Move(8); err != nil {
		return err
	}
	if driver.CurrentStep() != 8 {
		return fmt.Errorf("stepper step = %d, want 8", driver.CurrentStep())
	}
	if driver.MaxSpeed() == 0 {
		return fmt.Errorf("stepper max speed is zero")
	}
	return nil
}

func caseEasyDriver(env *Env) error {
	driver := gpio.NewEasyDriver(env.Adaptor, 0.9, "5", gpio.WithEasyEnablePin("6"), gpio.WithEasySleepPin("7"))
	if err := driver.Start(); err != nil {
		return err
	}
	if err := driver.Enable(); err != nil {
		return err
	}
	if !driver.IsEnabled() {
		return fmt.Errorf("easy driver not enabled")
	}
	if err := driver.Disable(); err != nil {
		return err
	}
	if driver.IsEnabled() {
		return fmt.Errorf("easy driver still enabled")
	}
	if err := driver.Wake(); err != nil {
		return err
	}
	if driver.IsSleeping() {
		return fmt.Errorf("easy driver still sleeping")
	}
	return nil
}

func caseHCSR04(env *Env) error {
	if _, err := env.Sim.RegisterDevice(env.Board, simulate.DeviceSpec{
		ID: "distance", Kind: simulate.DeviceKindDistance, Pins: []string{"12", "13"},
		State: map[string]interface{}{"distance": 1.0},
	}); err != nil {
		return err
	}
	driver := gpio.NewHCSR04Driver(env.Adaptor, "12", "13")
	if err := driver.Start(); err != nil {
		return err
	}
	distance, err := driver.MeasureDistance()
	if err != nil {
		return err
	}
	if distance < 0.9 || distance > 1.1 {
		return fmt.Errorf("hcsr04 distance = %v, want ~1.0", distance)
	}
	return nil
}

func caseGroveGPIO(env *Env) error {
	button := gpio.NewGroveButtonDriver(env.Adaptor, "11", gpio.WithButtonDefaultState(1)) //nolint:staticcheck // deprecated wrapper intentionally exercised
	if err := button.Start(); err != nil {
		return err
	}
	if err := env.Sim.BindPin(env.Board, "11", "", simulate.PinModeInputPullup); err != nil {
		return err
	}
	if err := env.Sim.SetDigital(env.Board, "11", 0, simulate.SourceSimulation); err != nil {
		return err
	}
	deadline := time.Now().Add(time.Second)
	for !button.Active() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !button.Active() {
		return fmt.Errorf("grove button not active after pull low")
	}
	led := gpio.NewGroveLedDriver(env.Adaptor, "7") //nolint:staticcheck // deprecated wrapper intentionally exercised
	if err := led.Start(); err != nil {
		return err
	}
	if err := led.On(); err != nil {
		return err
	}
	pin, err := env.Sim.Pin(env.Board, "7")
	if err != nil {
		return err
	}
	if pin.DigitalValue != 1 {
		return fmt.Errorf("grove led pin = %d, want 1", pin.DigitalValue)
	}
	return nil
}

func caseAnalogActuator(env *Env) error {
	driver := aio.NewAnalogActuatorDriver(env.Adaptor, "A1")
	if err := driver.Start(); err != nil {
		return err
	}
	if err := driver.WriteRaw(321); err != nil {
		return err
	}
	pin, err := env.Sim.Pin(env.Board, "A1")
	if err != nil {
		return err
	}
	if pin.AnalogValue != 321 {
		return fmt.Errorf("analog actuator raw = %d, want 321", pin.AnalogValue)
	}
	if err := driver.Write(42.5); err != nil {
		return err
	}
	if driver.Value() != 42.5 {
		return fmt.Errorf("analog actuator value = %v, want 42.5", driver.Value())
	}
	return nil
}

func caseTemperature(env *Env) error {
	if err := env.Sim.BindPin(env.Board, "A2", "", simulate.PinModeAnalog); err != nil {
		return err
	}
	if err := env.Sim.SetAnalog(env.Board, "A2", 300, simulate.SourceSimulation); err != nil {
		return err
	}
	driver := aio.NewTemperatureSensorDriver(env.Adaptor, "A2")
	if err := driver.Start(); err != nil {
		return err
	}
	driver.SetLinearScaler(0, 1023, -20.0, 80.0)
	raw, err := driver.ReadRaw()
	if err != nil {
		return err
	}
	if raw != 300 {
		return fmt.Errorf("temperature raw = %d, want 300", raw)
	}
	if driver.Value() <= -20 || driver.Value() >= 80 {
		return fmt.Errorf("temperature scaled value = %v out of range", driver.Value())
	}
	return nil
}

func caseGroveAIO(env *Env) error {
	if err := env.Sim.BindPin(env.Board, "A3", "", simulate.PinModeAnalog); err != nil {
		return err
	}
	if err := env.Sim.SetAnalog(env.Board, "A3", 600, simulate.SourceSimulation); err != nil {
		return err
	}
	rotary := aio.NewGroveRotaryDriver(env.Adaptor, "A3")
	if err := rotary.Start(); err != nil {
		return err
	}
	rotaryValue, err := rotary.Read()
	if err != nil {
		return err
	}
	if rotaryValue != 600 {
		return fmt.Errorf("grove rotary = %v, want 600", rotaryValue)
	}
	light := aio.NewGroveLightSensorDriver(env.Adaptor, "A3")
	if startErr := light.Start(); startErr != nil {
		return startErr
	}
	lightValue, err := light.Read()
	if err != nil {
		return err
	}
	if lightValue != 600 {
		return fmt.Errorf("grove light = %v, want 600", lightValue)
	}
	return nil
}

func addI2CChip(env *Env, id, model string, address int, options map[string]interface{}) error {
	return env.Sim.AddChip(env.Board, topology.Chip{
		ID: id, Bus: "i2c-1", Address: address, Model: model, Options: options,
	})
}

func caseBME280(env *Env) error {
	if err := addI2CChip(env, "bme280", "bme280", 0x77, nil); err != nil {
		return err
	}
	driver := i2c.NewBME280Driver(env.Adaptor)
	if err := driver.Start(); err != nil {
		return err
	}
	humidity, err := driver.Humidity()
	if err != nil {
		return err
	}
	if humidity < 45 || humidity > 60 {
		return fmt.Errorf("bme280 humidity = %v, want ~51.2", humidity)
	}
	temperature, err := driver.Temperature()
	if err != nil {
		return err
	}
	if temperature < -20 || temperature > 60 {
		return fmt.Errorf("bme280 temperature = %v out of range", temperature)
	}
	pressure, err := driver.Pressure()
	if err != nil {
		return err
	}
	if pressure <= 0 {
		return fmt.Errorf("bme280 pressure = %v", pressure)
	}
	return nil
}

func caseBMP280(env *Env) error {
	if err := addI2CChip(env, "bmp280", "bmp280", 0x77, nil); err != nil {
		return err
	}
	driver := i2c.NewBMP280Driver(env.Adaptor)
	if err := driver.Start(); err != nil {
		return err
	}
	temperature, err := driver.Temperature()
	if err != nil {
		return err
	}
	if temperature < 20 || temperature > 30 {
		return fmt.Errorf("bmp280 temperature = %v, want ~25.0", temperature)
	}
	pressure, err := driver.Pressure()
	if err != nil {
		return err
	}
	if pressure < 90000 || pressure > 110000 {
		return fmt.Errorf("bmp280 pressure = %v, want ~99545", pressure)
	}
	return nil
}

func caseBMP180(env *Env) error {
	defaults := map[string]interface{}{
		"aa": 0x01, "ab": 0x98, "ac": 0xFF, "ad": 0xB8, "ae": 0xC7, "af": 0xD1,
		"b0": 0x7F, "b1": 0xE5, "b2": 0x7F, "b3": 0xF5, "b4": 0x5A, "b5": 0x71,
		"b6": 0x18, "b7": 0x2E, "b8": 0x00, "b9": 0x04, "ba": 0x80, "bb": 0x00,
		"bc": 0xDD, "bd": 0xF9, "be": 0x0B, "bf": 0x34,
	}
	if err := addI2CChip(env, "bmp180", "scripted", 0x77, map[string]interface{}{
		"defaults": defaults,
		"responses": map[string]interface{}{
			"f42e": []interface{}{0x6C, 0xFA},
			"f434": []interface{}{0x5D, 0x23, 0x00},
		},
	}); err != nil {
		return err
	}
	driver := i2c.NewBMP180Driver(env.Adaptor)
	if err := driver.Start(); err != nil {
		return err
	}
	temperature, err := driver.Temperature()
	if err != nil {
		return err
	}
	if temperature < 10 || temperature > 20 {
		return fmt.Errorf("bmp180 temperature = %v, want ~15.0", temperature)
	}
	pressure, err := driver.Pressure()
	if err != nil {
		return err
	}
	if pressure < 65000 || pressure > 75000 {
		return fmt.Errorf("bmp180 pressure = %v, want ~69964", pressure)
	}
	return nil
}

func caseBMP388(env *Env) error {
	if err := addI2CChip(env, "bmp388", "bmp388", 0x77, nil); err != nil {
		return err
	}
	driver := i2c.NewBMP388Driver(env.Adaptor)
	if err := driver.Start(); err != nil {
		return err
	}
	temperature, err := driver.Temperature(i2c.BMP388AccuracyStandard)
	if err != nil {
		return err
	}
	if temperature < 20 || temperature > 26 {
		return fmt.Errorf("bmp388 temperature = %v, want ~22.9", temperature)
	}
	pressure, err := driver.Pressure(i2c.BMP388AccuracyStandard)
	if err != nil {
		return err
	}
	if pressure < 90000 || pressure > 110000 {
		return fmt.Errorf("bmp388 pressure = %v, want ~98874", pressure)
	}
	return nil
}

func caseMPL115A2(env *Env) error {
	if err := addI2CChip(env, "mpl115a2", "mpl115a2", 0x60, nil); err != nil {
		return err
	}
	driver := i2c.NewMPL115A2Driver(env.Adaptor)
	if err := driver.Start(); err != nil {
		return err
	}
	pressure, err := driver.Pressure()
	if err != nil {
		return err
	}
	temperature, err := driver.Temperature()
	if err != nil {
		return err
	}
	if pressure <= 0 || pressure > 200 {
		return fmt.Errorf("mpl115a2 pressure = %v", pressure)
	}
	if temperature < -40 || temperature > 85 {
		return fmt.Errorf("mpl115a2 temperature = %v", temperature)
	}
	return nil
}

func sensirionResponse(init byte, raw uint16) []interface{} {
	data := []byte{byte(raw >> 8), byte(raw)} //nolint:gosec // intentional width conversion
	table := crc8.MakeTable(crc8.Params{
		Poly: 0x31, Init: init, RefIn: false, RefOut: false, XorOut: 0x00, Check: 0x00, Name: "CRC-8/SENSIRION",
	})
	return []interface{}{data[0], data[1], crc8.Checksum(data, table)}
}

func caseSHT3x(env *Env) error {
	tempRaw := uint16(0x6666)
	humRaw := uint16(0x8000)
	response := append(sensirionResponse(0xff, tempRaw), sensirionResponse(0xff, humRaw)...)
	if err := addI2CChip(env, "sht3x", "scripted", 0x44, map[string]interface{}{
		"responses": map[string]interface{}{"2400": response},
	}); err != nil {
		return err
	}
	driver := i2c.NewSHT3xDriver(env.Adaptor)
	if err := driver.Start(); err != nil {
		return err
	}
	temperature, humidity, err := driver.Sample()
	if err != nil {
		return err
	}
	if temperature < 24 || temperature > 26 {
		return fmt.Errorf("sht3x temperature = %v, want ~25", temperature)
	}
	if humidity < 49 || humidity > 51 {
		return fmt.Errorf("sht3x humidity = %v, want ~50", humidity)
	}
	return nil
}

func caseSHT2x(env *Env) error {
	tempRaw := uint16(0x6666)
	humRaw := uint16(0x8000)
	if err := addI2CChip(env, "sht2x", "scripted", 0x40, map[string]interface{}{
		"responses": map[string]interface{}{
			"e7": []interface{}{0x02},
			"f3": sensirionResponse(0x00, tempRaw),
			"f5": sensirionResponse(0x00, humRaw),
		},
	}); err != nil {
		return err
	}
	driver := i2c.NewSHT2xDriver(env.Adaptor)
	if err := driver.Start(); err != nil {
		return err
	}
	temperature, err := driver.Temperature()
	if err != nil {
		return err
	}
	if temperature < 20 || temperature > 27 {
		return fmt.Errorf("sht2x temperature = %v, want ~23.4", temperature)
	}
	humidity, err := driver.Humidity()
	if err != nil {
		return err
	}
	if humidity < 50 || humidity > 60 {
		return fmt.Errorf("sht2x humidity = %v, want ~56.5", humidity)
	}
	return nil
}

func casePCF8591(env *Env) error {
	if err := addI2CChip(env, "pcf8591", "scripted", 0x48, map[string]interface{}{
		"defaults": map[string]interface{}{"00": 0x10, "01": 0x20, "02": 0x30, "03": 0x40},
	}); err != nil {
		return err
	}
	driver := i2c.NewPCF8591Driver(env.Adaptor)
	if err := driver.Start(); err != nil {
		return err
	}
	value, err := driver.AnalogRead("0")
	if err != nil {
		return err
	}
	if value < 0 {
		return fmt.Errorf("pcf8591 analog read = %d", value)
	}
	return nil
}

func casePCA953x(env *Env) error {
	if err := addI2CChip(env, "pca953x", "scripted", 0x63, nil); err != nil {
		return err
	}
	driver := i2c.NewPCA953xDriver(env.Adaptor)
	if err := driver.Start(); err != nil {
		return err
	}
	value, err := driver.ReadGPIO(0)
	if err != nil {
		return err
	}
	if value != 0 {
		return fmt.Errorf("pca953x gpio = %d, want 0", value)
	}
	if err := driver.WriteGPIO(0, 0x55); err != nil {
		return err
	}
	return nil
}

func casePCA9501(env *Env) error {
	if err := addI2CChip(env, "pca9501", "scripted", 0x3F, nil); err != nil {
		return err
	}
	if err := addI2CChip(env, "pca9501-eeprom", "scripted", 0x7F, nil); err != nil {
		return err
	}
	driver := i2c.NewPCA9501Driver(env.Adaptor)
	if err := driver.Start(); err != nil {
		return err
	}
	value, err := driver.ReadGPIO(0)
	if err != nil {
		return err
	}
	if value != 0 {
		return fmt.Errorf("pca9501 gpio = %d, want 0", value)
	}
	if writeErr := driver.WriteEEPROM(0x10, 0x42); writeErr != nil {
		return writeErr
	}
	eeprom, err := driver.ReadEEPROM(0x10)
	if err != nil {
		return err
	}
	if eeprom != 0x42 {
		return fmt.Errorf("pca9501 eeprom = 0x%x, want 0x42", eeprom)
	}
	return nil
}

func caseINA3221(env *Env) error {
	if err := addI2CChip(env, "ina3221", "scripted", 0x40, nil); err != nil {
		return err
	}
	driver := i2c.NewINA3221Driver(env.Adaptor)
	if err := driver.Start(); err != nil {
		return err
	}
	voltage, err := driver.GetBusVoltage(i2c.INA3221Channel1)
	if err != nil {
		return err
	}
	if voltage < 0 {
		return fmt.Errorf("ina3221 bus voltage = %v", voltage)
	}
	current, err := driver.GetCurrent(i2c.INA3221Channel1)
	if err != nil {
		return err
	}
	if current < 0 {
		return fmt.Errorf("ina3221 current = %v", current)
	}
	return nil
}

func caseADXL345(env *Env) error {
	if err := addI2CChip(env, "adxl345", "scripted", 0x53, map[string]interface{}{
		"defaults": map[string]interface{}{"00": 0xE5},
	}); err != nil {
		return err
	}
	driver := i2c.NewADXL345Driver(env.Adaptor)
	if err := driver.Start(); err != nil {
		return err
	}
	x, y, z, err := driver.RawXYZ()
	if err != nil {
		return err
	}
	if x != 0 || y != 0 || z != 0 {
		return fmt.Errorf("adxl345 raw = %d,%d,%d, want zeros", x, y, z)
	}
	return nil
}

func caseL3GD20H(env *Env) error {
	if err := addI2CChip(env, "l3gd20h", "scripted", 0x6B, nil); err != nil {
		return err
	}
	driver := i2c.NewL3GD20HDriver(env.Adaptor)
	if err := driver.Start(); err != nil {
		return err
	}
	x, y, z, err := driver.XYZ()
	if err != nil {
		return err
	}
	if x != 0 || y != 0 || z != 0 {
		return fmt.Errorf("l3gd20h xyz = %v,%v,%v, want zeros", x, y, z)
	}
	return nil
}

func caseMMA7660(env *Env) error {
	if err := addI2CChip(env, "mma7660", "scripted", 0x4C, nil); err != nil {
		return err
	}
	driver := i2c.NewMMA7660Driver(env.Adaptor)
	if err := driver.Start(); err != nil {
		return err
	}
	if _, _, _, err := driver.XYZ(); err != nil {
		return err
	}
	return nil
}

func caseMPU6050(env *Env) error {
	if err := addI2CChip(env, "mpu6050", "scripted", 0x68, map[string]interface{}{
		"responses": map[string]interface{}{"6b80": []interface{}{0x00}},
	}); err != nil {
		return err
	}
	driver := i2c.NewMPU6050Driver(env.Adaptor)
	if err := driver.Start(); err != nil {
		return err
	}
	if err := driver.GetData(); err != nil {
		return err
	}
	return nil
}

func caseHMC5883L(env *Env) error {
	if err := addI2CChip(env, "hmc5883l", "scripted", 0x1E, nil); err != nil {
		return err
	}
	driver := i2c.NewHMC5883LDriver(env.Adaptor)
	if err := driver.Start(); err != nil {
		return err
	}
	x, y, z, err := driver.Read()
	if err != nil {
		return err
	}
	if x != 0 || y != 0 || z != 0 {
		return fmt.Errorf("hmc5883l = %v,%v,%v, want zeros", x, y, z)
	}
	return nil
}

func caseHMC6352(env *Env) error {
	if err := addI2CChip(env, "hmc6352", "scripted", 0x21, map[string]interface{}{
		"responses": map[string]interface{}{"41": []interface{}{0x03, 0xE8}},
	}); err != nil {
		return err
	}
	driver := i2c.NewHMC6352Driver(env.Adaptor)
	if err := driver.Start(); err != nil {
		return err
	}
	heading, err := driver.Heading()
	if err != nil {
		return err
	}
	if heading != 100 {
		return fmt.Errorf("hmc6352 heading = %d, want 100", heading)
	}
	return nil
}

func caseDRV2605L(env *Env) error {
	if err := addI2CChip(env, "drv2605l", "scripted", 0x5A, nil); err != nil {
		return err
	}
	driver := i2c.NewDRV2605LDriver(env.Adaptor)
	if err := driver.Start(); err != nil {
		return err
	}
	if err := driver.SetMode(i2c.DRV2605ModeIntTrig); err != nil {
		return err
	}
	if err := driver.SetSequence([]uint8{1, 2, 3}); err != nil {
		return err
	}
	if err := driver.Go(); err != nil {
		return err
	}
	return nil
}

func caseLIDARLite(env *Env) error {
	if err := addI2CChip(env, "lidarlite", "scripted", 0x62, map[string]interface{}{
		"responses": map[string]interface{}{
			"0004": []interface{}{0x01},
			"0f":   []interface{}{0x02},
			"10":   []interface{}{0x03},
		},
	}); err != nil {
		return err
	}
	driver := i2c.NewLIDARLiteDriver(env.Adaptor)
	if err := driver.Start(); err != nil {
		return err
	}
	distance, err := driver.Distance()
	if err != nil {
		return err
	}
	if distance <= 0 {
		return fmt.Errorf("lidarlite distance = %d", distance)
	}
	return nil
}

func caseTSL2561(env *Env) error {
	defaults := map[string]interface{}{"0a": 0x5A, "8a": 0x5A}
	for reg := 0x0C; reg <= 0x0F; reg++ {
		defaults[fmt.Sprintf("%02x", reg)] = 0
		defaults[fmt.Sprintf("%02x", reg|0x80)] = 0
		defaults[fmt.Sprintf("%02x", reg|0xA0)] = 0
	}
	if err := addI2CChip(env, "tsl2561", "scripted", 0x39, map[string]interface{}{"defaults": defaults}); err != nil {
		return err
	}
	driver := i2c.NewTSL2561Driver(env.Adaptor)
	if err := driver.Start(); err != nil {
		return err
	}
	broadband, ir, err := driver.GetLuminocity()
	if err != nil {
		return err
	}
	if broadband != 0 || ir != 0 {
		return fmt.Errorf("tsl2561 = %d/%d, want zeros", broadband, ir)
	}
	return nil
}

func caseJHD1313M1(env *Env) error {
	if err := addI2CChip(env, "lcd", "jhd1313m1", 0x3E, nil); err != nil {
		return err
	}
	if err := addI2CChip(env, "backlight", "rgb-backlight", 0x62, nil); err != nil {
		return err
	}
	driver := i2c.NewJHD1313M1Driver(env.Adaptor)
	if err := driver.Start(); err != nil {
		return err
	}
	if err := driver.Write("hello"); err != nil {
		return err
	}
	if err := driver.SetRGB(10, 20, 30); err != nil {
		return err
	}
	model, err := env.Adaptor.BusManager().ChipModel("lcd")
	if err != nil {
		return err
	}
	display, ok := model.(bus.Display)
	if !ok {
		return fmt.Errorf("jhd1313m1 has no display snapshot")
	}
	if snapshot := display.Snapshot(); !strings.Contains(strings.Join(snapshot.Lines, "\n"), "hello") {
		return fmt.Errorf("jhd1313m1 buffer = %q, want hello", snapshot.Lines)
	}
	backlight, err := env.Adaptor.BusManager().ChipModel("backlight")
	if err != nil {
		return err
	}
	values := backlight.(bus.ValuesProvider).Values()
	if values["red"] != 10 || values["green"] != 20 || values["blue"] != 30 {
		return fmt.Errorf("backlight = %v,%v,%v", values["red"], values["green"], values["blue"])
	}
	return nil
}

func caseSSD1306SPI(env *Env) error {
	if err := env.Sim.AddChip(env.Board, topology.Chip{
		ID: "oled-spi", Bus: "spi-0", Address: 0, Model: "ssd1306-spi",
	}); err != nil {
		return err
	}
	driver := gobotSPI.NewSSD1306Driver(env.Adaptor)
	if err := driver.Start(); err != nil {
		return err
	}
	if err := driver.Clear(); err != nil {
		return err
	}
	for x := 0; x < 24; x++ {
		driver.Set(x, 0, 1)
	}
	if err := driver.Display(); err != nil {
		return err
	}
	model, err := env.Adaptor.BusManager().ChipModel("oled-spi")
	if err != nil {
		return err
	}
	display, ok := model.(bus.Display)
	if !ok {
		return fmt.Errorf("spi ssd1306 has no display snapshot")
	}
	for _, value := range display.Snapshot().Pixels {
		if value != 0 {
			return nil
		}
	}
	return fmt.Errorf("spi ssd1306 framebuffer is empty")
}

func caseMicrobit(env *Env) error {
	const (
		pinDataChara     = "e95d8d00251d470aa062fa1922dfa9a8"
		pinADConfigChara = "e95d5899251d470aa062fa1922dfa9a8"
		pinIOConfigChara = "e95db9fe251d470aa062fa1922dfa9a8"
	)
	for _, uuid := range []string{pinADConfigChara, pinIOConfigChara} {
		if err := env.Adaptor.WriteCharacteristic(uuid, []byte{0, 0, 0, 0}); err != nil {
			return err
		}
	}
	if err := env.Adaptor.WriteCharacteristic(pinDataChara, []byte{0, 1, 1, 1, 2, 0}); err != nil {
		return err
	}

	led := bleMicrobit.NewLEDDriver(env.Adaptor)
	if err := led.Start(); err != nil {
		return err
	}
	if err := led.Solid(); err != nil {
		return err
	}
	matrix, err := led.ReadMatrix()
	if err != nil {
		return err
	}
	if len(matrix) != 5 || matrix[0] != 0x1F {
		return fmt.Errorf("microbit matrix = %v", matrix)
	}
	if err := led.WriteText("hi"); err != nil {
		return err
	}

	ioDriver := bleMicrobit.NewIOPinDriver(env.Adaptor)
	if err := ioDriver.Start(); err != nil {
		return err
	}
	if err := ioDriver.WritePinData("0", 1); err != nil {
		return err
	}
	if _, err := ioDriver.ReadPinIOConfig(); err != nil {
		return err
	}

	for _, driver := range []interface{ Start() error }{
		bleMicrobit.NewButtonDriver(env.Adaptor),
		bleMicrobit.NewTemperatureDriver(env.Adaptor),
		bleMicrobit.NewAccelerometerDriver(env.Adaptor),
		bleMicrobit.NewMagnetometerDriver(env.Adaptor),
	} {
		if err := driver.Start(); err != nil {
			return err
		}
	}
	return nil
}

// CleanupHalter halts polled drivers after a case.
type CleanupHalter interface {
	Halt() error
}

// DefaultTopology provides every modelled chip used by the harness.
const DefaultTopology = `schema: "1"
boards:
  - id: sim
    chips:
      - id: generic
        bus: i2c-1
        address: 16
        model: generic
      - id: expander
        bus: i2c-1
        address: 32
        model: mcp23017
      - id: pca
        bus: i2c-1
        address: 64
        model: pca9685
      - id: ads
        bus: i2c-1
        address: 72
        model: ads1115
      - id: bh
        bus: i2c-1
        address: 35
        model: bh1750
      - id: oled
        bus: i2c-1
        address: 60
        model: ssd1306
      - id: temperature
        bus: onewire
        model: ds18b20
`
