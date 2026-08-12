package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	linuxoptions "github.com/wailsapp/wails/v2/pkg/options/linux"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/sys/unix"

	"openghub/internal/devices"
	"openghub/internal/hidpp"
)

//go:embed all:frontend/dist
var desktopAssets embed.FS

//go:embed packaging/openghub-icon.svg
var desktopIcon []byte

type DesktopController struct {
	ctx               context.Context
	cancel            context.CancelFunc
	opMu              sync.Mutex
	sessions          map[string]*DeviceSession
	selectedID        string
	perm              bool
	preferences       storedPreferences
	preferencesLoaded bool
}

type DeviceSession struct {
	Device       *hidpp.Device
	Identity     hidpp.DeviceIdentity
	Driver       devices.Driver
	Features     devices.FeatureSet
	Capabilities devices.Capabilities
	Name         string
	Rate         int
	// activeProfileSector remembers the onboard profile that owns the current
	// configuration while the mouse is temporarily in host mode for live DPI.
	// In host mode the firmware reports CurrentProfileSector as 0x0000.
	activeProfileSector int
	bhopWindow          int
	bhopKnown           bool
	bhopPreferred       bool
	surfaceMode         int
	surfaceKnown        bool
	surfacePreferred    bool
	wiredReportRate     int
	wirelessReportRate  int
}

type DeviceState struct {
	Connected             bool                 `json:"connected"`
	Permission            bool                 `json:"permissionDenied"`
	Name                  string               `json:"name"`
	Path                  string               `json:"path"`
	Battery               int                  `json:"battery"`
	Charging              bool                 `json:"charging"`
	HasBattery            bool                 `json:"hasBattery"`
	Profile               string               `json:"profile"`
	DPIX                  int                  `json:"dpiX"`
	DPIY                  int                  `json:"dpiY"`
	PollingRate           int                  `json:"pollingRate"`
	ConfiguredPollingRate int                  `json:"configuredPollingRate"`
	ConnectionType        string               `json:"connectionType"`
	WiredPollingRate      int                  `json:"wiredPollingRate"`
	WirelessPollingRate   int                  `json:"wirelessPollingRate"`
	OnboardModeAvailable  bool                 `json:"onboardModeAvailable"`
	OnboardModeEnabled    bool                 `json:"onboardModeEnabled"`
	DeviceID              string               `json:"deviceId"`
	ModelID               string               `json:"modelId"`
	Capabilities          devices.Capabilities `json:"capabilities"`
}

type DeviceSummaryDTO struct {
	ID               string               `json:"id"`
	ModelID          string               `json:"modelId"`
	Name             string               `json:"name"`
	Path             string               `json:"path"`
	VendorID         uint16               `json:"vendorId"`
	ProductID        uint16               `json:"productId"`
	Serial           string               `json:"serial"`
	Connected        bool                 `json:"connected"`
	Selected         bool                 `json:"selected"`
	Supported        bool                 `json:"supported"`
	PermissionDenied bool                 `json:"permissionDenied"`
	Capabilities     devices.Capabilities `json:"capabilities"`
}

type ProfileDTO struct {
	Index           int                `json:"index"`
	Sector          int                `json:"sector"`
	Enabled         bool               `json:"enabled"`
	Active          bool               `json:"active"`
	Name            string             `json:"name"`
	DPIX            int                `json:"dpiX"`
	DPIY            int                `json:"dpiY"`
	Rate            int                `json:"pollingRate"`
	DPIStages       []DPIStageDTO      `json:"dpiStages"`
	DefaultDPIStage int                `json:"defaultDpiStage"`
	CurrentDPIStage int                `json:"currentDpiStage"`
	HasDPIStages    bool               `json:"hasDpiStages"`
	ButtonMappings  []ProfileButtonDTO `json:"buttonMappings"`
}

type ProfileButtonDTO struct {
	Index      int    `json:"index"`
	Name       string `json:"name"`
	Assignment string `json:"assignment"`
}

type DPIStageDTO struct {
	Index   int  `json:"index"`
	X       int  `json:"x"`
	Y       int  `json:"y"`
	LOD     int  `json:"lod"`
	Enabled bool `json:"enabled"`
}

type ButtonDTO struct {
	Index       int                `json:"index"`
	Name        string             `json:"name"`
	Description string             `json:"description"`
	Action      hidpp.ButtonAction `json:"action"`
}

type ChoiceDTO struct {
	Name string `json:"name"`
	Code uint16 `json:"code"`
}

type ButtonChoicesDTO struct {
	Mouse     []ChoiceDTO `json:"mouse"`
	Keys      []ChoiceDTO `json:"keys"`
	Media     []ChoiceDTO `json:"media"`
	Functions []ChoiceDTO `json:"functions"`
}

type ButtonsPayload struct {
	ProfileName string      `json:"profileName"`
	Sector      int         `json:"sector"`
	Buttons     []ButtonDTO `json:"buttons"`
}

type HapticButtonDTO struct {
	Index               int    `json:"index"`
	Name                string `json:"name"`
	Actuation           int    `json:"actuation"`
	RapidTrigger        int    `json:"rapidTrigger"`
	RapidTriggerEnabled bool   `json:"rapidTriggerEnabled"`
	Haptics             int    `json:"haptics"`
}

type HapticsPayload struct {
	MaxActuation    int               `json:"maxActuation"`
	MaxRapidTrigger int               `json:"maxRapidTrigger"`
	MaxHaptics      int               `json:"maxHaptics"`
	Buttons         []HapticButtonDTO `json:"buttons"`
}

type AdvancedSettingsPayload struct {
	GamingSurfaceAvailable bool `json:"gamingSurfaceAvailable"`
	GamingSurfaceMode      int  `json:"gamingSurfaceMode"`
	BhopAvailable          bool `json:"bhopAvailable"`
	BhopKnown              bool `json:"bhopKnown"`
	BhopWindowMS           int  `json:"bhopWindowMs"`
}

