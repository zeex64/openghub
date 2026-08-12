package hidpp

import (
	"fmt"
	"time"
)

type LightingEffect struct {
	Mode       byte `json:"mode"`
	Red        byte `json:"red"`
	Green      byte `json:"green"`
	Blue       byte `json:"blue"`
	PeriodMS   int  `json:"periodMs"`
	Brightness int  `json:"brightness"`
}

var classicProfileLightingRecords = [2][2]int{{1, 3}, {0, 2}}
var classicProfileNormalLightingRecords = [2]int{1, 0}

func DecodeLightingEffect(raw []byte) LightingEffect {
	var effect LightingEffect
	if len(raw) < 11 {
		return effect
	}
	effect.Mode, effect.Red, effect.Green, effect.Blue = raw[0], raw[1], raw[2], raw[3]
	if effect.Mode == 2 {
		effect.PeriodMS = int(raw[4])<<8 | int(raw[5])
		effect.Brightness = int(raw[7])
	}
	if effect.Mode == 3 {
		effect.PeriodMS = int(raw[6])<<8 | int(raw[7])
		effect.Brightness = int(raw[8])
	}
	if effect.Brightness == 0 && effect.Mode != 0 {
		effect.Brightness = 100
	}
	return effect
}

func (effect LightingEffect) bytes() ([11]byte, error) {
	var out [11]byte
	if effect.Mode > 3 {
		return out, fmt.Errorf("unsupported lighting effect %d", effect.Mode)
	}
	out[0] = effect.Mode
	if effect.Mode == 1 || effect.Mode == 2 {
		out[1], out[2], out[3] = effect.Red, effect.Green, effect.Blue
	}
	period := effect.PeriodMS
	if period < 1000 {
		period = 1000
	}
	if period > 60000 {
		period = 60000
	}
	if effect.Mode == 1 {
		out[4] = 0x02
	}
	if effect.Mode == 2 {
		out[4], out[5] = byte(period>>8), byte(period)
	}
	if effect.Mode == 3 {
		out[6], out[7] = byte(period>>8), byte(period)
	}
	brightness := effect.Brightness
	if brightness < 1 {
		brightness = 1
	}
	if brightness > 100 {
		brightness = 100
	}
	if effect.Mode == 2 || effect.Mode == 3 {
		if effect.Mode == 2 {
			out[7] = byte(brightness)
		} else {
			out[8] = byte(brightness)
		}
	}
	return out, nil
}

func (d *Device) SetLightingEffect(zone int, effect LightingEffect) error {
	if zone < 0 || zone > 1 {
		return fmt.Errorf("lighting zone %d is out of range", zone)
	}
	f, err := d.FeatureIndex(FeatRGBEffects)
	if err != nil || f.Index == 0 {
		return fmt.Errorf("ColorLEDEffects (0x8070) unavailable")
	}
	encoded, err := effect.bytes()
	if err != nil {
		return err
	}
	payload := append([]byte{byte(zone)}, encoded[:]...)
	_, err = d.Call(f.Index, 0x03, payload...)
	if err == ErrTimeout {
		return nil
	}
	return err
}

// SetLightingSoftwareControl selects whether the host or the onboard profile
// owns ColorLEDEffects. G HUB claims control with function 8 and [1,1] before
// writing G502 zones; without it one zone can be reapplied by the firmware
// while the other is being changed.
func (d *Device) SetLightingSoftwareControl(enabled bool) error {
	f, err := d.FeatureIndex(FeatRGBEffects)
	if err != nil || f.Index == 0 {
		return fmt.Errorf("ColorLEDEffects (0x8070) unavailable")
	}
	owner := byte(0)
	if enabled {
		owner = 1
	}
	_, err = d.Call(f.Index, 0x08, owner, owner)
	if err == ErrTimeout {
		return nil
	}
	return err
}

