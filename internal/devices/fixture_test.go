package devices

import (
	"encoding/json"
	"strings"
	"testing"

	"openghub/internal/hidpp"
)

func TestProbeFixtureValidation(t *testing.T) {
	fixture := ProbeFixture{
		SchemaVersion: FixtureSchemaVersion,
		Identity:      FixtureIdentity{VendorID: 0x046D, ProductID: 0xC08B},
		DriverID:      "g502-se-hero",
		Features:      []FixtureFeature{{ID: hidpp.FeatIRoot, Name: "IRoot"}},
		Onboard:       &FixtureOnboard{Info: FixtureProfileInfo{SectorSize: 4}, Sectors: []FixtureSector{{Sector: 1, RawHex: "01020304"}}},
	}
	if err := fixture.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestProbeFixtureJSONRoundTrip(t *testing.T) {
	fixture := ProbeFixture{
		SchemaVersion: FixtureSchemaVersion,
		Identity:      FixtureIdentity{Name: "G502 HERO", VendorID: 0x046D, ProductID: 0xC08B},
		DriverID:      "g502-se-hero",
		Features:      []FixtureFeature{{ID: hidpp.FeatIRoot, Name: "IRoot"}},
	}
	data, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeFixture(data)
	if err != nil {
		t.Fatal(err)
	}
	if got.Identity.ProductID != 0xC08B || got.DriverID != "g502-se-hero" {
		t.Fatalf("DecodeFixture() = %+v", got)
	}
}

func TestProbeFixtureRejectsWrongSectorSize(t *testing.T) {
	fixture := ProbeFixture{
		SchemaVersion: FixtureSchemaVersion,
		Identity:      FixtureIdentity{VendorID: 0x046D, ProductID: 0xC08B},
		DriverID:      "g502-se-hero",
		Features:      []FixtureFeature{{ID: hidpp.FeatIRoot}},
		Onboard:       &FixtureOnboard{Info: FixtureProfileInfo{SectorSize: 4}, Sectors: []FixtureSector{{Sector: 1, RawHex: "0102"}}},
	}
	if err := fixture.Validate(); err == nil || !strings.Contains(err.Error(), "want 4") {
		t.Fatalf("Validate() = %v", err)
	}
}
