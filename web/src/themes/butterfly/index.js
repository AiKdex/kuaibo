import './style.css'
import './post.css'
import { defineAsyncComponent } from 'vue'
import manifest from './manifest.js'
import { registerTheme } from '../index.js'
import ButterflyPostView from './ButterflyPostView.vue'
import ButterflyCatView from './ButterflyCatView.vue'
import ButterflyAuthorView from './ButterflyAuthorView.vue'
import ButterflyArchiveView from './ButterflyArchiveView.vue'
import ButterflySearchView from './ButterflySearchView.vue'
import ButterflyTagView from './ButterflyTagView.vue'

const theme = {
  ...manifest,
  entry: defineAsyncComponent(() => import('./ButterflyView.vue')),
  entries: {
    ...(manifest.entries || {}),
    post: ButterflyPostView,
    cat: ButterflyCatView,
    category: ButterflyCatView,
    author: ButterflyAuthorView,
    archive: ButterflyArchiveView,
    search: ButterflySearchView,
    tag: ButterflyTagView,
  },
}

registerTheme(theme)
export default theme
