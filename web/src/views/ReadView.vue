<template>
  <div ref="rootRef" class="read-view" :style="readingCssVars" :class="{ zen: zenMode }" @click="onBodyClick">
    <!-- 极简顶栏（阅读模式） -->
    <div v-if="isReadingMode" class="read-topbar">
      <button class="icon-btn" :title="$t('返回')" @click="goBack">
        <AikIcon name="arrowLeft" :size="18" />
      </button>
      <span class="read-title">{{  file?.name || $t('阅读')  }}</span>
      <div class="read-tools">
        <button
          v-if="isText && !editMode"
          class="icon-btn"
          :class="{ active: findOpen }"
          :title="$t('页内查找（Ctrl+F）')"
          @click="toggleFind"
        ><AikIcon name="search" :size="16" /></button>
        <button
          v-if="isText && !editMode"
          class="icon-btn btn-edit"
          :title="$t('编辑文档')"
          @click="startEdit"
        ><AikIcon name="edit" :size="16" /></button>
        <button
          v-if="isText && !editMode"
          class="icon-btn"
          :class="{ active: versionsOpen }"
          :title="$t('版本历史（保存前的旧内容会自动留快照）')"
          @click="openVersions"
        ><AikIcon name="clock" :size="16" /></button>
        <button
          v-if="isText && !editMode"
          class="icon-btn"
          :class="{ active: linksOpen }"
          :title="$t('双链 / 反向链接')"
          @click="openLinks"
        ><AikIcon name="link" :size="16" /></button>
        <button
          v-if="file && !editMode"
          class="icon-btn"
          :class="{ active: subscribed }"
          :disabled="subBusy"
          :title="subscribed ? $t('已订阅：这篇文章更新时会通知你（点击取消）') : $t('订阅：这篇文章更新时通知我')"
          @click="toggleSubscribe"
        ><AikIcon name="star" :size="16" /></button>
        <button
          v-if="editMode"
          class="btn btn-sm btn-primary"
          :title="$t('保存（保存后自动重新解读进入知识库）')"
          :disabled="saving"
          @click="saveEdit"
        ><AikIcon name="check" :size="14" /> {{  saving ? $t('保存中…') : $t('保存')  }}</button>
        <button
          v-if="editMode"
          class="btn btn-sm"
          :title="$t('取消编辑')"
          :disabled="saving"
          @click="cancelEdit"
        ><AikIcon name="close" :size="14" /> {{  $t('取消')  }}</button>
        <button
          class="icon-btn"
          :class="{ active: showReadingSettings }"
          :title="$t('阅读设置')"
          @click="showReadingSettings = !showReadingSettings"
        ><AikIcon name="settings" :size="17" /></button>
        <button class="icon-btn" :title="zenMode ? $t('退出禅模式') : $t('禅模式')" @click="zenMode = !zenMode">
          <AikIcon name="fullscreen" :size="16" />
        </button>
        <button class="icon-btn" :title="$t('退出阅读模式')" @click="exitReading">
          <AikIcon name="close" :size="17" />
        </button>
      </div>
    </div>

    <!-- 页内查找条 -->
    <div v-if="findOpen" class="find-bar" @keydown.stop>
      <AikIcon name="search" :size="13" class="find-ico" />
      <input
        ref="findInputRef"
        v-model="findQuery"
        class="find-input"
        :placeholder="$t('在本文中查找…')"
        @input="runFind(1, true)"
        @keydown.enter.exact.prevent="runFind(1)"
        @keydown.shift.enter.prevent="runFind(-1)"
        @keydown.esc="closeFind"
      />
      <span class="find-count">{{  findTotal ? `${findIndex + 1}/${findTotal}` : '0/0'  }}</span>
      <button class="icon-btn find-nav" :title="$t('上一个（Shift+Enter）')" @click="runFind(-1)"><AikIcon name="chevronUp" :size="14" /></button>
      <button class="icon-btn find-nav" :title="$t('下一个（Enter）')" @click="runFind(1)"><AikIcon name="chevronDown" :size="14" /></button>
      <button class="icon-btn find-close" :title="$t('关闭（Esc）')" @click="closeFind"><AikIcon name="close" :size="14" /></button>
    </div>

    <!-- 双链 / 反向链接面板 -->
    <div v-if="linksOpen" class="links-panel">
        <div class="links-head">
          <span>{{  $t('双链 / 反向链接')  }}</span>
          <button class="icon-btn" :title="$t('关闭')" @click="linksOpen = false"><AikIcon name="close" :size="14" /></button>
        </div>
        <div v-if="linksLoading" class="links-empty muted-3">{{  $t('加载中…')  }}</div>
        <template v-else>
          <div class="links-group">
            <div class="links-title">{{  $t('出链 · 本文引用')  }}</div>
            <div v-if="!links.outgoing.length" class="links-empty muted-3">{{  $t('本文没有引用其他文档')  }}</div>
            <button
              v-for="l in links.outgoing"
              :key="'out' + l.id + (l.anchor || '')"
              class="links-item"
              @click="goLink(l)"
            ><AikIcon name="file" :size="13" /> <span class="links-name">{{  l.name  }}</span><span v-if="l.anchor" class="links-anchor">#{{  l.anchor  }}</span></button>
          </div>
          <div class="links-group">
            <div class="links-title">{{  $t('入链 · 引用本文')  }}</div>
            <div v-if="!links.incoming.length" class="links-empty muted-3">{{  $t('暂无文档引用本文')  }}</div>
            <button
              v-for="l in links.incoming"
              :key="'in' + l.id + (l.anchor || '')"
              class="links-item"
              @click="goLink(l)"
            ><AikIcon name="file" :size="13" /> <span class="links-name">{{  l.name  }}</span><span v-if="l.anchor" class="links-anchor">#{{  l.anchor  }}</span></button>
          </div>
        </template>
      </div>

    <!-- 未保存退出确认 -->
    <Transition name="fade">
      <div v-if="dirtyModal" class="modal-mask" @click.self="dirtyModal = null">
        <div class="modal modal-sm">
          <h3>{{ $t('有未保存的修改') }}</h3>
          <p class="modal-desc">{{ $t('当前文档有未保存的修改，保存后才会重新进入知识库解读。') }}</p>
          <div class="modal-actions">
            <button class="btn" @click="dirtyModal = null">{{ $t('继续编辑') }}</button>
            <button class="btn btn-danger-ghost" @click="dirtyDiscard">{{ $t('放弃修改') }}</button>
            <button class="btn btn-primary" @click="dirtySaveAndExit">{{ $t('保存') }}</button>
          </div>
        </div>
      </div>
    </Transition>

    <!-- 编辑态提示条 -->
    <div v-if="editMode" class="edit-bar">
      <span>{{ $t('编辑中 — 保存后自动重新解读并进入知识库') }}</span>
      <span v-if="savedMsg" class="edit-saved">{{  savedMsg  }}</span>
    </div>

    <!-- 博客文章编辑：封面图（仅博客目录下的文章显示；点保存时随元数据接口写入，不影响摘要/SEO） -->
    <div v-if="editMode && isBlogPost" class="edit-cover-bar">
      <span class="ecb-label">{{ $t('封面图') }}</span>
      <input v-model="blogCover" class="ecb-input" :placeholder="$t('https://… 或站内可公开访问的图片地址（留空不设）')" />
      <label class="btn btn-sm ecb-up">
        {{  blogCoverUploading ? $t('上传中…') : $t('上传封面')  }}
        <input type="file" accept="image/*" hidden @change="rvUploadCover" />
      </label>
      <img v-if="blogCover" :src="blogCover" class="ecb-thumb" :alt="$t('封面预览')" @error="$event.target.style.display='none'" />
    </div>

    <!-- 阅读设置抽屉 -->
    <Transition name="slide">
      <div v-if="showReadingSettings" class="reading-panel">
        <div class="rp-title">{{ $t('阅读设置') }}</div>
        <div class="rp-group">
          <span class="rp-label">{{ $t('字号') }}</span>
          <div class="rp-row">
            <button class="btn btn-sm" @click="reading.setSize(reading.size - 1)">A−</button>
            <span class="rp-value">{{  reading.size  }}px</span>
            <button class="btn btn-sm" @click="reading.setSize(reading.size + 1)">A+</button>
          </div>
        </div>
        <div class="rp-group">
          <span class="rp-label">{{ $t('行高') }}</span>
          <div class="rp-row">
            <button class="btn btn-sm" @click="reading.setLineHeight(+(reading.lineHeight - 0.1).toFixed(1))">−</button>
            <span class="rp-value">{{  reading.lineHeight.toFixed(1)  }}</span>
            <button class="btn btn-sm" @click="reading.setLineHeight(+(reading.lineHeight + 0.1).toFixed(1))">+</button>
          </div>
        </div>
        <div class="rp-group">
          <span class="rp-label">{{ $t('字体') }}</span>
          <div class="rp-row rp-switch">
            <button class="btn btn-sm" :class="{ active: reading.font === 'sans' }" @click="reading.setFont('sans')">{{ $t('无衬线') }}</button>
            <button class="btn btn-sm" :class="{ active: reading.font === 'serif' }" @click="reading.setFont('serif')">{{ $t('衬线') }}</button>
          </div>
        </div>
        <div class="rp-group">
          <span class="rp-label">{{ $t('背景') }}</span>
          <div class="rp-bg-row">
            <button
              v-for="b in BGS"
              :key="b.id"
              class="rp-bg"
              :class="{ active: reading.bg === b.id }"
              :style="{ background: b.color }"
              :title="b.name"
              @click="reading.setBg(b.id)"
            ></button>
          </div>
        </div>
        <div class="rp-group">
          <span class="rp-label">{{ $t('页面宽度') }}</span>
          <div class="rp-row rp-switch">
            <button class="btn btn-sm" :class="{ active: reading.width === 'standard' }" @click="reading.setWidth('standard')">{{ $t('标准') }}</button>
            <button class="btn btn-sm" :class="{ active: reading.width === 'wide' }" @click="reading.setWidth('wide')">{{ $t('宽') }}</button>
            <button class="btn btn-sm" :class="{ active: reading.width === 'auto' }" @click="reading.setWidth('auto')">{{ $t('自适应') }}</button>
          </div>
        </div>
      </div>
    </Transition>

    <!-- 版本历史抽屉（B5 写作增强） -->
    <Transition name="slide">
      <div v-if="versionsOpen" class="versions-panel">
        <div class="vp-title">
          <span>{{ $t('版本历史') }}</span>
          <button class="vp-close" :title="$t('关闭')" @click="versionsOpen = false"><AikIcon name="close" :size="14" /></button>
        </div>
        <p class="vp-hint">{{ $t('每次保存前，旧内容会自动留一份快照。恢复历史版本会') }}<b>{{ $t('产生一个新版本') }}</b>{{ $t('，所以恢复本身也能再恢复回去。') }}</p>
        <div v-if="versionsLoading" class="vp-empty">{{ $t('加载中…') }}</div>
        <div v-else-if="versionsErr" class="vp-empty vp-fail">{{  versionsErr  }}</div>
        <div v-else-if="!versions.length" class="vp-empty">{{ $t('还没有历史版本（首次保存后就会产生）') }}</div>
        <div v-else class="vp-list">
          <div v-for="v in versions" :key="v.version" class="vp-item" :class="{ cur: v.version === file?.version }">
            <div class="vp-item-main">
              <div class="vp-item-top">
                <b>v{{  v.version  }}</b>
                <span v-if="v.version === file?.version" class="vp-cur">{{ $t('当前') }}</span>
                <span class="vp-size">{{  formatSize(v.size)  }}</span>
              </div>
              <div class="vp-item-sub">
                <span>{{  fmtVerTime(v.created_at)  }}</span>
                <span v-if="v.note" class="vp-note-tag">{{  v.note  }}</span>
              </div>
            </div>
            <div class="vp-item-ops">
              <button class="btn btn-sm" :disabled="v.version === file?.version" @click="doDownload(v)">{{ $t('下载') }}</button>
              <button class="btn btn-sm" :disabled="v.version === file?.version || restoring" @click="pendingRestore = v.version">{{ $t('恢复') }}</button>
            </div>
            <div v-if="pendingRestore === v.version" class="vp-confirm">
              <span>{{ $t('用 v') }}{{  v.version  }} {{ $t('覆盖当前正文？') }}</span>
              <button class="btn btn-sm btn-primary" :disabled="restoring" @click="doRestore(v.version)">{{  restoring ? $t('恢复中…') : $t('确认恢复')  }}</button>
              <button class="btn btn-sm" :disabled="restoring" @click="pendingRestore = 0">{{ $t('取消') }}</button>
            </div>
          </div>
        </div>
      </div>
    </Transition>

    <!-- 正文 -->
    <div v-if="loading" class="read-loading"><div class="spinner"></div><span>{{ $t('加载中…') }}</span></div>

    <div v-else-if="error" class="read-error">
      <AikIcon name="info" :size="20" />
      <p>{{  error  }}</p>
      <button class="btn btn-primary" @click="goBack">{{ $t('返回') }}</button>
    </div>

    <!-- 文本类：Markdown 渲染（博客化文章排版，发布/分享复用） -->
    <template v-else-if="isText && !editMode">
      <!-- AI 入库解读卡片（仅应用内阅读页，公开页不展示）：摘要 + 元数据 -->
      <div v-if="aiInfo && aiInfo.summary" class="ai-read-card">
        <div class="ai-read-head">
          <AikIcon name="sparkles" :size="14" />
          <span>{{ $t('AI 解读') }}</span>
          <span v-if="aiInfo.meta?.doc_type" class="ai-read-tag">{{  aiInfo.meta.doc_type  }}</span>
          <span v-if="aiInfo.meta?.language" class="ai-read-tag">{{  aiInfo.meta.language === 'en' ? 'EN' : $t('中文')  }}</span>
        </div>
        <p class="ai-read-summary">{{  aiInfo.summary  }}</p>
        <div v-if="hasAiMeta" class="ai-read-meta">
          <span v-if="aiInfo.meta?.author" class="ai-read-kv"><b>{{ $t('作者') }}</b>{{  aiInfo.meta.author  }}</span>
          <span v-if="aiInfo.meta?.source" class="ai-read-kv"><b>{{ $t('来源') }}</b>{{  aiInfo.meta.source  }}</span>
          <span v-if="aiInfo.meta?.title" class="ai-read-kv"><b>{{ $t('标题') }}</b>{{  aiInfo.meta.title  }}</span>
        </div>
        <div v-if="aiInfo.meta?.keywords?.length" class="ai-read-kws">
          <span v-for="k in aiInfo.meta.keywords" :key="k" class="ai-read-kw">{{  k  }}</span>
        </div>
      </div>
      <MarkdownArticle
        :title="file?.name || ''"
        :html="html"
        :meta="readMeta"
      />
    </template>

    <!-- 文本类：编辑模式（编辑器本体轨：与写作轨共用同一编辑器偏好，2026-10-09 统一） -->
    <div v-else-if="isText && editMode" class="edit-wrap">
      <component :is="quickEditorComp" ref="editorRef" v-model="editContent" :height="Math.max(320, windowHeight - 160)" class="vd-wrap" />
    </div>

    <!-- 图片 -->
    <div v-else-if="isImage" class="media-view">
      <img :src="mediaUrl || ''" :alt="file.name" class="media-img" @error="error = t('图片加载失败')" />
      <div class="media-meta">{{  file.name  }} · {{  formatSize(file.size)  }}</div>
    </div>

    <!-- 视频 -->
    <div v-else-if="isVideo" class="media-view">
      <video :src="mediaUrl || ''" controls class="media-video"></video>
      <div class="media-meta">{{  file.name  }}</div>
    </div>

    <!-- 音频 -->
    <div v-else-if="isAudio" class="media-view">
      <audio :src="mediaUrl || ''" controls class="media-audio"></audio>
      <div class="media-meta">{{  file.name  }}</div>
    </div>

    <!-- PDF -->
    <div v-else-if="isPdf" class="pdf-view">
      <iframe :src="mediaUrl || ''" class="pdf-frame" :title="$t('PDF 预览')"></iframe>
    </div>

    <!-- 其他：下载 -->
    <div v-else class="media-view unsupported">
      <AikIcon name="file" :size="40" />
      <p class="unsupported-title">{{ $t('暂不支持在线预览此格式') }}</p>
      <p class="unsupported-desc">{{  file.name  }}</p>
      <a class="btn btn-primary" :href="mediaUrl || '#'" download>
        <AikIcon name="download" :size="15" />
        <span>{{ $t('下载文件') }}</span>
      </a>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, shallowRef, onMounted, watch, nextTick, onBeforeUnmount } from 'vue'
