package hidpp

import "testing"

func TestGamingSurfaceCaptureEncoding(t *testing.T) {
	tests := []struct {
		mode GamingSurfaceMode
		want []byte
	}{
		{GamingSurfaceAuto, []byte{0x00, 0x00, 0x00, 0x06}},
		{GamingSurfaceOn, []byte{0x00, 0x02, 0x00, 0x06}},
		{GamingSurfaceOff, []byte{0x00, 0x04, 0x00, 0x06}},
	}
	for _, tt := range tests {
		got, err := gamingSurfaceParams(tt.mode)
		if err != nil {
			t.Fatal(err)
		}
		for i := range tt.want {
			if got[i] != tt.want[i] {
				t.Fatalf("mode 0x%02x byte %d = 0x%02x, want 0x%02x", byte(tt.mode), i, got[i], tt.want[i])
			}
		}
	}
}

func TestDecodeGamingSurfaceModes(t *testing.T) {
	for _, tt := range []struct {
		raw  byte
		want GamingSurfaceMode
	}{{0x00, GamingSurfaceAuto}, {0x01, GamingSurfaceOn}, {0x02, GamingSurfaceOn}, {0x04, GamingSurfaceOff}} {
		got, err := decodeGamingSurfaceMode(tt.raw)
		if err != nil {
			t.Fatalf("raw 0x%02x: %v", tt.raw, err)
		}
		if got != tt.want {
			t.Fatalf("raw 0x%02x decoded as 0x%02x, want 0x%02x", tt.raw, byte(got), byte(tt.want))
		}
	}
	if _, err := decodeGamingSurfaceMode(0x03); err == nil {
		t.Fatal("expected unknown mode 0x03 to be rejected")
	}
}

func TestBhopCaptureEncoding(t *testing.T) {
	for _, tt := range []struct {
		ms   int
		wire byte
	}{{0, 0x00}, {100, 0x0A}, {400, 0x28}, {1000, 0x64}} {
		got, err := bhopWireValue(tt.ms)
		if err != nil {
			t.Fatalf("%d ms: %v", tt.ms, err)
		}
		if got != tt.wire {
			t.Fatalf("%d ms encoded as 0x%02x, want 0x%02x", tt.ms, got, tt.wire)
		}
	}
	for _, invalid := range []int{-1, 99, 105, 1010} {
		if _, err := bhopWireValue(invalid); err == nil {
			t.Fatalf("expected %d ms to be rejected", invalid)
		}
	}
}

func TestDecodeBhopWindow(t *testing.T) {
	for _, tt := range []struct {
		wire byte
		ms   int
	}{{0x00, 0}, {0x0A, 100}, {0x28, 400}, {0x64, 1000}} {
		got, err := decodeBhopWindow([]byte{tt.wire})
		if err != nil {
			t.Fatalf("wire 0x%02x: %v", tt.wire, err)
		}
		if got != tt.ms {
			t.Fatalf("wire 0x%02x decoded as %d ms, want %d", tt.wire, got, tt.ms)
		}
	}
}
