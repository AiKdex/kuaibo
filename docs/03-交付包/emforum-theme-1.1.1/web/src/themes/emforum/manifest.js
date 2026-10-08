import { defineAsyncComponent } from 'vue'

/**
 * EmForum 主题元数据 —— 字段名大小写敏感，逐字段对齐《第三方完整交付版》§1.1。
 * 内页组件用 defineAsyncComponent 懒加载，避免把全部页面塞进首屏包。
 */
const CatView = defineAsyncComponent(() => import('./EmforumCatView.vue'))
const AuthorView = defineAsyncComponent(() => import('./EmforumAuthorView.vue'))
const ArchiveView = defineAsyncComponent(() => import('./EmforumArchiveView.vue'))
const SearchView = defineAsyncComponent(() => import('./EmforumSearchView.vue'))

export default {
  id: 'emforum',
  title: '论坛社区 EmForum',
  desc: '深蓝鎏金论坛风：版块页签切换 + 帖子流列表 + 归档/搜索内页',
  version: '1.1.1',
  pages: ['posts'],
  tokens: {
    '--th-ink': '正文色（深蓝灰）',
    '--th-ink-2': '次要文字',
    '--th-ink-3': '弱化文字',
    '--th-paper': '页面底色（浅雾灰）',
    '--th-card': '卡片底色（纯白）',
    '--th-line': '分隔线',
    '--th-accent': '强调色（水鸭青）',
    '--th-accent-soft': '强调色的极浅底（标签底、选中态背景）',
    '--th-accent-2': '点缀色（鎏金，用于激活/精华）',
    '--th-accent-2-soft': '点缀色的半透明底（chip 选中态）',
    '--th-navy': '头部主色（深海军蓝）',
    '--th-navy-2': '头部渐变中间色',
    '--th-hot': '警示色（错误态文字）',
    '--th-font-d': '标题字体（衬线）',
    '--th-font-b': '正文字体（无衬线）',
  },
  ui: { themeSwitcher: true }, // 页头自带 <ThemeSwitch />，宿主不再补悬浮版
  dataSources: ['blog'],
  dataScope: 'shared',
  entries: {
    cat: CatView,
    category: CatView, // 兼容规范 §3.1 的 entries.category 命名
    author: AuthorView,
    archive: ArchiveView,
    search: SearchView,
    // post：不做 SPA 文章视图（规范 §4.3：一律跳 SSR 页 /{slug}）
    // tag：系统层标签数据未实现（规范 §4.6），暂不接管
  },
}
