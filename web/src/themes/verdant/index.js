import './style.css'
import './post.css'
import { defineAsyncComponent } from 'vue'
import manifest from './manifest.js'
import { registerTheme } from '../index.js'
import VerdantPostView from './VerdantPostView.vue'

const theme = {
  ...manifest,
  entry: defineAsyncComponent(() => import('./VerdantView.vue')),
  // 文章内页：整页交给 VerdantPostView（对照 企业主题设计稿/05-verdant-green.html 内页），
  // 数据来自 inject('themeContext').page.post，宿主 BlogPostView 自动分发；
  // 文章专属样式全部写在 post.css（构建插件加 .th-verdant.th-view-post 作用域，零泄漏）。
  entries: { ...(manifest.entries || {}), post: VerdantPostView },
}

registerTheme(theme)
export default theme
