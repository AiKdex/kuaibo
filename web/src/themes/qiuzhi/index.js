/**
 * 求智主题 —— 注册入口（博客主题插件第一个实战）。
 */
import QiuzhiView from './QiuzhiView.vue'
import QiuzhiPostView from './QiuzhiPostView.vue'
import QiuzhiCatView from './QiuzhiCatView.vue'
import QiuzhiAuthorView from './QiuzhiAuthorView.vue'
import QiuzhiArchiveView from './QiuzhiArchiveView.vue'
import QiuzhiSearchView from './QiuzhiSearchView.vue'
import QiuzhiTagView from './QiuzhiTagView.vue'
import manifest from './manifest'
import { registerTheme } from '../index'

const theme = {
  ...manifest,
  entry: QiuzhiView,
  entries: {
    ...(manifest.entries || {}),
    post: QiuzhiPostView,
    cat: QiuzhiCatView,
    category: QiuzhiCatView,
    author: QiuzhiAuthorView,
    archive: QiuzhiArchiveView,
    search: QiuzhiSearchView,
    tag: QiuzhiTagView,
  },
}

registerTheme(theme)

export default theme
