<script setup>
/**
 * Vditor Markdown 编辑器（编辑器插件化第一实现；任务 03 / 模块 09·24）。
 * - 所见即所得 + 源码 + 分屏预览 三模式（Vditor 内置）
 * - 图片粘贴/拖拽 → 走系统上传 API（uploadFiles），落库后可检索
 * - v-model 双向绑定 Markdown 原文，后端零改动
 * - 插件化：按 web/src/editor/plugins.js 协议加载插件（工具栏按钮 / 生命周期钩子 / 快捷键 / 状态栏）；
 *   未注册任何插件时，行为与旧版完全一致（零回归）。
 */
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import Vditor from 'vditor'
import 'vditor/dist/index.css'
import { uploadFiles, listWikiNames } from '@/api'
import { getToolbarItems, getShortcuts, runHook, renderStatusBars } from '@/editor/plugins'
import { t } from '@/i18n'

const props = defineProps({
  modelValue: { type: String, default: '' },
  height: { type: Number, default: 520 },
  placeholder: { type: String, default: t('在此输入文档内容…') }
})
const emit = defineEmits(['update:modelValue', 'save'])

const box = ref(null)
const statusItems = ref([])
let vd = null

// ---- 双链联想（K14）：输入 [[ 弹出同空间文件名清单，选中插入 [[文件名]]（Obsidian 同款）----
let wikiNames = [] // [{id, name}]
let wikiLoadedAt = 0

function escHtml(s) {
  return String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;')
}

async function loadWikiNames(force = false) {
  // 缓存 60s，避免每次输入 [[ 都请求；编辑器挂载时强制刷新一次
  if (!force && wikiNames.length && Date.now() - wikiLoadedAt < 60000) return
  try {
    const items = await listWikiNames()
    if (Array.isArray(items)) {
      wikiNames = items
      wikiLoadedAt = Date.now()
    }
  } catch (_) {
    /* 联想不可用不阻塞编辑 */
  }
}

function wikiHint() {
  return [
    {
      key: '[[',
      hint: async (value) => {
        await loadWikiNames()
        const kw = String(value || '').toLowerCase()
        const hits = wikiNames
          .filter((n) => n.name.toLowerCase().includes(kw))
          .slice(0, 20)
        return hits.map((n) => ({
          value: n.name + ']]', // 选中后 Vditor 替换 [[ + 已输入 → [[文件名]]
          html: `<span class="wiki-hint-item"><span class="wiki-hint-icon">K</span><span class="wiki-hint-name">${escHtml(n.name)}</span></span>`
        }))
      }
    }
  ]
}

// 插件未提供 icon 时的兜底图标（一个「拼图」形状，示意扩展）
const FALLBACK_ICON =
  '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" ' +
  'stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="8" height="8" rx="1.5"/>' +
  '<rect x="13" y="13" width="8" height="8" rx="1.5"/><path d="M13 7h4a2 2 0 0 1 2 2v3"/>' +
  '<path d="M11 17H7a2 2 0 0 1-2-2v-3"/></svg>'

// 传给插件的上下文（每次调用取最新 vd，避免闭包早绑定）
function makeCtx() {
  return {
    vditor: vd,
    el: box.value,
    insertValue: (md) => vd?.insertValue?.(md),
    getValue: () => vd?.getValue?.() || '',
    setValue: (md) => vd?.setValue?.(md || '', false)
  }
}

function refreshStatus() {
  try {
    statusItems.value = renderStatusBars(makeCtx())
  } catch (_) {
    statusItems.value = []
  }
}
const statusLeft = computed(() => statusItems.value.filter((s) => s.align !== 'right'))
const statusRight = computed(() => statusItems.value.filter((s) => s.align === 'right'))

// 博客固定目录 ID（与后端 service.BlogDirID 一致）；图片必须落在博客子树内，
// 公开端点 /api/v1/public/media/{id} 才对读者可见（仅限 image/*，非博客子树 404）
const BLOG_DIR_ID = '00000000-0000-0000-0000-000000000010'

