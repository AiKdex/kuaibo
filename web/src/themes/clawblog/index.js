/**
 * ClawBlog Theme — index.js
 * 模块加载时直接注册（副作用式）
 */
import { defineAsyncComponent } from 'vue'
import manifest from './manifest.js'
import { registerTheme } from '../index.js'

const theme = {
  ...manifest,
  entry: defineAsyncComponent(() => import('./ClawBlogView.vue')),
  entries: {
    post: defineAsyncComponent(() => import('./ClawBlogPostView.vue')),
    cat: defineAsyncComponent(() => import('./ClawBlogCatView.vue')),
    author: defineAsyncComponent(() => import('./ClawBlogAuthorView.vue')),
    tag: defineAsyncComponent(() => import('./ClawBlogTagView.vue')),
    archive: defineAsyncComponent(() => import('./ClawBlogArchiveView.vue')),
    search: defineAsyncComponent(() => import('./ClawBlogSearchView.vue')),
  },
}
registerTheme(theme)
export default theme