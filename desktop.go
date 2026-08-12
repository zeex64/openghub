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

	"openghub/internal/hidpp"
)

//go:embed all:frontend/dist
var desktopAssets embed.FS

//go:embed packaging/openghub-icon.svg
var desktopIcon []byte

type DesktopController struct {
	ctx    context.Context
	cancel context.CancelFunc
	opMu   sync.Mutex
	dev    *hidpp.Device
	name   string
	perm   bool
	rate   int
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
	preferencesLoaded   bool
}

type DeviceState struct {
	Connected             bool   `json:"connected"`
	Permission            bool   `json:"permissionDenied"`
	Name                  string `json:"name"`
	Path                  string `json:"path"`
	Battery               int    `json:"battery"`
	Charging              bool   `json:"charging"`
	HasBattery            bool   `json:"hasBattery"`
	Profile               string `json:"profile"`
	DPIX                  int    `json:"dpiX"`
	DPIY                  int    `json:"dpiY"`
	PollingRate           int    `json:"pollingRate"`
	ConfiguredPollingRate int    `json:"configuredPollingRate"`
	OnboardModeAvailable  bool   `json:"onboardModeAvailable"`
	OnboardModeEnabled    bool   `json:"onboardModeEnabled"`
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
	GamingSurfaceMode *int `json:"gamingSurfaceMode,omitempty"`
	BhopWindowMS      *int `json:"bhopWindowMs,omitempty"`
}

type storedPreferences struct {
	Superstrike advancedPreferences `json:"superstrike"`
}

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
	return preferences, nil
}

func writeStoredPreferences(path string, preferences storedPreferences) error {
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
	if c.dev != nil {
		// Onboard/host mode is a user-owned device setting. Closing openGhub must
		// preserve whichever mode is currently selected.
		c.dev.Close()
		c.dev = nil
	}
}

