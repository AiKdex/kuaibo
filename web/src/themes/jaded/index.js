import './style.css'
import './post.css'
import { defineAsyncComponent } from 'vue'
import manifest from './manifest.js'
import { registerTheme } from '../index.js'

const theme = {
  ...manifest,
  entry: defineAsyncComponent(() => import('./JadedView.vue')),
  entries: {
    post: defineAsyncComponent(() => import('./JadedPostView.vue')),
    cat: defineAsyncComponent(() => import('./JadedCatView.vue')),
    author: defineAsyncComponent(() => import('./JadedAuthorView.vue')),
    tag: defineAsyncComponent(() => import('./JadedTagView.vue')),
    archive: defineAsyncComponent(() => import('./JadedArchiveView.vue')),
    search: defineAsyncComponent(() => import('./JadedSearchView.vue')),
  },
}
registerTheme(theme)
export default theme