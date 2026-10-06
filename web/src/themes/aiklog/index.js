import './style.css'
/**
 * 爱库录主题 —— 注册入口（AiKlog 样板房公开门面）。
 */
import { defineAsyncComponent } from 'vue'
import AiklogView from './AiklogView.vue'
import manifest from './manifest'
import { registerTheme } from '../index'

const PostView = defineAsyncComponent(() => import('./AkPostView.vue'))
const CatView = defineAsyncComponent(() => import('./AkCatView.vue'))
const AuthorView = defineAsyncComponent(() => import('./AkAuthorView.vue'))
const ArchiveView = defineAsyncComponent(() => import('./AkArchiveView.vue'))
const SearchView = defineAsyncComponent(() => import('./AkSearchView.vue'))
const TagView = defineAsyncComponent(() => import('./AkTagView.vue'))

const theme = {
  ...manifest,
  entry: AiklogView,
  // 文章页与内页：声明 entries 后由宿主 BlogPostView / 路由分发（规范 §4.3B / §4.5-§4.8）
  entries: {
    ...(manifest.entries || {}),
    post: PostView,
    cat: CatView,
    category: CatView, // 兼容 entries 枚举里的 category 命名
    author: AuthorView,
    archive: ArchiveView,
    search: SearchView,
    tag: TagView,
  },
}

registerTheme(theme)

export default theme
