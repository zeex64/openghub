import topImage from '../assets/devices/g502-x-top.webp'
import sideImage from '../assets/devices/g502-x-side.webp'
import type { CatalogDevice } from './types'

export const g502XDevice: CatalogDevice = {
  id: 'g502-x',
  modelIds: ['g502-x'],
  productIds: [],
  name: 'G502 X',
  family: 'G Series',
  availability: 'development',
  image: topImage,
  sideImage,
}

