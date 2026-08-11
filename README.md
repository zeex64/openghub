# openGhub

**An open-source Logitech gaming mouse control app for Linux.** openGhub aims
to provide a polished, native alternative for configuring supported Logitech
mice without G HUB, Wine, a cloud account, or a background daemon.

The project began as a control app for the **Logitech PRO X 2 Superstrike** and
is now being expanded into a capability-driven application for multiple
Logitech gaming mice.

<img width="1408" height="1156" alt="openGhub device control interface" src="https://github.com/user-attachments/assets/e46e654b-2cf5-4313-8e67-e05007876688" />

> 🤖 **Built with AI assistance** — an AI coding assistant helped with parts of
> the app's development and documentation.

## Project status

openGhub is under active development. Device support is added and verified one
model at a time because Logitech mice expose different HID++ features, profile
formats, button layouts, and lighting systems.

| Device | Connection | Status |
| --- | --- | --- |
| Logitech PRO X 2 Superstrike | LIGHTSPEED | **Supported and verified** |
| Logitech G502 SE HERO (`046d:c08b`) | Wired USB | **Planned / in development** |

The interface currently uses Superstrike-specific artwork and controls. As
multi-device support lands, openGhub will select the correct pages, button map,
artwork, and protocol adapter for the connected mouse.

## Current features

The current Superstrike build provides:

- **DPI stages** — configure, enable, select, and persist onboard DPI slots.
- **Polling rate** — select supported USB report rates and store them onboard.
- **Onboard profiles** — inspect, rename, enable, activate, and edit profiles.
- **Button mapping** — assign mouse buttons, keyboard keys, media controls,
  device functions, or disable a control.
- **Device status** — show connection, battery, current DPI, profile, and
  polling information when exposed by the hardware.
- **Superstrike analog controls** — configure click haptics, actuation point,
  and rapid trigger through HID++ feature `0x1B0C`.
- **Interactive interface** — a Wails, React, and Three.js desktop UI with a
  device-specific product view.

Not every feature will be available on every mouse. As the multi-device UI is
introduced, unsupported pages will be hidden. For example, haptics and analog
actuation are Superstrike features and should not appear for a G502 Hero.

## Why this exists

