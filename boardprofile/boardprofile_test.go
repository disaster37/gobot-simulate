package boardprofile

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeneratedProfilesCoverInScopePlatforms(t *testing.T) {
	profiles := Profiles()
	require.NotEmpty(t, profiles)

	ids := make(map[string]struct{}, len(profiles))
	for _, profile := range profiles {
		ids[profile.ID] = struct{}{}
		assert.NotEmpty(t, profile.ID)
		assert.NotEmpty(t, profile.Name)
		for _, bus := range profile.I2CBuses {
			assert.Equal(t, "i2c", bus.Transport)
			assert.GreaterOrEqual(t, bus.Number, 0)
			assert.Less(t, bus.Number, 100, "bus number must not be a speed value")
		}
		for _, bus := range profile.SPIBuses {
			assert.Equal(t, "spi", bus.Transport)
			assert.GreaterOrEqual(t, bus.Number, 0)
			assert.Less(t, bus.Number, 100, "bus number must not be a speed value")
		}
	}
	for _, want := range []string{
		"raspi", "beaglebone", "tinkerboard", "jetson", "chip", "orangepi5pro",
		"rockpi", "rock64", "up2", "nanopct6", "dragonboard", "edison",
		"firmata", "particle", "arest", "simulated",
	} {
		assert.Contains(t, ids, want, "missing generated profile %s", want)
	}
}

func TestProfileByIDAndSortedIDs(t *testing.T) {
	profile, ok := ProfileByID("RASPI")
	require.True(t, ok)
	assert.Equal(t, "raspi", profile.ID)
	assert.NotEmpty(t, profile.DigitalPins)

	profile, ok = ProfileByID("firmata")
	require.True(t, ok)
	assert.True(t, profile.Permissive)

	_, ok = ProfileByID("does-not-exist")
	assert.False(t, ok)

	ids := SortedIDs()
	require.NotEmpty(t, ids)
	assert.IsIncreasing(t, ids)
}

func TestProfilesReturnsCopy(t *testing.T) {
	first := Profiles()
	first[0].ID = "mutated"
	second := Profiles()
	assert.NotEqual(t, "mutated", second[0].ID)
}

func TestRaspiProfileBuses(t *testing.T) {
	profile, ok := ProfileByID("raspi")
	require.True(t, ok)
	assert.Equal(t, 1, profile.DefaultI2CBus)
	assert.Equal(t, 0, profile.DefaultSPIBus)
	assert.Contains(t, profile.I2CBuses, Bus{Transport: "i2c", Number: 0})
	assert.Contains(t, profile.I2CBuses, Bus{Transport: "i2c", Number: 1})
	for _, pin := range profile.DigitalPins {
		if pin.ID == "3" {
			assert.Contains(t, pin.Label, "GPIO2")
		}
	}
}
