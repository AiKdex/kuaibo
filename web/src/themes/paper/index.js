import { defineAsyncComponent } from 'vue'
import './style.css'
import PaperView from './PaperView.vue'
import manifest from './manifest'
import { registerTheme } from '../index'

const PostView = defineAsyncComponent(() => import('./PaperPostView.vue'))
const CatView = defineAsyncComponent(() => import('./PaperCatView.vue'))
const AuthorView = defineAsyncComponent(() => import('./PaperAuthorView.vue'))
const ArchiveView = defineAsyncComponent(() => import('./PaperArchiveView.vue'))
const SearchView = defineAsyncComponent(() => import('./PaperSearchView.vue'))
const TagView = defineAsyncComponent(() => import('./PaperTagView.vue'))

const theme = {
  ...manifest,
  entry: PaperView,
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
