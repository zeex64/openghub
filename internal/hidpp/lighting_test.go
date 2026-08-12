package hidpp

import "testing"

func TestCapturedG502LightingEffects(t *testing.T) {
	tests := []struct {
		name   string
		effect LightingEffect
		want   [11]byte
	}{
		{"off", LightingEffect{Mode: 0}, [11]byte{}},
		{"fixed", LightingEffect{Mode: 1, Red: 0x13, Green: 0xff, Blue: 0x00}, [11]byte{1, 0x13, 0xff, 0x00, 2}},
		{"breathing", LightingEffect{Mode: 2, Red: 0x8d, Green: 0x40, Blue: 0xff, PeriodMS: 5000, Brightness: 100}, [11]byte{2, 0x8d, 0x40, 0xff, 0x13, 0x88, 0, 100}},
		{"cycle", LightingEffect{Mode: 3, PeriodMS: 9200, Brightness: 81}, [11]byte{3, 0, 0, 0, 0, 0, 0x23, 0xf0, 81}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := test.effect.bytes()
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("encoded = % x, want % x", got, test.want)
			}
			decoded := DecodeLightingEffect(got[:])
			if decoded.Mode != test.effect.Mode || decoded.Red != test.effect.Red || decoded.Green != test.effect.Green || decoded.Blue != test.effect.Blue {
				t.Fatalf("decoded = %+v, want %+v", decoded, test.effect)
			}
		})
	}
}

func TestClassicProfileLightingRecordOrder(t *testing.T) {
	if got := classicProfileLightingRecords[0]; got != [2]int{1, 3} {
		t.Fatalf("primary records = %v, want normal/power-save records [1 3]", got)
	}
	if got := classicProfileLightingRecords[1]; got != [2]int{0, 2} {
		t.Fatalf("logo records = %v, want normal/power-save records [0 2]", got)
	}
	if got := classicProfileNormalLightingRecords; got != [2]int{1, 0} {
		t.Fatalf("live primary/logo source records = %v, want [1 0]", got)
	}
}
