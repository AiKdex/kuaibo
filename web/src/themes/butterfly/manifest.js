import { t } from '@/i18n'
import { defineAsyncComponent } from 'vue'

/**
 * Butterfly（蝶语）主题元数据 —— 参照 hexo-theme-butterfly 的卡片式双栏设计语言重构为 AiKlog 主题。
 * 字段名大小写敏感，逐字段对齐《第三方完整交付版》v1.2 §1.1。
 * 内页组件用 defineAsyncComponent 懒加载，避免把全部页面塞进首屏包。
 */
const CatView = defineAsyncComponent(() => import('./ButterflyCatView.vue'))
const AuthorView = defineAsyncComponent(() => import('./ButterflyAuthorView.vue'))
const ArchiveView = defineAsyncComponent(() => import('./ButterflyArchiveView.vue'))
const SearchView = defineAsyncComponent(() => import('./ButterflySearchView.vue'))
const TagView = defineAsyncComponent(() => import('./ButterflyTagView.vue'))

export default {
  id: 'butterfly',
  title: t('蝶语 Butterfly'),
  desc: '卡片式双栏：半透明毛玻璃顶栏 + 文章卡片 + 侧栏卡片组 + 右下悬浮按钮 + 夜间模式，含分类/作者/归档/搜索内页',
  version: '1.0.0',
  pages: ['posts'],
  tokens: {
    '--th-sky': t('主色（蝴蝶蓝 #49b1f5）'),
    '--th-sky-deep': t('主色深阶（悬停/标题强调）'),
    '--th-sky-soft': t('主色浅底（选中态/引用底）'),
    '--th-orange': t('悬停强调色（#ff7242）'),
    '--th-ink': t('正文色'),
    '--th-ink-head': t('标题与高亮文字'),
    '--th-meta': t('次要信息（日期/计数）'),
    '--th-quote': t('引用块文字'),
    '--th-card': t('卡片底色'),
    '--th-page': t('页面底色'),
    '--th-line': t('描边与分隔线'),
    '--th-line-2': t('更浅的分隔线（表头/tab）'),
    '--th-dark-bg': t('深色块与代码块底色'),
    '--th-shadow': t('卡片投影'),
    '--th-shadow-hover': t('卡片悬停投影'),
    '--th-r-sm': t('小圆角'),
    '--th-r': t('常规圆角'),
    '--th-r-lg': t('大圆角（卡片）'),
    '--th-font-d': t('标题字体'),
    '--th-font-b': t('正文字体'),
    '--th-font-m': t('等宽字体（代码/编号）'),
    '--th-topbar-h': t('顶栏高度'),
    '--th-aside-w': t('侧栏宽度'),
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
    // post：在 index.js 中合并（整页交给 ButterflyPostView，正文样式由 post.css 提供）
    // tag：系统层标签数据未实现（规范 §4.6），暂不接管
  },
}
