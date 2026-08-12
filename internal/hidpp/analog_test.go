package hidpp

import "testing"

func TestRapidTriggerEnableBitFromCaptures(t *testing.T) {
	disabled := AnalogConfig{Actuation: 2, RapidTrigger: 2, RapidTriggerEnabled: false, Haptics: 5}
	enabled := disabled
	enabled.RapidTriggerEnabled = true

	disabledWire, err := encodeAnalogConfig(0, disabled)
	if err != nil {
		t.Fatal(err)
	}
	enabledWire, err := encodeAnalogConfig(0, enabled)
	if err != nil {
		t.Fatal(err)
	}
	if got := disabledWire[2]; got != 0x08 {
		t.Fatalf("disabled wire byte = 0x%02x, want capture value 0x08", got)
	}
	if got := enabledWire[2]; got != 0x09 {
		t.Fatalf("enabled wire byte = 0x%02x, want capture value 0x09", got)
	}
}

func TestDecodeRapidTriggerEnableBit(t *testing.T) {
	for _, tt := range []struct {
		wire    byte
		enabled bool
	}{{0x08, false}, {0x09, true}, {0x0D, true}} {
		got, err := decodeAnalogConfig([]byte{0x00, 0x08, tt.wire, 0x14})
		if err != nil {
			t.Fatal(err)
		}
		if got.RapidTriggerEnabled != tt.enabled {
			t.Fatalf("wire 0x%02x enabled=%v, want %v", tt.wire, got.RapidTriggerEnabled, tt.enabled)
		}
	}
}
