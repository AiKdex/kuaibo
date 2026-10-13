/**
 * 求智主题 —— 注册入口（博客主题插件第一个实战）。
 */
import QiuzhiView from './QiuzhiView.vue'
import manifest from './manifest'
import { registerTheme } from '../index'

const theme = {
  ...manifest,
  entry: QiuzhiView
}

registerTheme(theme)

export default theme
