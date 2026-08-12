# Device fixtures

These JSON files are read-only snapshots used to implement and test model
drivers without requiring the physical mouse for every development cycle.

Close the openGhub desktop app before capturing so it cannot consume replies
intended for the diagnostic process. Then run:

```sh
./openghub -capture-fixture testdata/device-fixtures/g502-hero.json -device 046d:c08b
```

Capture never calls a setter or changes onboard/host mode. It records USB and
HID++ identity, the complete feature table, derived capabilities, DPI and
report-rate getters, onboard mode, profile-memory metadata, and every readable
RAM/ROM profile sector. Existing files are never overwritten.

Inspect `identity.path` and `identity.serial` before sharing a fixture.
