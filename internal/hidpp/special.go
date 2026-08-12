package hidpp

import (
	"fmt"
	"time"
)

// GamingSurfaceMode is the sensor surface-processing mode exposed by
// MOUSE_TUNING (0x2250). The values are the exact wire values used by G HUB.
type GamingSurfaceMode byte

const (
	GamingSurfaceAuto GamingSurfaceMode = 0x00
	GamingSurfaceOn   GamingSurfaceMode = 0x02
	GamingSurfaceOff  GamingSurfaceMode = 0x04
)

func validGamingSurfaceMode(mode GamingSurfaceMode) bool {
	return mode == GamingSurfaceAuto || mode == GamingSurfaceOn || mode == GamingSurfaceOff
}

func decodeGamingSurfaceMode(raw byte) (GamingSurfaceMode, error) {
	switch raw {
	case byte(GamingSurfaceAuto):
		return GamingSurfaceAuto, nil
	case 0x01, byte(GamingSurfaceOn):
		// Some firmware reports the enabled state as 0x01 even though G HUB
		// writes and normally reads back 0x02.
		return GamingSurfaceOn, nil
	case byte(GamingSurfaceOff):
		return GamingSurfaceOff, nil
	default:
		return GamingSurfaceAuto, fmt.Errorf("unknown gaming-surface mode 0x%02x", raw)
	}
}

func gamingSurfaceParams(mode GamingSurfaceMode) ([]byte, error) {
	if !validGamingSurfaceMode(mode) {
		return nil, fmt.Errorf("invalid gaming-surface mode 0x%02x", byte(mode))
	}
	// sensor=0, mode, reserved=0, capability mask=6
	return []byte{0x00, byte(mode), 0x00, 0x06}, nil
}

func (d *Device) mouseTuningIndex() (byte, error) {
	f, err := d.FeatureIndex(FeatMouseTuning)
	if err != nil || f.Index == 0 {
		return 0, fmt.Errorf("MOUSE_TUNING (0x2250) unavailable on this device")
	}
	return f.Index, nil
}

// GamingSurfaceMode reads sensor 0's surface-processing mode. The response is
// [sensor, mode, ...].
func (d *Device) GamingSurfaceMode() (GamingSurfaceMode, error) {
	idx, err := d.mouseTuningIndex()
	if err != nil {
		return GamingSurfaceAuto, err
	}
	r, err := d.Call(idx, 0x00, 0x00)
	if err != nil {
		return GamingSurfaceAuto, err
	}
	if len(r) < 2 {
		return GamingSurfaceAuto, ErrShortRead
	}
	return decodeGamingSurfaceMode(r[1])
}

// SetGamingSurfaceMode applies sensor 0's surface-processing mode. This is a
// live device setting. The getter can briefly return stale state, so read-back
// is best-effort and a successful setter acknowledgement remains authoritative.
func (d *Device) SetGamingSurfaceMode(mode GamingSurfaceMode) error {
	idx, err := d.mouseTuningIndex()
	if err != nil {
		return err
	}
	params, err := gamingSurfaceParams(mode)
	if err != nil {
		return err
	}
	if _, err = d.Call(idx, 0x01, params...); err != nil {
		return err
	}
	for attempt := 0; attempt < 4; attempt++ {
		actual, readErr := d.GamingSurfaceMode()
		if readErr == nil && actual == mode {
			return nil
		}
		if attempt < 3 {
			time.Sleep(time.Duration(25*(1<<attempt)) * time.Millisecond)
		}
	}
	// Other Superstrike getters are known to remain stale after valid writes.
	// The setter acknowledgement proves the command was accepted; do not turn a
	// stale optional read-back into a user-visible write failure.
	return nil
}

func (d *Device) bunnyHoppingIndex() (byte, error) {
	f, err := d.FeatureIndex(FeatBunnyHopping)
	if err != nil || f.Index == 0 {
		return 0, fmt.Errorf("BUNNY_HOPPING (0x80E0) unavailable on this device")
	}
	return f.Index, nil
}

func bhopWireValue(windowMS int) (byte, error) {
	if windowMS == 0 {
		return 0, nil
	}
	if windowMS < 100 || windowMS > 1000 || windowMS%10 != 0 {
		return 0, fmt.Errorf("bhop window must be off (0) or 100–1000 ms in 10 ms steps")
	}
	return byte(windowMS / 10), nil
}

func decodeBhopWindow(r []byte) (int, error) {
	if len(r) < 1 {
		return 0, ErrShortRead
	}
	wire := int(r[0])
	if wire != 0 && (wire < 10 || wire > 100) {
		return 0, fmt.Errorf("unknown bhop window value 0x%02x", r[0])
	}
	return wire * 10, nil
}

// BhopWindow reads the current scroll-filter window with fn1. Zero means the
// feature is disabled; non-zero values are expressed in 10 ms units.
func (d *Device) BhopWindow() (int, error) {
	idx, err := d.bunnyHoppingIndex()
	if err != nil {
		return 0, err
	}
	r, err := d.Call(idx, 0x01)
	if err != nil {
		return 0, err
	}
	return decodeBhopWindow(r)
}

// SetBhopWindow configures the scroll-wheel filter. Zero disables it; enabled
// values are encoded in 10 ms units and written with fn2, as captured from
// G HUB. The setter response echoes the applied wire value.
func (d *Device) SetBhopWindow(windowMS int) error {
	idx, err := d.bunnyHoppingIndex()
	if err != nil {
		return err
	}
	wire, err := bhopWireValue(windowMS)
	if err != nil {
		return err
	}
	r, err := d.Call(idx, 0x02, wire)
	if err != nil {
		return err
	}
	if len(r) < 1 {
		return ErrShortRead
	}
	if r[0] != wire {
		return fmt.Errorf("bhop window did not apply: got %d, want %d", int(r[0])*10, windowMS)
	}
	return nil
}
