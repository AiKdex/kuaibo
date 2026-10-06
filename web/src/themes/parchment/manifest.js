import { t } from '@/i18n'
export default {
  id: 'parchment',
  title: t('暖纸杂志 Parchment'),
  desc: '暖纸杂志风格：衬线排版、琥珀色调、双列网格',
  version: '1.0.0',
  ui: { themeSwitcher: true }, // 主题页头自带切换器（宿主不再渲染右上角悬浮版）
  dataSources: ['blog'],
  dataScope: 'shared',
}
