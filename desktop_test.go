package main

import (
	"path/filepath"
	"testing"

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

func TestStoredDefaultDPIReturnsEnabledResolutionIndex(t *testing.T) {
	profile := hidpp.Profile{HasDPIStages: true, ResIndex: 1}
	profile.DPIStages[0] = hidpp.DPIStage{Index: 0, X: 800, Y: 800, Enabled: true}
	profile.DPIStages[1] = hidpp.DPIStage{Index: 1, X: 1600, Y: 1600, Enabled: true}
	if got, ok := storedDefaultDPI(profile); !ok || got != 1600 {
		t.Fatalf("storedDefaultDPI() = %d, %v; want 1600, true", got, ok)
	}
}

func TestStoredDefaultDPIRejectsDisabledStage(t *testing.T) {
	profile := hidpp.Profile{HasDPIStages: true, ResIndex: 2}
	profile.DPIStages[2] = hidpp.DPIStage{Index: 2, X: 3200, Y: 3200, Enabled: false}
	if got, ok := storedDefaultDPI(profile); ok {
		t.Fatalf("storedDefaultDPI() = %d, true; want false", got)
	}
}

func TestStoredPreferencesRoundTripExplicitOffValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "openghub", "settings.json")
	want := storedPreferences{Superstrike: advancedPreferences{
		GamingSurfaceMode: intPointer(int(hidpp.GamingSurfaceOff)),
		BhopWindowMS:      intPointer(0),
	}}
	if err := writeStoredPreferences(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := readStoredPreferences(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Superstrike.GamingSurfaceMode == nil || *got.Superstrike.GamingSurfaceMode != int(hidpp.GamingSurfaceOff) {
		t.Fatalf("surface preference = %v, want off", got.Superstrike.GamingSurfaceMode)
	}
	if got.Superstrike.BhopWindowMS == nil || *got.Superstrike.BhopWindowMS != 0 {
		t.Fatalf("bhop preference = %v, want explicit off", got.Superstrike.BhopWindowMS)
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
}