import { useRouter } from 'vue-router'
import AikIcon from '@/components/AikIcon.vue'
import { useReadingStore } from '@/stores/reading'
import { getFile, fetchTextContent, fetchMediaUrl, revokeMediaUrl, updateDocContent, getChunk, getLocateChunk, getFileLinks, resolveWikiTitle, getFileSummary, listFileVersions, restoreFileVersion, downloadFileVersion, subscriptionStatus, subscribeTarget, unsubscribeTarget, requestJSON, uploadFiles, mkdir } from '@/api'
import { renderMarkdown } from '@/utils/markdown'
import { getEditorLoader, readEditorPref } from '@/editors/registry'
import MarkdownArticle from '@/components/MarkdownArticle.vue'

const props = defineProps({
  fileId: { type: String, default: '' },
  // 溯源定位：搜索页点击"定位原文"传入的 chunk id
  hl: { type: String, default: '' },
  // 溯源定位：搜索关键词（优先精确命中处，其次 chunk 锚点）
  hlQ: { type: String, default: '' }
})
const emit = defineEmits(['dirty-change', 'title-change'])

const router = useRouter()
const reading = useReadingStore()

const file = ref(null)
const mediaUrl = ref('')
// F5 修复：记录当前 mediaUrl 对应的文件 id，切换/卸载时据此 revoke 旧 objectURL
const mediaId = ref('')
const html = ref('')
const loading = ref(true)
const error = ref('')
const isReadingMode = ref(true)
const zenMode = ref(false)
const showReadingSettings = ref(false)
// AI 入库解读（K19）：摘要 + 元数据（author/source/keywords/doc_type/language）
const aiInfo = ref(null)
// 编辑模式（Vditor Markdown 编辑器）
const editMode = ref(false)
const editContent = ref('')
const editorRef = ref(null)
// 编辑器本体轨：快编与写作共用同一偏好（readEditorPref），进编辑模式时解析组件
const quickEditorComp = shallowRef(null)
async function loadQuickEditor() {
  const id = readEditorPref()
  const loader = getEditorLoader(id)
  if (!loader) return
  try {
    const mod = await loader()
    quickEditorComp.value = mod.default || mod
  } catch (e) {
    console.warn('[read] 编辑器加载失败，回退 plain:', e)
    try {
      const m = await getEditorLoader('plain')()
      quickEditorComp.value = m.default || m
    } catch (_) { /* 组件都不可用时保持 null */ }
  }
}
const saving = ref(false)
const savedMsg = ref('')
const windowHeight = ref(window.innerHeight)
import { useToastStore } from '@/stores/toast'
import { t } from '@/i18n'
const toastStore = useToastStore()