type advancedPreferences struct {
	GamingSurfaceMode  *int `json:"gamingSurfaceMode,omitempty"`
	BhopWindowMS       *int `json:"bhopWindowMs,omitempty"`
	WiredReportRate    *int `json:"wiredReportRate,omitempty"`
	WirelessReportRate *int `json:"wirelessReportRate,omitempty"`
}

type storedPreferences struct {
	Version           int                            `json:"version"`
	Devices           map[string]advancedPreferences `json:"devices"`
	LegacySuperstrike *advancedPreferences           `json:"superstrike,omitempty"`
}

const storedPreferencesVersion = 3

func preferencesPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "openghub", "settings.json"), nil
}

func readStoredPreferences(path string) (storedPreferences, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return storedPreferences{}, err
	}
	var preferences storedPreferences
	if err := json.Unmarshal(data, &preferences); err != nil {
		return storedPreferences{}, err
	}
	if preferences.Devices == nil {
		preferences.Devices = make(map[string]advancedPreferences)
	}
	// Version 1 stored one global Superstrike object. Promote it to the model
	// fallback key so existing installations retain their choices.
	if preferences.LegacySuperstrike != nil {
		if _, exists := preferences.Devices["superstrike"]; !exists {
			preferences.Devices["superstrike"] = *preferences.LegacySuperstrike
		}
		preferences.LegacySuperstrike = nil
	}
	preferences.Version = storedPreferencesVersion
	return preferences, nil
}

