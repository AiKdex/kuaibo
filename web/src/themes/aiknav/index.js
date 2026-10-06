import './style.css'
import './post.css'
import { defineAsyncComponent } from 'vue'
import manifest from './manifest.js'
import { registerTheme } from '../index.js'

const theme = {
  ...manifest,
  entry: defineAsyncComponent(() => import('./AiknavView.vue')),
  entries: {
    post: defineAsyncComponent(() => import('./AiknavPostView.vue')),
    cat: defineAsyncComponent(() => import('./AiknavCatView.vue')),
    author: defineAsyncComponent(() => import('./AiknavAuthorView.vue')),
    tag: defineAsyncComponent(() => import('./AiknavTagView.vue')),
    archive: defineAsyncComponent(() => import('./AiknavArchiveView.vue')),
    search: defineAsyncComponent(() => import('./AiknavSearchView.vue')),
  },
}
registerTheme(theme)
export default theme