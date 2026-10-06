import './style.css'
import './post.css'
import { defineAsyncComponent } from 'vue'
import manifest from './manifest.js'
import { registerTheme } from '../index.js'
import VerdantPostView from './VerdantPostView.vue'
import VerdantCatView from './VerdantCatView.vue'
import VerdantAuthorView from './VerdantAuthorView.vue'
import VerdantArchiveView from './VerdantArchiveView.vue'
import VerdantSearchView from './VerdantSearchView.vue'
import VerdantTagView from './VerdantTagView.vue'

const theme = {
  ...manifest,
  entry: defineAsyncComponent(() => import('./VerdantView.vue')),
  entries: {
    ...(manifest.entries || {}),
    post: VerdantPostView,
    cat: VerdantCatView,
    category: VerdantCatView,
    author: VerdantAuthorView,
    archive: VerdantArchiveView,
    search: VerdantSearchView,
    tag: VerdantTagView,
  },
}

registerTheme(theme)
export default theme
