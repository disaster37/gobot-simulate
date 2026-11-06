// Command gobot-fat-demo mirrors the gobot-fat DFP + TFP + tank topology
// against the simulated adaptor, including the same scoped Viper-style pins.
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
		log.Fatal(err)
	}
	sim := simulate.New(simulate.WithConfig(cfg))
	if err := run(sim); err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	if err := sim.Start(ctx); err != nil {
		stop()
		log.Fatal(err)
	}
	defer stop()
	log.Printf("gobot-fat demo listening on %s", sim.URL()) //nolint:gosec // local URL
	<-ctx.Done()
	_ = sim.Stop(context.Background())
}

func run(sim *simulate.Simulator) error {
	dfp := sim.Adaptor("dfp",
		simulate.WithAdaptorName("fat"),
		simulate.WithPlatform("raspberry-pi"),
		simulate.WithInvertedInitialPins("10", "19"),
		simulate.WithInputPullup("11", "13", "15", "10", "12", "19"),
	)
	relayDrum := gpio.NewRelayDriver(dfp, "3")
	relayPump := gpio.NewRelayDriver(dfp, "5")
	ledGreen := gpio.NewLedDriver(dfp, "7")
	buttonStart := gpio.NewButtonDriver(dfp, "11", gpio.WithButtonDefaultState(1))
	captorSecurityUpper := gpio.NewButtonDriver(dfp, "10", gpio.WithButtonDefaultState(0))
	dfpRobot := gobot.NewRobot("fat",
		[]gobot.Connection{dfp},
		[]gobot.Device{relayDrum, relayPump, ledGreen, buttonStart, captorSecurityUpper},
		func() {},
	)
	if err := sim.AutoDiscover(dfpRobot,
		simulate.DeviceSpec{ID: "relay_drum", Label: "Drum Motor Relay", Kind: simulate.DeviceKindRelay, Pin: "3"},
		simulate.DeviceSpec{ID: "relay_pump", Label: "Washing Pump Relay", Kind: simulate.DeviceKindRelay, Pin: "5"},
		simulate.DeviceSpec{ID: "led_green", Label: "Green Status LED", Kind: simulate.DeviceKindLED, Pin: "7"},
		simulate.DeviceSpec{ID: "button_start", Label: "Start DFP", Kind: simulate.DeviceKindButton, Pin: "11", DefaultState: simulate.IntPtr(1)},
		simulate.DeviceSpec{ID: "captor_security_upper", Label: "Security Upper Captor", Kind: simulate.DeviceKindCaptor, Pin: "10", DefaultState: simulate.IntPtr(0)},
	); err != nil {
		return err
	}
	if err := dfpRobot.Start(false); err != nil {
		return err
	}

	tfp := sim.Adaptor("tfp",
		simulate.WithAdaptorName("tfp"),
		simulate.WithPlatform("esp8266-arest"),
		simulate.WithInitialValue("isRebooted", false),
	)
	tfpRobot := gobot.NewRobot("tfp", []gobot.Connection{tfp}, []gobot.Device{
		gpio.NewRelayDriver(tfp, "4", gpio.WithRelayInverted()),
	}, func() {})
	if err := sim.AutoDiscover(tfpRobot,
		simulate.DeviceSpec{ID: "relay_pond_pump", Label: "Pond Pump (NC)", Kind: simulate.DeviceKindRelay, Pin: "4"},
	); err != nil {
		return err
	}
	if err := tfpRobot.Start(false); err != nil {
		return err
	}
	if _, err := sim.RegisterDevice("tfp", simulate.DeviceSpec{
		ID: "value_isRebooted", Name: "isRebooted", Kind: simulate.DeviceKindValue, ValueName: "isRebooted",
	}); err != nil {
		return err
	}
	if _, err := sim.RegisterDevice("tfp", simulate.DeviceSpec{
		ID: "function_acknoledgeRebooted", Name: "acknoledgeRebooted", Kind: simulate.DeviceKindFunction, FunctionName: "acknoledgeRebooted",
	}); err != nil {
		return err
	}

	for _, name := range []string{"tank_pond", "tank_garden"} {
		_ = sim.Adaptor(name,
			simulate.WithAdaptorName(name),
			simulate.WithPlatform("esp8266-arest"),
			simulate.WithInitialValue("isRebooted", false),
			simulate.WithInitialValue("distance", float64(0)),
		)
		for _, spec := range []simulate.DeviceSpec{
			{ID: "value_isRebooted", Name: "isRebooted", Kind: simulate.DeviceKindValue, ValueName: "isRebooted"},
			{ID: "value_distance", Name: "distance", Kind: simulate.DeviceKindValue, ValueName: "distance"},
			{ID: "function_acknoledgeRebooted", Name: "acknoledgeRebooted", Kind: simulate.DeviceKindFunction, FunctionName: "acknoledgeRebooted"},
		} {
			if _, err := sim.RegisterDevice(name, spec); err != nil {
				return err
			}
		}
	}
	return nil
}
