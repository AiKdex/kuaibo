import './style.css'
import { defineAsyncComponent } from 'vue'
import manifest from './manifest.js'
import { registerTheme } from '../index.js'

const PostView = defineAsyncComponent(() => import('./ParchmentPostView.vue'))
const CatView = defineAsyncComponent(() => import('./ParchmentCatView.vue'))
const AuthorView = defineAsyncComponent(() => import('./ParchmentAuthorView.vue'))
const ArchiveView = defineAsyncComponent(() => import('./ParchmentArchiveView.vue'))
const SearchView = defineAsyncComponent(() => import('./ParchmentSearchView.vue'))
const TagView = defineAsyncComponent(() => import('./ParchmentTagView.vue'))

const theme = {
  ...manifest,
  entry: defineAsyncComponent(() => import('./ParchmentView.vue')),
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