// ---- 内容订阅（B6 订阅与通知）----
const subscribed = ref(false)
const subBusy = ref(false)

async function loadSubStatus() {
  if (!props.fileId) {
    subscribed.value = false
    return
  }
  try {
    const d = await subscriptionStatus('file', props.fileId)
    subscribed.value = !!d.subscribed
  } catch (_) {
    subscribed.value = false
  }
}

async function toggleSubscribe() {
  if (!props.fileId || subBusy.value) return
  subBusy.value = true
  try {
    const d = subscribed.value
      ? await unsubscribeTarget('file', props.fileId)
      : await subscribeTarget('file', props.fileId)
    subscribed.value = !!d.subscribed
    toastStore.push(subscribed.value ? t('已订阅：这篇文章更新时会通知你') : t('已取消订阅'))
  } catch (e) {
    toastStore.push(t('订阅操作失败：') + (e.message || e))
  } finally {
    subBusy.value = false
  }
}

watch(() => props.fileId, () => { loadSubStatus() }, { immediate: true })

// ---- 版本历史（B5 写作增强）----
const versionsOpen = ref(false)
const versions = ref([])
const versionsLoading = ref(false)
const versionsErr = ref('')
const restoring = ref(false)
const pendingRestore = ref(0)

async function openVersions() {
  versionsOpen.value = !versionsOpen.value
  if (versionsOpen.value) await loadVersions()
}

