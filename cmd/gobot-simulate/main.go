// Command gobot-simulate runs a standalone simulator with demo DFP, TFP, and
// tank boards, serving both the Hertz API and the embedded Nuxt UI.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	simulate "github.com/disaster37/gobot-simulate"
	"gobot.io/x/gobot/v2"
	"gobot.io/x/gobot/v2/drivers/gpio"
)

func main() {
	cfg, err := simulate.LoadConfig(os.Args[1:])
	if err != nil {
		log.Fatalf("gobot-simulate: %v", err)
	}

	sim := simulate.New(simulate.WithConfig(cfg))
	setupDFP(sim)
	setupTFP(sim)
	setupTank(sim, "tank_pond")

	ctx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	if err := sim.Start(ctx); err != nil {
		stopSignals()
		log.Fatalf("gobot-simulate: start server: %v", err)
	}
	defer stopSignals()
	log.Printf("gobot-simulate %s listening on %s", simulate.Version, sim.URL()) //nolint:gosec // local URL

	<-ctx.Done()
	shutdownCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := sim.Stop(shutdownCtx); err != nil {
		log.Printf("gobot-simulate: stop server: %v", err)
	}
}

func setupDFP(sim *simulate.Simulator) {
	board := sim.Adaptor("dfp",
		simulate.WithAdaptorName("fat"),
		simulate.WithPlatform("raspberry-pi"),
		simulate.WithInvertedInitialPins("10", "19"),
		simulate.WithInputPullup("11", "13", "15", "16", "18", "22", "10", "12", "19", "21"),
	)

	relayDrum := gpio.NewRelayDriver(board, "3")
	relayPump := gpio.NewRelayDriver(board, "5")
	ledGreen := gpio.NewLedDriver(board, "7")
	ledRed := gpio.NewLedDriver(board, "8")
	buttonStart := gpio.NewButtonDriver(board, "11", gpio.WithButtonDefaultState(1))
	buttonStop := gpio.NewButtonDriver(board, "13", gpio.WithButtonDefaultState(1))
	buttonWash := gpio.NewButtonDriver(board, "15", gpio.WithButtonDefaultState(1))
	buttonEmergency := gpio.NewButtonDriver(board, "16", gpio.WithButtonDefaultState(1))
	buttonForceDrum := gpio.NewButtonDriver(board, "18", gpio.WithButtonDefaultState(1))
	buttonForcePump := gpio.NewButtonDriver(board, "22", gpio.WithButtonDefaultState(1))
	captorSecurityUpper := gpio.NewButtonDriver(board, "10", gpio.WithButtonDefaultState(0))
	captorSecurityUnder := gpio.NewButtonDriver(board, "12", gpio.WithButtonDefaultState(1))
	captorWaterUpper := gpio.NewButtonDriver(board, "19", gpio.WithButtonDefaultState(0))
	captorWaterUnder := gpio.NewButtonDriver(board, "21", gpio.WithButtonDefaultState(1))

	robot := gobot.NewRobot("fat",
		[]gobot.Connection{board},
		[]gobot.Device{
			relayDrum, relayPump, ledGreen, ledRed,
			buttonStart, buttonStop, buttonWash, buttonEmergency, buttonForceDrum, buttonForcePump,
			captorSecurityUpper, captorSecurityUnder, captorWaterUpper, captorWaterUnder,
		},
		func() {},
	)

	must(sim.AutoDiscover(robot,
		simulate.DeviceSpec{ID: "relay_drum", Label: "Drum Motor Relay", Kind: simulate.DeviceKindRelay, Pin: "3"},
		simulate.DeviceSpec{ID: "relay_pump", Label: "Washing Pump Relay", Kind: simulate.DeviceKindRelay, Pin: "5"},
		simulate.DeviceSpec{ID: "led_green", Label: "Green Status LED", Kind: simulate.DeviceKindLED, Pin: "7"},
		simulate.DeviceSpec{ID: "led_red", Label: "Red Fault LED", Kind: simulate.DeviceKindLED, Pin: "8"},
		simulate.DeviceSpec{ID: "button_start", Label: "Start DFP", Kind: simulate.DeviceKindButton, Pin: "11", DefaultState: simulate.IntPtr(1)},
		simulate.DeviceSpec{ID: "button_stop", Label: "Stop DFP", Kind: simulate.DeviceKindButton, Pin: "13", DefaultState: simulate.IntPtr(1)},
		simulate.DeviceSpec{ID: "button_wash", Label: "Force Wash", Kind: simulate.DeviceKindButton, Pin: "15", DefaultState: simulate.IntPtr(1)},
		simulate.DeviceSpec{ID: "button_emergency_stop", Label: "Emergency Stop", Kind: simulate.DeviceKindButton, Pin: "16", DefaultState: simulate.IntPtr(1)},
		simulate.DeviceSpec{ID: "button_force_drum", Label: "Manual Drum", Kind: simulate.DeviceKindButton, Pin: "18", DefaultState: simulate.IntPtr(1)},
		simulate.DeviceSpec{ID: "button_force_pump", Label: "Manual Pump", Kind: simulate.DeviceKindButton, Pin: "22", DefaultState: simulate.IntPtr(1)},
		simulate.DeviceSpec{ID: "captor_security_upper", Label: "Security Upper Captor", Kind: simulate.DeviceKindCaptor, Pin: "10", DefaultState: simulate.IntPtr(0)},
		simulate.DeviceSpec{ID: "captor_security_under", Label: "Security Under Captor", Kind: simulate.DeviceKindCaptor, Pin: "12", DefaultState: simulate.IntPtr(1)},
		simulate.DeviceSpec{ID: "captor_water_upper", Label: "Water Upper Captor", Kind: simulate.DeviceKindCaptor, Pin: "19", DefaultState: simulate.IntPtr(0)},
		simulate.DeviceSpec{ID: "captor_water_under", Label: "Water Under Captor", Kind: simulate.DeviceKindCaptor, Pin: "21", DefaultState: simulate.IntPtr(1)},
	))
	must(robot.Start(false))
}

