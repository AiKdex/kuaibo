import './style.css'
import './post.css'
import { defineAsyncComponent } from 'vue'
import manifest from './manifest.js'
import { registerTheme } from '../index.js'
import BrutalPostView from './BrutalPostView.vue'
import BrutalCatView from './BrutalCatView.vue'
import BrutalAuthorView from './BrutalAuthorView.vue'
import BrutalArchiveView from './BrutalArchiveView.vue'
import BrutalSearchView from './BrutalSearchView.vue'
import BrutalTagView from './BrutalTagView.vue'

const theme = {
  ...manifest,
  entry: defineAsyncComponent(() => import('./BrutalView.vue')),
  entries: {
    ...(manifest.entries || {}),
    post: BrutalPostView,
    cat: BrutalCatView,
    category: BrutalCatView,
    author: BrutalAuthorView,
    archive: BrutalArchiveView,
    search: BrutalSearchView,
    tag: BrutalTagView,
  },
}

registerTheme(theme)
export default theme
