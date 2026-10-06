import { defineAsyncComponent } from 'vue'
import './style.css'
import MinimalView from './MinimalView.vue'
import manifest from './manifest'
import { registerTheme } from '../index'

const PostView = defineAsyncComponent(() => import('./MinimalPostView.vue'))
const CatView = defineAsyncComponent(() => import('./MinimalCatView.vue'))
const AuthorView = defineAsyncComponent(() => import('./MinimalAuthorView.vue'))
const ArchiveView = defineAsyncComponent(() => import('./MinimalArchiveView.vue'))
const SearchView = defineAsyncComponent(() => import('./MinimalSearchView.vue'))
const TagView = defineAsyncComponent(() => import('./MinimalTagView.vue'))

const theme = {
  ...manifest,
  entry: MinimalView,
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
