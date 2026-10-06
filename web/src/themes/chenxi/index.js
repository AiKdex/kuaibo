import './style.css'
import './post.css'
import { defineAsyncComponent } from 'vue'
import manifest from './manifest.js'
import { registerTheme } from '../index.js'

const theme = {
  ...manifest,
  entry: defineAsyncComponent(() => import('./ChenxiView.vue')),
  entries: {
    post: defineAsyncComponent(() => import('./ChenxiPostView.vue')),
    cat: defineAsyncComponent(() => import('./ChenxiCatView.vue')),
    author: defineAsyncComponent(() => import('./ChenxiAuthorView.vue')),
    tag: defineAsyncComponent(() => import('./ChenxiTagView.vue')),
    archive: defineAsyncComponent(() => import('./ChenxiArchiveView.vue')),
    search: defineAsyncComponent(() => import('./ChenxiSearchView.vue')),
  },
}
registerTheme(theme)
export default theme