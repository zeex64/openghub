package main

import (
	"os"
	"path/filepath"
	"testing"

	"openghub/internal/devices"
	"openghub/internal/hidpp"
)

func intPointer(value int) *int { return &value }

func TestResolveActiveProfileSectorPrefersReportedSector(t *testing.T) {
	profiles := []hidpp.Profile{{Sector: 1, Enabled: true}, {Sector: 2, Enabled: true}}
	if got := resolveActiveProfileSector(2, 1, profiles); got != 2 {
		t.Fatalf("got sector %d, want reported sector 2", got)
	}
}

func TestResolveActiveProfileSectorKeepsRememberedSectorInHostMode(t *testing.T) {
	profiles := []hidpp.Profile{{Sector: 1, Enabled: true}, {Sector: 2, Enabled: true}}
	if got := resolveActiveProfileSector(0, 2, profiles); got != 2 {
		t.Fatalf("got sector %d, want remembered sector 2", got)
	}
}

func TestResolveActiveProfileSectorFallsBackToFirstEnabled(t *testing.T) {
	profiles := []hidpp.Profile{{Sector: 1, Enabled: false}, {Sector: 2, Enabled: true}}
	if got := resolveActiveProfileSector(0, 9, profiles); got != 2 {
		t.Fatalf("got sector %d, want first enabled sector 2", got)
	}
}

func TestNormalizeDeviceNameFixesFirmwareTypo(t *testing.T) {
	for _, name := range []string{"PRO X2 SUPERSTRIIKE", "Logitech X2 SUPERSTRIKE"} {
		if got := normalizeDeviceName(name); got != "PRO X2 Superstrike" {
			t.Fatalf("normalizeDeviceName(%q) = %q", name, got)
		}
	}
}

func TestConnectionTypeDistinguishesReceiverAndCable(t *testing.T) {
	if got := connectionType(hidpp.DeviceIdentity{ProductID: 0xC54D}); got != "wireless" {
		t.Fatalf("receiver connection = %q, want wireless", got)
	}
	if got := connectionType(hidpp.DeviceIdentity{ProductID: 0x40BD}); got != "wireless" {
		t.Fatalf("paired Superstrike endpoint = %q, want wireless", got)
	}
	if got := connectionType(hidpp.DeviceIdentity{ProductID: 0xC09B, PhysicalSlot: 1}); got != "wireless" {
		t.Fatalf("paired child connection = %q, want wireless", got)
	}
	if got := connectionType(hidpp.DeviceIdentity{ProductID: 0xC09B}); got != "wired" {
		t.Fatalf("direct connection = %q, want wired", got)
	}
}

func TestStoredDefaultDPIReturnsEnabledResolutionIndex(t *testing.T) {
	profile := hidpp.Profile{HasDPIStages: true, ResIndex: 1}
	profile.DPIStages[0] = hidpp.DPIStage{Index: 0, X: 800, Y: 800, Enabled: true}
	profile.DPIStages[1] = hidpp.DPIStage{Index: 1, X: 1600, Y: 1600, Enabled: true}
	if got, ok := storedDefaultDPI(profile); !ok || got != 1600 {
		t.Fatalf("storedDefaultDPI() = %d, %v; want 1600, true", got, ok)
	}
	if got, ok := storedDefaultDPIStage(profile); !ok || got != 1 {
		t.Fatalf("storedDefaultDPIStage() = %d, %v; want slot index 1, true", got, ok)
	}
}

func TestStoredDefaultDPIRejectsDisabledStage(t *testing.T) {
	profile := hidpp.Profile{HasDPIStages: true, ResIndex: 2}
	profile.DPIStages[2] = hidpp.DPIStage{Index: 2, X: 3200, Y: 3200, Enabled: false}
	if got, ok := storedDefaultDPI(profile); ok {
		t.Fatalf("storedDefaultDPI() = %d, true; want false", got)
	}
	if got, ok := storedDefaultDPIStage(profile); ok {
		t.Fatalf("storedDefaultDPIStage() = %d, true; want false", got)
	}
}

