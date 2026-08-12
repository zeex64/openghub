package hidpp

import "testing"

func fiveByteProfileFixture() []byte {
	raw := make([]byte, 255)
	raw[0], raw[1] = 3, 0
	values := []int{800, 1200, 1600, 2400, 3200}
	for i, dpi := range values {
		off := 3 + i*5
		raw[off] = DPILODMedium
		raw[off+1], raw[off+2] = byte(dpi), byte(dpi>>8)
		raw[off+3], raw[off+4] = byte(dpi), byte(dpi>>8)
	}
	return raw
}

func TestDecodeFiveByteDPIProfile(t *testing.T) {
	raw := fiveByteProfileFixture()
	if !profileUsesFiveByteDPI(raw) {
		t.Fatal("five-byte DPI layout was not detected")
	}
	p := decodeProfile(0x0101, raw)
	if p.DPIX != 800 || p.DPIY != 800 {
		t.Fatalf("active DPI = %d/%d, want 800/800", p.DPIX, p.DPIY)
	}
	want := [5]int{800, 1200, 1600, 2400, 3200}
	if p.DPI != want {
		t.Fatalf("DPI stages = %v, want %v", p.DPI, want)
	}
}

func TestPatchFiveByteDPIUpdatesOnlyDefaultStage(t *testing.T) {
	raw := fiveByteProfileFixture()
	raw[1] = 2
	if err := patchResolution(raw, 1650, 1700); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		off := 3 + i*5
		x := int(raw[off+1]) | int(raw[off+2])<<8
		y := int(raw[off+3]) | int(raw[off+4])<<8
		wantX, wantY := []int{800, 1200, 1650, 2400, 3200}[i], []int{800, 1200, 1700, 2400, 3200}[i]
		if x != wantX || y != wantY || raw[off] != DPILODMedium {
			t.Fatalf("stage %d = %d/%d LOD %02x", i, x, y, raw[off])
		}
	}
}

func TestPatchFiveByteDPIStageEnableAndDefault(t *testing.T) {
	raw := fiveByteProfileFixture()
	if err := patchDPIStage(raw, 3, 2450, 2500, DPILODHigh, true, true); err != nil {
		t.Fatal(err)
	}
	if raw[1] != 3 || raw[3+3*5] != DPILODHigh {
		t.Fatalf("stage 4 was not enabled and selected: index=%d LOD=%02x", raw[1], raw[3+3*5])
	}
	if err := patchDPIStage(raw, 3, 2450, 2500, DPILODHigh, false, false); err != nil {
		t.Fatal(err)
	}
	off := 3 + 3*5
	if raw[off+1] != 0xFF || raw[off+2] != 0xFF || raw[off+3] != 0xFF || raw[off+4] != 0xFF || raw[1] == 3 {
		t.Fatalf("stage 4 was not disabled or default was not moved: index=%d bytes=% x", raw[1], raw[off:off+5])
	}
}

func TestCannotDisableEveryDPIStage(t *testing.T) {
	raw := fiveByteProfileFixture()
	for i := 1; i < 5; i++ {
		if err := patchDPIStage(raw, i, 800, 800, DPILODMedium, false, false); err != nil {
			t.Fatal(err)
		}
	}
	if err := patchDPIStage(raw, 0, 800, 800, DPILODMedium, false, false); err == nil {
		t.Fatal("disabling the final DPI stage should fail")
	}
}

func TestDecodeSuperstrikeCaptureLayoutAndLOD(t *testing.T) {
	raw := make([]byte, 255)
	copy(raw, []byte{0x03, 0x03, 0x01, 0x02, 0x8c, 0x05, 0x8c, 0x05, 0x01, 0xe8, 0x03, 0xe8, 0x03, 0x03, 0x00, 0x0a, 0x00, 0x0a, 0x02, 0xb4, 0x14, 0xb4, 0x14, 0x02, 0x54, 0x24, 0x54, 0x24})
	p := decodeProfile(1, raw)
	if !p.HasDPIStages || p.ResIndex != 3 {
		t.Fatalf("layout/default = %v/%d, want Superstrike/3", p.HasDPIStages, p.ResIndex)
	}
	wantDPI := [5]int{1420, 1000, 2560, 5300, 9300}
	if p.DPI != wantDPI {
		t.Fatalf("DPI stages = %v, want %v", p.DPI, wantDPI)
	}
	wantLOD := [5]byte{DPILODMedium, DPILODLow, DPILODHigh, DPILODMedium, DPILODMedium}
	for i, stage := range p.DPIStages {
		if stage.LOD != wantLOD[i] || !stage.Enabled {
			t.Fatalf("stage %d LOD/enabled = %d/%v, want %d/true", i, stage.LOD, stage.Enabled, wantLOD[i])
		}
	}
}

func TestProfileEnableFlagMatchesCapturedControlTable(t *testing.T) {
	control := make([]byte, 255)
	copy(control, []byte{
		0x00, 0x01, 0x01, 0xFF,
		0x00, 0x02, 0x01, 0xFF,
		0x00, 0x03, 0x01, 0xFF,
		0x00, 0x04, 0x00, 0xFF,
		0x00, 0x05, 0x00, 0xFF,
	})
	if err := patchProfileEnabledControl(control, 3, false); err != nil {
		t.Fatal(err)
	}
	if control[10] != 0x00 || control[6] != 0x01 || control[14] != 0x00 {
		t.Fatalf("disable capture flags = %02x/%02x/%02x, want 01/00/00", control[6], control[10], control[14])
	}
	if err := patchProfileEnabledControl(control, 3, true); err != nil {
		t.Fatal(err)
	}
	if control[10] != 0x01 {
		t.Fatalf("enable capture flag = %02x, want 01", control[10])
	}
}

func TestLegacyDPIProfileRemainsSupported(t *testing.T) {
	raw := make([]byte, 255)
	raw[0], raw[1] = 3, 1
	raw[9], raw[10] = 0x40, 0x06
	raw[11], raw[12] = 0x40, 0x06
	if profileUsesFiveByteDPI(raw) {
		t.Fatal("legacy profile misdetected as five-byte layout")
	}
	if err := patchResolution(raw, 2400, 2500); err != nil {
		t.Fatal(err)
	}
	p := decodeProfile(1, raw)
	if p.DPIX != 2400 || p.DPIY != 2500 {
		t.Fatalf("active DPI = %d/%d, want 2400/2500", p.DPIX, p.DPIY)
	}
}

func TestValidProfileSectorIncludesROM(t *testing.T) {
	for _, sector := range []int{1, 5, 0x0101, 0x0105} {
		if !validProfileSector(sector, 5) {
			t.Fatalf("sector 0x%04x should be valid", sector)
		}
	}
	for _, sector := range []int{0, 6, 0x0100, 0x0106} {
		if validProfileSector(sector, 5) {
			t.Fatalf("sector 0x%04x should be invalid", sector)
		}
	}
}

func TestPatchProfileNameRoundTrips(t *testing.T) {
	raw := make([]byte, 255)
	if err := patchProfileName(raw, "Gaming Profile 🎮"); err != nil {
		t.Fatal(err)
	}
	if got := decodeProfileName(raw); got != "Gaming Profile 🎮" {
		t.Fatalf("decoded name %q, want %q", got, "Gaming Profile 🎮")
	}
}

func TestPatchProfileNameRejectsMoreThan24Characters(t *testing.T) {
	raw := make([]byte, 255)
	if err := patchProfileName(raw, "1234567890123456789012345"); err == nil {
		t.Fatal("expected an overlong profile name to be rejected")
	}
}
