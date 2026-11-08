package display

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHD44780FourBitDecoder(t *testing.T) {
	decoder := New("hd44780", []string{"rs", "en", "d4", "d5", "d6", "d7"})
	require.NotNil(t, decoder)

	writeNibble := func(rs int, value byte) {
		for index := 0; index < 4; index++ {
			decoder.Pin([]string{"d4", "d5", "d6", "d7"}[index], int((value>>index)&1))
		}
		decoder.Pin("rs", rs)
		decoder.Pin("en", 1)
		decoder.Pin("en", 0)
	}
	writeByte := func(rs int, value byte) {
		writeNibble(rs, value>>4)
		writeNibble(rs, value&0x0F)
	}

	// Initialization sequence (single nibbles) followed by the real commands.
	writeNibble(0, 0x03)
	writeNibble(0, 0x03)
	writeNibble(0, 0x03)
	writeNibble(0, 0x02)
	writeByte(0, 0x28) // function set: 4-bit, 2 lines
	writeByte(0, 0x0C) // display on
	writeByte(0, 0x01) // clear
	writeByte(0, 0x06) // entry mode
	for _, char := range "hello" {
		writeByte(1, byte(char)) //nolint:gosec // test vector
	}

	snapshot := decoder.Snapshot()
	require.Len(t, snapshot.Lines, 2)
	assert.Equal(t, "hello", snapshot.Lines[0])
	assert.True(t, snapshot.Enabled)
}

func TestMAX7219Decoder(t *testing.T) {
	decoder := New("max7219", []string{"clk", "din", "cs"})
	require.NotNil(t, decoder)

	decoder.Pin("cs", 0)
	send := func(value byte) {
		for bit := 7; bit >= 0; bit-- {
			decoder.Pin("din", int((value>>bit)&1))
			decoder.Pin("clk", 0)
			decoder.Pin("clk", 1)
		}
	}
	send(0x01)
	send(0xFF)
	decoder.Pin("cs", 1)

	snapshot := decoder.Snapshot()
	require.NotEmpty(t, snapshot.Pixels)
	assert.Equal(t, byte(0xFF), snapshot.Pixels[0])
}

func TestShiftRegisterDecoder(t *testing.T) {
	decoder := New("aip1640", []string{"clk", "din"})
	require.NotNil(t, decoder)

	send := func(value byte) {
		for bit := 0; bit < 8; bit++ {
			decoder.Pin("clk", 0)
			decoder.Pin("din", int((value>>bit)&1))
			decoder.Pin("clk", 1)
		}
	}
	send(0x44)
	send(0xC0)
	send(0xFF)
	send(0x44)
	send(0xC1)
	send(0x01)

	snapshot := decoder.Snapshot()
	require.NotEmpty(t, snapshot.Pixels)
	assert.Equal(t, byte(0xFF), snapshot.Pixels[0])
	assert.Equal(t, byte(0x01), snapshot.Pixels[1])
}

func TestTM1638TransactionDecoder(t *testing.T) {
	decoder := New("tm1638", []string{"clk", "din", "strobe"})
	require.NotNil(t, decoder)

	send := func(value byte) {
		for bit := 0; bit < 8; bit++ {
			decoder.Pin("clk", 0)
			decoder.Pin("din", int((value>>bit)&1))
			decoder.Pin("clk", 1)
		}
	}
	transaction := func(values ...byte) {
		decoder.Pin("strobe", 0)
		for _, value := range values {
			send(value)
		}
		decoder.Pin("strobe", 1)
	}
	transaction(0x44)
	transaction(0xC0, 0xFF)
	transaction(0x44)
	transaction(0xC1, 0x01)

	snapshot := decoder.Snapshot()
	require.NotEmpty(t, snapshot.Pixels)
	assert.Equal(t, byte(0xFF), snapshot.Pixels[0])
	assert.Equal(t, byte(0x01), snapshot.Pixels[1])
}

func TestUnknownDisplayModel(t *testing.T) {
	assert.Nil(t, New("bogus", nil))
}
