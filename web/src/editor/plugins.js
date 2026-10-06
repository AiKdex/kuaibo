/**
 * 编辑器插件协议 + 注册表（Editor Plugin Protocol）—— AiKlog（爱库录）编辑器插件化基础。
 *
 * 目标：把编辑器（Vditor）的扩展能力以「插件」为单位解耦、可注册、可组合；
 * 第三方（含后续 E03「博客插件化」）复用同一套协议扩展工具栏 / 钩子 / 快捷键 / 状态栏，
 * 无需改动编辑器组件（VditorEditor.vue）核心。
 *
 * ── 协议（插件对象）─────────────────────────────────────────────
 * {
 *   name: string,                 // 唯一标识（kebab-case）
 *   title?: string,               // 展示名
 *   toolbar?: Array<{             // 工具栏按钮（追加到内置工具栏右侧）
 *     id: string,                 // 按钮标识（同一编辑器内唯一）
 *     icon?: string,              // SVG 字符串（缺省用内置兜底图标）
 *     title: string,              // 悬浮提示
 *     onClick(ctx): void          // 点击回调；ctx 见 EditorContext
 *   }>,
 *   hooks?: {                     // 生命周期钩子（均可选）
 *     afterRender(el, ctx): void,          // 编辑器渲染完成后
 *     beforeSave(value, ctx): string|void, // 保存前加工 Markdown 原文（返回值即最终内容）
 *     onInput(value, ctx): void            // 内容变化时
 *   },
 *   shortcuts?: Array<{           // 键盘快捷键
 *     key: string,                // 组合键，如 'mod+shift+t'（mod=Ctrl/Cmd，另支持 shift/alt）
 *     handler(ctx): void
 *   }>,
 *   statusBar?: Array<{           // 状态栏条目（编辑器底部；本实现补充的扩展字段）
 *     id: string,
 *     align?: 'left'|'right',     // 默认 left
 *     render(ctx): string         // 返回展示文本
 *   }>
 * }
 *
 * EditorContext: { vditor, el, insertValue(md), getValue(), setValue(md) }
 *
 * 说明：协议核心为 { name, toolbar, hooks, shortcuts }；statusBar 为本实现补充的扩展字段，
 * 用于承载「字数统计」类状态展示（Vditor 原生无独立状态栏）。
 * 单项插件抛错不影响其它插件与编辑器核心（注册/执行处均 try-catch 隔离）。
 */

const registry = []
const registeredNames = new Set()

function assertName(v) {
  if (typeof v !== 'string' || !v.trim()) {
    throw new Error('[editor-plugin] 插件缺少合法 name')
  }
}

/**
 * 注册一个插件（幂等：同名覆盖，便于热更新/开发调试）。
 * @param {object} plugin 符合上述协议的对象
 * @returns {object} 归一化后的插件
 */
export function registerPlugin(plugin) {
  if (!plugin || typeof plugin !== 'object') {
    throw new Error('[editor-plugin] 插件必须是对象')
  }
  assertName(plugin.name)
  const p = {
    name: plugin.name,
    title: plugin.title || plugin.name,
    toolbar: Array.isArray(plugin.toolbar)
      ? plugin.toolbar.filter((t) => t && t.id && typeof t.onClick === 'function')
      : [],
    hooks: plugin.hooks && typeof plugin.hooks === 'object' ? plugin.hooks : {},
    shortcuts: Array.isArray(plugin.shortcuts)
      ? plugin.shortcuts.filter((s) => s && s.key && typeof s.handler === 'function')
      : [],
    statusBar: Array.isArray(plugin.statusBar)
      ? plugin.statusBar.filter((s) => s && s.id && typeof s.render === 'function')
      : []
  }
  const idx = registry.findIndex((x) => x.name === p.name)
  if (idx >= 0) registry.splice(idx, 1, p)
  else registry.push(p)
  registeredNames.add(p.name)
  return p
}

