/**
 * 编辑器本体注册表（Editor Registry）—— AiKlog 编辑器插件化（本体轨）。
 *
 * 与 editor/plugins.js 的区别：那边是「编辑器内部扩展插件」（工具栏/钩子/快捷键），
 * 这里是「编辑器本体」的注册与切换（极简 Markdown / Vditor 所见即所得 / 未来第三方）。
 *
 * 模式与主题注册表（themes/index.js）同构：
 * - registryVersion(ref) 驱动响应式（懒注册后依赖 listEditors 的 computed 自动重算）；
 * - 组件用动态 import 拆独立懒 chunk（vditor 体积大，不进首屏）；
 * - 「已安装」由 blog_plugins(kind=editor, enabled=1) 驱动（公开接口 /api/v1/public/blog/editors），
 *   未安装不进选择列表；installedIds === null 表示未拉取（降级为全部可见，离线兜底）。
 * - plain（极简 Markdown）为常驻兜底编辑器：不可卸载、未装其它时永远可用。
 *
 * 用户偏好：localStorage（设备级），键 PREF_KEY；写作轨与阅读页快编轨共用，
 * 保证两轨编辑器一致（不一致即本次改版要解决的缺陷）。
 */
import { ref } from 'vue'

export const PREF_KEY = 'aiklog.editor'
export const DEFAULT_EDITOR_ID = 'plain'

/** 编辑器组件懒加载表（显式路径，Vite 拆 chunk） */
const EDITOR_LOADERS = {
  plain: () => import('@/components/PlainEditor.vue'),
  vditor: () => import('@/components/VditorEditor.vue'),
}

/** 内置编辑器元数据（元数据常驻主 bundle；组件体在懒 chunk） */
const BUILTIN_EDITORS = [
  { id: 'plain', title: '极简 Markdown', desc: '分栏编辑 + 实时预览（常驻兜底，不可卸载）' },
  { id: 'vditor', title: '所见即所得（Vditor）', desc: '所见即所得 + 源码 + 分屏预览三模式，支持双链联想' },
]

const registry = new Map() // id -> { id, title, desc, loader }
const registryVersion = ref(0)
const installedIds = ref(null) // null=未拉取 → 降级全量可见

for (const e of BUILTIN_EDITORS) {
  registry.set(e.id, { ...e, loader: EDITOR_LOADERS[e.id] })
}

export function registerEditor(meta) {
  if (!meta || typeof meta.id !== 'string' || !meta.id.trim()) {
    throw new Error('[editor-registry] 缺少合法 id')
  }
  if (typeof meta.loader !== 'function') {
    throw new Error(`[editor-registry] ${meta.id} 缺少 loader（动态 import）`)
  }
  registry.set(meta.id, {
    id: meta.id,
    title: meta.title || meta.id,
    desc: meta.desc || '',
    loader: meta.loader,
  })
  registryVersion.value++
}

/**
 * 列出编辑器元数据：已安装（或未拉取时全部内置）。
 * plain 常驻兜底，永远在列。
 */
export function listEditors() {
  registryVersion.value // 建立响应式依赖
  const inst = installedIds.value
  const out = []
  for (const e of registry.values()) {
    if (e.id === DEFAULT_EDITOR_ID) { out.push(e); continue }
    if (inst === null || inst.has(e.id)) out.push(e)
  }
  return out
}

export function getEditorMeta(id) {
  registryVersion.value
  return registry.get(id) || null
}

/** 编辑器组件 loader（不存在返回 null） */
export function getEditorLoader(id) {
  return registry.get(id)?.loader || null
}

export function isEditorInstalled(id) {
  if (id === DEFAULT_EDITOR_ID) return true
  const inst = installedIds.value
  return inst === null ? registry.has(id) : inst.has(id)
}

/** 拉取公开接口后调用；ids 为空数组/异常时传 [] 也会保留 plain */
export function applyInstalledEditors(ids) {
  installedIds.value = new Set(Array.isArray(ids) ? ids : [])
  registryVersion.value++
}

/** 读取本地偏好（非法/未装 → 回退 plain） */
export function readEditorPref() {
  try {
    const v = localStorage.getItem(PREF_KEY)
    if (v && registry.has(v) && isEditorInstalled(v)) return v
  } catch (_) { /* 隐私模式等 */ }
  return DEFAULT_EDITOR_ID
}

export function saveEditorPref(id) {
  try {
    if (id && registry.has(id)) localStorage.setItem(PREF_KEY, id)
  } catch (_) { /* 忽略 */ }
}