async function loadVersions() {
  if (!file.value?.id) return
  versionsLoading.value = true
  versionsErr.value = ''
  try {
    const d = await listFileVersions(file.value.id)
    versions.value = d.versions || []
  } catch (e) {
    versionsErr.value = t('加载版本历史失败：') + (e.message || e)
  } finally {
    versionsLoading.value = false
  }
}

async function doDownload(v) {
  try {
    await downloadFileVersion(file.value.id, v.version, file.value.name)
  } catch (e) {
    showToast(t('下载版本失败：') + (e.message || e))
  }
}

async function doRestore(version) {
  if (restoring.value) return
  restoring.value = true
  try {
    await restoreFileVersion(file.value.id, version)
    pendingRestore.value = 0
    await load()
    await loadVersions()
    showToast(t('已恢复 v') + version + t('（生成了新版本 v') + (file.value?.version ?? '') + '）')
  } catch (e) {
    showToast(t('恢复失败：') + (e.message || e))
  } finally {
    restoring.value = false
  }
}

function fmtVerTime(ts) {
  if (!ts) return '—'
  const dt = new Date(ts)
  const diff = Date.now() - dt.getTime()
  if (diff >= 0 && diff < 60e3) return t('刚刚')
  if (diff >= 0 && diff < 3600e3) return Math.floor(diff / 60e3) + t(' 分钟前')
  if (diff >= 0 && diff < 86400e3) return Math.floor(diff / 3600e3) + t(' 小时前')
  const p = (x) => String(x).padStart(2, '0')
  return dt.getFullYear() + '-' + p(dt.getMonth() + 1) + '-' + p(dt.getDate()) + ' ' + p(dt.getHours()) + ':' + p(dt.getMinutes())
}

// ---- 页内查找（Ctrl+F / Enter / Shift+Enter / Esc）----
const findOpen = ref(false)
const findQuery = ref('')
const findIndex = ref(-1)
const findTotal = ref(0)
const findInputRef = ref(null)
const rootRef = ref(null)
let findMarks = []

// 当前实例内的正文容器（多标签下多个 ReadView 并存，必须限定本实例）
function artBodyEl() {
  return rootRef.value ? rootRef.value.querySelector('.art-body') : null
}

// 在正文中标记全部命中并返回 mark 数组（调用方负责先 clearFindMarks）
function markAll(body, q) {
  const lower = q.toLowerCase()
  const walker = document.createTreeWalker(body, NodeFilter.SHOW_TEXT, {
    acceptNode(n) {
      return n.textContent.toLowerCase().includes(lower)
        ? NodeFilter.FILTER_ACCEPT
        : NodeFilter.FILTER_REJECT
    }
  })
  const nodes = []
  let n
  while ((n = walker.nextNode())) nodes.push(n)
  const marks = []
  // 文本节点内可能多次出现：splitText 逐个包裹
  for (let node of nodes) {
    let text = node.textContent
    let idx = text.toLowerCase().indexOf(lower)
    while (idx !== -1) {
      const mark = document.createElement('mark')
      mark.className = 'find-mark'
      const tail = node.splitText(idx)
      const hit = tail.splitText(q.length)
      mark.textContent = tail.textContent
      tail.replaceWith(mark)
      marks.push(mark)
      node = hit
      text = hit.textContent
      idx = text.toLowerCase().indexOf(lower)
    }
  }
  return marks
}

function toggleFind() {
  findOpen.value = !findOpen.value
  if (findOpen.value) {
    nextTick(() => findInputRef.value && findInputRef.value.focus())
  } else {
    clearFindMarks()
  }
}

function closeFind() {
  findOpen.value = false
  clearFindMarks()
}

// 清除全部高亮（mark → 原文本节点，保持 DOM 结构不变）
function clearFindMarks() {
  const body = artBodyEl()
  if (!body) return
  const marks = body.querySelectorAll('mark.find-mark')
  marks.forEach((m) => {
    const txt = document.createTextNode(m.textContent)
    m.replaceWith(txt)
  })
  findMarks = []
  findTotal.value = 0
  findIndex.value = -1
}

// 查找并定位：dir=1 下一个 / -1 上一个（循环）；queryChanged=输入变化（重新从第一个开始）
function runFind(dir, queryChanged = false) {
  const q = (findQuery.value || '').trim()
  if (!q) {
    clearFindMarks()
    return
  }
  const body = artBodyEl()
  if (!body) return
  const prevIndex = findIndex.value
  // 重新查找（每次输入变化全量重建）
  clearFindMarks()
  findMarks = markAll(body, q)
  findTotal.value = findMarks.length
  if (!findMarks.length) {
    findIndex.value = -1
    return
  }
  // 定位：输入变化从头开始；导航基于上一次位置步进（循环）
  let next
  if (queryChanged) {
    next = dir > 0 ? 0 : findMarks.length - 1
  } else {
    next = dir > 0 ? prevIndex + 1 : prevIndex - 1
    if (next >= findMarks.length) next = 0
    if (next < 0) next = findMarks.length - 1
  }
  findIndex.value = next
  gotoMark(next)
}

function gotoMark(i) {
  const body = artBodyEl()
  if (!body) return
  const marks = body.querySelectorAll('mark.find-mark')
  marks.forEach((m, k) => m.classList.toggle('find-active', k === i))
  const m = marks[i]
  if (m) m.scrollIntoView({ block: 'center', behavior: 'smooth' })
}

// Ctrl+F 快捷键（仅当前可见实例响应，避免多标签隐藏实例同时开关）
function onKeydown(e) {
  if ((e.ctrlKey || e.metaKey) && (e.key === 'f' || e.key === 'F')) {
    if (!isText.value || editMode.value) return
    if (rootRef.value && !rootRef.value.offsetParent) return
    e.preventDefault()
    toggleFind()
  }
}
onMounted(() => window.addEventListener('keydown', onKeydown))
onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKeydown)
  clearFindMarks()
  // F5 修复：卸载时释放媒体 objectURL
  if (mediaId.value) {
    revokeMediaUrl(mediaId.value)
    mediaId.value = ''
  }
})

const BGS = [
  { id: 'default', name: t('默认'), color: '#fbfbf9' },
  { id: 'sepia', name: t('纸感'), color: '#f7f3ea' },
  { id: 'green', name: t('护眼'), color: '#eef3ea' },
  { id: 'dark', name: t('暗色'), color: '#161821' }
]

