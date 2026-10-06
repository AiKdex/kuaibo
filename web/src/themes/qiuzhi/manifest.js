import { t } from '@/i18n'
/**
 * 求智主题 manifest —— 博客主题插件第一个实战（源自第四方「求智」就业情报 webUI）。
 *
 * 定位说明：本主题为「采集型视图」（dataSources:['collect']），展示的是岗位情报/培训补贴
 * 采集数据，非通用博客主题。通用博客主题请声明 dataSources:['blog']，BlogView 会自动
 * 注入 themeContext（siteName/posts/tags/postUrl），安装即显示博客分享内容。
 *
 * 数据：当前为主题自带样例（data.js loadData()）；结构与主系统采集产物
 * （kind=job/training 的 front matter）对齐，后续对接采集 API/入库文件时替换 loadData
 * 实现即可，主题组件无需改动。
 */
export default {
  id: 'qiuzhi',
  title: t('求智 · 就业情报'),
  desc: '岗位情报 + 培训补贴联动：地区/类型/薪资筛选、我的条件匹配、三态岗位、过期触动、补贴直达（采集型视图）',
  version: '0.1.0',
  pages: ['jobs', 'training'],
  tokens: {
    // 设计令牌（style.css .qz-root 定义，--th-* 前缀防污染主应用）
    '--th-ink': t('主文字色'), '--th-paper': t('页面底色'), '--th-card': t('卡片底色'),
    '--th-c-main': t('主色(青) 品牌/链接'), '--th-c-coral': t('强调色(珊瑚) 薪资/缺口'),
    '--th-c-mint': t('进行中/补贴'), '--th-c-amber': t('即将开启'), '--th-c-sky': t('来源链接'),
    '--th-st-live': t('进行中状态色'), '--th-st-soon': t('即将开启状态色'), '--th-st-ended': t('已结束状态色')
  },
  dataSources: ['collect'], // collect=采集型视图（特殊场景，非通用博客主题）
  dataScope: 'library' // library=直读文件库（采集目录+标签）。自用内页特例：
                       // 数据来自情报/就业、情报/培训 标签下的采集入库文件，
                       // 非「已分享文件」集合；公开部署时不得对外暴露此主题
}
