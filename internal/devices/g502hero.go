package devices

import (
	"fmt"
	"strings"

	"openghub/internal/hidpp"
)

const g502HeroMaxDPI = 25600

type g502HeroDriver struct{}

func (g502HeroDriver) ID() string          { return "g502-se-hero" }
func (g502HeroDriver) DisplayName() string { return "G502 HERO" }
func (g502HeroDriver) Supported() bool     { return true }

func (g502HeroDriver) Matches(identity hidpp.DeviceIdentity, _ FeatureSet) bool {
	name := strings.ToUpper(identity.Name)
	return identity.ProductID == 0xC08B || strings.Contains(name, "G502 HERO") || strings.Contains(name, "G502 SE HERO")
}

func (g502HeroDriver) Capabilities(features FeatureSet) Capabilities {
	out := genericCapabilities(features)
	out.DPI, out.DPIStages = true, 5
	out.DPIMin, out.DPIMax, out.DPIStep = 100, g502HeroMaxDPI, 50
	out.DPILiftOff = false
	out.PollingRates = []int{1000, 500, 250, 125}
	out.SeparateRates = false
	out.Profiles, out.OnboardMode, out.ButtonMapping = true, true, true
	out.Haptics, out.GamingSurface, out.Bhop = false, false, false
	out.Lighting, out.StartupEffect, out.DPILighting = true, true, true
	return out
}

func (g502HeroDriver) Profiles(device *hidpp.Device) ([]hidpp.Profile, error) {
	return device.Profiles()
}

func (g502HeroDriver) WriteDPIStage(device *hidpp.Device, sector, stage, x, _ int, _ byte, enabled, makeDefault bool) (int, error) {
	if stage < 0 || stage > 4 {
		return 0, fmt.Errorf("DPI stage %d is out of range", stage+1)
	}
	return device.WriteClassicProfileDPIStage(sector, stage, x, enabled, makeDefault, g502HeroMaxDPI)
}

func (g502HeroDriver) SaveDPISettings(device *hidpp.Device, sector int, stages []hidpp.DPIStage, defaultStage, currentStage, shiftStage int) (int, error) {
	return device.WriteClassicProfileDPISettings(sector, stages, defaultStage, currentStage, shiftStage, g502HeroMaxDPI)
}

func (g502HeroDriver) WriteResolution(device *hidpp.Device, sector, x, _ int) error {
	return device.WriteClassicProfileResolution(sector, x, g502HeroMaxDPI)
}

func (g502HeroDriver) SetReportRate(device *hidpp.Device, sector, hz int) (int, error) {
	return device.SetClassicProfileReportRateHz(sector, hz)
}

func (g502HeroDriver) SetCurrentProfile(device *hidpp.Device, sector int) error {
	return device.SetCurrentProfileSector(sector)
}

func (g502HeroDriver) SetProfileName(device *hidpp.Device, sector int, name string) (int, error) {
	return device.SetProfileName(sector, name)
}

func (g502HeroDriver) SetProfileEnabled(device *hidpp.Device, index int, enabled bool) error {
	return device.SetProfileEnabled(index, enabled)
}

func (g502HeroDriver) SetProfileButton(device *hidpp.Device, sector, index int, action hidpp.ButtonAction) (int, error) {
	return device.SetClassicProfileButton(sector, index, action)
}
