import type { DeviceSummary } from '../backend'
import { g502HeroDevice } from './g502Hero'
import { g502XDevice } from './g502X'
import { superstrikeDevice } from './superstrike'
import { matchesCatalogDevice, type CatalogDevice } from './types'

export const deviceCatalog: CatalogDevice[] = [superstrikeDevice, g502HeroDevice, g502XDevice]

export function catalogEntryFor(device: DeviceSummary) {
  return deviceCatalog.find(entry => matchesCatalogDevice(entry, device))
}

export type { CatalogDevice }

