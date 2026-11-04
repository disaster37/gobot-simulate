package state

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateID(t *testing.T) {
	tests := []struct {
		name string
		id   string
		ok   bool
	}{
		{"simple", "dfp", true},
		{"with allowed chars", "a_B.c:9-", true},
		{"empty", "", false},
		{"space", "has space", false},
		{"slash", "a/b", false},
		{"too long", strings.Repeat("a", 65), false},
		{"unicode", "café", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateID(tt.id)
			if tt.ok {
				assert.NoError(t, err)
			} else {
				assert.ErrorIs(t, err, ErrInvalidID)
			}
		})
	}

	err := ValidateIdentifier("board", "bad id")
	assert.ErrorIs(t, err, ErrInvalidID)
	assert.Contains(t, err.Error(), "board")
}

func TestNormalizeIdentifier(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"unchanged", "relay_drum", "relay_drum"},
		{"spaces to underscore", "relay drum", "relay_drum"},
		{"slashes to underscore", "a/b/c", "a_b_c"},
		{"empty", "", ""},
		{"all invalid", "!!!", "___"},
		{"truncates to 64", strings.Repeat("a", 100), strings.Repeat("a", 64)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, NormalizeIdentifier(tt.in))
		})
	}
}

func TestPinModeValid(t *testing.T) {
	for _, m := range []PinMode{
		PinModeUnset, PinModeInput, PinModeOutput, PinModeInputPullup,
		PinModePWM, PinModeServo, PinModeAnalog,
	} {
		assert.True(t, m.Valid(), "%s should be valid", m)
	}
	assert.False(t, PinMode("teleport").Valid())
}

func TestDeviceKindValid(t *testing.T) {
	for _, k := range []DeviceKind{
		DeviceKindRelay, DeviceKindLED, DeviceKindButton, DeviceKindCaptor,
		DeviceKindSensor, DeviceKindPWM, DeviceKindServo, DeviceKindAnalog,
		DeviceKindValue, DeviceKindFunction,
	} {
		assert.True(t, k.Valid(), "%s should be valid", k)
	}
	assert.False(t, DeviceKind("widget").Valid())
}
