import { defineAsyncComponent } from 'vue'

/**
 * 青璃 Zircon —— 青绿胶囊 · 社区资讯式企业博客主题 · 元数据。
 *
 * 设计语言：青绿主色 #16B597 + 胶囊控件 + 发丝线白卡 + 柔和投影（子比风格调性）。
 * 字段名大小写敏感，逐字段对齐《AiKlog 博客主题开发规范（第三方完整交付版）》v1.2 §1.1。
 * 内页组件一律 defineAsyncComponent 懒加载，避免把全部页面塞进首屏包。
 */
const CatView = defineAsyncComponent(() => import('./ZirconCatView.vue'))
const AuthorView = defineAsyncComponent(() => import('./ZirconAuthorView.vue'))
const ArchiveView = defineAsyncComponent(() => import('./ZirconArchiveView.vue'))
const SearchView = defineAsyncComponent(() => import('./ZirconSearchView.vue'))

export default {
  id: 'zircon',
  title: '青璃 Zircon',
  desc: '青绿胶囊 · 社区资讯式双栏布局：渐变头图 + 卡片流 + 信息侧栏，含分类/作者/归档/搜索内页',
  version: '1.0.0',
  pages: ['posts'],
  // 令牌台账：与 style.css 的 --th-* 定义逐项一致（theme-lint.sh 第 2 项会卡）
  tokens: {
    '--th-ink': '正文与标题主色',
    '--th-ink-2': '次要文字',
    '--th-ink-3': '弱化文字',
    '--th-paper': '页面底色（浅灰）',
    '--th-card': '卡片底色（纯白）',
    '--th-muted': '次级底色（控件/标签底）',
    '--th-line': '发丝线描边',
    '--th-primary': '主色（青绿）',
    '--th-primary-deep': '主色深阶（悬停）',
    '--th-primary-soft': '主色极浅底',
    '--th-badge-top': '置顶角标橙',
    '--th-badge-hot': '强调角标红',
    '--th-g1': '渐变起点（青蓝）',
    '--th-g2': '渐变终点（青绿）',
    '--th-font-d': '标题字体',
    '--th-font-b': '正文字体',
    '--th-r-sm': '小控件圆角',
    '--th-r-md': '卡片圆角',
    '--th-r-pill': '胶囊圆角',
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
