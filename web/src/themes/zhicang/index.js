import './style.css'
import './post.css'
import { defineAsyncComponent } from 'vue'
import manifest from './manifest.js'
import { registerTheme } from '../index.js'

const theme = {
  ...manifest,
  entry: defineAsyncComponent(() => import('./ZhicangView.vue')),
  entries: {
    post: defineAsyncComponent(() => import('./ZhicangPostView.vue')),
    cat: defineAsyncComponent(() => import('./ZhicangCatView.vue')),
    author: defineAsyncComponent(() => import('./ZhicangAuthorView.vue')),
    tag: defineAsyncComponent(() => import('./ZhicangTagView.vue')),
    archive: defineAsyncComponent(() => import('./ZhicangArchiveView.vue')),
    search: defineAsyncComponent(() => import('./ZhicangSearchView.vue')),
  },
}
registerTheme(theme)
export default theme