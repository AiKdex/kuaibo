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

export default {
  id: 'aurora',
  title: '极光 Aurora',
  desc: 'Bento Grid 卡片式 · Apple 风格：模块化栅格 + 玻璃页头，含分类/作者/归档/搜索内页',
  version: '1.0.0',
  pages: ['posts'],
  // 令牌台账：与 style.css 的 --th-* 定义逐项一致（theme-lint.sh 第 2 项会卡）
  tokens: {
    '--th-ink': '正文与标题主色',
    '--th-ink-2': '次要文字',
    '--th-ink-3': '弱化文字',
    '--th-paper': '页面底色（浅灰）',
    '--th-card': '卡片底色（纯白）',
    '--th-line': '描边与分隔线',
    '--th-accent': '主强调色（Apple 蓝）',
    '--th-accent-2': '次强调色（极光紫）',
    '--th-accent-3': '第三强调色（极光青）',
    '--th-accent-soft': '主强调极浅底',
    '--th-tint': '卡片紫调渐变尾色',
    '--th-tint-2': '卡片青调渐变尾色',
    '--th-dark': '深色卡底色（头条卡）',
    '--th-font-d': '标题字体',
    '--th-font-b': '正文字体',
    '--th-font-mono': '等宽字体',
    '--th-r-sm': '小控件圆角',
    '--th-r-md': '中号圆角',
    '--th-r-lg': '卡片圆角',
    '--th-r-xl': '大区块圆角',
    '--th-shadow': '卡片常态阴影',
    '--th-shadow-lg': '卡片悬停阴影',
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
    // post 由 index.js 追加（避免 manifest 与入口组件互相引用）
    // tag：系统层标签数据未实现（规范 §4.6），暂不接管
  },
}
