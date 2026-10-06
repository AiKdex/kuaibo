import { t } from '@/i18n'
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
    '--th-ink': t('正文色'),
    '--th-ink-2': t('次要文字'),
    '--th-ink-3': t('弱化文字'),
    '--th-paper': t('页面底色'),
    '--th-card': t('卡片底色'),
    '--th-line': t('分隔线'),
    '--th-accent': t('强调色'),
    '--th-accent-soft': t('强调色浅底'),
    '--th-font-d': t('标题字体'),
    '--th-font-b': t('正文字体'),
  },
  ui: { themeSwitcher: true },
  dataSources: ['blog'],
  dataScope: 'shared',
}