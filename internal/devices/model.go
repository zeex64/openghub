// Package devices maps discovered HID++ endpoints to application-level mouse
// models. Protocol transport stays in hidpp; profile layouts and model-specific
// behavior live behind Driver implementations here.
package devices

import (
	"fmt"
	"strings"

	"openghub/internal/hidpp"
)

type FeatureSet map[uint16]bool

func NewFeatureSet(features []hidpp.Feature) FeatureSet {
	out := make(FeatureSet, len(features))
	for _, feature := range features {
		out[feature.ID] = true
	}
	return out
}

func (features FeatureSet) Has(id uint16) bool { return features[id] }

// ProbeFeatures prefers full enumeration and falls back to the application
// feature set for devices whose firmware does not expose IFeatureSet cleanly.
func ProbeFeatures(device *hidpp.Device) FeatureSet {
	if features, err := device.EnumerateFeatures(); err == nil {
		return NewFeatureSet(features)
	}
	known := []uint16{
		hidpp.FeatBatteryStatus, hidpp.FeatUnifiedBattery,
		hidpp.FeatReprogControls, hidpp.FeatAdjustableDPI, hidpp.FeatExtAdjDPI,
		hidpp.FeatMouseTuning, hidpp.FeatReportRate, hidpp.FeatExtReportRate,
		hidpp.FeatBunnyHopping, hidpp.FeatOnboardProfile, hidpp.FeatAnalogButtons,
	}
	out := make(FeatureSet)
	for _, id := range known {
		if device.Has(id) {
			out[id] = true
		}
	}
	return out
}

type Capabilities struct {
	Battery       bool  `json:"battery"`
	DPI           bool  `json:"dpi"`
	DPIStages     int   `json:"dpiStages"`
	PollingRates  []int `json:"pollingRates"`
	Profiles      bool  `json:"profiles"`
	OnboardMode   bool  `json:"onboardMode"`
	ButtonMapping bool  `json:"buttonMapping"`
	Haptics       bool  `json:"haptics"`
	GamingSurface bool  `json:"gamingSurface"`
	Bhop          bool  `json:"bhop"`
}

// Driver owns model-specific profile interpretation and mutation. Generic
// feature calls such as battery and live DPI remain in hidpp.
type Driver interface {
	ID() string
	DisplayName() string
	Supported() bool
	Matches(hidpp.DeviceIdentity, FeatureSet) bool
	Capabilities(FeatureSet) Capabilities
	Profiles(*hidpp.Device) ([]hidpp.Profile, error)
	WriteDPIStage(*hidpp.Device, int, int, int, int, bool, bool) (int, error)
	WriteResolution(*hidpp.Device, int, int, int) error
	SetReportRate(*hidpp.Device, int, int) (int, error)
	SetCurrentProfile(*hidpp.Device, int) error
	SetProfileName(*hidpp.Device, int, string) (int, error)
	SetProfileEnabled(*hidpp.Device, int, bool) error
	SetProfileButton(*hidpp.Device, int, int, hidpp.ButtonAction) (int, error)
}

func Match(identity hidpp.DeviceIdentity, features FeatureSet) Driver {
	for _, driver := range registeredDrivers {
		if driver.Matches(identity, features) {
			return driver
		}
	}
	return unsupportedDriver{id: "unknown-logitech", name: cleanName(identity.Name, "Logitech HID++ device")}
}

var registeredDrivers = []Driver{
	superstrikeDriver{},
	unsupportedDriver{id: "g502-se-hero", name: "G502 HERO / SE", productIDs: []uint16{0xC08B}},
}

func genericCapabilities(features FeatureSet) Capabilities {
	out := Capabilities{
		Battery:       features.Has(hidpp.FeatUnifiedBattery) || features.Has(hidpp.FeatBatteryStatus),
		DPI:           features.Has(hidpp.FeatExtAdjDPI) || features.Has(hidpp.FeatAdjustableDPI),
		PollingRates:  nil,
		Profiles:      features.Has(hidpp.FeatOnboardProfile),
		OnboardMode:   features.Has(hidpp.FeatOnboardProfile),
		ButtonMapping: features.Has(hidpp.FeatReprogControls) || features.Has(hidpp.FeatOnboardProfile),
		Haptics:       features.Has(hidpp.FeatAnalogButtons),
		GamingSurface: features.Has(hidpp.FeatMouseTuning),
		Bhop:          features.Has(hidpp.FeatBunnyHopping),
	}
	if features.Has(hidpp.FeatReportRate) || features.Has(hidpp.FeatExtReportRate) || features.Has(hidpp.FeatOnboardProfile) {
		out.PollingRates = append([]int(nil), hidpp.ReportRates...)
	}
	return out
}

func cleanName(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

func unsupported(operation string) error {
	return fmt.Errorf("%s is not implemented for this mouse model", operation)
}
