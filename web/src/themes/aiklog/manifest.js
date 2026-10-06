import { t } from '@/i18n'
/**
 * 爱库录主题 manifest —— AiKlog 产品样板房公开门面。
 * 通用博客主题：dataSources:['blog']，BlogView 注入 themeContext。
 */
export default {
  id: 'aiklog',
  title: t('爱库录 · AI 知识库博客'),
  desc: '目录即博客：冷纸底 + 玉色目录卡，产品样板房默认门面',
  version: '0.1.0',
  pages: ['posts'],
  tokens: {
    '--th-ink': t('正文墨色'),
    '--th-paper': t('冷纸底'),
    '--th-card': t('卡片白'),
    '--th-jade': t('品牌玉色（脊线/链接）'),
    '--th-amber': t('强调琥珀'),
    '--th-line': t('细线')
  },
  ui: { themeSwitcher: true }, // 主题页头自带切换器（宿主不再渲染右上角悬浮版）
  dataSources: ['blog'],
  dataScope: 'shared'
}
