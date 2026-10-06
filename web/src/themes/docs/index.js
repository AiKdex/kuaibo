import { defineAsyncComponent } from 'vue'
import './style.css'
import DocsView from './DocsView.vue'
import manifest from './manifest'
import { registerTheme } from '../index'

const PostView = defineAsyncComponent(() => import('./DocsPostView.vue'))
const CatView = defineAsyncComponent(() => import('./DocsCatView.vue'))
const AuthorView = defineAsyncComponent(() => import('./DocsAuthorView.vue'))
const ArchiveView = defineAsyncComponent(() => import('./DocsArchiveView.vue'))
const SearchView = defineAsyncComponent(() => import('./DocsSearchView.vue'))
const TagView = defineAsyncComponent(() => import('./DocsTagView.vue'))

const theme = {
  ...manifest,
  entry: DocsView,
  entries: {
    ...(manifest.entries || {}),
    post: PostView,
    cat: CatView,
    category: CatView,
    author: AuthorView,
    archive: ArchiveView,
    search: SearchView,
    tag: TagView,
  },
}

registerTheme(theme)

export default theme
