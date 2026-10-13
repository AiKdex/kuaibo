import './style.css'
import './post.css'
import { defineAsyncComponent } from 'vue'
import manifest from './manifest.js'
import { registerTheme } from '../index.js'
import AuroraPostView from './AuroraPostView.vue'

const theme = {
  ...manifest,
  entry: defineAsyncComponent(() => import('./AuroraView.vue')),
  // 文章内页：声明 entries.post 后整页交给 AuroraPostView 渲染，
  // 数据来自 inject('themeContext').page.post，宿主 BlogPostView 自动分发（规范 §4.3B）。
  entries: { ...(manifest.entries || {}), post: AuroraPostView },
}

registerTheme(theme)

export default theme
