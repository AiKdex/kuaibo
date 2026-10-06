import { t } from '@/i18n'
import { defineAsyncComponent } from 'vue'

/**
 * Verdant（青野）主题元数据 —— 字段名大小写敏感，逐字段对齐《第三方完整交付版》v1.2 §1.1。
 * 内页组件用 defineAsyncComponent 懒加载，避免把全部页面塞进首屏包。
 */
const CatView = defineAsyncComponent(() => import('./VerdantCatView.vue'))
const AuthorView = defineAsyncComponent(() => import('./VerdantAuthorView.vue'))
const ArchiveView = defineAsyncComponent(() => import('./VerdantArchiveView.vue'))
const SearchView = defineAsyncComponent(() => import('./VerdantSearchView.vue'))
const TagView = defineAsyncComponent(() => import('./VerdantTagView.vue'))

export default {
  id: 'verdant',
  title: t('青野 Verdant'),
  desc: '明亮清爽：暖白底 + 森林绿 + 有机圆角，适合生活/自然/产品类内容，含分类/作者/归档/搜索内页',
  version: '1.0.0',
  pages: ['posts'],
  tokens: {
    '--th-ink': t('正文色（深墨绿）'),
    '--th-ink-2': t('次要文字'),
    '--th-ink-3': t('弱化文字'),
    '--th-paper': t('页面底色（暖白）'),
    '--th-card': t('卡片底色（纯白）'),
    '--th-line': t('描边与分隔线（浅绿灰）'),
    '--th-green': t('主强调色（森林绿）'),
    '--th-green-deep': t('强调色深阶（悬停/标题）'),
    '--th-mint': t('辅助色（薄荷）'),
    '--th-leaf': t('浅叶色底（徽章/标签底）'),
    '--th-sun': t('点缀色（琥珀）'),
    '--th-r-sm': t('小圆角'),
    '--th-r': t('常规圆角'),
    '--th-r-lg': t('大圆角（卡片）'),
    '--th-font-d': t('标题字体（无衬线，字重对比）'),
    '--th-font-b': t('正文字体'),
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
    tag: TagView, // 标签页（entries.tag）：按 file.tags 过滤
    // post：在 index.js 中合并（整页交给 VerdantPostView，正文样式由 post.css 提供）
  },
}
