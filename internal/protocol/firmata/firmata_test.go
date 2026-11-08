package firmata

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// nilBackend is a minimal Backend whose reads return fixed values.
type nilBackend struct{}

func (nilBackend) SetFirmataPinMode(pin, mode int) error       { return nil }
func (nilBackend) FirmataDigitalWrite(pin, value int) error    { return nil }
func (nilBackend) FirmataDigitalRead(pin int) (int, error)     { return 0, nil }
func (nilBackend) FirmataAnalogRead(pin int) (int, error)      { return 512, nil }
func (nilBackend) FirmataI2C(int, []byte, int) ([]byte, error) { return nil, nil }

// TestResponderPollIsRaceFree drives the responder's poll goroutine while the
// frame handler mutates lastPort/lastAnalog. Under -race this would fail on the
// previous unsynchronized reads in poll.
func TestResponderPollIsRaceFree(t *testing.T) {
	server, client := net.Pipe()
	defer func() { _ = server.Close() }()
	defer func() { _ = client.Close() }()

	responder, err := New(server, nilBackend{}, 24, 8)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = responder.Serve(ctx) }()

	// Drain responses so poll's writes never block the synchronous pipe.
	go func() {
		buf := make([]byte, 256)
		for {
			if _, err := client.Read(buf); err != nil {
				return
			}
		}
	}()

	// Enable digital reporting and flip a pin mode, both of which write
	// responder.lastPort while poll reads it.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			_, _ = client.Write([]byte{reportDigital, 0x01})
			_, _ = client.Write([]byte{pinMode, 0x00, ModeInput})
			time.Sleep(time.Millisecond)
		}
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()
	<-done
}