func (c *DesktopController) refreshLoop() {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	go c.rateLoop()
	for {
		state := c.GetDeviceState()
		if c.ctx != nil {
			runtime.EventsEmit(c.ctx, "device:update", state)
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
	advanced := preferences.Superstrike
	if advanced.GamingSurfaceMode != nil && validStoredSurfaceMode(*advanced.GamingSurfaceMode) {
		c.surfaceMode = *advanced.GamingSurfaceMode
		c.surfaceKnown = true
		c.surfacePreferred = true
	}
	if advanced.BhopWindowMS != nil && validStoredBhopWindow(*advanced.BhopWindowMS) {
		c.bhopWindow = *advanced.BhopWindowMS
		c.bhopKnown = true
		c.bhopPreferred = true
	}
}

func (c *DesktopController) savePreferencesLocked() error {
	path, err := preferencesPath()
	if err != nil {
		return err
	}
	var advanced advancedPreferences
	if c.surfacePreferred {
		mode := c.surfaceMode
		advanced.GamingSurfaceMode = &mode
	}
	if c.bhopPreferred {
		window := c.bhopWindow
		advanced.BhopWindowMS = &window
	}
	return writeStoredPreferences(path, storedPreferences{Superstrike: advanced})
}

func (c *DesktopController) connectLocked() {
	c.loadPreferencesLocked()
	devs, perm, _ := hidpp.Discover()
	c.perm = perm
	if len(devs) == 0 {
		return
	}
	pick := pickDevice(devs)
	for _, d := range devs {
		if d != pick {
			d.Close()
		}
	}
	c.dev = pick
	c.perm = false
	// Read live advanced state before changing the device mode. Both settings
	// are volatile on the Superstrike and a host/onboard transition can restore
	// firmware defaults before the app has had a chance to observe them.
	if !c.surfacePreferred && pick.Has(hidpp.FeatMouseTuning) {
		if mode, err := pick.GamingSurfaceMode(); err == nil {
			c.surfaceMode, c.surfaceKnown = int(mode), true
		}
	}
	if !c.bhopPreferred && pick.Has(hidpp.FeatBunnyHopping) {
		if window, err := pick.BhopWindow(); err == nil {
			c.bhopWindow, c.bhopKnown = window, true
		}
	}
	mode, modeErr := pick.OnboardMode()
	if sector, err := pick.CurrentProfileSector(); err == nil && sector != 0 {
		c.activeProfileSector = sector
	}
	if n, err := pick.DeviceName(); err == nil && n != "" {
		c.name = normalizeDeviceName(n)
	} else {
		c.name = normalizeDeviceName(pick.Name)
	}
	// Host-only choices are restored only while software control is active.
	// Merely opening openGhub must never change the mode selected on the mouse.
	if modeErr == nil && mode == hidpp.OnboardModeHost && c.surfacePreferred && c.surfaceKnown && pick.Has(hidpp.FeatMouseTuning) {
		_ = pick.SetGamingSurfaceMode(hidpp.GamingSurfaceMode(c.surfaceMode))
	}
	if modeErr == nil && mode == hidpp.OnboardModeHost && c.bhopPreferred && c.bhopKnown && pick.Has(hidpp.FeatBunnyHopping) {
		_ = pick.SetBhopWindow(c.bhopWindow)
	}
}

func normalizeDeviceName(name string) string {
	upper := strings.ToUpper(name)
	if strings.Contains(upper, "SUPERSTRIKE") || strings.Contains(upper, "SUPERSTRIIKE") {
		return "PRO X2 Superstrike"
	}
	return name
}

func (c *DesktopController) ensureDeviceLocked() error {
	if c.dev != nil {
		if _, err := c.dev.Ping(); err == nil {
			return nil
		}
		c.dev.Close()
		c.dev = nil
		c.rate = 0
		c.activeProfileSector = 0
		if !c.bhopPreferred {
			c.bhopWindow = 0
			c.bhopKnown = false
		}
		if !c.surfacePreferred {
			c.surfaceMode = 0
			c.surfaceKnown = false
		}
	}
	c.connectLocked()
	if c.dev == nil {
		if c.perm {
			return fmt.Errorf("mouse found, but access was denied; install the udev rule and reconnect it")
		}
		return fmt.Errorf("no Superstrike mouse detected")
	}
	return nil
}

func (c *DesktopController) GetDeviceState() DeviceState {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	if err := c.ensureDeviceLocked(); err != nil {
		return DeviceState{Permission: c.perm}
	}
	s := DeviceState{Connected: true, Name: c.name, Path: c.dev.Path, PollingRate: c.rate}
	if c.dev.Has(hidpp.FeatOnboardProfile) {
		s.OnboardModeAvailable = true
		if mode, err := c.dev.OnboardMode(); err == nil {
			s.OnboardModeEnabled = mode == hidpp.OnboardModeOnboard
		}
	}
	if b, err := c.dev.Battery(); err == nil {
		s.Battery, s.Charging, s.HasBattery = b.Percent, b.Charging, b.Available
	}
	if p, err := c.activeProfileLocked(); err == nil {
		s.Profile, s.DPIX, s.DPIY = p.Name, p.DPIX, p.DPIY
		s.ConfiguredPollingRate = p.ReportRateHz
		if s.Profile == "" {
			s.Profile = fmt.Sprintf("Profile %d", p.Index)
		}
		if s.PollingRate == 0 {
			s.PollingRate = p.ReportRateHz
		}
	}
	if dpi, err := c.dev.CurrentDPI(); err == nil && dpi > 0 {
		s.DPIX, s.DPIY = dpi, dpi
	}
	return s
}

func (c *DesktopController) GetProfiles() ([]ProfileDTO, error) {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	if err := c.ensureDeviceLocked(); err != nil {
		return nil, err
	}
	profiles, err := c.dev.Profiles()
	if err != nil && len(profiles) == 0 {
		return nil, err
	}
	reported, _ := c.dev.CurrentProfileSector()
	liveDPI, _ := c.dev.CurrentDPI()
	active := resolveActiveProfileSector(reported, c.activeProfileSector, profiles)
	if active != 0 {
		c.activeProfileSector = active
	}
	buttonCount := 5
	if info, infoErr := c.dev.ProfileInfo(); infoErr == nil && info.Buttons > 0 && info.Buttons <= 16 {
		buttonCount = info.Buttons
	}
	out := make([]ProfileDTO, 0, len(profiles))
	for _, p := range profiles {
		stages := make([]DPIStageDTO, 0, len(p.DPIStages))
		if p.HasDPIStages {
			for _, stage := range p.DPIStages {
				stages = append(stages, DPIStageDTO{Index: stage.Index, X: stage.X, Y: stage.Y, Enabled: stage.Enabled})
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

func (c *DesktopController) UpdateDPIStage(sector, stage, x, y int, enabled, makeDefault bool) (int, error) {
	if x < 100 || x > hidpp.MaxDPI || y < 100 || y > hidpp.MaxDPI {
		return 0, fmt.Errorf("DPI must be between 100 and %d", hidpp.MaxDPI)
	}
	c.opMu.Lock()
	defer c.opMu.Unlock()
	if err := c.ensureDeviceLocked(); err != nil {
		return 0, err
	}
	if err := c.requireHostModeLocked(); err != nil {
		return 0, err
	}
	defer c.restoreHostModeLocked()
	actual, err := c.dev.WriteProfileDPIStage(sector, stage, x, y, enabled, makeDefault)
	if err != nil {
		return 0, err
	}
	c.activeProfileSector = actual
	return actual, nil
}

func (c *DesktopController) SelectDPI(sector, dpi int) error {
	if dpi < 100 || dpi > hidpp.MaxDPI {
		return fmt.Errorf("DPI must be between 100 and %d", hidpp.MaxDPI)
	}
	if sector <= 0 {
		return fmt.Errorf("invalid active profile sector 0x%04X", sector)
	}
	return c.withDevice(func(d *hidpp.Device) error {
		if err := d.SelectLiveDPI(dpi); err != nil {
			return err
		}
		c.activeProfileSector = sector
		return nil
	})
}

func (c *DesktopController) UpdateDPI(sector, x, y int) error {
	if x < 100 || x > hidpp.MaxDPI || y < 100 || y > hidpp.MaxDPI {
		return fmt.Errorf("DPI must be between 100 and %d", hidpp.MaxDPI)
	}
	return c.withDevice(func(d *hidpp.Device) error { return d.WriteProfileResolution(sector, x, y) })
}

func (c *DesktopController) UpdatePollingRate(sector, hz int) (int, error) {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	if err := c.ensureDeviceLocked(); err != nil {
		return 0, err
	}
	if err := c.requireHostModeLocked(); err != nil {
		return 0, err
	}
	defer c.restoreHostModeLocked()
	actual, err := c.dev.SetProfileReportRateHz(sector, hz)
	if err != nil {
		return 0, err
	}
	c.activeProfileSector = actual
	return actual, nil
}

func (c *DesktopController) ActivateProfile(sector int) error {
	return c.withDevice(func(d *hidpp.Device) error {
		if err := d.SetCurrentProfileSector(sector); err != nil {
			return err
		}
		c.activeProfileSector = sector
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
	if err := c.requireHostModeLocked(); err != nil {
		return 0, err
	}
	defer c.restoreHostModeLocked()
	actual, err := c.dev.SetProfileName(sector, name)
	if err != nil {
		return 0, err
	}
	if sector == c.activeProfileSector || sector >= 0x0100 {
		c.activeProfileSector = actual
	}
	return actual, nil
}

func (c *DesktopController) SetProfileEnabled(index int, enabled bool) error {
	return c.withDevice(func(d *hidpp.Device) error { return d.SetProfileEnabled(index, enabled) })
}

func (c *DesktopController) GetButtons() (ButtonsPayload, error) {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	if err := c.ensureDeviceLocked(); err != nil {
		return ButtonsPayload{}, err
	}
	info, _ := c.dev.ProfileInfo()
	p, err := c.activeProfileLocked()
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
func (c *DesktopController) activeProfileLocked() (hidpp.Profile, error) {
	profiles, err := c.dev.Profiles()
	if err == nil && len(profiles) > 0 {
		reported, _ := c.dev.CurrentProfileSector()
		sector := resolveActiveProfileSector(reported, c.activeProfileSector, profiles)
		for _, profile := range profiles {
			if profile.Sector == sector {
				c.activeProfileSector = sector
				return profile, nil
			}
		}
	}
	p, fallbackErr := c.dev.ActiveProfile()
	if fallbackErr == nil {
		c.activeProfileSector = p.Sector
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
	if !profile.HasDPIStages || profile.ResIndex < 0 || profile.ResIndex >= len(profile.DPIStages) {
		return 0, false
	}
	stage := profile.DPIStages[profile.ResIndex]
	if !stage.Enabled || stage.X < 100 || stage.X > hidpp.MaxDPI {
		return 0, false
	}
	return stage.X, true
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
	if err := c.requireHostModeLocked(); err != nil {
		return 0, err
	}
	defer c.restoreHostModeLocked()
	actual, err := c.dev.SetProfileButton(sector, index, a)
	if err != nil {
		return 0, err
	}
	c.activeProfileSector = actual
	return actual, nil
}

func (c *DesktopController) GetHaptics() (HapticsPayload, error) {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	if err := c.ensureDeviceLocked(); err != nil {
		return HapticsPayload{}, err
	}
	caps, err := c.dev.AnalogCaps()
	if err != nil {
		return HapticsPayload{}, err
	}
	out := HapticsPayload{MaxActuation: caps.MaxActuation, MaxRapidTrigger: caps.MaxRapidTrigger, MaxHaptics: caps.MaxHaptics}
	for i := 0; i < caps.Buttons; i++ {
		cfg, err := c.dev.AnalogConfig(i)
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
	return c.withDevice(func(d *hidpp.Device) error {
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

	var out AdvancedSettingsPayload
	if c.dev.Has(hidpp.FeatMouseTuning) {
		out.GamingSurfaceAvailable = true
		if c.surfaceKnown {
			out.GamingSurfaceMode = c.surfaceMode
		} else {
			mode, err := c.dev.GamingSurfaceMode()
			if err != nil {
				return AdvancedSettingsPayload{}, err
			}
			c.surfaceMode, c.surfaceKnown = int(mode), true
			out.GamingSurfaceMode = c.surfaceMode
		}
	}
	if c.dev.Has(hidpp.FeatBunnyHopping) {
		out.BhopAvailable = true
		if !c.bhopKnown {
			if window, err := c.dev.BhopWindow(); err == nil {
				c.bhopWindow, c.bhopKnown = window, true
			}
		}
		out.BhopWindowMS, out.BhopKnown = c.bhopWindow, c.bhopKnown
	}
	return out, nil
}

func (c *DesktopController) SetGamingSurfaceMode(mode int) error {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	if err := c.ensureDeviceLocked(); err != nil {
		return err
	}
	if err := c.requireHostModeLocked(); err != nil {
		return err
	}
	if err := c.dev.SetGamingSurfaceMode(hidpp.GamingSurfaceMode(mode)); err != nil {
		return err
	}
	c.surfaceMode, c.surfaceKnown, c.surfacePreferred = mode, true, true
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
	if err := c.requireHostModeLocked(); err != nil {
		return err
	}
	if err := c.dev.SetBhopWindow(windowMS); err != nil {
		return err
	}
	c.bhopWindow, c.bhopKnown, c.bhopPreferred = windowMS, true, true
	if err := c.savePreferencesLocked(); err != nil {
		return fmt.Errorf("bhop mode applied, but the preference could not be saved: %w", err)
	}
	return nil
}

func (c *DesktopController) withDevice(fn func(*hidpp.Device) error) error {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	if err := c.ensureDeviceLocked(); err != nil {
		return err
	}
	if err := c.requireHostModeLocked(); err != nil {
		return err
	}
	defer c.restoreHostModeLocked()
	return fn(c.dev)
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
	target := byte(hidpp.OnboardModeHost)
	if enabled {
		target = hidpp.OnboardModeOnboard
	}
	if err := c.dev.SetOnboardMode(target); err != nil {
		return err
	}
	time.Sleep(180 * time.Millisecond)
	mode, err := c.dev.OnboardMode()
	if err != nil {
		return fmt.Errorf("onboard mode changed, but its state could not be verified: %w", err)
	}
	if mode != target {
		return fmt.Errorf("onboard mode did not apply: got 0x%02X, want 0x%02X", mode, target)
	}
	if !enabled {
		c.applyHostPreferencesLocked()
	}
	return nil
}

func (c *DesktopController) requireHostModeLocked() error {
	mode, err := c.dev.OnboardMode()
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

func (c *DesktopController) restoreHostModeLocked() {
	mode, err := c.dev.OnboardMode()
	if err == nil && mode == hidpp.OnboardModeHost {
		return
	}
	if c.dev.SetOnboardMode(hidpp.OnboardModeHost) == nil {
		time.Sleep(180 * time.Millisecond)
		c.applyHostPreferencesLocked()
	}
}

func (c *DesktopController) applyHostPreferencesLocked() {
	if c.surfacePreferred && c.surfaceKnown && c.dev.Has(hidpp.FeatMouseTuning) {
		_ = c.dev.SetGamingSurfaceMode(hidpp.GamingSurfaceMode(c.surfaceMode))
	}
	if c.bhopPreferred && c.bhopKnown && c.dev.Has(hidpp.FeatBunnyHopping) {
		_ = c.dev.SetBhopWindow(c.bhopWindow)
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
		if c.dev != nil {
			cur = c.dev.Path
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
				c.rate = measured
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
