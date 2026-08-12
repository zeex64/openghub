import topImage from '../assets/devices/g502-se-hero-top.webp'
import sideImage from '../assets/devices/g502-se-hero-side.webp'
import type { CatalogDevice } from './types'

export const g502HeroDevice: CatalogDevice = {
  id: 'g502-se-hero',
  modelIds: ['g502-se-hero'],
  productIds: [0xc08b],
  name: 'G502 HERO / SE',
  family: 'G Series',
  availability: 'supported',
  image: topImage,
  sideImage,
}