func (d *Device) SetClassicProfileLighting(sector, zone int, effect LightingEffect) (int, error) {
	if zone < 0 || zone > 1 {
		return 0, fmt.Errorf("lighting zone %d is out of range", zone)
	}
	encoded, err := effect.bytes()
	if err != nil {
		return 0, err
	}
	liveDPI, _ := d.CurrentDPI()
	// Profile-format-2 stores four effects in this order: logo and primary for
	// normal operation, then logo and primary for power saving. The live 0x8070
	// feature uses the opposite logical order: primary (0), logo (1).
	var profileLighting [2]LightingEffect
	actual, err := d.patchProfileSectorResolved(sector, func(raw []byte) error {
		for _, record := range classicProfileLightingRecords[zone] {
			off := 208 + record*11
			if off+11 > len(raw)-2 {
				return fmt.Errorf("profile sector is too small for lighting zone %d", zone)
			}
			copy(raw[off:off+11], encoded[:])
		}
		for currentZone := 0; currentZone < 2; currentZone++ {
			currentOffset := 208 + classicProfileNormalLightingRecords[currentZone]*11
			profileLighting[currentZone] = DecodeLightingEffect(raw[currentOffset : currentOffset+11])
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	if mode, modeErr := d.OnboardMode(); modeErr == nil && mode != OnboardModeHost {
		if err := d.SetOnboardMode(OnboardModeHost); err != nil {
			return 0, fmt.Errorf("enter host control to apply lighting: %w", err)
		}
		time.Sleep(180 * time.Millisecond)
	}
	if err := d.SetLightingSoftwareControl(true); err != nil {
		return 0, err
	}
	// A profile-sector write can make the firmware reapply both zones. Restore
	// both records explicitly so updating Primary never resets Logo (or vice
	// versa). The untouched zone comes from the same profile copy we preserved.
	for currentZone := 0; currentZone < 2; currentZone++ {
		if err := d.SetLightingEffect(currentZone, profileLighting[currentZone]); err != nil {
			return 0, err
		}
	}
	// Writing/bootstrap-selecting an onboard profile can reset the sensor to
	// that profile's default slot. Restore the value that was live when the user
	// pressed Save so lighting changes never alter sensitivity.
	if liveDPI > 0 {
		if err := d.SetDPI(liveDPI); err != nil {
			return 0, fmt.Errorf("restore DPI after lighting: %w", err)
		}
	}
	return actual, nil
}

func (d *Device) DPILighting() (bool, error) {
	f, err := d.FeatureIndex(FeatLEDControl)
	if err != nil || f.Index == 0 {
		return false, fmt.Errorf("DPI lighting feature unavailable")
	}
	r, err := d.Call(f.Index, 0x06, 0x00)
	return err == nil && len(r) >= 2 && r[1] == 0x02, err
}

func (d *Device) SetDPILighting(enabled bool) error {
	f, err := d.FeatureIndex(FeatLEDControl)
	if err != nil || f.Index == 0 {
		return fmt.Errorf("DPI lighting feature unavailable")
	}
	value := byte(0)
	if enabled {
		value = 2
	}
	_, err = d.Call(f.Index, 0x07, 0x00, value)
	if err == ErrTimeout {
		return nil
	}
	return err
}

// SetDPIIndicator updates the three-bar G502 display after selecting a live
// DPI slot. G HUB sends [0,0,2,0,slot+1] with LEDControl function 5; setting
// sensor DPI alone changes sensitivity but leaves the bar pattern stale.
func (d *Device) SetDPIIndicator(stage int) error {
	if stage < 0 || stage > 4 {
		return fmt.Errorf("DPI indicator stage %d is out of range", stage+1)
	}
	f, err := d.FeatureIndex(FeatLEDControl)
	if err != nil || f.Index == 0 {
		return fmt.Errorf("DPI lighting feature unavailable")
	}
	_, err = d.Call(f.Index, 0x05, 0x00, 0x00, 0x02, 0x00, byte(stage+1))
	if err == ErrTimeout {
		return nil
	}
	return err
}

func (d *Device) StartupLighting() (bool, error) {
	f, err := d.FeatureIndex(FeatRGBEffects)
	if err != nil || f.Index == 0 {
		return false, fmt.Errorf("ColorLEDEffects unavailable")
	}
	r, err := d.Call(f.Index, 0x04, 0x00, 0x01)
	return err == nil && len(r) >= 3 && r[2] == 1, err
}

func (d *Device) SetStartupLighting(enabled bool) error {
	f, err := d.FeatureIndex(FeatRGBEffects)
	if err != nil || f.Index == 0 {
		return fmt.Errorf("ColorLEDEffects unavailable")
	}
	value := byte(0)
	if enabled {
		value = 1
	}
	_, err = d.Call(f.Index, 0x05, 0x00, 0x01, value)
	if err == ErrTimeout {
		return nil
	}
	return err
}
