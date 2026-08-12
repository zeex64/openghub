package devices

import (
	"testing"

	"openghub/internal/hidpp"
)

func TestMatchSuperstrikeByName(t *testing.T) {
	driver := Match(hidpp.DeviceIdentity{Name: "PRO X2 SUPERSTRIIKE"}, FeatureSet{})
	if driver.ID() != "superstrike" || !driver.Supported() {
		t.Fatalf("Match() = %s supported=%v, want supported superstrike", driver.ID(), driver.Supported())
	}
}

func TestMatchG502HeroByProductID(t *testing.T) {
	driver := Match(hidpp.DeviceIdentity{ProductID: 0xC08B, Name: "Logitech G502 HERO"}, FeatureSet{})
	if driver.ID() != "g502-se-hero" || !driver.Supported() {
		t.Fatalf("Match() = %s supported=%v, want supported g502-se-hero", driver.ID(), driver.Supported())
	}
}

func TestG502Capabilities(t *testing.T) {
	caps := (g502HeroDriver{}).Capabilities(FeatureSet{})
	if !caps.DPI || !caps.Profiles || !caps.ButtonMapping || caps.DPIMax != 25600 || caps.DPILiftOff || caps.SeparateRates {
		t.Fatalf("Capabilities() = %+v", caps)
	}
}

func TestSuperstrikeCapabilitiesComeFromFeatures(t *testing.T) {
	features := FeatureSet{
		hidpp.FeatOnboardProfile: true,
		hidpp.FeatExtAdjDPI:      true,
		hidpp.FeatAnalogButtons:  true,
		hidpp.FeatBunnyHopping:   true,
	}
	caps := (superstrikeDriver{}).Capabilities(features)
	if !caps.Profiles || !caps.DPI || !caps.Haptics || !caps.Bhop || caps.DPIStages != 5 {
		t.Fatalf("Capabilities() = %+v", caps)
	}
}
