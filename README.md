# openGhub

openGhub is an open-source Logitech gaming mouse control application for
Linux. It provides a native desktop interface for configuring supported mice
without G HUB, Wine, a cloud account, or a background daemon.

The application communicates directly with Logitech devices using HID++ over
Linux `hidraw`. Its backend is capability-driven and uses a separate model
driver for behavior that is not safe to share between mice.

<img width="1408" height="1156" alt="openGhub device control interface" src="https://github.com/user-attachments/assets/e46e654b-2cf5-4313-8e67-e05007876688" />

> openGhub is under active development. Back up important onboard profiles
> before testing development builds or newly added device support.

## Device support

| Device | USB ID | Connection | Status |
| --- | --- | --- | --- |
| Logitech PRO X 2 Superstrike | `046d:c54d` receiver / `046d:40bd` paired endpoint | LIGHTSPEED | Supported and hardware-tested |
| Logitech G502 HERO / SE | `046d:c08b` | Wired USB | Supported from USB captures; hardware verification in progress |
| Logitech G502 X | To be verified | Wired USB | Catalog entry; support planned |

Connected devices appear first in the device library. openGhub distinguishes
separate physical mice while merging duplicate receiver and paired-child
interfaces exposed by LIGHTSPEED devices.

Pages are selected from the capabilities reported by the active mouse. A model
without analog switches, for example, will not be shown the Superstrike
Haptics page.

## Superstrike features

The current Superstrike driver supports:

- Five onboard DPI stages, including enable state and default-stage selection
- Onboard polling-rate configuration from 125 Hz through 8000 Hz
- Profile inspection, naming, enabling, selection, and persistence
- Mouse, keyboard, media, and device-function button assignments
- Battery, active DPI, profile, and measured polling-rate status
- Click haptics, actuation point, rapid-trigger sensitivity, and rapid trigger
- Gaming Surface Auto, On, and Off modes
- Configurable or disabled Bhop scroll filtering
- Explicit onboard-memory mode matching the official application

When onboard-memory mode is enabled, the stored hardware profile controls the
mouse and openGhub locks its setting editors. Disable onboard mode from the
Profiles page to return to software control and edit settings.

## G502 HERO / SE features

The G502 driver supports its five onboard profiles, five DPI stages from 100
through 25600 DPI, default and active DPI selection, 125/250/500/1000 Hz report
rates, onboard-memory mode, profile naming/enabling/selection, and all eleven
stored button assignments. Primary/logo RGB effects, DPI indicator lighting,
and the device-startup lighting effect are also exposed. The implementation uses the G502's profile-format-2
layout and never applies the Superstrike's extended profile offsets to it.

## Architecture

openGhub keeps generic protocol handling separate from model-specific formats:

```text
Linux hidraw transport
└── HID++ feature implementation
    ├── device discovery and identity
    ├── battery, DPI, report rate, and onboard mode
    └── model registry
        ├── PRO X 2 Superstrike driver
        ├── G502 HERO / SE profile-format-2 driver
        └── unsupported-device fallback
```

Each connected endpoint receives its own session containing its identity,
feature table, capabilities, selected profile, measured report rate, and
device-specific preferences. The frontend catalog is also split into per-model
definitions so artwork and presentation metadata do not accumulate in one
component.

Superstrike profile writes patch the existing sector in place, preserve unknown
bytes, recompute its CRC-16/CCITT checksum, and verify the result. A profile
layout is never reused for another model without hardware verification.

The desktop application is built with Go, Wails, React, TypeScript, and
Three.js.

## Installation

### Release binary

1. Download `openghub` from the [Releases](../../releases) page.
2. Make it executable:

   ```sh
   chmod +x openghub
   ```

3. Install the udev rule so the signed-in desktop user can access Logitech HID
   devices:

   ```sh
   sudo tee /etc/udev/rules.d/70-openghub.rules >/dev/null <<'EOF'
   KERNEL=="hidraw*", SUBSYSTEM=="hidraw", ATTRS{idVendor}=="046d", MODE="0660", TAG+="uaccess"
   EOF
   sudo udevadm control --reload-rules
   sudo udevadm trigger
   ```

4. Reconnect the mouse and launch openGhub:

   ```sh
   ./openghub
   ```