Logitech does not provide G HUB for Linux. Existing community projects such as
[Solaar](https://github.com/pwr-Solaar/Solaar) and
[libratbag](https://github.com/libratbag/libratbag) support many devices, but
newer or device-specific gaming features are not always available through one
consistent interface.

openGhub communicates with compatible devices using **HID++ 2.0** over
`/dev/hidraw`. Depending on the device, settings are applied through live HID++
features, onboard profile memory, or both. Device-specific protocol handling is
kept separate so one mouse's profile layout is never assumed safe for another.

## Install

### Download a release

1. Download the latest binary from the [**Releases**](../../releases) page.
   During the openGhub transition, release binaries may still be named
   `superstrike`.

2. Make the binary executable:

   ```sh
   chmod +x superstrike
   ```

3. Install a udev rule so openGhub can communicate with Logitech HID devices
   without running as root:

   ```sh
   sudo tee /etc/udev/rules.d/70-openghub.rules >/dev/null <<'EOF'
   KERNEL=="hidraw*", SUBSYSTEM=="hidraw", ATTRS{idVendor}=="046d", MODE="0660", TAG+="uaccess"
   SUBSYSTEM=="usb", ATTRS{idVendor}=="046d", MODE="0660", TAG+="uaccess"
   EOF
   sudo udevadm control --reload-rules
   sudo udevadm trigger
   ```

4. Reconnect the mouse, then launch the app:

   ```sh
   ./superstrike
   ```

> The rule must sort before systemd's `73-seat-late.rules`, which converts the
> `uaccess` tag into permission for the signed-in desktop user. The `70-`
> prefix is intentional. Remove older `99-logitech-superstrike.rules` files if
> they are still installed.

The desktop window uses the system WebKitGTK runtime. Install
`libwebkit2gtk-4.1-0` or your distribution's equivalent if it is missing.

### Build from source

Requirements:

- Go 1.26 or newer
- Node.js 22 or newer
- npm
- GTK 3 development files
- WebKitGTK 4.1 development files

```sh
git clone https://github.com/zeex64/linux-superstrike.git
cd linux-superstrike
npm --prefix frontend ci --include=dev
npm --prefix frontend run build
go build -tags "desktop,production,webkit2_41" -o superstrike .
```

Install the current desktop launcher, icon, binary, and udev rule with:

```sh
./packaging/install.sh
```

The repository and executable are being renamed incrementally. Existing
`linux-superstrike`, `superstrike`, and packaging names remain valid during the
transition to openGhub.

If Go is missing, the installer downloads the version declared in `go.mod` to
the repository's local `.tools` directory. It does not replace or modify the
system Go installation.

## How it works

The Go backend discovers Logitech HID++ devices through `/dev/hidraw`, resolves
their runtime HID++ feature table, and serializes device operations. The
multi-device transition is introducing explicit device adapters so the React
frontend can display only the capabilities supported by the selected mouse.

For the PRO X 2 Superstrike, openGhub can:

- read and patch onboard profile sectors;
- preserve unknown profile bytes;
- recompute CRC-16/CCITT checksums;
- verify writes before activating a profile;
- configure five DPI stages, polling rate, profile state, and button mappings;
- access the analog-button controls used for haptics and rapid trigger; and
- measure real input report frequency when the firmware's reported value is
  stale.

Other Logitech mice may expose standard live DPI, report-rate, remappable-key,
lighting, or onboard-profile features with different wire formats. Those paths
must be detected and implemented independently rather than reusing the
Superstrike sector layout.

The desktop shell uses Wails. React and Three.js render the interface, while a
typed Go controller handles connection state and HID++ communication. The
desktop app and diagnostic CLI share the same HID++ implementation.

## Diagnostics

The current binary also provides command-line diagnostics:

```text
superstrike -probe        Device information and complete HID++ feature table
superstrike -profiles     Onboard profile and control-sector summary
superstrike -measurerate  Measure real input reports while moving the mouse
superstrike -scan         List Logitech hidraw nodes and HID++ responses
superstrike -bhop-probe   Conservative Superstrike BunnyHopping feature probe
```

The `-probe` and `-scan` output are the best starting point when requesting a
new device. The BunnyHopping probe is Superstrike-specific and intentionally
avoids the probable setter function.

## Adding another Logitech mouse

Support begins with identifying the exact model and the capabilities reported
by its firmware. Include the following information in a device-support issue:

1. The model name printed on the device.
2. The USB identifier from `lsusb`, such as `046d:c08b`.
3. The complete output from:

   ```sh
   ./superstrike -probe
   ```

4. Whether the mouse is connected by cable, LIGHTSPEED receiver, Bluetooth,
   or more than one of those methods.
5. A `solaar show` report when Solaar detects the device.

Do not test profile writes copied from another model. Profile-memory offsets
that are correct for one Logitech mouse can corrupt the configuration of
another.

## Protocol documentation

[`REVERSE_ENGINEERING.md`](REVERSE_ENGINEERING.md) documents the verified PRO
X 2 Superstrike HID++ features, DPI layouts, onboard-profile memory format,
button assignments, haptics, polling-rate encoding, and diagnostic findings.
As support expands, device-specific notes should be kept clearly separated.

## Credits and disclaimer

Protocol work was informed by the excellent
[Solaar](https://github.com/pwr-Solaar/Solaar) and
[libratbag](https://github.com/libratbag/libratbag) projects.

openGhub is an unofficial community project. It is not affiliated with,
endorsed by, or sponsored by Logitech. Logitech, G HUB, LIGHTSPEED, G502, and
other product names are trademarks of their respective owners.

This software can write to supported devices' onboard memory. Writes are
validated where the format is understood, but experimental device support
should still be used carefully and at your own risk.

## License

[MIT](LICENSE)
