import './style.css'
import './post.css'
import { defineAsyncComponent } from 'vue'
import manifest from './manifest.js'
import { registerTheme } from '../index.js'
import BrutalPostView from './BrutalPostView.vue'

const theme = {
  ...manifest,
  entry: defineAsyncComponent(() => import('./BrutalView.vue')),
  // 文章内页：整页交给 BrutalPostView（对照 06-设计稿/design-brutal.html 内页），
  // 数据来自 inject('themeContext').page.post，宿主 BlogPostView 自动分发。
  entries: { ...(manifest.entries || {}), post: BrutalPostView },
}

registerTheme(theme)
export default theme
