import { t } from '@/i18n'
export default {
  id: 'paper',
  title: t('暖纸情报'),
  desc: '暖纸底 + 青绿顶栏 + 珊瑚强调（对齐求智情报站视觉）',
  version: '0.1.0',
  pages: ['posts'],
  tokens: {
    '--th-paper': t('暖纸底'),
    '--th-c-main': t('青绿主色'),
    '--th-c-coral': t('珊瑚强调'),
  },
  ui: { themeSwitcher: true }, // 主题页头自带切换器（宿主不再渲染右上角悬浮版）
  dataSources: ['blog'],
  dataScope: 'shared',
}
