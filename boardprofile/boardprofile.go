// Package boardprofile exposes generated board pin and bus catalogs.
package boardprofile

import (
	"sort"
	"strings"
)

// Pin describes one physical header pin or labelled connector.
type Pin struct {
	ID    string `json:"id"`
	Label string `json:"label,omitempty"`
	Mode  string `json:"mode,omitempty"`
}

// Bus describes one bus exposed by a platform.
type Bus struct {
	Transport string `json:"transport"`
	Number    int    `json:"number"`
}

// Profile is a generated board profile.
type Profile struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Platform      string `json:"platform"`
	DigitalPins   []Pin  `json:"digitalPins,omitempty"`
	AnalogPins    []Pin  `json:"analogPins,omitempty"`
	PWMPins       []Pin  `json:"pwmPins,omitempty"`
	I2CBuses      []Bus  `json:"i2cBuses,omitempty"`
	SPIBuses      []Bus  `json:"spiBuses,omitempty"`
	DefaultI2CBus int    `json:"defaultI2cBus,omitempty"`
	DefaultSPIBus int    `json:"defaultSpiBus,omitempty"`
	Permissive    bool   `json:"permissive,omitempty"`
}

// Profiles returns every generated profile.
func Profiles() []Profile {
	out := make([]Profile, len(generatedProfiles))
	copy(out, generatedProfiles)
	for i := range out {
		out[i].DigitalPins = append([]Pin(nil), out[i].DigitalPins...)
		out[i].AnalogPins = append([]Pin(nil), out[i].AnalogPins...)
		out[i].PWMPins = append([]Pin(nil), out[i].PWMPins...)
		out[i].I2CBuses = append([]Bus(nil), out[i].I2CBuses...)
		out[i].SPIBuses = append([]Bus(nil), out[i].SPIBuses...)
	}
	return out
}

// ProfileByID finds a generated profile by ID or platform name.
func ProfileByID(id string) (Profile, bool) {
	needle := strings.ToLower(strings.TrimSpace(id))
	for _, profile := range generatedProfiles {
		if strings.ToLower(profile.ID) == needle || strings.ToLower(profile.Platform) == needle {
			return profile, true
		}
	}
	return Profile{}, false
}

// SortedIDs returns the generated profile IDs.
func SortedIDs() []string {
	out := make([]string, 0, len(generatedProfiles))
	for _, profile := range generatedProfiles {
		out = append(out, profile.ID)
	}
	sort.Strings(out)
	return out
}
