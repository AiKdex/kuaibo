import { t } from '@/i18n'
/**
 * 极简阅读主题 manifest —— 沉浸列表，无侧栏。
 */
export default {
  id: 'minimal',
  title: t('极简阅读'),
  desc: '单栏沉浸列表，衬线标题，少装饰',
  version: '0.1.0',
  pages: ['posts'],
  tokens: {
    '--th-ink': t('正文'),
    '--th-paper': t('纸色底'),
    '--th-accent': t('强调色'),
  },
  ui: { themeSwitcher: true }, // 主题页头自带切换器（宿主不再渲染右上角悬浮版）
  dataSources: ['blog'],
  dataScope: 'shared',
}
