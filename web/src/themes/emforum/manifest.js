import { t } from '@/i18n'
import { defineAsyncComponent } from 'vue'

/**
 * EmForum 主题元数据 —— 字段名大小写敏感，逐字段对齐《第三方完整交付版》§1.1。
 * 内页组件用 defineAsyncComponent 懒加载，避免把全部页面塞进首屏包。
 */
const CatView = defineAsyncComponent(() => import('./EmforumCatView.vue'))
const AuthorView = defineAsyncComponent(() => import('./EmforumAuthorView.vue'))
const ArchiveView = defineAsyncComponent(() => import('./EmforumArchiveView.vue'))
const SearchView = defineAsyncComponent(() => import('./EmforumSearchView.vue'))
const TagView = defineAsyncComponent(() => import('./EmforumTagView.vue'))

export default {
  id: 'emforum',
  title: t('论坛社区 EmForum'),
  desc: '深蓝鎏金论坛风：版块页签切换 + 帖子流列表 + 归档/搜索内页',
  version: '1.1.1',
  pages: ['posts'],
  tokens: {
    '--th-ink': t('正文色（深蓝灰）'),
    '--th-ink-2': t('次要文字'),
    '--th-ink-3': t('弱化文字'),
    '--th-paper': t('页面底色（浅雾灰）'),
    '--th-card': t('卡片底色（纯白）'),
    '--th-line': t('分隔线'),
    '--th-accent': t('强调色（水鸭青）'),
    '--th-accent-soft': '强调色的极浅底（标签底、选中态背景）',
    '--th-accent-2': t('点缀色（鎏金，用于激活/精华）'),
    '--th-accent-2-soft': t('点缀色的半透明底（chip 选中态）'),
    '--th-navy': t('头部主色（深海军蓝）'),
    '--th-navy-2': t('头部渐变中间色'),
    '--th-hot': t('警示色（错误态文字）'),
    '--th-font-d': t('标题字体（衬线）'),
    '--th-font-b': t('正文字体（无衬线）'),
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
    tag: TagView, // 标签页（规范 §4.6：ctx.tags 已真实下发，标签为可点链接）
    // post：已由 index.js 接管 → EmforumPostView（BBS 帖子版式，含侧栏）
  },
}
