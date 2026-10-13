/**
 * Huajian Theme — manifest.js
 * AiKlog 博客主题 · 暖纸杂志风（衬线标题 / 琥珀强调 / 纸纹噪点 / 杂志式双栏）
 */
export default {
  id: 'huajian',
  title: '花笺 Huajian',
  desc: '暖纸杂志风博客主题：衬线标题、琥珀强调、杂志式双栏版式',
  version: '1.1.1',
  pages: ['posts'],
  tokens: {
    '--th-ink': '正文色（深棕）',
    '--th-ink-2': '次要文字',
    '--th-ink-3': '弱化文字',
    '--th-paper': '暖纸页面底',
    '--th-paper-2': '页脚底色',
    '--th-card': '卡片底',
    '--th-line': '分隔线',
    '--th-accent': '琥珀强调色',
    '--th-accent-deep': '陶土深色',
    '--th-accent-soft': '强调色浅底',
    '--th-olive': '橄榄绿（分类色点）',
    '--th-font-d': '衬线标题字体',
    '--th-font-b': '正文字体',
  },
  ui: { themeSwitcher: true },
  dataSources: ['blog'],
  dataScope: 'shared',
  entries: { // 五页必交（§4.10）；cat/category 双 key 系统都认，这里用 cat
    post: () => import('./HuajianPostView.vue'),
    cat: () => import('./HuajianCatView.vue'),
    author: () => import('./HuajianAuthorView.vue'),
    tag: () => import('./HuajianTagView.vue'),
    archive: () => import('./HuajianArchiveView.vue'),
    search: () => import('./HuajianSearchView.vue'),
  },
}