const EXT = {
  image: ['png', 'jpg', 'jpeg', 'gif', 'webp', 'svg', 'bmp', 'avif'],
  video: ['mp4', 'mov', 'mkv', 'webm', 'avi'],
  audio: ['mp3', 'wav', 'm4a', 'flac', 'ogg'],
  text: ['md', 'markdown', 'txt', 'log', 'json', 'html', 'htm', 'css', 'js', 'ts', 'py', 'go', 'java', 'c', 'cpp', 'vue', 'sql', 'sh'],
  pdf: ['pdf']
}

function ext(name) {
  const i = name.lastIndexOf('.')
  return i >= 0 ? name.slice(i + 1).toLowerCase() : ''
}

const kind = computed(() => {
  if (!file.value) return ''
  const e = ext(file.value.name)
  for (const [k, arr] of Object.entries(EXT)) {
    if (arr.includes(e)) return k
  }
  return 'other'
})

const isText = computed(() => kind.value === 'text')
const isImage = computed(() => kind.value === 'image')
const isVideo = computed(() => kind.value === 'video')
const isAudio = computed(() => kind.value === 'audio')
const isPdf = computed(() => kind.value === 'pdf')

// AI 解读元数据是否有可展示项（作者/来源/标题至少一项）
const hasAiMeta = computed(() => {
  const m = aiInfo.value?.meta
  return !!(m && (m.author || m.source || m.title))
})

const readingCssVars = computed(() => reading.cssVars())

function formatSize(n) {
  if (n < 1024) return n + ' B'
  if (n < 1024 * 1024) return (n / 1024).toFixed(1) + ' KB'
  if (n < 1024 * 1024 * 1024) return (n / 1024 / 1024).toFixed(1) + ' MB'
  return (n / 1024 / 1024 / 1024).toFixed(2) + ' GB'
}

function formatTime(sec) {
  if (!sec) return ''
  const d = new Date(sec * 1000)
  const p = (n) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`
}

// 文章元信息（博客化展示）：大小 · 更新时间 · 来源
const readMeta = computed(() => {
  if (!file.value) return []
  const meta = []
  if (file.value.size > 0) meta.push(formatSize(file.value.size))
  if (file.value.updated_at) meta.push(formatTime(file.value.updated_at) + t(' 更新'))
  meta.push(t('本地存储'))
  return meta
})

async function load() {
  const id = props.fileId
  if (!id) {
    error.value = t('缺少文件 ID')
    loading.value = false
    return
  }
  loading.value = true
  error.value = ''
  // F5 修复：切换文件/重载前释放上一个媒体 objectURL（blob 常驻内存，不 revoke 会随浏览累积）
  if (mediaId.value) {
    revokeMediaUrl(mediaId.value)
    mediaId.value = ''
  }
  mediaUrl.value = ''
  try {
    file.value = await getFile(id)
    emit('title-change', file.value?.name)
    if (isText.value && kind.value !== 'html') {
      const text = await fetchTextContent(id)
      html.value = renderMarkdown(text)
    } else if (kind.value === 'html') {
      const text = await fetchTextContent(id)
      html.value = text
    } else if (!isText.value) {
      // 媒体类（图片/音视频/PDF）：鉴权后 blob 加载
      mediaUrl.value = await fetchMediaUrl(id)
      mediaId.value = id
    }
    // AI 入库解读（摘要+元数据 K19）：失败静默，不影响正文
    try {
      const s = await getFileSummary(id)
      if (s && (s.status === 'done' || (s.summary && s.tags))) {
        aiInfo.value = {
          status: s.status === 'done' ? 'done' : 'pending',
          summary: s.summary || '',
          tags: s.tags || [],
          meta: s.meta || {}
        }
      }
    } catch (_) {
      aiInfo.value = null
    }
  } catch (e) {
    error.value = e.message
  } finally {
    loading.value = false
  }
}

function goBack() {
  if (editMode.value && dirtyEdit.value) {
    openDirtyModal(doGoBack)
    return
  }
  doGoBack()
}

function doGoBack() {
  if (window.history.length > 1) router.back()
  else router.push('/files')
}

function exitReading() {
  goBack()
}

// ---- 编辑模式 ----

// 博客文章封面（阅读页编辑态也可设封面；仅博客目录下的文章显示此栏）
const BLOG_DIR = '00000000-0000-0000-0000-000000000010'
const isBlogPost = ref(false)
const blogCover = ref('')
const blogCoverUploading = ref(false)
let blogMediaDirId = ''
async function blogMediaDir() {
  if (blogMediaDirId) return blogMediaDirId
  try {
    const items = await requestJSON('/files?parent=' + BLOG_DIR)
    const hit = (Array.isArray(items) ? items : items?.items || []).find((x) => x.kind === 'dir' && x.name === '媒体')
    if (hit?.id) { blogMediaDirId = hit.id; return blogMediaDirId }
  } catch (_) { /* 列目录失败则直接建 */ }
  const d = await mkdir(t('媒体'), BLOG_DIR)
  blogMediaDirId = d?.id || ''
  return blogMediaDirId
}
async function rvUploadCover(e) {
  const f = e.target?.files?.[0]
  e.target.value = ''
  if (!f || !/^image\//.test(f.type || '')) return
  blogCoverUploading.value = true
  try {
    const parent = await blogMediaDir()
    if (!parent) throw new Error(t('媒体目录不可用'))
    const rs = await uploadFiles([{ file: f, path: f.name }], parent)
    const fid = rs?.[0]?.file?.id
    if (!fid) throw new Error(t('上传无结果'))
    blogCover.value = '/api/v1/public/media/' + fid
  } catch (err) {
    showToast(t('封面上传失败：') + (err.message || err))
  } finally {
    blogCoverUploading.value = false
  }
}
// 进入编辑时探测：当前文档是否为博客文章（是则带出已设封面）。非博客/无权限时静默不显示。
async function probeBlogMeta() {
  isBlogPost.value = false
  blogCover.value = ''
  try {
    const d = await requestJSON('/blog/manage')
    const p = (d.posts || []).find((x) => x.id === file.value?.id)
    if (p) {
      isBlogPost.value = true
      blogCover.value = p.cover || ''
    }
  } catch (_) { /* 非博客管理员等：不显示封面栏 */ }
}

async function startEdit() {
  try {
    const text = await fetchTextContent(file.value.id)
    editContent.value = text
    editOriginal.value = text
    editMode.value = true
    savedMsg.value = ''
    if (!quickEditorComp.value) await loadQuickEditor()
    probeBlogMeta() // 异步探测，不阻塞编辑器打开
  } catch (e) {
    error.value = e.message
  }
}

// 编辑脏检测：进入编辑后的内容与原文不一致 → 未保存
const editOriginal = ref('')
const dirtyEdit = computed(() => editMode.value && editContent.value !== editOriginal.value)

// 未保存退出确认：保存 / 放弃 / 继续编辑
const dirtyModal = ref(null) // { onSave, onDiscard }
function openDirtyModal(onDiscard) {
  dirtyModal.value = { onDiscard }
}
async function dirtySaveAndExit() {
  const cb = dirtyModal.value?.onDiscard
  dirtyModal.value = null
  if (editMode.value) {
    await saveEdit()
    if (editMode.value) return // 保存失败，留在编辑态
  }
  if (cb) cb()
}
function dirtyDiscard() {
  const cb = dirtyModal.value?.onDiscard
  dirtyModal.value = null
  editMode.value = false
  editContent.value = ''
  savedMsg.value = ''
  if (cb) cb()
}

function cancelEdit() {
  if (dirtyEdit.value) {
    openDirtyModal(() => {
      editMode.value = false
      editContent.value = ''
      savedMsg.value = ''
    })
    return
  }
  editMode.value = false
  editContent.value = ''
  savedMsg.value = ''
}

async function saveEdit() {
  if (saving.value) return
  saving.value = true
  savedMsg.value = ''
  try {
    // 先走编辑器插件 beforeSave 钩子链（getSaveValue），无插件时回退 v-model 原文
    const finalContent =
      editorRef.value && typeof editorRef.value.getSaveValue === 'function'
        ? editorRef.value.getSaveValue()
        : editContent.value
    await updateDocContent(file.value.id, finalContent)
    // 博客文章：封面一并保存（后端指针语义，只传 cover 不会动摘要/SEO；失败不阻断正文保存）
    if (isBlogPost.value) {
      try {
        await requestJSON('/blog/posts/meta', {
          method: 'POST',
          body: JSON.stringify({ id: file.value.id, cover: blogCover.value }),
        })
      } catch (me) {
        showToast(t('正文已保存，但封面保存失败：') + (me.message || me))
      }
    }
    // 重新加载渲染 + 提示（AI 后台会自动重新解读/向量化）
    editMode.value = false
    savedMsg.value = ''
    file.value = await getFile(file.value.id)
    emit('title-change', file.value?.name)
    if (kind.value !== 'html') {
      html.value = renderMarkdown(finalContent)
    } else {
      html.value = finalContent
    }
    showToast(t('已保存，已重新进入知识库解读'))
  } catch (e) {
    error.value = e.message
  } finally {
    saving.value = false
  }
}

function showToast(text) {
  toastStore.push(text)
}

onMounted(load)
watch(() => props.fileId, load)

// 未保存编辑标记 → 通知 tab store（TabBar 显示圆点 / 关闭时确认）
watch([editMode, dirtyEdit], () => {
  emit('dirty-change', !!(editMode.value && dirtyEdit.value))
}, { immediate: true })

// ---- 溯源定位（?hl=chunkId&q=搜索词 → 渲染后高亮命中段落）----
// applyHl：定位并高亮 chunk（chunkId 为空静默跳过）；被 hl/hlQ watch 与锚点跳转复用。
async function applyHl(chunkId, q) {
  if (!chunkId || editMode.value) return
  // 等正文内容渲染完成（文档内容异步加载；.art-body 容器可能先于 html 存在）
  let tries = 0
  while (!html.value && tries < 50) {
    await new Promise(r => setTimeout(r, 100))
    tries++
  }
  await nextTick()
  const body = artBodyEl()
  if (!body) return
  try {
    const c = await getChunk(chunkId)
    const content = (c.content || '').trim()
    if (!content) return
    if (findOpen.value) closeFind()
    clearFindMarks()
    // 清洗 markdown 标记 → 渲染后 DOM 里真实存在的纯文本
    const cleanMd = (s) =>
      (s || '')
        .replace(/\[([^\]]*)\]\([^)]*\)/g, '$1') // 链接 → 链接文字
        .replace(/!?\[\[|\]\]|\*\*|__|~~|`/g, '') // 双链/加粗/行内码/删除线
        .split('\n')
        .map(l => l.replace(/^#{1,6}\s*|^>\s*|^[-*+]\s+|^\d+[.、]\s*/, '').trim())
        .filter(Boolean)
        .join(' ')
        .replace(/\s+/g, ' ')
        .trim()
    // 候选定位串：搜索词 > chunk 锚点 > chunk 开头；逐级加长重试
    const candidates = []
    if (q) candidates.push(cleanMd(q).slice(0, 40), cleanMd(q).slice(0, 20))
    if (c.anchor) candidates.push(cleanMd(c.anchor).slice(0, 40), cleanMd(c.anchor).slice(0, 20))
    const full = cleanMd(content)
    if (full) candidates.push(full.slice(0, 40), full.slice(0, 20))
    let marks = []
    for (const seg of candidates) {
      if (!seg) continue
      marks = markAll(body, seg)
      if (marks.length) break
    }
    if (marks.length) {
      marks[0].classList.add('find-active')
      marks[0].scrollIntoView({ block: 'center', behavior: 'smooth' })
    }
  } catch (_) { /* 定位失败静默（块已删/内容已改） */ }
}

