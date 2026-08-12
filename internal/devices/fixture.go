package devices

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"

	"openghub/internal/hidpp"
)

const FixtureSchemaVersion = 1

// ProbeFixture is a portable, read-only snapshot used to develop and test a
// model driver without requiring the physical mouse for every iteration.
type ProbeFixture struct {
	SchemaVersion int                `json:"schemaVersion"`
	CapturedAt    time.Time          `json:"capturedAt"`
	Identity      FixtureIdentity    `json:"identity"`
	HIDPPVersion  string             `json:"hidppVersion,omitempty"`
	DriverID      string             `json:"driverId"`
	Capabilities  Capabilities       `json:"capabilities"`
	Features      []FixtureFeature   `json:"features"`
	Battery       *FixtureBattery    `json:"battery,omitempty"`
	DPI           *FixtureDPI        `json:"dpi,omitempty"`
	ReportRate    *FixtureReportRate `json:"reportRate,omitempty"`
	Onboard       *FixtureOnboard    `json:"onboard,omitempty"`
	Errors        []string           `json:"errors,omitempty"`
}

type FixtureIdentity struct {
	Name         string `json:"name"`
	Path         string `json:"path"`
	Index        byte   `json:"index"`
	VendorID     uint16 `json:"vendorId"`
	ProductID    uint16 `json:"productId"`
	Serial       string `json:"serial,omitempty"`
	PhysicalID   string `json:"physicalId,omitempty"`
	PhysicalSlot byte   `json:"physicalSlot,omitempty"`
}

type FixtureFeature struct {
	ID          uint16 `json:"id"`
	Index       byte   `json:"index"`
	Name        string `json:"name"`
	Obsolete    bool   `json:"obsolete,omitempty"`
	Hidden      bool   `json:"hidden,omitempty"`
	Engineering bool   `json:"engineering,omitempty"`
}

type FixtureReportRate struct {
	Current   int   `json:"current"`
	Supported []int `json:"supported"`
}

type FixtureBattery struct {
	Percent  int    `json:"percent"`
	Charging bool   `json:"charging"`
	Source   string `json:"source"`
}

type FixtureDPI struct {
	Current int  `json:"current"`
	Default int  `json:"default"`
	LOD     byte `json:"lod"`
	Min     int  `json:"min"`
	Max     int  `json:"max"`
	Step    int  `json:"step"`
}

type FixtureOnboard struct {
	Mode                 *byte              `json:"mode,omitempty"`
	CurrentProfileSector *int               `json:"currentProfileSector,omitempty"`
	Info                 FixtureProfileInfo `json:"info"`
	Sectors              []FixtureSector    `json:"sectors"`
}

type FixtureProfileInfo struct {
	SectorSize int `json:"sectorSize"`
	Count      int `json:"count"`
	Buttons    int `json:"buttons"`
}

type FixtureSector struct {
	Sector int    `json:"sector"`
	RawHex string `json:"rawHex"`
}

// CaptureFixture performs getters and raw profile-memory reads only. It never
// calls a setter or changes onboard/host mode.
func CaptureFixture(device *hidpp.Device) ProbeFixture {
	identity := device.Identity()
	if marketingName, err := device.DeviceName(); err == nil && marketingName != "" {
		identity.Name = marketingName
	}
	fixture := ProbeFixture{
		SchemaVersion: FixtureSchemaVersion,
		CapturedAt:    time.Now().UTC(),
		Identity: FixtureIdentity{
			Name: identity.Name, Path: identity.Path, Index: identity.Index,
			VendorID: identity.VendorID, ProductID: identity.ProductID, Serial: identity.Serial, PhysicalID: identity.PhysicalID, PhysicalSlot: identity.PhysicalSlot,
		},
	}
	if version, err := device.Ping(); err == nil {
		fixture.HIDPPVersion = version
	} else {
		fixture.Errors = append(fixture.Errors, "ping: "+err.Error())
	}

	features, err := device.EnumerateFeatures()
	if err != nil {
		fixture.Errors = append(fixture.Errors, "features: "+err.Error())
	}
	featureSet := NewFeatureSet(features)
	for _, feature := range features {
		fixture.Features = append(fixture.Features, FixtureFeature{
			ID: feature.ID, Index: feature.Index, Name: feature.Name(),
			Obsolete: feature.Obsolete, Hidden: feature.Hidden, Engineering: feature.Engineering,
		})
	}
	driver := Match(identity, featureSet)
	fixture.DriverID = driver.ID()
	fixture.Capabilities = driver.Capabilities(featureSet)

	if battery, readErr := device.Battery(); readErr == nil && battery.Available {
		fixture.Battery = &FixtureBattery{Percent: battery.Percent, Charging: battery.Charging, Source: battery.Source}
	}
	if dpi, readErr := device.DPI(); readErr == nil {
		fixture.DPI = &FixtureDPI{Current: dpi.Current, Default: dpi.Default, LOD: dpi.LOD, Min: dpi.Min, Max: dpi.Max, Step: dpi.Step}
	}
	if current, supported, readErr := device.ReportRate(); readErr == nil {
		fixture.ReportRate = &FixtureReportRate{Current: current, Supported: supported}
	}
	if featureSet.Has(hidpp.FeatOnboardProfile) {
		fixture.captureOnboard(device)
	}
	return fixture
}

