package hidpp

import "testing"

func TestSideButtonEncoding(t *testing.T) {
	back := (ButtonAction{Kind: ButtonMouse, Code: 0x0008}).encode()
	forward := (ButtonAction{Kind: ButtonMouse, Code: 0x0010}).encode()
	if back != [4]byte{0x80, 0x01, 0x00, 0x08} {
		t.Fatalf("Back encoded as % x", back)
	}
	if forward != [4]byte{0x80, 0x01, 0x00, 0x10} {
		t.Fatalf("Forward encoded as % x", forward)
	}
}

func TestClassicFunctionEncoding(t *testing.T) {
	shift := (ButtonAction{Kind: ButtonFunction, Code: 0x07}).encodeClassic()
	if shift != [4]byte{0x90, 0x07, 0xFF, 0xFF} {
		t.Fatalf("DPI Shift encoded as % x", shift)
	}

	superstrikeShift := (ButtonAction{Kind: ButtonFunction, Code: 0x07}).encode()
	if superstrikeShift != [4]byte{0x90, 0x07, 0xFF, 0x00} {
		t.Fatalf("Superstrike DPI Shift encoded as % x", superstrikeShift)
	}
}
