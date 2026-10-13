import './style.css'
import DocsView from './DocsView.vue'
import manifest from './manifest'
import { registerTheme } from '../index'

const theme = {
  ...manifest,
  entry: DocsView,
}

registerTheme(theme)

export default theme
