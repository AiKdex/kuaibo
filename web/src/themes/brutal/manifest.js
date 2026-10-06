import { t } from '@/i18n'
import { defineAsyncComponent } from 'vue'

/**
 * Brutal 主题元数据 —— 字段名大小写敏感，逐字段对齐《第三方完整交付版》§1.1。
 * 内页组件用 defineAsyncComponent 懒加载，避免把全部页面塞进首屏包。
 */
const CatView = defineAsyncComponent(() => import('./BrutalCatView.vue'))
const AuthorView = defineAsyncComponent(() => import('./BrutalAuthorView.vue'))
const ArchiveView = defineAsyncComponent(() => import('./BrutalArchiveView.vue'))
const SearchView = defineAsyncComponent(() => import('./BrutalSearchView.vue'))
const TagView = defineAsyncComponent(() => import('./BrutalTagView.vue'))

export default {
  id: 'brutal',
  title: t('新粗野 Brutal'),
  desc: '新粗野风：硬边框硬阴影 + 高饱和撞色 + 几何图案卡片，含分类/作者/归档/搜索内页',
  version: '1.0.1',
  pages: ['posts'],
  tokens: {
    '--th-ink': t('正文色（近黑）'),
    '--th-ink-2': t('次要文字'),
    '--th-ink-3': t('弱化文字'),
    '--th-paper': t('页面底色（米白）'),
    '--th-card': t('卡片底色（纯白）'),
    '--th-line': t('描边与分隔线（纯黑）'),
    '--th-bw': t('硬边框宽度'),
    '--th-bw-2': t('加粗边框宽度（悬停/强调态）'),
    '--th-sh': t('硬阴影偏移（零模糊）'),
    '--th-sh-lg': t('大号硬阴影（悬停加厚）'),
    '--th-sh-sm': t('小号硬阴影（小控件）'),
    '--th-accent': t('主强调（明黄）'),
    '--th-accent-2': t('次强调（亮粉）'),
    '--th-accent-3': t('状态色（薄荷绿）'),
    '--th-deco-1': t('装饰色（电光紫）'),
    '--th-deco-2': t('装饰色（天蓝）'),
    '--th-deco-3': t('装饰色（橙）'),
    '--th-font-d': t('标题字体（超粗无衬线）'),
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
    // post：暂未接管 → 走系统默认文章页 BlogPostView（挂 .th-brutal 钩子，配色跟随）；
    //       如需自定义文章版式，按规范 §4.3B 补 BrutalPostView 并在此声明
  },
}