async function uploadImage(file) {
  try {
    // Vditor handler 传入原生 File，包成 uploadFiles 期望的 {file, path}；
    // path 带 assets/ 前缀 → 后端 EnsurePath 自动建 博客/assets/ 目录链
    const path = 'assets/' + (file.name || `image-${Date.now()}.png`)
    const r = await uploadFiles([{ file, path }], BLOG_DIR_ID)
    const arr = Array.isArray(r) ? r : (r && r.items) || []
    const it = arr[0]
    const id = it && it.file && it.file.id
    if (!id) {
      console.warn(t('[editor] 图片上传失败:'), it && it.error, r)
      try { vd?.tip?.(t('图片上传失败，请重试'), 3000) } catch (_) {}
      return ''
    }
    // 同源相对路径：编辑器预览与公开博客页（含 SSR 主题）都能直接渲染
    return `/api/v1/public/media/${id}`
  } catch (e) {
    console.warn(t('[editor] 图片上传异常:'), e)
    try { vd?.tip?.(t('图片上传失败，请重试'), 3000) } catch (_) {}
    return ''
  }
}

// 插件工具栏按钮 → Vditor 自定义 toolbar item
function pluginToolbarItems() {
  return getToolbarItems().map((t) => ({
    name: t.id,
    icon: t.icon || FALLBACK_ICON,
    tip: t.title || t.id,
    tipPosition: 'ne',
    click: () => {
      try {
        t.onClick(makeCtx())
      } catch (e) {
        console.warn(`[editor-plugin] ${t.plugin} 工具栏点击失败:`, e)
      }
    }
  }))
}

function buildToolbar() {
  const base = [
    'undo', 'redo', '|',
    'headings', 'bold', 'italic', 'strike', '|',
    'list', 'ordered-list', 'check', 'outdent', 'indent', '|',
    'quote', 'line', 'code', 'inline-code', '|',
    'upload', 'link', 'table', '|',
    'record', 'edit-mode', 'both', 'preview', 'fullscreen', '|',
    'info'
  ]
  const extras = pluginToolbarItems()
  return extras.length ? [...base, '|', ...extras] : base
}

// 快捷键匹配：'mod+shift+t' / 'alt+x' / 'ctrl+k'
function matchShortcut(combo, e) {
  const parts = String(combo).toLowerCase().split('+').map((s) => s.trim())
  const needMod = parts.includes('mod') || parts.includes('ctrl') || parts.includes('cmd') || parts.includes('meta')
  const needShift = parts.includes('shift')
  const needAlt = parts.includes('alt')
  const key = parts.filter((p) => !['mod', 'ctrl', 'cmd', 'meta', 'shift', 'alt'].includes(p))[0]
  if (!key) return false
  const mod = e.ctrlKey || e.metaKey
  if (needMod !== mod) return false
  if (needShift !== e.shiftKey) return false
  if (needAlt !== e.altKey) return false
  return String(e.key || '').toLowerCase() === key
}

function onKeydown(e) {
  for (const s of getShortcuts()) {
    if (matchShortcut(s.key, e)) {
      e.preventDefault()
      try {
        s.handler(makeCtx())
      } catch (err) {
        console.warn(`[editor-plugin] ${s.plugin} 快捷键执行失败:`, err)
      }
      return
    }
  }
}

onMounted(async () => {
  await nextTick()
  if (!box.value) return
  loadWikiNames(true) // 编辑器挂载时刷新联想清单（不阻塞渲染）
  vd = new Vditor(box.value, {
    height: props.height,
    mode: 'ir',
    theme: 'classic',
    placeholder: props.placeholder,
    cache: { enable: false },
    value: props.modelValue || '',
    hint: { delay: 200, extend: wikiHint() },
    toolbarConfig: { pin: true },
    preview: {
      hljs: { lineNumber: true },
      markdown: {
        toc: true,
        mark: true,
        footnote: true,
        autoSpace: true
      }
    },
    upload: {
      accept: 'image/*',
      multiple: false,
      fieldName: 'files',
      handler: (files) => uploadImage(files[0])
    },
    toolbar: buildToolbar(),
    input: (v) => {
      emit('update:modelValue', v)
      runHook('onInput', v, makeCtx())
      refreshStatus()
    },
    after: () => {
      const cur = vd?.getValue?.() ?? ''
      if (cur !== props.modelValue) emit('update:modelValue', cur)
      runHook('afterRender', undefined, makeCtx())
      refreshStatus()
    }
  })
})

