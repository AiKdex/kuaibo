import './style.css'
import { defineAsyncComponent } from 'vue'
import manifest from './manifest.js'
import { registerTheme } from '../index.js'

const theme = {
  ...manifest,
  entry: defineAsyncComponent(() => import('./ElevatedView.vue')),
}

registerTheme(theme)
export default theme
