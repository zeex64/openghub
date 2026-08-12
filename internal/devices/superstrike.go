package devices

import (
	"strings"

	"openghub/internal/hidpp"
)

type superstrikeDriver struct{}

func (superstrikeDriver) ID() string          { return "superstrike" }
func (superstrikeDriver) DisplayName() string { return "PRO X2 Superstrike" }
func (superstrikeDriver) Supported() bool     { return true }

func (superstrikeDriver) Matches(identity hidpp.DeviceIdentity, features FeatureSet) bool {
	name := strings.ToUpper(identity.Name)
	return strings.Contains(name, "SUPERSTRIKE") || strings.Contains(name, "SUPERSTRIIKE") ||
		(features.Has(hidpp.FeatAnalogButtons) && features.Has(hidpp.FeatBunnyHopping))
}

func (superstrikeDriver) Capabilities(features FeatureSet) Capabilities {
	out := genericCapabilities(features)
	out.DPIStages = 5
	out.Profiles = true
	out.OnboardMode = true
	out.ButtonMapping = true
	return out
}

func (superstrikeDriver) Profiles(device *hidpp.Device) ([]hidpp.Profile, error) {
	return device.Profiles()
}

func (superstrikeDriver) WriteDPIStage(device *hidpp.Device, sector, stage, x, y int, enabled, makeDefault bool) (int, error) {
	return device.WriteProfileDPIStage(sector, stage, x, y, enabled, makeDefault)
}

func (superstrikeDriver) WriteResolution(device *hidpp.Device, sector, x, y int) error {
	return device.WriteProfileResolution(sector, x, y)
}

func (superstrikeDriver) SetReportRate(device *hidpp.Device, sector, hz int) (int, error) {
	return device.SetProfileReportRateHz(sector, hz)
}

func (superstrikeDriver) SetCurrentProfile(device *hidpp.Device, sector int) error {
	return device.SetCurrentProfileSector(sector)
}

func (superstrikeDriver) SetProfileName(device *hidpp.Device, sector int, name string) (int, error) {
	return device.SetProfileName(sector, name)
}

func (superstrikeDriver) SetProfileEnabled(device *hidpp.Device, index int, enabled bool) error {
	return device.SetProfileEnabled(index, enabled)
}

func (superstrikeDriver) SetProfileButton(device *hidpp.Device, sector, index int, action hidpp.ButtonAction) (int, error) {
	return device.SetProfileButton(sector, index, action)
}
