/**
 * ClawBlog Theme — manifest.js
 * AiKlog 博客主题 · 现代渐变 + 玻璃拟态风格
 */
export default {
  id: 'clawblog',
  title: 'ClawBlog',
  desc: '现代渐变风格博客主题：玻璃拟态导航、混合布局卡片、极致阅读体验',
  version: '1.0.0',
  pages: ['posts'],
  tokens: {
    '--th-ink': '正文色',
    '--th-ink-2': '次要文字',
    '--th-ink-3': '弱化文字',
    '--th-paper': '页面底色',
    '--th-card': '卡片底色',
    '--th-line': '分隔线',
    '--th-accent': '强调色',
    '--th-accent-soft': '强调色浅底',
    '--th-font-d': '标题字体',
    '--th-font-b': '正文字体',
  },
  ui: { themeSwitcher: true },
  dataSources: ['blog'],
  dataScope: 'shared',
}