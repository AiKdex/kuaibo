import './style.css'
import './post.css'
import { defineAsyncComponent } from 'vue'
import manifest from './manifest.js'
import { registerTheme } from '../index.js'

const theme = {
  ...manifest,
  entry: defineAsyncComponent(() => import('./EmforumView.vue')),
  // 文章内页交给主题整页渲染（论坛帖子版式，对照 EmForum 视觉身份）
  entries: {
    ...(manifest.entries || {}),
    post: defineAsyncComponent(() => import('./EmforumPostView.vue')),
  },
}

registerTheme(theme)
export default theme
