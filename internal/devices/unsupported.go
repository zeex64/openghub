package devices

import "openghub/internal/hidpp"

type unsupportedDriver struct {
	id         string
	name       string
	productIDs []uint16
}

func (driver unsupportedDriver) ID() string          { return driver.id }
func (driver unsupportedDriver) DisplayName() string { return driver.name }
func (unsupportedDriver) Supported() bool            { return false }

func (driver unsupportedDriver) Matches(identity hidpp.DeviceIdentity, _ FeatureSet) bool {
	for _, productID := range driver.productIDs {
		if identity.ProductID == productID {
			return true
		}
	}
	return false
}

func (unsupportedDriver) Capabilities(features FeatureSet) Capabilities {
	return genericCapabilities(features)
}

func (unsupportedDriver) Profiles(*hidpp.Device) ([]hidpp.Profile, error) {
	return nil, unsupported("onboard profiles")
}
func (unsupportedDriver) WriteDPIStage(*hidpp.Device, int, int, int, int, byte, bool, bool) (int, error) {
	return 0, unsupported("DPI-stage writes")
}
func (unsupportedDriver) SaveDPISettings(*hidpp.Device, int, []hidpp.DPIStage, int, int, int) (int, error) {
	return 0, unsupported("DPI-settings save")
}
func (unsupportedDriver) WriteResolution(*hidpp.Device, int, int, int) error {
	return unsupported("profile DPI writes")
}
func (unsupportedDriver) SetReportRate(*hidpp.Device, int, int) (int, error) {
	return 0, unsupported("profile polling-rate writes")
}
func (unsupportedDriver) SetCurrentProfile(*hidpp.Device, int) error {
	return unsupported("profile selection")
}
func (unsupportedDriver) SetProfileName(*hidpp.Device, int, string) (int, error) {
	return 0, unsupported("profile naming")
}
func (unsupportedDriver) SetProfileEnabled(*hidpp.Device, int, bool) error {
	return unsupported("profile enabling")
}
func (unsupportedDriver) SetProfileButton(*hidpp.Device, int, int, hidpp.ButtonAction) (int, error) {
	return 0, unsupported("button mapping")
}
