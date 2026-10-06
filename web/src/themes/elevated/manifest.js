import { t } from '@/i18n'
export default {
  id: 'elevated',
  title: t('高端暗色 Elevated'),
  desc: '暗色高端博客主题：玉色点缀、衬线正文、AI 侧栏',
  version: '1.0.0',
  ui: { themeSwitcher: true }, // 主题页头自带切换器（宿主不再渲染右上角悬浮版）
  dataSources: ['blog'],
  dataScope: 'shared',
}