func writeStoredPreferences(path string, preferences storedPreferences) error {
	preferences.Version = storedPreferencesVersion
	preferences.LegacySuperstrike = nil
	if preferences.Devices == nil {
		preferences.Devices = make(map[string]advancedPreferences)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(preferences, "", "  ")
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".settings-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(append(data, '\n')); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func validStoredSurfaceMode(mode int) bool {
	return mode == int(hidpp.GamingSurfaceAuto) || mode == int(hidpp.GamingSurfaceOn) || mode == int(hidpp.GamingSurfaceOff)
}

func validStoredBhopWindow(windowMS int) bool {
	return windowMS == 0 || (windowMS >= 100 && windowMS <= 1000 && windowMS%10 == 0)
}

func validStoredReportRate(hz int) bool {
	for _, supported := range hidpp.ReportRates {
		if hz == supported {
			return true
		}
	}
	return false
}

func runDesktop() {
	c := &DesktopController{}
	err := wails.Run(&options.App{
		Title:                    "openGhub",
		Width:                    1320,
		Height:                   860,
		MinWidth:                 1000,
		MinHeight:                700,
		MaxWidth:                 8192,
		BackgroundColour:         &options.RGBA{R: 7, G: 9, B: 13, A: 1},
		AssetServer:              &assetserver.Options{Assets: desktopAssets},
		OnStartup:                c.startup,
		OnShutdown:               c.shutdown,
		Bind:                     []interface{}{c},
		Frameless:                false,
		EnableDefaultContextMenu: false,
		Linux: &linuxoptions.Options{
			Icon:        desktopIcon,
			ProgramName: "openghub",
		},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "desktop:", err)
	}
}

func (c *DesktopController) startup(ctx context.Context) {
	c.ctx, c.cancel = context.WithCancel(ctx)
	go c.refreshLoop()
}

func (c *DesktopController) shutdown(context.Context) {
	if c.cancel != nil {
		c.cancel()
	}
	c.opMu.Lock()
	defer c.opMu.Unlock()
	for id, session := range c.sessions {
		// Onboard/host mode is a user-owned device setting. Closing openGhub must
		// preserve whichever mode is currently selected.
		session.Device.Close()
		delete(c.sessions, id)
	}
}

func (c *DesktopController) refreshLoop() {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	go c.rateLoop()
	for {
		state := c.GetDeviceState()
		deviceList := c.GetDevices()
		if c.ctx != nil {
			runtime.EventsEmit(c.ctx, "device:update", state)
			runtime.EventsEmit(c.ctx, "devices:update", deviceList)
		}
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (c *DesktopController) loadPreferencesLocked() {
	if c.preferencesLoaded {
		return
	}
	c.preferencesLoaded = true
	path, err := preferencesPath()
	if err != nil {
		return
	}
	preferences, err := readStoredPreferences(path)
	if err != nil {
		return
	}
	c.preferences = preferences
}

func (c *DesktopController) applyStoredPreferencesLocked(session *DeviceSession) {
	var advanced advancedPreferences
	keys := preferenceLookupKeys(session)
	for i := len(keys) - 1; i >= 0; i-- {
		key := keys[i]
		if value, exists := c.preferences.Devices[key]; exists {
			if value.GamingSurfaceMode != nil {
				advanced.GamingSurfaceMode = value.GamingSurfaceMode
			}
			if value.BhopWindowMS != nil {
				advanced.BhopWindowMS = value.BhopWindowMS
			}
			if value.WiredReportRate != nil {
				advanced.WiredReportRate = value.WiredReportRate
			}
			if value.WirelessReportRate != nil {
				advanced.WirelessReportRate = value.WirelessReportRate
			}
		}
	}
	if advanced.GamingSurfaceMode != nil && validStoredSurfaceMode(*advanced.GamingSurfaceMode) {
		session.surfaceMode = *advanced.GamingSurfaceMode
		session.surfaceKnown = true
		session.surfacePreferred = true
	}
	if advanced.BhopWindowMS != nil && validStoredBhopWindow(*advanced.BhopWindowMS) {
		session.bhopWindow = *advanced.BhopWindowMS
		session.bhopKnown = true
		session.bhopPreferred = true
	}
	if advanced.WiredReportRate != nil && validStoredReportRate(*advanced.WiredReportRate) {
		session.wiredReportRate = *advanced.WiredReportRate
	}
	if advanced.WirelessReportRate != nil && validStoredReportRate(*advanced.WirelessReportRate) {
		session.wirelessReportRate = *advanced.WirelessReportRate
	}
}

func (c *DesktopController) savePreferencesLocked() error {
	session := c.selectedSessionLocked()
	if session == nil {
		return fmt.Errorf("no mouse selected")
	}
	path, err := preferencesPath()
	if err != nil {
		return err
	}
	var advanced advancedPreferences
	if session.surfacePreferred {
		mode := session.surfaceMode
		advanced.GamingSurfaceMode = &mode
	}
	if session.bhopPreferred {
		window := session.bhopWindow
		advanced.BhopWindowMS = &window
	}
	if validStoredReportRate(session.wiredReportRate) {
		rate := session.wiredReportRate
		advanced.WiredReportRate = &rate
	}
	if validStoredReportRate(session.wirelessReportRate) {
		rate := session.wirelessReportRate
		advanced.WirelessReportRate = &rate
	}
	if c.preferences.Devices == nil {
		c.preferences.Devices = make(map[string]advancedPreferences)
	}
	c.preferences.Version = storedPreferencesVersion
	c.preferences.Devices[preferenceStorageKey(session)] = advanced
	// Transport preferences must follow the physical mouse between its cable
	// endpoint and receiver endpoint, whose sysfs serials may differ.
	modelPreferences := c.preferences.Devices[session.Driver.ID()]
	modelPreferences.WiredReportRate = advanced.WiredReportRate
	modelPreferences.WirelessReportRate = advanced.WirelessReportRate
	c.preferences.Devices[session.Driver.ID()] = modelPreferences
	return writeStoredPreferences(path, c.preferences)
}

func preferenceStorageKey(session *DeviceSession) string {
	modelID := session.Driver.ID()
	if session.Identity.Serial != "" {
		return modelID + "/" + session.Identity.Serial
	}
	return modelID
}

func preferenceLookupKeys(session *DeviceSession) []string {
	exact := preferenceStorageKey(session)
	if exact == session.Driver.ID() {
		return []string{exact}
	}
	return []string{exact, session.Driver.ID()}
}

func connectionType(identity hidpp.DeviceIdentity) string {
	if identity.ProductID == 0xC54D || identity.ProductID == 0x40BD || identity.PhysicalSlot != 0 || strings.Contains(strings.ToUpper(identity.Name), "RECEIVER") {
		return "wireless"
	}
	return "wired"
}

func (c *DesktopController) initializeSessionLocked(device *hidpp.Device) *DeviceSession {
	identity := device.Identity()
	matchIdentity := identity
	if name, err := device.DeviceName(); err == nil && name != "" {
		device.Name = name
		matchIdentity.Name = name
	}
	features := devices.ProbeFeatures(device)
	driver := devices.Match(matchIdentity, features)
	session := &DeviceSession{
		Device:       device,
		Identity:     identity,
		Driver:       driver,
		Features:     features,
		Capabilities: driver.Capabilities(features),
		Name:         driver.DisplayName(),
	}
	if driver.ID() == "unknown-logitech" {
		session.Name = normalizeDeviceName(matchIdentity.Name)
	}
	c.applyStoredPreferencesLocked(session)
	transport := connectionType(identity)
	knownRate := session.wiredReportRate
	if transport == "wireless" {
		knownRate = session.wirelessReportRate
	}
	if validStoredReportRate(knownRate) {
		session.Rate = knownRate
	} else if current, _, err := device.ReportRate(); err == nil && validStoredReportRate(current) {
		// fn2 is cached in some firmware states, so use it only as an initial
		// fallback. The input-event measurement loop replaces it once the mouse
		// produces enough movement reports.
		session.Rate = current
		if transport == "wireless" {
			session.wirelessReportRate = current
		} else {
			session.wiredReportRate = current
		}
	}
	if !session.surfacePreferred && session.Capabilities.GamingSurface {
		if mode, err := device.GamingSurfaceMode(); err == nil {
			session.surfaceMode, session.surfaceKnown = int(mode), true
		}
	}
	if !session.bhopPreferred && session.Capabilities.Bhop {
		if window, err := device.BhopWindow(); err == nil {
			session.bhopWindow, session.bhopKnown = window, true
		}
	}
	mode, modeErr := device.OnboardMode()
	if sector, err := device.CurrentProfileSector(); err == nil && sector != 0 {
		session.activeProfileSector = sector
	}
	if modeErr == nil && mode == hidpp.OnboardModeHost {
		c.applyHostPreferencesToSessionLocked(session)
	}
	return session
}

func (c *DesktopController) syncDevicesLocked() {
	c.loadPreferencesLocked()
	if c.sessions == nil {
		c.sessions = make(map[string]*DeviceSession)
	}
	discovered, perm, _ := hidpp.Discover()
	c.perm = perm
	seen := make(map[string]bool, len(discovered))
	for _, device := range discovered {
		identity := device.Identity()
		seen[identity.ID] = true
		if existing := c.sessions[identity.ID]; existing != nil {
			if _, err := existing.Device.Ping(); err == nil && endpointPriority(existing.Identity) >= endpointPriority(identity) {
				device.Close()
				continue
			}
			existing.Device.Close()
			delete(c.sessions, identity.ID)
		}
		c.sessions[identity.ID] = c.initializeSessionLocked(device)
	}
	for id, session := range c.sessions {
		if seen[id] {
			continue
		}
		if _, err := session.Device.Ping(); err != nil {
			session.Device.Close()
			delete(c.sessions, id)
		}
	}
	if _, ok := c.sessions[c.selectedID]; !ok {
		c.selectedID = ""
	}
	if c.selectedID == "" {
		ids := make([]string, 0, len(c.sessions))
		for id, session := range c.sessions {
			if session.Driver.Supported() {
				ids = append(ids, id)
			}
		}
		sort.Strings(ids)
		if len(ids) > 0 {
			c.selectedID = ids[0]
		}
	}
}

func endpointPriority(identity hidpp.DeviceIdentity) int {
	priority := 0
	if !strings.Contains(strings.ToUpper(identity.Name), "RECEIVER") {
		priority += 10
	}
	if identity.ProductID != 0xC54D {
		priority += 2
	}
	return priority
}

func (c *DesktopController) selectedSessionLocked() *DeviceSession {
	return c.sessions[c.selectedID]
}

func (c *DesktopController) GetDevices() []DeviceSummaryDTO {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	c.syncDevicesLocked()
	out := make([]DeviceSummaryDTO, 0, len(c.sessions))
	for id, session := range c.sessions {
		out = append(out, DeviceSummaryDTO{
			ID: id, ModelID: session.Driver.ID(), Name: session.Name,
			Path: session.Identity.Path, VendorID: session.Identity.VendorID,
			ProductID: session.Identity.ProductID, Serial: session.Identity.Serial,
			Connected: true, Selected: id == c.selectedID, Supported: session.Driver.Supported(),
			Capabilities: session.Capabilities,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Selected != out[j].Selected {
			return out[i].Selected
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func (c *DesktopController) SelectDevice(id string) (DeviceState, error) {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	c.syncDevicesLocked()
	session := c.sessions[id]
	if session == nil {
		return DeviceState{}, fmt.Errorf("mouse is no longer connected")
	}
	if !session.Driver.Supported() {
		return DeviceState{}, fmt.Errorf("%s support is still in development", session.Name)
	}
	c.selectedID = id
	return c.deviceStateLocked(session), nil
}

func normalizeDeviceName(name string) string {
	upper := strings.ToUpper(name)
	if strings.Contains(upper, "SUPERSTRIKE") || strings.Contains(upper, "SUPERSTRIIKE") {
		return "PRO X2 Superstrike"
	}
	return name
}

func (c *DesktopController) ensureDeviceLocked() error {
	if session := c.selectedSessionLocked(); session != nil {
		if _, err := session.Device.Ping(); err == nil {
			return nil
		}
		session.Device.Close()
		delete(c.sessions, c.selectedID)
		c.selectedID = ""
	}
	c.syncDevicesLocked()
	if c.selectedSessionLocked() == nil {
		if c.perm {
			return fmt.Errorf("mouse found, but access was denied; install the udev rule and reconnect it")
		}
		return fmt.Errorf("no supported mouse detected")
	}
	return nil
}

func (c *DesktopController) GetDeviceState() DeviceState {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	if err := c.ensureDeviceLocked(); err != nil {
		return DeviceState{Permission: c.perm}
	}
	return c.deviceStateLocked(c.selectedSessionLocked())
}

func (c *DesktopController) deviceStateLocked(session *DeviceSession) DeviceState {
	device := session.Device
	s := DeviceState{
		Connected: true, Name: session.Name, Path: device.Path, PollingRate: session.Rate,
		DeviceID: session.Identity.ID, ModelID: session.Driver.ID(), Capabilities: session.Capabilities,
	}
	s.ConnectionType = connectionType(session.Identity)
	s.WiredPollingRate = session.wiredReportRate
	s.WirelessPollingRate = session.wirelessReportRate
	if session.Capabilities.OnboardMode {
		s.OnboardModeAvailable = true
		if mode, err := device.OnboardMode(); err == nil {
			s.OnboardModeEnabled = mode == hidpp.OnboardModeOnboard
		}
	}
	if b, err := device.Battery(); err == nil {
		s.Battery, s.Charging, s.HasBattery = b.Percent, b.Charging, b.Available
	}
	if p, err := c.activeProfileLocked(session); err == nil {
		s.Profile, s.DPIX, s.DPIY = p.Name, p.DPIX, p.DPIY
		s.ConfiguredPollingRate = p.ReportRateHz
		if s.Profile == "" {
			s.Profile = fmt.Sprintf("Profile %d", p.Index)
		}
		if s.PollingRate == 0 {
			s.PollingRate = p.ReportRateHz
		}
	}
	if dpi, err := device.CurrentDPI(); err == nil && dpi > 0 {
		s.DPIX, s.DPIY = dpi, dpi
	}
	if s.ConnectionType == "wireless" && s.WirelessPollingRate > 0 {
		s.ConfiguredPollingRate = s.WirelessPollingRate
	} else if s.ConnectionType == "wired" && s.WiredPollingRate > 0 {
		s.ConfiguredPollingRate = s.WiredPollingRate
	}
	return s
}

func (c *DesktopController) GetProfiles() ([]ProfileDTO, error) {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	if err := c.ensureDeviceLocked(); err != nil {
		return nil, err
	}
	session := c.selectedSessionLocked()
	device := session.Device
	profiles, err := session.Driver.Profiles(device)
	if err != nil && len(profiles) == 0 {
		return nil, err
	}
	reported, _ := device.CurrentProfileSector()
	liveDPI, _ := device.CurrentDPI()
	active := resolveActiveProfileSector(reported, session.activeProfileSector, profiles)
	if active != 0 {
		session.activeProfileSector = active
	}
	buttonCount := 5
	if info, infoErr := device.ProfileInfo(); infoErr == nil && info.Buttons > 0 && info.Buttons <= 16 {
		buttonCount = info.Buttons
	}
	out := make([]ProfileDTO, 0, len(profiles))
	for _, p := range profiles {
		stages := make([]DPIStageDTO, 0, len(p.DPIStages))
		if p.HasDPIStages {
			for _, stage := range p.DPIStages {
				lod := int(stage.LOD)
				if !hidpp.ValidDPILOD(byte(lod)) {
					lod = int(hidpp.DPILODMedium)
				}
				x, y := stage.X, stage.Y
				if !stage.Enabled {
					x, y = p.DPIX, p.DPIY
					if x < 100 || x > hidpp.MaxDPI || y < 100 || y > hidpp.MaxDPI {
						x, y = 800, 800
					}
				}
				stages = append(stages, DPIStageDTO{Index: stage.Index, X: x, Y: y, LOD: lod, Enabled: stage.Enabled})
			}
		}
		currentStage := -1
		if p.Sector == active && liveDPI > 0 {
			for _, stage := range p.DPIStages {
				if stage.Enabled && stage.X == liveDPI {
					currentStage = stage.Index
					break
				}
			}
		}
		mappings := make([]ProfileButtonDTO, 0, buttonCount)
		for i := 0; i < buttonCount; i++ {
			mappings = append(mappings, ProfileButtonDTO{Index: i, Name: physicalButtonLabel(i), Assignment: p.Buttons[i].Describe()})
		}
		out = append(out, ProfileDTO{Index: p.Index, Sector: p.Sector, Enabled: p.Enabled, Active: p.Sector == active, Name: p.Name, DPIX: p.DPIX, DPIY: p.DPIY, Rate: p.ReportRateHz, DPIStages: stages, DefaultDPIStage: p.ResIndex, CurrentDPIStage: currentStage, HasDPIStages: p.HasDPIStages, ButtonMappings: mappings})
	}
	return out, nil
}

func (c *DesktopController) UpdateDPIStage(sector, stage, x, y, lod int, enabled, makeDefault bool) (int, error) {
	if x < 100 || x > hidpp.MaxDPI || y < 100 || y > hidpp.MaxDPI {
		return 0, fmt.Errorf("DPI must be between 100 and %d", hidpp.MaxDPI)
	}
	c.opMu.Lock()
	defer c.opMu.Unlock()
	if err := c.ensureDeviceLocked(); err != nil {
		return 0, err
	}
	session := c.selectedSessionLocked()
	if err := c.requireHostModeLocked(session); err != nil {
		return 0, err
	}
	defer c.restoreHostModeLocked(session)
	actual, err := session.Driver.WriteDPIStage(session.Device, sector, stage, x, y, byte(lod), enabled, makeDefault)
	if err != nil {
		return 0, err
	}
	session.activeProfileSector = actual
	return actual, nil
}

func (c *DesktopController) SaveDPIToProfile(sector int, stages []DPIStageDTO, defaultStage, currentStage int) (int, error) {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	if err := c.ensureDeviceLocked(); err != nil {
		return 0, err
	}
	session := c.selectedSessionLocked()
	if session.Driver.ID() != "superstrike" {
		return 0, fmt.Errorf("DPI profile saving is only available for the Superstrike")
	}
	if err := c.requireHostModeLocked(session); err != nil {
		return 0, err
	}
	// A first write may clone the factory profile into RAM. Activating that new
	// RAM sector temporarily enters onboard mode, so always return ownership to
	// the host after the save. Otherwise the next launch merely reports the
	// onboard state accidentally left behind here.
	defer c.restoreHostModeLocked(session)
	converted := make([]hidpp.DPIStage, len(stages))
	for i, stage := range stages {
		converted[i] = hidpp.DPIStage{Index: i, X: stage.X, Y: stage.Y, LOD: byte(stage.LOD), Enabled: stage.Enabled}
	}
	actual, err := session.Driver.SaveDPISettings(session.Device, sector, converted, defaultStage, currentStage)
	if err != nil {
		return 0, err
	}
	session.activeProfileSector = actual
	return actual, nil
}

func (c *DesktopController) SelectDPI(sector, dpi, lod int) error {
	if dpi < 100 || dpi > hidpp.MaxDPI {
		return fmt.Errorf("DPI must be between 100 and %d", hidpp.MaxDPI)
	}
	if sector <= 0 {
		return fmt.Errorf("invalid active profile sector 0x%04X", sector)
	}
	return c.withSession(func(session *DeviceSession) error {
		if session.Driver.ID() != "superstrike" {
			return fmt.Errorf("lift-off distance is only available for the Superstrike")
		}
		if err := session.Device.SelectLiveDPIWithLOD(dpi, byte(lod)); err != nil {
			return err
		}
		session.activeProfileSector = sector
		return nil
	})
}

func (c *DesktopController) UpdateDPI(sector, x, y int) error {
	if x < 100 || x > hidpp.MaxDPI || y < 100 || y > hidpp.MaxDPI {
		return fmt.Errorf("DPI must be between 100 and %d", hidpp.MaxDPI)
	}
	return c.withSession(func(session *DeviceSession) error {
		return session.Driver.WriteResolution(session.Device, sector, x, y)
	})
}

func (c *DesktopController) UpdatePollingRate(sector, hz int) (int, error) {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	if err := c.ensureDeviceLocked(); err != nil {
		return 0, err
	}
	session := c.selectedSessionLocked()
	if err := c.requireHostModeLocked(session); err != nil {
		return 0, err
	}
	defer c.restoreHostModeLocked(session)
	actual, err := session.Driver.SetReportRate(session.Device, sector, hz)
	if err != nil {
		return 0, err
	}
	session.activeProfileSector = actual
	return actual, nil
}

// UpdateTransportPollingRate matches the Superstrike's 0x8061 fn3 behavior:
// the command changes the report rate for the connection currently carrying
// HID++ traffic. Wired and wireless captures use the same command, so an
// inactive transport cannot be programmed until the mouse is connected by it.
func (c *DesktopController) UpdateTransportPollingRate(transport string, hz int) error {
	transport = strings.ToLower(strings.TrimSpace(transport))
	if transport != "wired" && transport != "wireless" {
		return fmt.Errorf("unknown polling-rate transport %q", transport)
	}
	if !validStoredReportRate(hz) {
		return fmt.Errorf("unsupported polling rate %d Hz", hz)
	}
	c.opMu.Lock()
	defer c.opMu.Unlock()
	if err := c.ensureDeviceLocked(); err != nil {
		return err
	}
	session := c.selectedSessionLocked()
	if err := c.requireHostModeLocked(session); err != nil {
		return err
	}
	defer c.restoreHostModeLocked(session)
	activeTransport := connectionType(session.Identity)
	if transport != activeTransport {
		return fmt.Errorf("connect the mouse over %s to change its %s polling rate", transport, transport)
	}
	profile, err := c.activeProfileLocked(session)
	if err != nil {
		return fmt.Errorf("could not identify the active profile: %w", err)
	}
	actualSector, err := session.Driver.SetReportRate(session.Device, profile.Sector, hz)
	if err != nil {
		return fmt.Errorf("save polling rate to the active profile: %w", err)
	}
	session.activeProfileSector = actualSector
	// Reloading a profile applies its persistent rate in onboard mode. Return
	// to host control before issuing the captured transport-specific live write.
	c.restoreHostModeLocked(session)
	if err := session.Device.SetReportRate(hz); err != nil {
		return fmt.Errorf("apply live %s polling rate: %w", transport, err)
	}
	session.Rate = hz
	if transport == "wireless" {
		session.wirelessReportRate = hz
	} else {
		session.wiredReportRate = hz
	}
	if err := c.savePreferencesLocked(); err != nil {
		return fmt.Errorf("polling rate applied, but the preference could not be saved: %w", err)
	}
	return nil
}

func (c *DesktopController) ActivateProfile(sector int) error {
	return c.withSession(func(session *DeviceSession) error {
		if err := session.Driver.SetCurrentProfile(session.Device, sector); err != nil {
			return err
		}
		session.activeProfileSector = sector
		return nil
	})
}

func (c *DesktopController) RenameProfile(sector int, name string) (int, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return 0, fmt.Errorf("profile name cannot be empty")
	}
	c.opMu.Lock()
	defer c.opMu.Unlock()
	if err := c.ensureDeviceLocked(); err != nil {
		return 0, err
	}
	session := c.selectedSessionLocked()
	if err := c.requireHostModeLocked(session); err != nil {
		return 0, err
	}
	defer c.restoreHostModeLocked(session)
	actual, err := session.Driver.SetProfileName(session.Device, sector, name)
	if err != nil {
		return 0, err
	}
	if sector == session.activeProfileSector || sector >= 0x0100 {
		session.activeProfileSector = actual
	}
	return actual, nil
}

func (c *DesktopController) SetProfileEnabled(index int, enabled bool) error {
	return c.withSession(func(session *DeviceSession) error {
		return session.Driver.SetProfileEnabled(session.Device, index, enabled)
	})
}

func (c *DesktopController) GetButtons() (ButtonsPayload, error) {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	if err := c.ensureDeviceLocked(); err != nil {
		return ButtonsPayload{}, err
	}
	session := c.selectedSessionLocked()
	info, _ := session.Device.ProfileInfo()
	p, err := c.activeProfileLocked(session)
	if err != nil {
		return ButtonsPayload{}, err
	}
	count := info.Buttons
	if count < 1 || count > 16 {
		count = 5
	}
	name := p.Name
	if name == "" {
		name = fmt.Sprintf("Profile %d", p.Index)
	}
	out := ButtonsPayload{ProfileName: name, Sector: p.Sector}
	for i := 0; i < count; i++ {
		out.Buttons = append(out.Buttons, ButtonDTO{i, physicalButtonLabel(i), p.Buttons[i].Describe(), p.Buttons[i]})
	}
	return out, nil
}

// activeProfileLocked returns the profile associated with the current live
// settings. The caller must hold opMu. Host-control mode reports sector zero,
// so use the last known onboard sector before falling back to device defaults.
func (c *DesktopController) activeProfileLocked(session *DeviceSession) (hidpp.Profile, error) {
	profiles, err := session.Driver.Profiles(session.Device)
	if err == nil && len(profiles) > 0 {
		reported, _ := session.Device.CurrentProfileSector()
		sector := resolveActiveProfileSector(reported, session.activeProfileSector, profiles)
		for _, profile := range profiles {
			if profile.Sector == sector {
				session.activeProfileSector = sector
				return profile, nil
			}
		}
	}
	p, fallbackErr := session.Device.ActiveProfile()
	if fallbackErr == nil {
		session.activeProfileSector = p.Sector
	}
	return p, fallbackErr
}

func resolveActiveProfileSector(reported, remembered int, profiles []hidpp.Profile) int {
	for _, profile := range profiles {
		if profile.Sector == reported {
			return reported
		}
	}
	for _, profile := range profiles {
		if profile.Sector == remembered {
			return remembered
		}
	}
	for _, profile := range profiles {
		if profile.Enabled {
			return profile.Sector
		}
	}
	if len(profiles) > 0 {
		return profiles[0].Sector
	}
	return 0
}

func storedDefaultDPI(profile hidpp.Profile) (int, bool) {
	stageIndex, ok := storedDefaultDPIStage(profile)
	if !ok {
		return 0, false
	}
	stage := profile.DPIStages[stageIndex]
	if stage.X < 100 || stage.X > hidpp.MaxDPI {
		return 0, false
	}
	return stage.X, true
}

func storedDefaultDPIStage(profile hidpp.Profile) (int, bool) {
	if !profile.HasDPIStages || profile.ResIndex < 0 || profile.ResIndex >= len(profile.DPIStages) {
		return 0, false
	}
	if !profile.DPIStages[profile.ResIndex].Enabled {
		return 0, false
	}
	return profile.ResIndex, true
}

func (c *DesktopController) GetButtonChoices() ButtonChoicesDTO {
	return ButtonChoicesDTO{toChoices(hidpp.MouseButtonChoices), toChoices(hidpp.KeyChoices), toChoices(hidpp.MediaChoices), toChoices(hidpp.FunctionChoices)}
}

func (c *DesktopController) SetButton(sector, index, kind int, code uint16, mods byte) (int, error) {
	if kind < int(hidpp.ButtonDisabled) || kind > int(hidpp.ButtonFunction) {
		return 0, fmt.Errorf("invalid button action")
	}
	a := hidpp.ButtonAction{Kind: hidpp.ButtonKind(kind), Code: code, Mods: mods}
	c.opMu.Lock()
	defer c.opMu.Unlock()
	if err := c.ensureDeviceLocked(); err != nil {
		return 0, err
	}
	session := c.selectedSessionLocked()
	if err := c.requireHostModeLocked(session); err != nil {
		return 0, err
	}
	defer c.restoreHostModeLocked(session)
	actual, err := session.Driver.SetProfileButton(session.Device, sector, index, a)
	if err != nil {
		return 0, err
	}
	session.activeProfileSector = actual
	return actual, nil
}

func (c *DesktopController) GetHaptics() (HapticsPayload, error) {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	if err := c.ensureDeviceLocked(); err != nil {
		return HapticsPayload{}, err
	}
	session := c.selectedSessionLocked()
	if !session.Capabilities.Haptics {
		return HapticsPayload{}, fmt.Errorf("haptics are not available for %s", session.Name)
	}
	caps, err := session.Device.AnalogCaps()
	if err != nil {
		return HapticsPayload{}, err
	}
	out := HapticsPayload{MaxActuation: caps.MaxActuation, MaxRapidTrigger: caps.MaxRapidTrigger, MaxHaptics: caps.MaxHaptics}
	for i := 0; i < caps.Buttons; i++ {
		cfg, err := session.Device.AnalogConfig(i)
		if err != nil {
			return HapticsPayload{}, err
		}
		name := fmt.Sprintf("Button %d", i+1)
		if i < len(hidpp.AnalogButtonNames) {
			name = hidpp.AnalogButtonNames[i]
		}
		out.Buttons = append(out.Buttons, HapticButtonDTO{
			Index:               i,
			Name:                name,
			Actuation:           cfg.Actuation,
			RapidTrigger:        cfg.RapidTrigger,
			RapidTriggerEnabled: cfg.RapidTriggerEnabled,
			Haptics:             cfg.Haptics,
		})
	}
	return out, nil
}

func (c *DesktopController) SetHaptic(index int, field string, value int) error {
	return c.withSession(func(session *DeviceSession) error {
		d := session.Device
		switch field {
		case "haptics":
			return d.SetHaptics(index, value)
		case "actuation":
			return d.SetActuation(index, value)
		case "rapidTrigger":
			return d.SetRapidTrigger(index, value)
		case "rapidTriggerEnabled":
			return d.SetRapidTriggerEnabled(index, value != 0)
		default:
			return fmt.Errorf("unknown haptic setting")
		}
	})
}

func (c *DesktopController) GetAdvancedSettings() (AdvancedSettingsPayload, error) {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	if err := c.ensureDeviceLocked(); err != nil {
		return AdvancedSettingsPayload{}, err
	}
	session := c.selectedSessionLocked()
	device := session.Device

	var out AdvancedSettingsPayload
	if session.Capabilities.GamingSurface {
		out.GamingSurfaceAvailable = true
		if session.surfaceKnown {
			out.GamingSurfaceMode = session.surfaceMode
		} else {
			mode, err := device.GamingSurfaceMode()
			if err != nil {
				return AdvancedSettingsPayload{}, err
			}
			session.surfaceMode, session.surfaceKnown = int(mode), true
			out.GamingSurfaceMode = session.surfaceMode
		}
	}
	if session.Capabilities.Bhop {
		out.BhopAvailable = true
		if !session.bhopKnown {
			if window, err := device.BhopWindow(); err == nil {
				session.bhopWindow, session.bhopKnown = window, true
			}
		}
		out.BhopWindowMS, out.BhopKnown = session.bhopWindow, session.bhopKnown
	}
	return out, nil
}

func (c *DesktopController) SetGamingSurfaceMode(mode int) error {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	if err := c.ensureDeviceLocked(); err != nil {
		return err
	}
	session := c.selectedSessionLocked()
	if err := c.requireHostModeLocked(session); err != nil {
		return err
	}
	if err := session.Device.SetGamingSurfaceMode(hidpp.GamingSurfaceMode(mode)); err != nil {
		return err
	}
	session.surfaceMode, session.surfaceKnown, session.surfacePreferred = mode, true, true
	if err := c.savePreferencesLocked(); err != nil {
		return fmt.Errorf("gaming surface mode applied, but the preference could not be saved: %w", err)
	}
	return nil
}

func (c *DesktopController) SetBhopWindow(windowMS int) error {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	if err := c.ensureDeviceLocked(); err != nil {
		return err
	}
	session := c.selectedSessionLocked()
	if err := c.requireHostModeLocked(session); err != nil {
		return err
	}
	if err := session.Device.SetBhopWindow(windowMS); err != nil {
		return err
	}
	session.bhopWindow, session.bhopKnown, session.bhopPreferred = windowMS, true, true
	if err := c.savePreferencesLocked(); err != nil {
		return fmt.Errorf("bhop mode applied, but the preference could not be saved: %w", err)
	}
	return nil
}

func (c *DesktopController) withSession(fn func(*DeviceSession) error) error {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	if err := c.ensureDeviceLocked(); err != nil {
		return err
	}
	session := c.selectedSessionLocked()
	if err := c.requireHostModeLocked(session); err != nil {
		return err
	}
	defer c.restoreHostModeLocked(session)
	return fn(session)
}

// SetOnboardMode mirrors the official app's profile-page control. Onboard
// mode makes the stored hardware profile authoritative; host mode unlocks
// software-managed settings.
func (c *DesktopController) SetOnboardMode(enabled bool) error {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	if err := c.ensureDeviceLocked(); err != nil {
		return err
	}
	session := c.selectedSessionLocked()
	profileSector := 0
	profileDefaultStage := -1
	if enabled && session.Driver.ID() == "superstrike" {
		profile, err := c.activeProfileLocked(session)
		if err != nil {
			return fmt.Errorf("could not identify the active profile before enabling onboard mode: %w", err)
		}
		profileSector = profile.Sector
		if stage, ok := storedDefaultDPIStage(profile); ok {
			profileDefaultStage = stage
		}
	}
	target := byte(hidpp.OnboardModeHost)
	if enabled {
		target = hidpp.OnboardModeOnboard
	}
	if err := session.Device.SetOnboardMode(target); err != nil {
		return err
	}
	time.Sleep(180 * time.Millisecond)
	mode, err := session.Device.OnboardMode()
	if err != nil {
		return fmt.Errorf("onboard mode changed, but its state could not be verified: %w", err)
	}
	if mode != target {
		return fmt.Errorf("onboard mode did not apply: got 0x%02X, want 0x%02X", mode, target)
	}
	if enabled && profileSector != 0 {
		// Merely changing owners makes some Superstrike firmware restore its
		// stale runtime DPI stage. Reload the profile, then explicitly reset the
		// separate runtime index to byte 1's stored default.
		if err := session.Device.ReloadProfile(profileSector); err != nil {
			return fmt.Errorf("onboard mode enabled, but the active profile could not be reloaded: %w", err)
		}
		time.Sleep(120 * time.Millisecond)
		if profileDefaultStage >= 0 {
			if err := session.Device.SetCurrentDPIIndex(profileDefaultStage); err != nil {
				return fmt.Errorf("onboard mode enabled, but the stored default DPI slot could not be selected: %w", err)
			}
			time.Sleep(40 * time.Millisecond)
		}
		sector, err := session.Device.CurrentProfileSector()
		if err != nil {
			return fmt.Errorf("onboard profile reloaded, but its active sector could not be verified: %w", err)
		}
		if sector != profileSector {
			return fmt.Errorf("onboard profile did not apply: got sector 0x%04X, want 0x%04X", sector, profileSector)
		}
		session.activeProfileSector = profileSector
	}
	if !enabled {
		c.applyHostPreferencesToSessionLocked(session)
	}
	return nil
}

func (c *DesktopController) requireHostModeLocked(session *DeviceSession) error {
	if !session.Capabilities.OnboardMode {
		return nil
	}
	mode, err := session.Device.OnboardMode()
	if err != nil {
		return fmt.Errorf("could not read onboard mode: %w", err)
	}
	if mode == hidpp.OnboardModeOnboard {
		return fmt.Errorf("turn off onboard memory mode in Profiles before changing settings")
	}
	if mode != hidpp.OnboardModeHost {
		return fmt.Errorf("unknown onboard mode 0x%02X", mode)
	}
	return nil
}

func (c *DesktopController) restoreHostModeLocked(session *DeviceSession) {
	if !session.Capabilities.OnboardMode {
		return
	}
	mode, err := session.Device.OnboardMode()
	if err == nil && mode == hidpp.OnboardModeHost {
		return
	}
	if session.Device.SetOnboardMode(hidpp.OnboardModeHost) == nil {
		time.Sleep(180 * time.Millisecond)
		c.applyHostPreferencesToSessionLocked(session)
	}
}

func (c *DesktopController) applyHostPreferencesToSessionLocked(session *DeviceSession) {
	if session.surfacePreferred && session.surfaceKnown && session.Capabilities.GamingSurface {
		_ = session.Device.SetGamingSurfaceMode(hidpp.GamingSurfaceMode(session.surfaceMode))
	}
	if session.bhopPreferred && session.bhopKnown && session.Capabilities.Bhop {
		_ = session.Device.SetBhopWindow(session.bhopWindow)
	}
}

func toChoices(in []hidpp.NamedCode) []ChoiceDTO {
	out := make([]ChoiceDTO, len(in))
	for i, v := range in {
		out[i] = ChoiceDTO{v.Name, v.Code}
	}
	return out
}

func physicalButtonLabel(i int) string {
	names := []string{"Left Click", "Right Click", "Middle / Wheel", "Back", "Forward"}
	if i < len(names) {
		return names[i]
	}
	return fmt.Sprintf("Button %d", i+1)
}

func (c *DesktopController) rateLoop() {
	var f *os.File
	var path string
	var stamps []time.Time
	buf := make([]byte, 64)
	defer func() {
		if f != nil {
			f.Close()
		}
	}()
	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}
		c.opMu.Lock()
		cur := ""
		if session := c.selectedSessionLocked(); session != nil {
			cur = session.Device.Path
		}
		c.opMu.Unlock()
		if cur != path {
			if f != nil {
				f.Close()
				f = nil
			}
			path, stamps = cur, stamps[:0]
			if path != "" {
				f, _ = os.OpenFile(path, os.O_RDONLY, 0)
			}
		}
		if f == nil {
			time.Sleep(time.Second)
			continue
		}
		fds := []unix.PollFd{{Fd: int32(f.Fd()), Events: unix.POLLIN}}
		n, err := unix.Poll(fds, 400)
		if err != nil && err != unix.EINTR {
			f.Close()
			f = nil
			continue
		}
		now := time.Now()
		if n > 0 {
			nr, readErr := f.Read(buf)
			if readErr != nil {
				f.Close()
				f = nil
				continue
			}
			if nr > 0 && buf[0] != hidpp.ReportShort && buf[0] != hidpp.ReportLong && buf[0] != hidpp.ReportVeryLong {
				stamps = append(stamps, now)
			}
		}
		cutoff := now.Add(-time.Second)
		i := 0
		for i < len(stamps) && stamps[i].Before(cutoff) {
			i++
		}
		stamps = stamps[i:]
		if len(stamps) >= 16 {
			diffs := make([]float64, 0, len(stamps)-1)
			for j := 1; j < len(stamps); j++ {
				diffs = append(diffs, stamps[j].Sub(stamps[j-1]).Seconds())
			}
			sort.Float64s(diffs)
			if med := diffs[len(diffs)/2]; med > 0 {
				measured := nearestSupportedRate(1 / med)
				c.opMu.Lock()
				if session := c.selectedSessionLocked(); session != nil && session.Device.Path == path {
					session.Rate = measured
				}
				c.opMu.Unlock()
			}
		}
	}
}

func nearestSupportedRate(hz float64) int {
	best := hidpp.ReportRates[0]
	delta := math.Abs(hz - float64(best))
	for _, rate := range hidpp.ReportRates[1:] {
		if d := math.Abs(hz - float64(rate)); d < delta {
			best, delta = rate, d
		}
	}
	return best
}
