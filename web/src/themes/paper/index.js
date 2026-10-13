import './style.css'
import PaperView from './PaperView.vue'
import manifest from './manifest'
import { registerTheme } from '../index'

const theme = {
  ...manifest,
  entry: PaperView,
}

registerTheme(theme)

export default theme
