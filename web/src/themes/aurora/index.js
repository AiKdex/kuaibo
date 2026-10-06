import './style.css'
import './post.css'
import { defineAsyncComponent } from 'vue'
import manifest from './manifest.js'
import { registerTheme } from '../index.js'
import AuroraPostView from './AuroraPostView.vue'
import AuroraCatView from './AuroraCatView.vue'
import AuroraAuthorView from './AuroraAuthorView.vue'
import AuroraArchiveView from './AuroraArchiveView.vue'
import AuroraSearchView from './AuroraSearchView.vue'
import AuroraTagView from './AuroraTagView.vue'

const theme = {
  ...manifest,
  entry: defineAsyncComponent(() => import('./AuroraView.vue')),
  entries: {
    ...(manifest.entries || {}),
    post: AuroraPostView,
    cat: AuroraCatView,
    category: AuroraCatView,
    author: AuroraAuthorView,
    archive: AuroraArchiveView,
    search: AuroraSearchView,
    tag: AuroraTagView,
  },
}

registerTheme(theme)

export default theme
