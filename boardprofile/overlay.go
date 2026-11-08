package boardprofile

// pinLabels is the only hand-written overlay: gobot's pin maps carry header
// numbers but not the alternate functions printed on the silkscreen.
var pinLabels = map[string]map[string]string{
	"raspi": {
		"3": "GPIO2 (SDA1)", "5": "GPIO3 (SCL1)", "7": "GPIO4 (GPCLK0)",
		"8": "GPIO14 (TXD0)", "10": "GPIO15 (RXD0)", "11": "GPIO17",
		"12": "GPIO18 (PCM_CLK)", "13": "GPIO27", "15": "GPIO22",
		"16": "GPIO23", "18": "GPIO24", "19": "GPIO10 (MOSI1)",
		"21": "GPIO9 (MISO1)", "22": "GPIO25", "23": "GPIO11 (SCLK1)",
		"24": "GPIO8 (CE0)", "26": "GPIO7 (CE1)", "27": "ID_SD",
		"28": "ID_SC", "29": "GPIO5", "31": "GPIO6", "32": "GPIO12",
		"33": "GPIO13", "35": "GPIO19", "36": "GPIO16", "37": "GPIO26",
		"38": "GPIO20", "40": "GPIO21",
	},
	"beaglebone": {
		"P9_33": "AIN4", "P9_35": "AIN6", "P9_36": "AIN5",
		"P9_37": "AIN2", "P9_38": "AIN3", "P9_39": "AIN0", "P9_40": "AIN1",
	},
}

func applyOverlay(profile *Profile) {
	labels, ok := pinLabels[profile.ID]
	if !ok {
		return
	}
	for index := range profile.DigitalPins {
		if label, ok := labels[profile.DigitalPins[index].ID]; ok {
			profile.DigitalPins[index].Label = label
		}
	}
	for index := range profile.AnalogPins {
		if label, ok := labels[profile.AnalogPins[index].ID]; ok {
			profile.AnalogPins[index].Label = label
		}
	}
}
