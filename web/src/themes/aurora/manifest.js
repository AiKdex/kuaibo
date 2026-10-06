import { t } from '@/i18n'
import { defineAsyncComponent } from 'vue'

/**
 * 极光 Aurora —— Bento Grid 卡片式企业博客主题 · 元数据。
 *
 * 字段名大小写敏感，逐字段对齐《AiKlog 博客主题开发规范（第三方完整交付版）》v1.2 §1.1。
 * 内页组件一律 defineAsyncComponent 懒加载，避免把全部页面塞进首屏包。
 */
const CatView = defineAsyncComponent(() => import('./AuroraCatView.vue'))
const AuthorView = defineAsyncComponent(() => import('./AuroraAuthorView.vue'))
const ArchiveView = defineAsyncComponent(() => import('./AuroraArchiveView.vue'))
const SearchView = defineAsyncComponent(() => import('./AuroraSearchView.vue'))
const TagView = defineAsyncComponent(() => import('./AuroraTagView.vue'))

export default {
  id: 'aurora',
  title: t('极光 Aurora'),
  desc: 'Bento Grid 卡片式 · Apple 风格：模块化栅格 + 玻璃页头，含分类/作者/归档/搜索内页',
  version: '1.0.0',
  pages: ['posts'],
  // 令牌台账：与 style.css 的 --th-* 定义逐项一致（theme-lint.sh 第 2 项会卡）
  tokens: {
    '--th-ink': t('正文与标题主色'),
    '--th-ink-2': t('次要文字'),
    '--th-ink-3': t('弱化文字'),
    '--th-paper': t('页面底色（浅灰）'),
    '--th-card': t('卡片底色（纯白）'),
    '--th-line': t('描边与分隔线'),
    '--th-accent': t('主强调色（Apple 蓝）'),
    '--th-accent-2': t('次强调色（极光紫）'),
    '--th-accent-3': t('第三强调色（极光青）'),
    '--th-accent-soft': t('主强调极浅底'),
    '--th-tint': t('卡片紫调渐变尾色'),
    '--th-tint-2': t('卡片青调渐变尾色'),
    '--th-dark': t('深色卡底色（头条卡）'),
    '--th-font-d': t('标题字体'),
    '--th-font-b': t('正文字体'),
    '--th-font-mono': t('等宽字体'),
    '--th-r-sm': t('小控件圆角'),
    '--th-r-md': t('中号圆角'),
    '--th-r-lg': t('卡片圆角'),
    '--th-r-xl': t('大区块圆角'),
    '--th-shadow': t('卡片常态阴影'),
    '--th-shadow-lg': t('卡片悬停阴影'),
  },
  ui: { themeSwitcher: true }, // 页头自带 <ThemeSwitch />，宿主不再补右上角悬浮版
  dataSources: ['blog'],
  dataScope: 'shared',
  entries: {
    cat: CatView,
    category: CatView, // 兼容 entries 枚举里的 category 命名
    author: AuthorView,
    archive: ArchiveView,
    search: SearchView,
    tag: TagView, // 标签页（entries.tag）：按 file.tags 过滤
    // post 由 index.js 追加（避免 manifest 与入口组件互相引用）
  },
}