/** 全部已注册插件（快照）。 */
export function getPlugins() {
  return registry.slice()
}

/** 是否注册过某插件。 */
export function hasPlugin(name) {
  return registeredNames.has(name)
}

/** 汇总所有插件的工具栏按钮（按注册顺序）。 */
export function getToolbarItems() {
  return registry.flatMap((p) => p.toolbar.map((t) => ({ plugin: p.name, ...t })))
}

/** 汇总所有插件的状态栏条目。 */
export function getStatusBarItems() {
  return registry.flatMap((p) =>
    p.statusBar.map((s) => ({ plugin: p.name, align: s.align || 'left', ...s }))
  )
}

/** 汇总所有插件的快捷键。 */
export function getShortcuts() {
  return registry.flatMap((p) => p.shortcuts.map((s) => ({ plugin: p.name, ...s })))
}

/**
 * 运行某生命周期钩子。
 * beforeSave 会串联各插件返回值（链式加工）；其它钩子仅通知。
 * @returns 加工后的值（仅 beforeSave 有意义）
 */
export function runHook(name, value, ctx) {
  let v = value
  for (const p of registry) {
    const fn = p.hooks && p.hooks[name]
    if (typeof fn !== 'function') continue
    try {
      const r = fn(v, ctx)
      if (name === 'beforeSave' && typeof r === 'string') v = r
    } catch (e) {
      console.warn(`[editor-plugin] ${p.name}.${name} 执行失败:`, e)
    }
  }
  return v
}

/** 渲染所有状态栏条目为 { id, align, text } 列表（单项失败不影响其它）。 */
export function renderStatusBars(ctx) {
  const out = []
  for (const p of registry) {
    for (const s of p.statusBar || []) {
      try {
        out.push({ id: `${p.name}:${s.id}`, align: s.align || 'left', text: String(s.render(ctx)) })
      } catch (e) {
        console.warn(`[editor-plugin] ${p.name}.${s.id} 状态栏渲染失败:`, e)
      }
    }
  }
  return out
}

// ══════════════════════════════════════════════════════════════════════
// 内置示例插件 ①：插入表格模板（演示 toolbar + shortcuts）
// ══════════════════════════════════════════════════════════════════════
const TABLE_TEMPLATE = '\n| 列1 | 列2 | 列3 |\n| --- | --- | --- |\n|  |  |  |\n|  |  |  |\n'
const TABLE_ICON =
  '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" ' +
  'stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="4" width="18" height="16" rx="1.5"/>' +
  '<line x1="3" y1="10" x2="21" y2="10"/><line x1="9" y1="4" x2="9" y2="20"/>' +
  '<line x1="15" y1="4" x2="15" y2="20"/></svg>'

registerPlugin({
  name: 'insert-table-template',
  title: '插入表格模板',
  toolbar: [
    {
      id: 'insert-table-template',
      icon: TABLE_ICON,
      title: '插入 3×3 表格模板',
      onClick(ctx) {
        ctx.insertValue(TABLE_TEMPLATE)
      }
    }
  ],
  shortcuts: [
    {
      key: 'mod+shift+t',
      handler(ctx) {
        ctx.insertValue(TABLE_TEMPLATE)
      }
    }
  ]
})

// ══════════════════════════════════════════════════════════════════════
// 内置示例插件 ②：字数统计状态栏（演示 statusBar + onInput 钩子）
// ══════════════════════════════════════════════════════════════════════
function countStats(text) {
  const t = text || ''
  const chars = [...t].length
  const words = (t.match(/[\p{L}\p{N}]+/gu) || []).length
  const lines = t ? t.split('\n').length : 0
  return { chars, words, lines }
}

registerPlugin({
  name: 'word-count',
  title: '字数统计',
  statusBar: [
    {
      id: 'count',
      align: 'right',
      render(ctx) {
        const { chars, words, lines } = countStats(ctx.getValue())
        return `字数 ${chars} · 词 ${words} · 行 ${lines}`
      }
    }
  ]
})