watch(
  [() => props.hl, () => props.hlQ],
  ([chunkId]) => applyHl(chunkId, props.hlQ),
  { immediate: true }
)

// ---- 双链/反向链接面板 ----
const linksOpen = ref(false)
const linksLoading = ref(false)
const links = ref({ outgoing: [], incoming: [] })

async function openLinks() {
  linksOpen.value = !linksOpen.value
  if (!linksOpen.value || !props.fileId) return
  if (linksLoading.value) return
  linksLoading.value = true
  try {
    const data = await getFileLinks(props.fileId)
    links.value = { outgoing: data.outgoing || [], incoming: data.incoming || [] }
  } catch (_) {
    links.value = { outgoing: [], incoming: [] }
  } finally {
    linksLoading.value = false
  }
}

// 正文双链点击：[[标题]] 渲染为 a.wiki-link（data-title/data-anchor）→ 解析文件 id → 跳转（Obsidian 阅读态同款）
async function onBodyClick(e) {
  const a = e.target && e.target.closest ? e.target.closest('a.wiki-link') : null
  if (!a || editMode.value) return
  const title = a.getAttribute('data-title') || ''
  if (!title) return
  e.preventDefault()
  e.stopPropagation()
  try {
    const r = await resolveWikiTitle(title)
    if (r && r.id) goLink({ id: r.id, anchor: a.getAttribute('data-anchor') || '' })
  } catch (_) {
    /* 未匹配文件：静默（Obsidian 未创建链接同款，不跳转） */
  }
}

// 跳转双链目标：带段落锚点（#标题 / #^块ID）时先定位 chunk 再带 hl+q 高亮跳转，复用溯源定位链路。
async function goLink(link) {
  linksOpen.value = false
  const id = link.id
  const sameFile = router.currentRoute.value.params.id === id
  if (link.anchor) {
    try {
      const loc = await getLocateChunk(id, link.anchor)
      if (loc && loc.chunk_id) {
        if (sameFile) {
          applyHl(String(loc.chunk_id), link.anchor)
        } else {
          router.push({ path: `/read/${id}`, query: { hl: loc.chunk_id, q: link.anchor } })
        }
        return
      }
    } catch (_) { /* 锚点定位失败 → 回退整文件打开 */ }
  }
  if (!sameFile) router.push({ path: `/read/${id}` })
}
</script>

