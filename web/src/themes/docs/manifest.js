import { t } from '@/i18n'
export default {
  id: 'docs',
  title: t('技术文档'),
  desc: '侧栏目录 + 正文栏，适合文档站与长文',
  version: '0.1.0',
  pages: ['posts'],
  tokens: {
    '--th-ink': t('正文'),
    '--th-paper': t('底色'),
    '--th-accent': t('强调'),
  },
  ui: { themeSwitcher: true }, // 主题页头自带切换器（宿主不再渲染右上角悬浮版）
  dataSources: ['blog'],
  dataScope: 'shared',
}