The `70-` prefix is intentional. It places the rule before systemd's
`73-seat-late.rules`, allowing the `uaccess` tag to become a per-user ACL.
Remove obsolete `70-logitech-superstrike.rules` or
`99-logitech-superstrike.rules` files left by older builds.

The desktop application requires the system GTK 3 and WebKitGTK 4.1 runtime.
Package names vary by distribution; Debian-based systems commonly provide
WebKit through `libwebkit2gtk-4.1-0`.

### Build from source

Requirements:

- Go 1.26.3 or a compatible newer toolchain
- Node.js 22 or newer
- npm
- GTK 3 and WebKitGTK 4.1 development packages

```sh
git clone https://github.com/zeex64/linux-superstrike.git
cd linux-superstrike
npm --prefix frontend ci --include=dev
npm --prefix frontend run build
go build -tags "desktop,production,webkit2_41" -o openghub .
```

To build and install the binary, desktop launcher, icon, and udev rule for the
current user:

```sh
./packaging/install.sh
```

The installer uses the system Go toolchain when available. If Go is missing,
it downloads the version declared in `go.mod` into the repository's `.tools`
directory without modifying the system installation or shell profile.

The repository retains its historical `linux-superstrike` name. The
application, executable, launcher, configuration directory, and package
metadata use `openGhub`.

## Development

Build and verify the project with:

```sh
npm --prefix frontend run build
go test -tags desktop ./...
go build -tags "desktop,production,webkit2_41" -o openghub .
```

Important directories:

```text
frontend/src/devices/       Per-model frontend catalog definitions
internal/devices/           Model registry, drivers, and fixture schema
internal/hidpp/             Generic HID++ transport and features
packaging/                  Desktop launcher, icon, installer, and udev rule
testdata/device-fixtures/   Read-only device snapshots for driver development
```

Runtime preferences are stored in:

```text
~/.config/openghub/settings.json
```

Preferences are isolated by model and, when available, device serial number.
Older global Superstrike preferences are migrated automatically.

## Device diagnostics

The application binary includes headless diagnostic commands:

```text
openghub -scan          List Logitech hidraw nodes and HID++ responses
openghub -probe         Print device information and the HID++ feature table
openghub -profile       Dump the active onboard profile
openghub -profiles      List onboard profiles and control sectors
openghub -measurerate   Measure real input reports while moving the mouse
openghub -bhop-probe    Run the read-only Superstrike Bhop probe
```

### Capture a device fixture

A fixture contains the device identity, complete feature table, derived
capabilities, DPI and report-rate responses, onboard state, profile-memory
metadata, and every readable raw RAM/ROM sector.

Close the openGhub desktop application before capturing so it cannot consume
responses intended for the diagnostic process. Then run:

```sh
./openghub \
  -capture-fixture testdata/device-fixtures/g502-hero.json \
  -device 046d:c08b
```

Fixture capture is read-only: it uses getters and raw memory reads, never calls
a setter, and never changes onboard/host mode. It also refuses to overwrite an
existing file. Inspect the generated path and serial fields before sharing or
committing it.

The `-device` selector accepts a model ID, USB ID, device name, or explicit
`/dev/hidrawN` path.

## Adding another mouse

Device support starts with captured hardware data, not assumptions from a
similar model. For a new mouse:

1. Record its exact name and connection type.
2. Record its USB ID from `lsusb`.
3. Capture a read-only fixture.
4. Add a backend model driver and frontend catalog definition.
5. Implement and test only the capabilities confirmed by the device.
6. Verify every write on hardware before marking the driver supported.

Never test profile-memory offsets copied from another model. Incorrect offsets
can corrupt onboard configuration.

## Protocol notes

[REVERSE_ENGINEERING.md](REVERSE_ENGINEERING.md) documents the verified
Superstrike HID++ feature table, profile layouts, button assignments, haptics,
polling-rate encoding, Gaming Surface behavior, Bhop behavior, and packet
capture findings.

Protocol work was informed by the excellent
[Solaar](https://github.com/pwr-Solaar/Solaar) and
[libratbag](https://github.com/libratbag/libratbag) projects.

## Disclaimer

openGhub is an unofficial community project. It is not affiliated with,
endorsed by, or sponsored by Logitech. Logitech, G HUB, LIGHTSPEED, G502, and
other product names are trademarks of their respective owners.

This project was developed with AI assistance. All device support should still
be reviewed and verified against physical hardware and captured protocol data.

## License

[MIT](LICENSE)
