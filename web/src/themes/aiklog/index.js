import './style.css'
/**
 * 爱库录主题 —— 注册入口（AiKlog 样板房公开门面）。
 */
import AiklogView from './AiklogView.vue'
import manifest from './manifest'
import { registerTheme } from '../index'

const theme = {
  ...manifest,
  entry: AiklogView
}

registerTheme(theme)

export default theme