watch(
  () => props.modelValue,
  (v) => {
    if (vd && v !== vd.getValue()) vd.setValue(v || '', false)
  }
)

onBeforeUnmount(() => {
  vd?.destroy?.()
  vd = null
})

defineExpose({
  // 原始 Markdown（不做任何加工）
  getValue: () => vd?.getValue?.() || '',
  // 保存用：先跑插件 beforeSave 钩子链，再返回最终内容
  getSaveValue: () => runHook('beforeSave', vd?.getValue?.() || '', makeCtx()),
  setValue: (md) => vd?.setValue(md || '', false),
  focus: () => vd?.focus?.()
})
</script>

<template>
  <div class="aik-editor" @keydown="onKeydown">
    <div ref="box" class="aik-vditor"></div>
    <div v-if="statusItems.length" class="aik-editor-status">
      <span class="aik-status-group aik-status-left">
        <span v-for="s in statusLeft" :key="s.id" class="aik-status-item">{{ s.text }}</span>
      </span>
      <span class="aik-status-group aik-status-right">
        <span v-for="s in statusRight" :key="s.id" class="aik-status-item">{{ s.text }}</span>
      </span>
    </div>
  </div>
</template>

<style scoped>
.aik-editor {
  border: 1px solid var(--border);
  border-radius: var(--radius);
  overflow: hidden;
  background: var(--surface);
}
.aik-vditor {
  border: none;
  border-radius: 0;
  background: var(--surface);
}
.aik-vditor :deep(.vditor) {
  border: none;
  border-radius: 0;
  background: var(--surface);
}
.aik-vditor :deep(.vditor-toolbar) {
  background: var(--surface-2);
  border-color: var(--border);
}
.aik-vditor :deep(.vditor-toolbar button) {
  color: var(--text-2);
}
.aik-vditor :deep(.vditor-toolbar button:hover) {
  background: var(--surface-3);
  color: var(--text);
}
.aik-vditor :deep(.vditor-content),
.aik-vditor :deep(.vditor-ir),
.aik-vditor :deep(.vditor-preview) {
  background: var(--surface);
}
.aik-vditor :deep(.vditor-ir pre input) {
  background: var(--surface-2);
  color: var(--text-2);
}
.aik-vditor :deep(.vditor-preview pre code) {
  background: var(--surface-2);
}
/* 双链联想弹窗：Vditor 默认 max-width 250px 会截断长文件名，加宽 */
.aik-vditor :deep(.vditor-hint) {
  max-width: 460px;
  min-width: 260px;
}
/* 双链联想项（[[ 触发）：K 徽标 + 文件名 */
.aik-vditor :deep(.wiki-hint-item) {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 2px 4px;
  max-width: 100%;
  box-sizing: border-box;
}
.aik-vditor :deep(.wiki-hint-icon) {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 16px;
  height: 16px;
  border-radius: 4px;
  background: var(--accent, #4a6cf7);
  color: #fff;
  font-size: 10px;
  font-weight: 700;
  flex: 0 0 auto;
}
.aik-vditor :deep(.wiki-hint-name) {
  font-size: 13px;
  color: var(--text, #1a1b1c);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  min-width: 0;
}
/* 插件状态栏（编辑器底部）：左区 / 右区 */
.aik-editor-status {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 4px 12px;
  border-top: 1px solid var(--border);
  background: var(--surface-2);
  color: var(--text-2);
  font-size: 12px;
  line-height: 1.6;
  user-select: none;
}
.aik-status-group {
  display: inline-flex;
  align-items: center;
  gap: 14px;
  min-height: 18px;
}
.aik-status-left {
  justify-content: flex-start;
}
.aik-status-right {
  justify-content: flex-end;
}
.aik-status-item {
  white-space: nowrap;
}
</style>