func (fixture *ProbeFixture) captureOnboard(device *hidpp.Device) {
	onboard := &FixtureOnboard{}
	fixture.Onboard = onboard
	if mode, err := device.OnboardMode(); err == nil {
		onboard.Mode = &mode
	} else {
		fixture.Errors = append(fixture.Errors, "onboard mode: "+err.Error())
	}
	if sector, err := device.CurrentProfileSector(); err == nil {
		onboard.CurrentProfileSector = &sector
	} else {
		fixture.Errors = append(fixture.Errors, "current profile: "+err.Error())
	}
	info, err := device.ProfileInfo()
	if err != nil {
		fixture.Errors = append(fixture.Errors, "profile info: "+err.Error())
		return
	}
	onboard.Info = FixtureProfileInfo{SectorSize: info.SectorSize, Count: info.Count, Buttons: info.Buttons}
	candidates := []int{0x0000, 0x0100}
	for index := 1; index <= info.Count; index++ {
		candidates = append(candidates, index, 0x0100+index)
	}
	for _, sector := range candidates {
		raw, readErr := device.ReadSector(sector)
		if readErr != nil {
			continue
		}
		onboard.Sectors = append(onboard.Sectors, FixtureSector{Sector: sector, RawHex: hex.EncodeToString(raw)})
	}
	sort.Slice(onboard.Sectors, func(i, j int) bool { return onboard.Sectors[i].Sector < onboard.Sectors[j].Sector })
}

func DecodeFixture(data []byte) (ProbeFixture, error) {
	var fixture ProbeFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		return ProbeFixture{}, err
	}
	if err := fixture.Validate(); err != nil {
		return ProbeFixture{}, err
	}
	return fixture, nil
}

func LoadFixture(path string) (ProbeFixture, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ProbeFixture{}, err
	}
	return DecodeFixture(data)
}

func (fixture ProbeFixture) Validate() error {
	if fixture.SchemaVersion != FixtureSchemaVersion {
		return fmt.Errorf("fixture schema %d is unsupported", fixture.SchemaVersion)
	}
	if fixture.Identity.VendorID == 0 || fixture.Identity.ProductID == 0 {
		return fmt.Errorf("fixture is missing USB identity")
	}
	if fixture.DriverID == "" {
		return fmt.Errorf("fixture is missing a driver id")
	}
	if len(fixture.Features) == 0 {
		return fmt.Errorf("fixture has no HID++ features")
	}
	if fixture.Onboard != nil && fixture.Onboard.Info.SectorSize > 0 {
		for _, sector := range fixture.Onboard.Sectors {
			raw, err := hex.DecodeString(sector.RawHex)
			if err != nil {
				return fmt.Errorf("sector 0x%04x is not valid hex: %w", sector.Sector, err)
			}
			if len(raw) != fixture.Onboard.Info.SectorSize {
				return fmt.Errorf("sector 0x%04x has %d bytes, want %d", sector.Sector, len(raw), fixture.Onboard.Info.SectorSize)
			}
		}
	}
	return nil
}
