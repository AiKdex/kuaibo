import './style.css'
import { defineAsyncComponent } from 'vue'
import manifest from './manifest.js'
import { registerTheme } from '../index.js'

const PostView = defineAsyncComponent(() => import('./ElevatedPostView.vue'))
const CatView = defineAsyncComponent(() => import('./ElevatedCatView.vue'))
const AuthorView = defineAsyncComponent(() => import('./ElevatedAuthorView.vue'))
const ArchiveView = defineAsyncComponent(() => import('./ElevatedArchiveView.vue'))
const SearchView = defineAsyncComponent(() => import('./ElevatedSearchView.vue'))
const TagView = defineAsyncComponent(() => import('./ElevatedTagView.vue'))

const theme = {
  ...manifest,
  entry: defineAsyncComponent(() => import('./ElevatedView.vue')),
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