<style scoped>
.read-view {
  min-height: 100%;
  background: var(--bg);
  position: relative;
}

/* 页内查找条 */
.find-bar {
  position: sticky;
  top: var(--topbar-h);
  z-index: var(--z-topbar);
  display: flex;
  align-items: center;
  gap: 6px;
  margin: 10px auto 0;
  width: 420px;
  max-width: calc(100% - 32px);
  padding: 6px 8px;
  background: var(--surface-1);
  border: 1px solid var(--border);
  border-radius: 10px;
  box-shadow: 0 8px 28px rgba(0, 0, 0, 0.14);
}

.find-ico {
  color: var(--text-3);
  flex-shrink: 0;
}

.find-input {
  flex: 1;
  min-width: 0;
  height: 26px;
  border: none;
  outline: none;
  background: transparent;
  color: var(--text);
  font-size: 13px;
}

.find-count {
  font-size: 12px;
  color: var(--text-3);
  white-space: nowrap;
  min-width: 40px;
  text-align: center;
}

.find-nav,
.find-close {
  width: 26px;
  height: 26px;
  flex-shrink: 0;
}

/* 双链 / 反向链接面板 */
.links-panel {
  position: sticky;
  top: calc(var(--topbar-h) + 8px);
  z-index: calc(var(--z-topbar) + 1);
  width: 320px;
  max-width: calc(100% - 32px);
  margin: 10px auto 0;
  padding: 10px 12px;
  background: var(--surface-1);
  border: 1px solid var(--border);
  border-radius: 10px;
  box-shadow: 0 8px 28px rgba(0, 0, 0, 0.14);
}

.links-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 13px;
  font-weight: 600;
  color: var(--text);
  margin-bottom: 8px;
}

.links-group + .links-group {
  margin-top: 10px;
}

.links-title {
  font-size: 12px;
  color: var(--text-3);
  margin-bottom: 5px;
}

.links-empty {
  font-size: 12px;
  padding: 4px 2px;
}

.links-item {
  display: flex;
  align-items: center;
  gap: 6px;
  width: 100%;
  padding: 5px 6px;
  border: none;
  border-radius: 6px;
  background: transparent;
  color: var(--text-2);
  font-size: 13px;
  text-align: left;
  cursor: pointer;
}

.links-item:hover {
  background: var(--surface-2, rgba(0, 0, 0, 0.05));
  color: var(--primary);
}

