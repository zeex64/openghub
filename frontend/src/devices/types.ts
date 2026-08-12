import type { DeviceSummary } from '../backend'

export type CatalogDevice = {
  id: string
  modelIds: string[]
  productIds: number[]
  name: string
  family: string
  availability: 'supported' | 'development'
  image?: string
  sideImage?: string
}

export function matchesCatalogDevice(device: CatalogDevice, connected: DeviceSummary) {
  return device.modelIds.includes(connected.modelId) || device.productIds.includes(connected.productId)
}