func setupTFP(sim *simulate.Simulator) {
	board := sim.Adaptor("tfp",
		simulate.WithAdaptorName("tfp"),
		simulate.WithPlatform("esp8266-arest"),
		simulate.WithInitialValue("isRebooted", false),
	)
	relayPond := gpio.NewRelayDriver(board, "4", gpio.WithRelayInverted())
	relayWaterfall := gpio.NewRelayDriver(board, "5")
	relayUVC1 := gpio.NewRelayDriver(board, "12", gpio.WithRelayInverted())
	relayUVC2 := gpio.NewRelayDriver(board, "13", gpio.WithRelayInverted())
	relayFilter := gpio.NewRelayDriver(board, "14", gpio.WithRelayInverted())
	relayPondBubble := gpio.NewRelayDriver(board, "15", gpio.WithRelayInverted())

	robot := gobot.NewRobot("tfp",
		[]gobot.Connection{board},
		[]gobot.Device{relayPond, relayWaterfall, relayUVC1, relayUVC2, relayFilter, relayPondBubble},
		func() {},
	)
	must(sim.AutoDiscover(robot,
		simulate.DeviceSpec{ID: "relay_pond_pump", Label: "Pond Pump (NC)", Kind: simulate.DeviceKindRelay, Pin: "4"},
		simulate.DeviceSpec{ID: "relay_waterfall_pump", Label: "Waterfall Pump", Kind: simulate.DeviceKindRelay, Pin: "5"},
		simulate.DeviceSpec{ID: "relay_uvc1", Label: "UVC 1 (NC)", Kind: simulate.DeviceKindRelay, Pin: "12"},
		simulate.DeviceSpec{ID: "relay_uvc2", Label: "UVC 2 (NC)", Kind: simulate.DeviceKindRelay, Pin: "13"},
		simulate.DeviceSpec{ID: "relay_filter_bubble", Label: "Filter Bubble (NC)", Kind: simulate.DeviceKindRelay, Pin: "14"},
		simulate.DeviceSpec{ID: "relay_pond_bubble", Label: "Pond Bubble (NC)", Kind: simulate.DeviceKindRelay, Pin: "15"},
	))
	must(robot.Start(false))

	mustDevice(sim.RegisterDevice("tfp", simulate.DeviceSpec{
		ID: "value_isRebooted", Name: "isRebooted", Label: "Rebooted flag",
		Kind: simulate.DeviceKindValue, ValueName: "isRebooted",
	}))
	mustDevice(sim.RegisterDevice("tfp", simulate.DeviceSpec{
		ID: "function_acknoledgeRebooted", Name: "acknoledgeRebooted", Label: "Acknowledge reboot",
		Kind: simulate.DeviceKindFunction, FunctionName: "acknoledgeRebooted",
	}))
}

func setupTank(sim *simulate.Simulator, name string) {
	board := sim.Adaptor(name,
		simulate.WithAdaptorName(name),
		simulate.WithPlatform("esp8266-arest"),
		simulate.WithInitialValue("isRebooted", false),
		simulate.WithInitialValue("distance", float64(0)),
	)
	_ = board
	mustDevice(sim.RegisterDevice(name, simulate.DeviceSpec{
		ID: "value_isRebooted", Name: "isRebooted", Label: "Rebooted flag",
		Kind: simulate.DeviceKindValue, ValueName: "isRebooted",
	}))
	mustDevice(sim.RegisterDevice(name, simulate.DeviceSpec{
		ID: "value_distance", Name: "distance", Label: "Water distance",
		Kind: simulate.DeviceKindValue, ValueName: "distance",
	}))
	mustDevice(sim.RegisterDevice(name, simulate.DeviceSpec{
		ID: "function_acknoledgeRebooted", Name: "acknoledgeRebooted", Label: "Acknowledge reboot",
		Kind: simulate.DeviceKindFunction, FunctionName: "acknoledgeRebooted",
	}))
}

func must(err error) {
	if err != nil {
		log.Fatalf("gobot-simulate: %v", err)
	}
}

func mustDevice(_ simulate.DeviceState, err error) {
	if err != nil {
		log.Fatalf("gobot-simulate: %v", err)
	}
}
