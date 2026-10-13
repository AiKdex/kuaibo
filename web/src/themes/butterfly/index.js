import './style.css'
import './post.css'
import { defineAsyncComponent } from 'vue'
import manifest from './manifest.js'
import { registerTheme } from '../index.js'
import ButterflyPostView from './ButterflyPostView.vue'

const theme = {
  ...manifest,
  entry: defineAsyncComponent(() => import('./ButterflyView.vue')),
  // 文章内页：整页交给 ButterflyPostView（双栏：正文 + 右侧目录 / 版权卡 / 上下篇 / 相关阅读），
  // 数据来自 inject('themeContext').page.post，宿主 BlogPostView 自动分发；
  // 文章专属样式全部写在 post.css（构建插件加 .th-butterfly.th-view-post 作用域，零泄漏）。
  entries: { ...(manifest.entries || {}), post: ButterflyPostView },
}

registerTheme(theme)
export default theme