.links-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.links-anchor {
  flex: 0 0 auto;
  max-width: 40%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 11px;
  color: var(--accent, #4a6cf7);
  background: rgba(74, 108, 247, 0.08);
  border-radius: 4px;
  padding: 1px 5px;
  margin-left: 6px;
}

/* 正文高亮（作用于 MarkdownArticle 子组件 DOM） */:deep(.art-body mark.find-mark) {
  background: rgba(250, 173, 20, 0.35);
  color: inherit;
  border-radius: 2px;
  padding: 0 1px;
}

:deep(.art-body mark.find-mark.find-active) {
  background: rgba(250, 173, 20, 0.75);
  color: #1a1b1c;
  outline: 1.5px solid rgba(180, 110, 0, 0.6);
}

/* 正文双链（[[标题]] → a.wiki-link，Obsidian 阅读态同款：主色 + hover 下划线） */
:deep(.art-body a.wiki-link) {
  color: var(--accent, #4a6cf7);
  text-decoration: none;
  border-bottom: 1px dashed rgba(74, 108, 247, 0.45);
  cursor: pointer;
  padding: 0 1px;
}
:deep(.art-body a.wiki-link:hover) {
  background: rgba(74, 108, 247, 0.08);
  border-bottom-style: solid;
}

/* 阅读顶栏 */
.read-topbar {
  height: var(--topbar-h);
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 18px;
  background: var(--surface);
  border-bottom: 1px solid var(--border);
  position: sticky;
  top: 0;
  z-index: var(--z-topbar);
}

.read-title {
  font-size: var(--fs-base);
  font-weight: 600;
  color: var(--text);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  flex: 1;
  min-width: 0;
}

.read-tools {
  display: flex;
  align-items: center;
  gap: 2px;
}

.icon-btn.active {
  background: var(--primary-soft);
  color: var(--primary);
}

.btn-edit {
  color: var(--primary);
}

.btn-edit[disabled] {
  opacity: 0.5;
  pointer-events: none;
}

/* 编辑态 */
.edit-bar {
  height: 36px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 0 20px;
  background: var(--primary-soft);
  color: var(--primary);
  font-size: var(--fs-small);
  border-bottom: 1px solid var(--border);
}

/* 博客文章编辑：封面图栏 */
.edit-cover-bar {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 20px;
  background: var(--primary-soft);
  border-bottom: 1px solid var(--border);
  font-size: var(--fs-small);
}

.ecb-label {
  flex-shrink: 0;
  color: var(--text-2, #4b5563);
  font-weight: 600;
}

.ecb-input {
  flex: 1;
  min-width: 0;
  border: 1px solid var(--border, #e3e6eb);
  border-radius: 6px;
  padding: 5px 8px;
  font-size: 12px;
  font-family: inherit;
  background: var(--surface, #fff);
  color: var(--text, #1f2329);
}

.ecb-input:focus {
  outline: none;
  border-color: var(--primary, #4c7df0);
}

.ecb-up {
  flex-shrink: 0;
  position: relative;
}

.ecb-thumb {
  flex-shrink: 0;
  width: 46px;
  height: 30px;
  object-fit: cover;
  border-radius: 5px;
  border: 1px solid var(--border, #e3e6eb);
}

.edit-saved {
  color: var(--success, #16a34a);
}

.edit-wrap {
  padding: 12px 20px 20px;
  height: calc(100vh - var(--topbar-h) - 36px);
}

.vd-wrap :deep(.vditor) {
  border-radius: var(--radius);
}

/* 未保存确认弹窗 */
.modal-mask {
  position: fixed;
  inset: 0;
  background: rgba(15, 20, 35, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: var(--z-modal, 1000);
}

.modal {
  width: 420px;
  max-width: calc(100vw - 48px);
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  box-shadow: var(--shadow-lg);
  padding: 20px 22px;
}

.modal h3 {
  margin: 0 0 10px;
  font-size: 16px;
  color: var(--text);
}

.modal-desc {
  margin: 0 0 18px;
  font-size: 13px;
  color: var(--text-3);
  line-height: 1.7;
}

.modal-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}

.btn-danger-ghost {
  color: var(--danger);
  border-color: var(--danger);
  background: transparent;
}

.btn-danger-ghost:hover {
  background: var(--danger-soft, rgba(220, 38, 38, 0.1));
}

/* 阅读设置面板 */
.versions-panel {
  position: fixed;
  top: var(--topbar-h);
  right: 16px;
  width: 300px;
  max-height: 70vh;
  overflow-y: auto;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  box-shadow: var(--shadow-lg);
  padding: 14px;
  z-index: var(--z-drawer);
}
.vp-title { display: flex; align-items: center; justify-content: space-between; font-size: 13.5px; font-weight: 600; margin-bottom: 6px; }
.vp-close { display: inline-flex; align-items: center; justify-content: center; width: 24px; height: 24px; border: 0; border-radius: 6px; background: transparent; color: var(--text-3); cursor: pointer; }
.vp-close:hover { background: var(--surface-2); color: var(--text); }
.vp-hint { font-size: 11.5px; line-height: 1.6; color: var(--text-3); margin: 0 0 10px; }
.vp-empty { padding: 22px 4px; text-align: center; font-size: 12px; color: var(--text-3); }
.vp-fail { color: #e03e3e; }
.vp-list { display: flex; flex-direction: column; gap: 7px; }
.vp-item { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; padding: 9px 10px; border: 1px solid var(--border); border-radius: 9px; background: var(--surface-2); }
.vp-item.cur { border-color: var(--primary); background: var(--primary-soft); }
.vp-item-main { flex: 1; min-width: 0; }
.vp-item-top { display: flex; align-items: center; gap: 7px; font-size: 12.5px; }
.vp-item-top b { color: var(--text); }
.vp-cur { font-size: 10.5px; padding: 1px 6px; border-radius: 8px; background: var(--primary); color: #fff; }
.vp-size { color: var(--text-3); font-size: 11.5px; }
.vp-item-sub { display: flex; align-items: center; gap: 7px; flex-wrap: wrap; margin-top: 2px; font-size: 11px; color: var(--text-3); }
.vp-note-tag { padding: 1px 6px; border-radius: 8px; background: var(--surface); border: 1px solid var(--border); }
.vp-item-ops { display: flex; gap: 5px; }
.vp-confirm { flex: 1 1 100%; display: flex; align-items: center; gap: 7px; flex-wrap: wrap; padding-top: 7px; border-top: 1px dashed var(--border); font-size: 11.5px; color: var(--text-2); }

.reading-panel {
  position: fixed;
  top: var(--topbar-h);
  right: 16px;
  width: 240px;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  box-shadow: var(--shadow-lg);
  padding: 16px;
  z-index: var(--z-drawer);
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.rp-title {
  font-size: var(--fs-base);
  font-weight: 600;
}

.rp-group {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.rp-label {
  font-size: var(--fs-micro);
  color: var(--text-3);
}

.rp-row {
  display: flex;
  align-items: center;
  gap: 8px;
}

.rp-value {
  font-size: var(--fs-small);
  color: var(--text-2);
  min-width: 40px;
  text-align: center;
}

.rp-switch .btn.active {
  background: var(--primary-soft);
  color: var(--primary);
  border-color: var(--primary);
}

.rp-bg-row {
  display: flex;
  gap: 8px;
}

.rp-bg {
  width: 28px;
  height: 28px;
  border-radius: 8px;
  border: 2px solid var(--border);
  transition: transform 0.15s ease, border-color 0.15s ease;
}

.rp-bg.active {
  border-color: var(--primary);
  transform: scale(1.1);
}

/* 正文排版由 MarkdownArticle 共享组件提供（博客化，发布/分享复用） */

/* 禅模式：隐藏一切，只剩正文 */
.read-view.zen .read-topbar {
  opacity: 0;
  pointer-events: none;
  transition: opacity 0.3s ease;
}

.read-view.zen .read-topbar:hover {
  opacity: 1;
  pointer-events: auto;
}

/* 媒体预览 */
.media-view {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 40px 20px 80px;
  gap: 16px;
  min-height: 60vh;
}

.media-img {
  max-width: 100%;
  max-height: 78vh;
  border-radius: var(--radius);
  box-shadow: var(--shadow);
  object-fit: contain;
}

.media-video {
  max-width: 100%;
  max-height: 78vh;
  border-radius: var(--radius);
  background: #000;
}

.media-audio {
  width: min(560px, 92%);
}

.media-meta {
  color: var(--text-3);
  font-size: var(--fs-small);
}

/* AI 入库解读卡片（K19：摘要 + 元数据） */
.ai-read-card {
  max-width: var(--reading-maxw, 100%);
  margin: 0 auto;
  padding: 14px 18px;
  border: 1px solid var(--border);
  border-left: 3px solid var(--primary);
  border-radius: 10px;
  background: linear-gradient(135deg, rgba(139, 200, 234, 0.08), rgba(139, 200, 234, 0.02));
  font-size: 13.5px;
  line-height: 1.65;
  color: var(--text-2);
}
.ai-read-head {
  display: flex;
  align-items: center;
  gap: 6px;
  font-weight: 600;
  color: var(--text);
  margin-bottom: 6px;
}
.ai-read-tag {
  font-weight: 400;
  font-size: 11px;
  padding: 1px 8px;
  border-radius: 20px;
  background: var(--border);
  color: var(--text-2);
}
.ai-read-summary {
  margin: 0;
  color: var(--text);
}
.ai-read-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 6px 18px;
  margin-top: 8px;
  font-size: 12.5px;
}
.ai-read-kv b {
  font-weight: 600;
  color: var(--text-3);
  margin-right: 4px;
}
.ai-read-kws {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 8px;
}
.ai-read-kw {
  font-size: 11.5px;
  padding: 1px 10px;
  border-radius: 20px;
  background: var(--primary-weak, rgba(139, 200, 234, 0.18));
  color: var(--text-2);
}

.unsupported {
  color: var(--text-3);
  text-align: center;
}

.unsupported-title {
  font-size: var(--fs-medium);
  color: var(--text-2);
}

.unsupported-desc {
  font-size: var(--fs-small);
  word-break: break-all;
  max-width: 400px;
}

.pdf-view {
  height: calc(100vh - var(--topbar-h));
  padding: 12px 20px 20px;
}

.pdf-frame {
  width: 100%;
  height: 100%;
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  background: var(--surface);
}

.read-loading,
.read-error {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 14px;
  padding: 90px 20px;
  color: var(--text-3);
}

.read-error p {
  color: var(--danger);
}

.spinner {
  width: 26px;
  height: 26px;
  border: 3px solid var(--border);
  border-top-color: var(--primary);
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

.slide-enter-active,
.slide-leave-active {
  transition: transform 0.2s ease, opacity 0.2s ease;
}
.slide-enter-from,
.slide-leave-to {
  transform: translateX(16px);
  opacity: 0;
}

/* 响应式（正文移动端排版由 MarkdownArticle 组件内部处理） */
@media (max-width: 767px) {
  .read-topbar {
    padding: 0 10px;
  }
}
</style>
