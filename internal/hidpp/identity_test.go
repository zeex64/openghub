package hidpp

import "testing"

func TestDeviceIdentityIncludesReceiverSlot(t *testing.T) {
	d := Device{Path: "/dev/hidraw7", Index: 3, Name: "Mouse", VendorID: 0x046D, ProductID: 0xC08B, Serial: "abc"}
	identity := d.Identity()
	if identity.ID != "046d:c08b:abc:03" {
		t.Fatalf("Identity().ID = %q", identity.ID)
	}
	if identity.Path != d.Path || identity.Index != d.Index || identity.Name != d.Name {
		t.Fatalf("Identity() = %+v", identity)
	}
}

func TestDeviceIdentityUsesPhysicalPathWithoutSerial(t *testing.T) {
	d := Device{Path: "/dev/hidraw7", Index: 1, VendorID: 0x046D, ProductID: 0xC09B, PhysicalID: "usb-0000:01:00.0-2"}
	if got := d.Identity().ID; got != "hid:usb-0000:01:00.0-2:01" {
		t.Fatalf("Identity().ID = %q", got)
	}
}

func TestDeviceIdentityUsesPairedChildSlot(t *testing.T) {
	d := Device{Index: 0xFF, VendorID: 0x046D, ProductID: 0x40BD, PhysicalID: "usb-0000:01:00.0-2", PhysicalSlot: 1}
	if got := d.Identity().ID; got != "hid:usb-0000:01:00.0-2:01" {
		t.Fatalf("Identity().ID = %q", got)
	}
}

func TestNormalizePhysicalIDCollapsesHIDInterfaces(t *testing.T) {
	for _, value := range []string{"usb-0000:01:00.0-2/input0", "usb-0000:01:00.0-2/input3", "usb-0000:01:00.0-2/input2:1"} {
		if got := normalizePhysicalID(value); got != "usb-0000:01:00.0-2" {
			t.Fatalf("normalizePhysicalID(%q) = %q", value, got)
		}
	}
}

func TestPhysicalDeviceSlotReadsPairedChildSuffix(t *testing.T) {
	if got := physicalDeviceSlot("usb-0000:0d:00.0-2.4/input2:1"); got != 1 {
		t.Fatalf("physicalDeviceSlot() = %d", got)
	}
	if got := physicalDeviceSlot("usb-0000:0d:00.0-2.4/input2"); got != 0 {
		t.Fatalf("receiver physicalDeviceSlot() = %d", got)
	}
}
