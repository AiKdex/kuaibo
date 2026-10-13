import './style.css'
import MinimalView from './MinimalView.vue'
import manifest from './manifest'
import { registerTheme } from '../index'

const theme = {
  ...manifest,
  entry: MinimalView,
}

registerTheme(theme)

export default theme
