import './style.css'
import './post.css'
import { defineAsyncComponent } from 'vue'
import manifest from './manifest.js'
import { registerTheme } from '../index.js'
import ZirconPostView from './ZirconPostView.vue'
import ZirconCatView from './ZirconCatView.vue'
import ZirconAuthorView from './ZirconAuthorView.vue'
import ZirconArchiveView from './ZirconArchiveView.vue'
import ZirconSearchView from './ZirconSearchView.vue'
import ZirconTagView from './ZirconTagView.vue'

const theme = {
  ...manifest,
  entry: defineAsyncComponent(() => import('./ZirconView.vue')),
  entries: {
    ...(manifest.entries || {}),
    post: ZirconPostView,
    cat: ZirconCatView,
    category: ZirconCatView,
    author: ZirconAuthorView,
    archive: ZirconArchiveView,
    search: ZirconSearchView,
    tag: ZirconTagView,
  },
}

registerTheme(theme)

export default theme
