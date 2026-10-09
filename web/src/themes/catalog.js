/**
 * 懒加载主题目录 —— 「只内置 aiklog，其余主题进应用中心按需安装」的 SPA 侧基座。
 *
 * 背景：此前 18 个主题全部经 all.js 静态 import 注册，访客端永远全量可见；
 * 现改为「注册表瘦身为 default + aiklog，其余 17 个主题按需动态加载」：
 *  - 本文件只静态引入各主题的 manifest.js（纯元数据，体积可忽略，不会把 .vue 拉进首屏包）；
 *  - 主题组件本体走 THEME_LOADERS 的动态 import()（Vite 拆独立 chunk，按需请求）；
 *  - 「已安装」由后端 blog_plugins（kind=theme, enabled=1）驱动：
 *    应用中心装/卸主题 → 公开接口 /api/v1/public/blog/themes → 前端 applyInstalledThemes()。
 *
 * 注意：SPA 主题代码仍随主系统构建分发（Vite 无法运行时编译 .vue，见主题开发规范 §11），
 * 「安装」的语义 = 解锁注册与可见性 + 投放声明式资产（data/themes/<id>/，若包内提供）；
 * 卸载 = 撤销登记（主题从切换器消失，已选该主题的站点回落默认皮）。
 */
import minimal from './minimal/manifest.js'
import docs from './docs/manifest.js'
import paper from './paper/manifest.js'
import elevated from './elevated/manifest.js'
import parchment from './parchment/manifest.js'
import emforum from './emforum/manifest.js'
import brutal from './brutal/manifest.js'
import aurora from './aurora/manifest.js'
import verdant from './verdant/manifest.js'
import zircon from './zircon/manifest.js'
import butterfly from './butterfly/manifest.js'
import chenxi from './chenxi/manifest.js'
import jaded from './jaded/manifest.js'
import zhicang from './zhicang/manifest.js'
import aiknav from './aiknav/manifest.js'
import clawblog from './clawblog/manifest.js'
import huajian from './huajian/manifest.js'

/** 全部可安装主题的 manifest（id → manifest）。aiklog/default 不在此列（常驻内置）。 */
const MANIFESTS = {
  minimal, docs, paper, elevated, parchment, emforum, brutal, aurora,
  verdant, zircon, butterfly, chenxi, jaded, zhicang, aiknav, clawblog, huajian,
}

/** 懒加载器：id → () => import(主题目录 index.js)（index.js 副作用完成 registerTheme）。 */
export const THEME_LOADERS = {
  minimal: () => import('./minimal/index.js'),
  docs: () => import('./docs/index.js'),
  paper: () => import('./paper/index.js'),
  elevated: () => import('./elevated/index.js'),
  parchment: () => import('./parchment/index.js'),
  emforum: () => import('./emforum/index.js'),
  brutal: () => import('./brutal/index.js'),
  aurora: () => import('./aurora/index.js'),
  verdant: () => import('./verdant/index.js'),
  zircon: () => import('./zircon/index.js'),
  butterfly: () => import('./butterfly/index.js'),
  chenxi: () => import('./chenxi/index.js'),
  jaded: () => import('./jaded/index.js'),
  zhicang: () => import('./zhicang/index.js'),
  aiknav: () => import('./aiknav/index.js'),
  clawblog: () => import('./clawblog/index.js'),
  huajian: () => import('./huajian/index.js'),
}

/** 管理端/切换器用的全量主题元数据（不含组件）：default 与 aiklog 常驻，其余来自 MANIFESTS。 */
export function themeCatalog() {
  const arr = [
    { id: 'aiklog', title: '爱库录', desc: 'AiKlog 样板房门面主题（内置）', version: '' },
  ]
  for (const m of Object.values(MANIFESTS)) {
    arr.push({ id: m.id, title: m.title, desc: m.desc || '', version: m.version || '' })
  }
  return arr
}

export default MANIFESTS
