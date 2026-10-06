/**
 * Huajian Theme — index.js
 * 模块加载时直接注册（副作用式，§1.2）
 * entries 声明在 manifest.js（§1.1 必填字段），此处仅挂列表页 entry
 */
import { defineAsyncComponent } from 'vue'
import manifest from './manifest.js'
import { registerTheme } from '../index.js'

const theme = {
  ...manifest,
  entry: defineAsyncComponent(() => import('./HuajianView.vue')),
}
registerTheme(theme)
export default theme
