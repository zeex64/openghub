export type DeviceState = {
  connected: boolean; permissionDenied: boolean; name: string; path: string
  battery: number; charging: boolean; hasBattery: boolean; profile: string
  dpiX: number; dpiY: number; pollingRate: number; configuredPollingRate: number
  connectionType: 'wired'|'wireless'; wiredPollingRate: number; wirelessPollingRate: number
  onboardModeAvailable: boolean; onboardModeEnabled: boolean
  deviceId: string; modelId: string; capabilities: DeviceCapabilities
}

export type DeviceCapabilities = {
  battery: boolean; dpi: boolean; dpiStages: number; dpiMin: number; dpiMax: number; dpiStep: number; dpiLiftOff: boolean
  pollingRates: number[]; separatePollingRates: boolean
  profiles: boolean; onboardMode: boolean; buttonMapping: boolean
  haptics: boolean; gamingSurface: boolean; bhop: boolean; lighting:boolean; startupEffect:boolean; dpiLighting:boolean
}

export type DeviceSummary = {
  id: string; modelId: string; name: string; path: string
  vendorId: number; productId: number; serial: string
  connected: boolean; selected: boolean; supported: boolean; permissionDenied: boolean
  capabilities: DeviceCapabilities
}

export type Profile = {
  index: number; sector: number; enabled: boolean; active: boolean; name: string
  dpiX: number; dpiY: number; pollingRate: number
  dpiStages: DPIStage[]; defaultDpiStage: number; shiftDpiStage: number; currentDpiStage: number; hasDpiStages: boolean
  buttonMappings: Array<{index: number; name: string; assignment: string}>
  lighting: LightingEffect[]
}
export type LightingEffect = {mode:number;red:number;green:number;blue:number;periodMs:number;brightness:number}
export type DPIStage = { index: number; x: number; y: number; lod: number; enabled: boolean }

export type ButtonAction = { Kind: number; Code: number; Mods: number; Raw: number[] }
export type ButtonPayload = { profileName: string; sector: number; buttons: Array<{index: number; name: string; description: string; action: ButtonAction}> }
export type Choice = { name: string; code: number }
export type Choices = { mouse: Choice[]; keys: Choice[]; media: Choice[]; functions: Choice[] }
export type Haptics = { maxActuation: number; maxRapidTrigger: number; maxHaptics: number; buttons: Array<{index: number; name: string; actuation: number; rapidTrigger: number; rapidTriggerEnabled: boolean; haptics: number}> }
export type AdvancedSettings = { gamingSurfaceAvailable: boolean; gamingSurfaceMode: number; bhopAvailable: boolean; bhopKnown: boolean; bhopWindowMs: number; startupEffectAvailable:boolean;startupEffectEnabled:boolean;dpiLightingAvailable:boolean;dpiLightingEnabled:boolean }

declare global {
  interface Window {
    go?: { main?: { DesktopController?: Record<string, (...args: unknown[]) => Promise<unknown>> } }
    runtime?: { EventsOn?: (name: string, callback: (data: unknown) => void) => () => void }
  }
}

const emptyCapabilities: DeviceCapabilities = {battery:false,dpi:false,dpiStages:0,dpiMin:100,dpiMax:44000,dpiStep:50,dpiLiftOff:false,pollingRates:[],separatePollingRates:false,profiles:false,onboardMode:false,buttonMapping:false,haptics:false,gamingSurface:false,bhop:false,lighting:false,startupEffect:false,dpiLighting:false}
const demoState: DeviceState = { connected: false, permissionDenied: false, name: '', path: '', battery: 0, charging: false, hasBattery: false, profile: '', dpiX: 0, dpiY: 0, pollingRate: 0, configuredPollingRate: 0, connectionType:'wired', wiredPollingRate:0, wirelessPollingRate:0, onboardModeAvailable: false, onboardModeEnabled: false, deviceId:'', modelId:'', capabilities:emptyCapabilities }

async function invoke<T>(method: string, ...args: unknown[]): Promise<T> {
  const fn = window.go?.main?.DesktopController?.[method]
  if (!fn) {
    if (method === 'GetDeviceState') return demoState as T
    throw new Error('Desktop bridge is unavailable. Run this interface through Wails.')
  }
  return fn(...args) as Promise<T>
}

export const api = {
  state: () => invoke<DeviceState>('GetDeviceState'),
  devices: () => invoke<DeviceSummary[]>('GetDevices'),
  selectDevice: (id: string) => invoke<DeviceState>('SelectDevice', id),
  profiles: () => invoke<Profile[]>('GetProfiles'),
  updateDPI: (sector: number, x: number, y: number) => invoke<void>('UpdateDPI', sector, x, y),
  updateDPIStage: (sector: number, stage: number, x: number, y: number, lod: number, enabled: boolean, makeDefault: boolean) => invoke<number>('UpdateDPIStage', sector, stage, x, y, lod, enabled, makeDefault),
  saveDPIToProfile: (sector: number, stages: DPIStage[], defaultStage: number, currentStage: number, shiftStage: number) => invoke<number>('SaveDPIToProfile', sector, stages, defaultStage, currentStage, shiftStage),
  selectDPI: (sector: number, stage: number, dpi: number, lod: number) => invoke<void>('SelectDPI', sector, stage, dpi, lod),
  updateRate: (sector: number, rate: number) => invoke<number>('UpdatePollingRate', sector, rate),
  updateTransportRate: (transport: 'wired'|'wireless', rate: number) => invoke<void>('UpdateTransportPollingRate', transport, rate),
  activate: (sector: number) => invoke<void>('ActivateProfile', sector),
  rename: (sector: number, name: string) => invoke<number>('RenameProfile', sector, name),
  enable: (index: number, enabled: boolean) => invoke<void>('SetProfileEnabled', index, enabled),
  buttons: () => invoke<ButtonPayload>('GetButtons'),
  choices: () => invoke<Choices>('GetButtonChoices'),
  setButton: (sector: number, index: number, kind: number, code: number, mods: number) => invoke<number>('SetButton', sector, index, kind, code, mods),
  haptics: () => invoke<Haptics>('GetHaptics'),
  setHaptic: (index: number, field: string, value: number) => invoke<void>('SetHaptic', index, field, value),
  advanced: () => invoke<AdvancedSettings>('GetAdvancedSettings'),
  setGamingSurface: (mode: number) => invoke<void>('SetGamingSurfaceMode', mode),
  setBhop: (windowMs: number) => invoke<void>('SetBhopWindow', windowMs),
  setOnboardMode: (enabled: boolean) => invoke<void>('SetOnboardMode', enabled),
  setLighting: (sector:number,zone:number,effect:LightingEffect) => invoke<number>('SetLighting',sector,zone,effect.mode,effect.red,effect.green,effect.blue,effect.periodMs,effect.brightness),
  setStartupEffect: (enabled:boolean) => invoke<void>('SetStartupEffect',enabled),
  setDPILighting: (enabled:boolean) => invoke<void>('SetDPILighting',enabled),
}

export function onDeviceUpdate(callback: (state: DeviceState) => void) {
  return window.runtime?.EventsOn?.('device:update', data => callback(data as DeviceState))
}

export function onDevicesUpdate(callback: (devices: DeviceSummary[]) => void) {
  return window.runtime?.EventsOn?.('devices:update', data => callback(data as DeviceSummary[]))
}