func TestStoredPreferencesRoundTripExplicitOffValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "openghub", "settings.json")
	want := storedPreferences{Devices: map[string]advancedPreferences{"superstrike/unit-1": {
		GamingSurfaceMode:  intPointer(int(hidpp.GamingSurfaceOff)),
		BhopWindowMS:       intPointer(0),
		WiredReportRate:    intPointer(8000),
		WirelessReportRate: intPointer(2000),
	}}}
	if err := writeStoredPreferences(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := readStoredPreferences(path)
	if err != nil {
		t.Fatal(err)
	}
	advanced := got.Devices["superstrike/unit-1"]
	if advanced.GamingSurfaceMode == nil || *advanced.GamingSurfaceMode != int(hidpp.GamingSurfaceOff) {
		t.Fatalf("surface preference = %v, want off", advanced.GamingSurfaceMode)
	}
	if advanced.BhopWindowMS == nil || *advanced.BhopWindowMS != 0 {
		t.Fatalf("bhop preference = %v, want explicit off", advanced.BhopWindowMS)
	}
	if advanced.WiredReportRate == nil || *advanced.WiredReportRate != 8000 {
		t.Fatalf("wired report rate = %v, want 8000", advanced.WiredReportRate)
	}
	if advanced.WirelessReportRate == nil || *advanced.WirelessReportRate != 2000 {
		t.Fatalf("wireless report rate = %v, want 2000", advanced.WirelessReportRate)
	}
}

func TestStoredPreferencesMigratesLegacySuperstrike(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"superstrike":{"gamingSurfaceMode":4,"bhopWindowMs":400}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := readStoredPreferences(path)
	if err != nil {
		t.Fatal(err)
	}
	advanced, ok := got.Devices["superstrike"]
	if !ok || advanced.GamingSurfaceMode == nil || *advanced.GamingSurfaceMode != 4 || advanced.BhopWindowMS == nil || *advanced.BhopWindowMS != 400 {
		t.Fatalf("migrated preferences = %+v", got)
	}
	if got.Version != storedPreferencesVersion || got.LegacySuperstrike != nil {
		t.Fatalf("migration metadata = %+v", got)
	}
}

func TestPreferenceKeysPreferSerialThenModel(t *testing.T) {
	identity := hidpp.DeviceIdentity{Name: "PRO X2 SUPERSTRIIKE", Serial: "unit-7"}
	session := &DeviceSession{Driver: devices.Match(identity, devices.FeatureSet{}), Identity: identity}
	keys := preferenceLookupKeys(session)
	if len(keys) != 2 || keys[0] != "superstrike/unit-7" || keys[1] != "superstrike" {
		t.Fatalf("preferenceLookupKeys() = %v", keys)
	}
}

func TestStoredAdvancedPreferenceValidation(t *testing.T) {
	for _, mode := range []int{0, 2, 4} {
		if !validStoredSurfaceMode(mode) {
			t.Fatalf("surface mode %d should be valid", mode)
		}
	}
	if validStoredSurfaceMode(1) || validStoredSurfaceMode(3) {
		t.Fatal("noncanonical surface modes should not be persisted")
	}
	for _, window := range []int{0, 100, 400, 1000} {
		if !validStoredBhopWindow(window) {
			t.Fatalf("bhop window %d should be valid", window)
		}
	}
	for _, window := range []int{-1, 50, 105, 1010} {
		if validStoredBhopWindow(window) {
			t.Fatalf("bhop window %d should be invalid", window)
		}
	}
	for _, rate := range hidpp.ReportRates {
		if !validStoredReportRate(rate) {
			t.Fatalf("report rate %d should be valid", rate)
		}
	}
	if validStoredReportRate(333) {
		t.Fatal("unsupported report rate should be invalid")
	}
}
