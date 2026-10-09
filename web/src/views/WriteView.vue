<template>
  <div class="wv-view">
    <div class="wv-head">
      <h2 class="wv-title">{{  editingId ? $t('编辑文章') : $t('写作工作台')  }}</h2>
      <span class="wv-draft-hint" v-if="hasLocalDraft">{{  $t('本地草稿已自动保存')  }}</span>
      <span class="wv-spacer"></span>
      <select v-if="!editingId" v-model="cat" class="wv-input wv-cat">
        <option value="">{{  $t('未分类')  }}</option>
        <option v-for="c in cats" :key="c.id" :value="c.name">{{  c.name  }}</option>
      </select>
      <!-- 编辑器选择（2026-10-09 编辑器插件化）：写作轨与阅读页快编轨共用同一偏好 -->
      <select v-model="editorId" class="wv-input wv-cat" :title="$t('选择编辑器（阅读页快编同步生效）')" @change="onEditorChange">
        <option v-for="e in editorOptions" :key="e.id" :value="e.id">{{  e.title  }}</option>
      </select>
      <button v-if="editorId === 'plain'" class="btn btn-sm" :class="{ 'bm-primary': showToc }" @click="showToc = !showToc" :title="$t('显示/隐藏正文大纲')">{{  $t('☰ 大纲')  }}</button>
      <button class="btn btn-sm" :disabled="busy || !canSave" @click="savePost">{{  editingId ? $t('保存修改') : $t('发布文章')  }}</button>
    </div>

    <div class="wv-meta">
      <input v-model="title" class="wv-input wv-title-in" :placeholder="$t('文章标题')" maxlength="200" :disabled="!!editingId" />
      <select v-if="editingId" class="wv-input wv-load" @change="loadPost($event.target.value)">
        <option value="">{{  $t('载入已有文章编辑…')  }}</option>
        <option v-for="p in posts" :key="p.id" :value="p.id">{{  p.name  }}</option>
      </select>
      <span v-else class="wv-hint-new">{{  $t('发布后将自动进入编辑模式')  }}</span>
    </div>

    <!-- AI 工具栏 -->
    <div class="wv-ai-bar">
      <button class="btn btn-sm" :disabled="aiBusy" @click="aiOutline" :title="$t('基于标题与知识库素材生成大纲')">{{  $t('✦ 大纲')  }}</button>
      <button class="btn btn-sm" :disabled="aiBusy || !content" @click="aiContinue" :title="$t('从结尾自然续写')">{{  $t('✦ 续写')  }}</button>
      <button class="btn btn-sm" :disabled="aiBusy || !content" @click="aiPolish('polish')" :title="$t('优化表达不改原意')">{{  $t('✦ 润色')  }}</button>
      <button class="btn btn-sm" :disabled="aiBusy || !content" @click="aiPolish('expand')" :title="$t('补充细节，篇幅 1.5-2 倍')">{{  $t('✦ 扩写')  }}</button>
      <button class="btn btn-sm" :disabled="aiBusy || !content" @click="aiPolish('shorten')" :title="$t('精炼到一半篇幅')">{{  $t('✦ 缩写')  }}</button>
      <button class="btn btn-sm" :disabled="aiBusy || (!title && !content)" @click="aiTags" :title="$t('推荐 3-6 个标签')">{{  $t('✦ 标签建议')  }}</button>
      <button class="btn btn-sm" :class="{ 'bm-primary': promptOpen }" @click="togglePrompts" :title="$t('用可配置的提示词模板生成（变量填值即用）')">{{  $t('✦ 模板')  }}</button>
      <span v-if="aiBusy" class="wv-ai-busy">{{  $t('AI 生成中…')  }}</span>
      <span v-if="aiErr" class="wv-ai-err">{{  aiErr  }}</span>
    </div>

    <!-- 提示词模板（B5 写作增强） -->
    <div v-if="promptOpen" class="wv-tpl">
      <div class="wv-tpl-head">
        <span class="wv-tpl-title">{{  $t('提示词模板')  }}</span>
        <a class="wv-tpl-link" href="#/prompts" target="_blank" rel="noopener" :title="$t('在「提示词模板」页新建 / 编辑模板')">{{  $t('管理模板 ↗')  }}</a>
        <span class="wv-spacer"></span>
        <button class="btn btn-sm" :disabled="tplLoading" @click="loadTemplates">{{  $t('刷新')  }}</button>
        <button class="btn btn-sm" @click="promptOpen = false">{{  $t('收起')  }}</button>
      </div>
      <div v-if="tplLoading" class="wv-tpl-empty">{{  $t('加载中…')  }}</div>
      <div v-else-if="!tplList.length" class="wv-tpl-empty">
        {{  $t('还没有提示词模板。去「提示词模板」页新建一个，即写即用。')  }}
      </div>
      <template v-else>
        <div class="wv-tpl-row">
          <select v-model="tplId" class="wv-input wv-tpl-sel">
            <option value="">{{  $t('选择一个模板…')  }}</option>
            <option v-for="t in tplList" :key="t.id" :value="t.id">{{  t.category ? '[' + t.category + '] ' : ''  }}{{  t.name  }}</option>
          </select>
          <button class="btn btn-sm" :disabled="!tplId || tplBusy" @click="tplShowRaw">{{  $t('仅看提示词')  }}</button>
          <button class="btn btn-sm bm-primary" :disabled="!tplId || tplBusy" @click="tplRun">{{  tplBusy ? $t('生成中…') : $t('用模板生成')  }}</button>
        </div>
        <div v-if="tplCur && tplCur.variables.length" class="wv-tpl-vars">
          <label v-for="v in tplCur.variables" :key="v.name" class="wv-tpl-field">
            <span>{{  v.label || v.name  }}</span>
            <input v-model="tplVals[v.name]" class="wv-input" :placeholder="v.default || ('{{ ' + v.name + ' }}')" />
          </label>
        </div>
        <p v-else-if="tplCur" class="wv-tpl-hint">{{  $t('该模板没有变量，直接点「用模板生成」即可。')  }}</p>
        <p v-if="tplErr" class="wv-tpl-err">{{  tplErr  }}</p>
      </template>
    </div>

    <!-- AI 结果面板 -->
    <div v-if="aiText" class="wv-ai-result">
      <div class="wv-ai-result-head">
        <span>{{ $t('AI 输出') }}</span>
        <span class="wv-ai-tags" v-if="aiTagsList.length">
          <button v-for="t in aiTagsList" :key="t" class="wv-tag-chip" @click="appendTag(t)" :title="$t('点击追加到文末')">#{{  t  }}</button>
        </span>
        <span class="wv-spacer"></span>
        <button class="btn btn-sm" @click="insertAtEnd(aiText)" v-if="aiMode !== 'polish'">{{ $t('插入到文末') }}</button>
        <button class="btn btn-sm" @click="copyAI">{{ $t('复制') }}</button>
        <button class="btn btn-sm bm-primary" @click="replaceContent(aiText)" v-if="aiMode === 'polish'">{{ $t('替换正文') }}</button>
        <button class="btn btn-sm" @click="aiText = ''; aiTagsList = []">{{ $t('关闭') }}</button>
      </div>
      <div class="wv-ai-out" v-html="aiHtml"></div>
    </div>

    <!-- 素材检索 -->
    <div class="wv-mat">
      <input v-model="matQ" class="wv-input" :placeholder="$t('素材检索：输入关键词，从本站知识库找素材（回车检索）')" @keyup.enter="doSearch" />
      <div v-if="mats.length" class="wv-mat-list">
        <div v-for="m in mats" :key="m.path" class="wv-mat-item" @click="insertAtEnd('[' + m.title + '](#/p/blog?path=' + encodeURIComponent(m.path) + ')')" :title="t('点击插入内链：') + m.path">
          <span class="wv-mat-title">{{  m.title  }}</span>
          <span class="wv-mat-preview">{{  m.preview  }}</span>
        </div>
      </div>
    </div>

    <!-- SEO 与封面（新文章可先填好，发布时随首保存；已有文章直接改） -->
    <details class="wv-seo">
      <summary>{{ $t('封面图 / 摘要 / SEO') }}</summary>
      <div class="wv-seo-grid">
        <label class="wv-seo-label">{{ $t('封面图 URL') }}
          <div class="wv-cover-row">
            <input v-model="cover" class="wv-input" :placeholder="$t('https://… 或站内可公开访问的图片地址（留空不设）')" />
            <label class="btn btn-sm wv-cover-up">
              {{  uploading ? $t('上传中…') : $t('上传封面')  }}
              <input type="file" accept="image/*" hidden @change="uploadCover" />
            </label>
            <img v-if="cover" :src="cover" class="wv-cover-thumb" :alt="$t('封面预览')" @error="$event.target.style.display='none'" />
          </div>
        </label>
        <label class="wv-seo-label">{{ $t('自定义摘要') }}<textarea v-model="excerpt" class="wv-input" rows="2" maxlength="600" :placeholder="$t('留空则自动取正文开头 / AI 摘要')"></textarea></label>
        <label class="wv-seo-label">{{ $t('SEO 标题') }}<input v-model="seoTitle" class="wv-input" maxlength="200" :placeholder="$t('留空则用文章标题')" /></label>
        <label class="wv-seo-label">{{ $t('SEO 描述') }}<textarea v-model="seoDesc" class="wv-input" rows="2" maxlength="400" :placeholder="$t('留空则用摘要（搜索引擎结果页描述）')"></textarea></label>
      </div>
      <p class="wv-seo-hint">{{ $t('保存文章时一并保存本面板；SEO 字段用于 SSR 页 title/description/OG，缺省自动回退。') }}</p>
    </details>

    <!-- 收录评论（B20）：编辑页直接把本文章的已通过评论带署名补录进正文 -->
    <details v-if="editingId" class="wv-ingest">
      <summary>{{  ingestSummary  }}</summary>
      <div class="wv-ingest-bar">
        <button class="btn btn-sm" :disabled="ingestBusy" @click="loadIngestComments">{{ $t('刷新评论') }}</button>
        <button class="btn btn-sm" :disabled="ingestBusy" @click="loadIngestRecords">{{ $t('刷新记录') }}</button>
        <span v-if="ingestErr" class="wv-ingest-err">{{  ingestErr  }}</span>
      </div>
      <div v-if="ingestBusy && !ingestComments.length" class="wv-ingest-empty">{{ $t('加载中…') }}</div>
      <div v-else-if="!ingestComments.length" class="wv-ingest-empty">{{ $t('本文章暂无已通过评论可收录。') }}</div>
      <div v-else class="wv-ingest-list">
        <div v-for="c in ingestComments" :key="c.id" class="wv-ingest-row">
          <span class="wv-ingest-meta">{{  c.guest_name || $t('访客')  }} · {{  fmtIngestTime(c.created_at)  }}</span>
          <p class="wv-ingest-body">{{  c.body  }}</p>
          <button class="btn btn-sm bm-primary" :disabled="ingestBusy" @click="ingestCmt(c)">{{ $t('收录此评论') }}</button>
        </div>
      </div>
      <div v-if="ingestRecords.length" class="wv-ingest-recs">
        <div class="wv-ingest-sub">{{ $t('已收录记录（最近 20 条）') }}</div>
        <div v-for="g in ingestRecords" :key="g.id" class="wv-ingest-rec">
          <span :class="['wv-ingest-status', g.status]">{{  ingestStatusText(g.status)  }}</span>
          <button v-if="g.status === 'accepted'" class="btn btn-sm" :disabled="ingestBusy" @click="revertIngest(g)">{{ $t('撤销') }}</button>
        </div>
      </div>
    </details>

    <!-- 双栏：编辑 / 预览（左侧可选大纲） -->
    <div class="wv-panes" :class="{ 'has-toc': showToc && editorId === 'plain' }">
      <aside v-if="showToc && editorId === 'plain'" class="wv-toc">
        <p class="wv-toc-title">{{ $t('大纲') }}</p>
        <p v-if="!toc.length" class="wv-toc-empty">{{ $t('用 # 标题生成大纲') }}</p>
        <a
          v-for="t in toc"
          :key="t.idx"
          class="wv-toc-item"
          :class="'d' + t.depth"
          :title="t.text"
          @click="jumpToc(t)"
        >{{  t.text  }}</a>
      </aside>
      <!-- 编辑器本体轨（2026-10-09 插件化）：plain = 分栏 textarea + 右侧预览（本页渲染）；
           其它编辑器（Vditor 等）= 整栏动态组件，预览/双链由编辑器自理。v-model 统一走 content。 -->
      <template v-if="editorId === 'plain'">
        <PlainEditor
          ref="plainRef"
          v-model="content"
          class="wv-plain-host"
          :height="editorHeight"
          :placeholder="$t('Markdown 正文…（左侧编辑，右侧实时预览；可直接粘贴/拖入图片）')"
          @paste="onPaste"
          @drop="onDrop"
        ></PlainEditor>
        <div ref="previewEl" class="wv-preview prose" v-html="previewHtml"></div>
      </template>
      <component :is="editorComp" v-else-if="editorComp" ref="edRef" v-model="content" :height="editorHeight" :placeholder="$t('Markdown 正文…（可直接粘贴/拖入图片）')" class="wv-host-editor"></component>
    </div>
    <span v-if="uploading" class="wv-uploading">{{ $t('图片上传中…（') }}{{  uploading  }}）</span>
  </div>
</template>

<script setup>
import { t as i18t } from '@/i18n'
import { ref, computed, shallowRef, nextTick, onMounted, onUnmounted, watch } from 'vue'
import { useToastStore } from '@/stores/toast'
import { requestJSON, fetchFileText, uploadFiles, mkdir, listPrompts, renderPrompt, aiChat, publicBlogEditors } from '@/api'
import { renderMarkdown, extractToc, enhanceMermaid } from '@/utils/markdown'
import {
  listEditors, getEditorLoader, isEditorInstalled,
  readEditorPref, saveEditorPref, applyInstalledEditors,
} from '@/editors/registry'
import PlainEditor from '@/components/PlainEditor.vue'
import { t } from '@/i18n'

const toast = useToastStore()
const busy = ref(false)
const aiBusy = ref(false)
const aiErr = ref('')
const aiText = ref('')
const aiHtml = ref('')
const aiTagsList = ref([])
const aiMode = ref('')

const title = ref('')
const cat = ref('')
const content = ref('')
const editingId = ref('')
const cats = ref([])
const posts = ref([])
const cover = ref('')
const excerpt = ref('')
const seoTitle = ref('')
const seoDesc = ref('')

// ===== 收录评论（B20）：编辑页直接把本文章的已通过评论带署名补录进正文 =====
// 后端 ingest 接口（/ingests/quote、/ingests、/blog/comments）已就绪，本页只是入口补全。
const ingestComments = ref([])
const ingestRecords = ref([])
const ingestBusy = ref(false)
const ingestErr = ref('')
const INGEST_STATUS_TEXT = { draft: t(i18t('草稿')), accepted: t('已收录'), reverted: t('已撤销'), discarded: t('已放弃') }
const ingestSummary = computed(() =>
  ingestComments.value.length ? t('收录评论（') + ingestComments.value.length + '）' : t('收录评论')
)
function ingestStatusText(s) {
  return INGEST_STATUS_TEXT[s] || s
}
function fmtIngestTime(ms) {
  if (!ms) return ''
  const d = new Date(Number(ms))
  if (isNaN(d.getTime())) return ''
  const p = (n) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`
}
async function loadIngestComments() {
  if (!editingId.value) return
  ingestBusy.value = true
  ingestErr.value = ''
  try {
    const d = await requestJSON('/blog/comments?status=approved&limit=200')
    const all = d?.items || []
    ingestComments.value = all.filter((c) => c.file_id === editingId.value)
  } catch (e) {
    ingestErr.value = t('评论加载失败：') + (e.message || e)
    ingestComments.value = []
  } finally {
    ingestBusy.value = false
  }
}
async function loadIngestRecords() {
  if (!editingId.value) return
  try {
    const d = await requestJSON('/ingests?file_id=' + encodeURIComponent(editingId.value))
    ingestRecords.value = (d?.items || []).slice(0, 20)
  } catch (_) {
    ingestRecords.value = []
  }
}
async function ingestCmt(c) {
  if (ingestBusy.value || !editingId.value) return
  ingestBusy.value = true
  ingestErr.value = ''
  try {
    await requestJSON('/ingests/quote', {
      method: 'POST',
      body: JSON.stringify({ file_id: editingId.value, comment_id: c.id }),
    })
    toast.success(t('已带署名与回指链接补录进正文'))
    loadIngestComments()
    loadIngestRecords()
  } catch (e) {
    const m = String(e?.message || e)
    if (m.includes('已被收录')) toast.error(i18t('该评论已被收录过'))
    else if (m.includes('冻结收录')) toast.error(i18t('该文章已冻结收录（生长策略=frozen）'))
    else if (m.includes('仅文本')) toast.error(i18t('仅文本文章可收录'))
    else toast.error(t('收录失败：') + m)
  } finally {
    ingestBusy.value = false
  }
}
async function revertIngest(g) {
  if (ingestBusy.value) return
  ingestBusy.value = true
  try {
    await requestJSON(`/ingests/${g.id}`, { method: 'DELETE' })
    toast.success(t('已撤销收录（正文按锚点精确摘除）'))
    loadIngestRecords()
  } catch (e) {
    const m = String(e?.message || e)
    if (m.includes('锚点缺失')) toast.error(i18t('正文锚点已丢失，请人工处理'))
    else toast.error(t('撤销失败：') + m)
  } finally {
    ingestBusy.value = false
  }
}
// 载入 / 切换文章时拉取本文章的评论与收录记录
watch(editingId, (id) => {
  if (id) {
    loadIngestComments()
    loadIngestRecords()
  } else {
    ingestComments.value = []
    ingestRecords.value = []
  }
})

const matQ = ref('')
const mats = ref([])

const hasLocalDraft = ref(false)
let saveTimer = null

const canSave = computed(() => (editingId.value ? !!content.value : !!title.value.trim() && !!content.value.trim()))
const previewHtml = computed(() => {
  try {
    return renderMarkdown(content.value || '')
  } catch (_) {
    return ''
  }
})
const toc = computed(() => extractToc(content.value))

// ---- 大纲跳转（id 失配时按标题序号兜底）----
const previewEl = ref(null)
const showToc = ref(true)

// ---- 编辑器本体轨（2026-10-09 编辑器插件化）----
// plain 在本页直接渲染（PlainEditor + 分栏预览）；其它编辑器动态加载组件整栏渲染。
// 已安装列表来自 /public/blog/editors（见 loadInstalledEditors）；拉取失败降级为全部内置可见。
const editorId = ref('plain')
const editorComp = shallowRef(null)
const plainRef = ref(null)
const edRef = ref(null)
const windowHeight = typeof window !== 'undefined' ? window.innerHeight : 800
const editorHeight = computed(() => Math.max(420, windowHeight - 320))
const editorOptions = computed(() => listEditors())
async function mountEditor(id) {
  const loader = getEditorLoader(id)
  if (!loader) { editorComp.value = null; return }
  try {
    const mod = await loader()
    editorComp.value = mod.default || mod
  } catch (e) {
    console.warn('[write] 编辑器加载失败，回退 plain:', e)
    editorId.value = 'plain'
    saveEditorPref('plain')
    editorComp.value = null
  }
}
function onEditorChange() {
  if (!isEditorInstalled(editorId.value)) editorId.value = 'plain'
  saveEditorPref(editorId.value)
  if (editorId.value !== 'plain') mountEditor(editorId.value)
  else editorComp.value = null
}
async function loadInstalledEditors() {
  try {
    const d = await publicBlogEditors()
    applyInstalledEditors((d?.items || []).map((x) => x.id))
  } catch (_) { /* 离线兜底：installedIds 保持 null → 全部内置可见 */ }
  const pref = readEditorPref()
  editorId.value = pref
  if (pref !== 'plain') await mountEditor(pref)
}
function jumpToc(item) {
  const root = previewEl.value
  if (!root) return
  let el = null
  try {
    el = root.querySelector('#' + CSS.escape(item.id))
  } catch (_) { /* 老浏览器无 CSS.escape */ }
  if (!el) el = root.querySelectorAll('h1,h2,h3,h4,h5,h6')[item.idx]
  el?.scrollIntoView({ behavior: 'smooth', block: 'start' })
}

// ---- 图片上传（粘贴 / 拖拽 / 封面）----
// 图片统一存博客目录下「媒体」子目录，经公开媒体路由 /api/v1/public/media/{id} 访问。
const BLOG_DIR = '00000000-0000-0000-0000-000000000010'
const uploading = ref(0)
let mediaDirId = ''
async function ensureMediaDir() {
  if (mediaDirId) return mediaDirId
  try {
    const items = await requestJSON('/files?parent=' + BLOG_DIR)
    const hit = (Array.isArray(items) ? items : items?.items || []).find((x) => x.kind === 'dir' && x.name === '媒体')
    if (hit?.id) {
      mediaDirId = hit.id
      return mediaDirId
    }
  } catch (_) { /* 列目录失败则直接建 */ }
  const d = await mkdir(t(i18t('媒体')), BLOG_DIR)
  mediaDirId = d?.id || ''
  return mediaDirId
}
function insertAtCursor(text) {
  // 编辑器本体轨：委托给当前编辑器实例（PlainEditor/VditorEditor 均实现 insertAtCursor）
  const inst = editorId.value === 'plain' ? plainRef.value : edRef.value
  if (inst && typeof inst.insertAtCursor === 'function') {
    inst.insertAtCursor(text)
    return
  }
  content.value = (content.value || '') + text
}
async function uploadImageFile(file, setCover = false) {
  if (!file || !/^image\//.test(file.type || '')) return false
  uploading.value++
  try {
    const parent = await ensureMediaDir()
    if (!parent) throw new Error(t('媒体目录不可用'))
    const rs = await uploadFiles([{ file, path: file.name }], parent)
    const f = rs?.[0]?.file
    if (!f?.id) throw new Error(t('上传无结果'))
    if (setCover) {
      cover.value = '/api/v1/public/media/' + f.id
      toast.success(t('封面已上传'))
    } else {
      insertAtCursor(`![](/api/v1/public/media/${f.id})\n`)
      toast.success(t('图片已插入'))
    }
    return true
  } catch (e) {
    toast.error(t('图片上传失败：') + (e.message || e))
    return false
  } finally {
    uploading.value--
  }
}
function onPaste(e) {
  for (const it of e.clipboardData?.items || []) {
    if (it.kind === 'file' && /^image\//.test(it.type)) {
      const f = it.getAsFile()
      if (f && uploadImageFile(f)) e.preventDefault()
    }
  }
}
function onDrop(e) {
  for (const f of e.dataTransfer?.files || []) {
    if (/^image\//.test(f.type)) uploadImageFile(f)
  }
}
async function uploadCover(e) {
  const f = e.target?.files?.[0]
  if (f) await uploadImageFile(f, true)
  e.target.value = ''
}

// 预览更新后增强 Mermaid（动态加载，无图不加载）
watch(previewHtml, () => {
  nextTick(() => {
    if (previewEl.value) enhanceMermaid(previewEl.value)
  })
})

// 本地草稿自动保存（localStorage；避免误关页面丢稿）
const DRAFT_KEY = 'aiklog_write_draft'
function localSave() {
  try {
    localStorage.setItem(DRAFT_KEY, JSON.stringify({ title: title.value, cat: cat.value, content: content.value, at: Date.now() }))
    hasLocalDraft.value = true
  } catch (_) { /* 隐私模式等 */ }
}
function localLoad() {
  try {
    const raw = localStorage.getItem(DRAFT_KEY)
    if (!raw) return
    const d = JSON.parse(raw)
    if (d && (d.title || d.content)) {
      title.value = d.title || ''
      cat.value = d.cat || ''
      content.value = d.content || ''
      hasLocalDraft.value = true
    }
  } catch (_) { /* 忽略 */ }
}
watch([title, cat, content], () => {
  if (saveTimer) clearTimeout(saveTimer)
  saveTimer = setTimeout(localSave, 800)
})

async function loadManage() {
  try {
    const d = await requestJSON('/blog/manage')
    cats.value = d.categories || []
    posts.value = d.posts || []
  } catch (_) { /* 静默 */ }
}

async function loadPost(id) {
  if (!id) return
  try {
    const d = await requestJSON('/blog/manage')
    const p = (d.posts || []).find((x) => x.id === id)
    if (!p) return
    const text = await fetchFileText(id)
    editingId.value = id
    title.value = (p.name || '').replace(/\.(md|markdown)$/i, '')
    content.value = text
    cover.value = p.cover || ''
    excerpt.value = p.excerpt || ''
    seoTitle.value = p.seo_title || ''
    seoDesc.value = p.seo_desc || ''
    aiText.value = ''
    aiTagsList.value = []
    toast.success(t('已载入「') + title.value + t('」，编辑后点"保存修改"'))
  } catch (e) {
    toast.error(t('载入失败：') + (e.message || e))
  }
}

async function savePost() {
  busy.value = true
  try {
    if (editingId.value) {
      await requestJSON(`/files/${editingId.value}/content`, { method: 'PUT', body: JSON.stringify({ content: content.value }) })
      // 单篇元数据一并保存（封面/摘要/SEO；失败不阻断正文保存）
      try {
        await requestJSON('/blog/posts/meta', {
          method: 'POST',
          body: JSON.stringify({ id: editingId.value, cover: cover.value, excerpt: excerpt.value, seo_title: seoTitle.value, seo_desc: seoDesc.value }),
        })
      } catch (me) {
        toast.error(t('正文已保存，但封面/SEO 保存失败：') + (me.message || me))
      }
      toast.success(t('文章已保存'))
    } else {
      const d = await requestJSON('/blog/posts', {
        method: 'POST',
        body: JSON.stringify({ title: title.value.trim(), category: cat.value, content: content.value }),
      })
      toast.success(t('已发布：') + (d?.path || title.value))
      editingId.value = d?.id || ''
      // 新文章：发布前填的封面/摘要/SEO 随发布一并保存（元数据接口需要 id，故在发布成功后补写）
      if (editingId.value && (cover.value || excerpt.value || seoTitle.value || seoDesc.value)) {
        try {
          await requestJSON('/blog/posts/meta', {
            method: 'POST',
            body: JSON.stringify({ id: editingId.value, cover: cover.value, excerpt: excerpt.value, seo_title: seoTitle.value, seo_desc: seoDesc.value }),
          })
        } catch (me) {
          toast.error(t('正文已发布，但封面/SEO 保存失败：') + (me.message || me))
        }
      }
      try { localStorage.removeItem(DRAFT_KEY) } catch (_) {}
      hasLocalDraft.value = false
    }
    loadManage()
  } catch (e) {
    toast.error(t('保存失败：') + (e.message || e))
  } finally {
    busy.value = false
  }
}

// ---- AI 工具 ----

async function aiCall(path, body) {
  aiBusy.value = true
  aiErr.value = ''
  try {
    const d = await requestJSON(path, { method: 'POST', body: JSON.stringify(body) })
    return d
  } catch (e) {
    aiErr.value = t('AI 调用失败：') + (e.message || e)
    return null
  } finally {
    aiBusy.value = false
  }
}

function showAI(text, mode) {
  aiText.value = text || ''
  aiMode.value = mode
  try { aiHtml.value = renderMarkdown(aiText.value) } catch (_) { aiHtml.value = '' }
}

// ---- 提示词模板（B5 写作增强）----
const promptOpen = ref(false)
const tplList = ref([])
const tplLoading = ref(false)
const tplId = ref('')
const tplVals = ref({})
const tplBusy = ref(false)
const tplErr = ref('')

const tplCur = computed(() => tplList.value.find((t) => t.id === tplId.value) || null)

function togglePrompts() {
  promptOpen.value = !promptOpen.value
  if (promptOpen.value && !tplList.value.length) loadTemplates()
}

async function loadTemplates() {
  tplLoading.value = true
  tplErr.value = ''
  try {
    const d = await listPrompts()
    tplList.value = d.templates || []
    // 选中项失效（被删/权限变化）时清空，避免「选着空模板」的迷惑态
    if (tplId.value && !tplList.value.some((t) => t.id === tplId.value)) {
      tplId.value = ''
      tplVals.value = {}
    }
  } catch (e) {
    tplErr.value = t('加载模板失败：') + (e.message || e)
  } finally {
    tplLoading.value = false
  }
}

// 选中模板变化 → 按变量声明重建填值表单（保留已填同名值）
watch(tplCur, (t) => {
  const prev = tplVals.value || {}
  const vals = {}
  for (const v of t?.variables || []) vals[v.name] = prev[v.name] || ''
  tplVals.value = vals
})

async function renderTpl() {
  if (!tplId.value) {
    tplErr.value = t('请先选择一个模板')
    return null
  }
  tplErr.value = ''
  try {
    const r = await renderPrompt(tplId.value, tplVals.value)
    if (r.missing && r.missing.length) {
      tplErr.value = t('未解析的变量（已原样保留）：') + r.missing.join('、')
    }
    return r.text || ''
  } catch (e) {
    tplErr.value = t('渲染失败：') + (e.message || e)
    return null
  }
}

// 仅看提示词：把渲染后的提示词放进输出面板（不调用 AI，便于自行复制到别处）
async function tplShowRaw() {
  const text = await renderTpl()
  if (text === null) return
  showAI('<!-- 提示词（未经 AI 生成） -->\n\n' + text, 'template-raw')
}

// 用模板生成：渲染 → 送 AI → 结果进输出面板（复用「插入到文末 / 复制」）
async function tplRun() {
  const text = await renderTpl()
  if (text === null) return
  tplBusy.value = true
  tplErr.value = ''
  try {
    const d = await aiChat([{ role: 'user', content: text }])
    if (d?.error) {
      tplErr.value = t('AI 返回错误：') + d.error
      return
    }
    if (!d?.reply) {
      tplErr.value = t('AI 没有返回内容（模型可能未配置）')
      return
    }
    showAI(d.reply, 'template')
  } catch (e) {
    tplErr.value = t('AI 调用失败：') + (e.message || e)
  } finally {
    tplBusy.value = false
  }
}

async function aiOutline() {
  const topic = title.value.trim()
  if (!topic) {
    aiErr.value = t('请先填写文章标题（作为大纲主题）')
    return
  }
  const d = await aiCall('/blog/ai/outline', { topic })
  if (d?.text) showAI(d.text, 'outline')
}

async function aiContinue() {
  const d = await aiCall('/blog/ai/continue', { content: content.value, title: title.value })
  if (d?.text) showAI(d.text, 'continue')
}

async function aiPolish(mode) {
  const d = await aiCall('/blog/ai/polish', { content: content.value, mode })
  if (d?.text) showAI(d.text, mode === 'polish' ? 'polish' : 'continue')
}

async function aiTags() {
  const d = await aiCall('/blog/ai/tags', { title: title.value, content: content.value })
  if (d && Array.isArray(d.tags)) {
    aiTagsList.value = d.tags
    if (!d.tags.length) aiErr.value = t('AI 未给出标签建议')
  }
}

function insertAtEnd(text) {
  content.value = (content.value || '') + (content.value ? '\n\n' : '') + text
  toast.success(t('已插入到文末'))
}

function replaceContent(text) {
  content.value = text
  toast.success(t('已替换正文（原内容可用浏览器撤销 Ctrl+Z 找回）'))
}

function appendTag(t) {
  content.value = (content.value || '') + (content.value ? '\n\n' : '') + t('标签：') + t
}

function copyAI() {
  navigator.clipboard?.writeText(aiText.value || '').then(
    () => toast.success(t('已复制')),
    () => toast.error(t('复制失败'))
  )
}

async function doSearch() {
  try {
    const d = await requestJSON('/blog/ai/search', { method: 'POST', body: JSON.stringify({ q: matQ.value, limit: 8 }) })
    mats.value = d?.items || []
  } catch (_) {
    mats.value = []
  }
}

onMounted(() => {
  localLoad()
  loadManage()
  loadInstalledEditors()
})
onUnmounted(() => {
  if (saveTimer) clearTimeout(saveTimer)
})
</script>

<style scoped>
.wv-view {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 20px 24px 40px;
  max-width: 1200px;
  margin: 0 auto;
  height: 100%;
  overflow: auto;
}
.wv-head {
  display: flex;
  align-items: center;
  gap: 12px;
}
.wv-title {
  margin: 0;
  font-size: 18px;
}
.wv-spacer {
  flex: 1;
}
.wv-draft-hint {
  font-size: 12px;
  color: var(--text-3, #8a919f);
}
.wv-meta {
  display: flex;
  gap: 8px;
}
.wv-title-in {
  flex: 1;
  font-size: 15px;
  font-weight: 600;
}
.wv-load {
  max-width: 280px;
}
.wv-input {
  padding: 7px 10px;
  border: 1px solid var(--border, #e3e6eb);
  border-radius: 8px;
  background: var(--bg-1, #fff);
  color: var(--text-1, #1f2329);
  font-size: 13px;
  outline: none;
}
.wv-input:focus {
  border-color: var(--primary, #4f7cff);
}
.wv-cat {
  max-width: 180px;
}
.wv-tpl {
  border: 1px solid var(--border, #e3e6ee);
  border-radius: 10px;
  background: var(--surface-2, #f7f8fa);
  padding: 10px 12px;
  margin: 8px 0;
}
.wv-tpl-head { display: flex; align-items: center; gap: 8px; margin-bottom: 8px; flex-wrap: wrap; }
.wv-tpl-title { font-size: 13px; font-weight: 600; }
.wv-tpl-link { font-size: 12px; color: var(--primary, #2f6bff); text-decoration: none; }
.wv-tpl-link:hover { text-decoration: underline; }
.wv-tpl-row { display: flex; align-items: center; gap: 7px; flex-wrap: wrap; }
.wv-tpl-sel { flex: 1; min-width: 200px; max-width: 420px; }
.wv-tpl-vars { display: flex; flex-direction: column; gap: 7px; margin-top: 9px; }
.wv-tpl-field { display: flex; align-items: center; gap: 8px; font-size: 12px; color: var(--text-3, #8a919f); }
.wv-tpl-field > span { flex: 0 0 92px; text-align: right; }
.wv-tpl-field .wv-input { flex: 1; }
.wv-tpl-empty { font-size: 12.5px; color: var(--text-3, #8a919f); padding: 6px 0; }
.wv-tpl-hint { font-size: 12px; color: var(--text-3, #8a919f); margin: 8px 0 0; }
.wv-tpl-err { font-size: 12px; color: #d98200; margin: 8px 0 0; }

.wv-ai-bar {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.wv-ai-busy {
  font-size: 12px;
  color: var(--primary, #4f7cff);
}
.wv-ai-err {
  font-size: 12px;
  color: var(--danger, #d64545);
}
.wv-ai-result {
  border: 1px solid var(--primary, #4f7cff);
  border-radius: 10px;
  overflow: hidden;
}
.wv-ai-result-head {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 10px;
  background: var(--bg-2, #f7f8fa);
  font-size: 13px;
  font-weight: 600;
}
.wv-ai-tags {
  display: inline-flex;
  gap: 6px;
  flex-wrap: wrap;
}
.wv-tag-chip {
  border: 1px solid var(--border, #e3e6eb);
  background: var(--bg-1, #fff);
  border-radius: 999px;
  padding: 2px 10px;
  font-size: 12px;
  cursor: pointer;
}
.wv-ai-out {
  padding: 10px 14px;
  max-height: 260px;
  overflow: auto;
  font-size: 13px;
}
.wv-mat {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.wv-mat-list {
  display: flex;
  gap: 8px;
  overflow-x: auto;
  padding-bottom: 4px;
}
.wv-mat-item {
  flex: none;
  width: 240px;
  border: 1px solid var(--border, #e3e6eb);
  border-radius: 8px;
  padding: 8px 10px;
  cursor: pointer;
  font-size: 12px;
  background: var(--bg-1, #fff);
}
.wv-mat-item:hover {
  border-color: var(--primary, #4f7cff);
}
.wv-mat-title {
  display: block;
  font-weight: 600;
  margin-bottom: 2px;
}
.wv-mat-preview {
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
  color: var(--text-3, #8a919f);
}
.wv-panes {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
  flex: 1;
  min-height: 420px;
}
.wv-panes.has-toc {
  grid-template-columns: 200px 1fr 1fr;
}
.wv-toc {
  border: 1px solid var(--border, #e3e6eb);
  border-radius: 10px;
  padding: 10px;
  overflow: auto;
  max-height: 70vh;
  font-size: 12px;
  background: var(--bg-1, #fff);
}
.wv-toc-title {
  margin: 0 0 6px;
  font-weight: 600;
  color: var(--text-2, #4a5568);
}
.wv-toc-empty {
  margin: 0;
  color: var(--text-3, #8a919f);
}
.wv-toc-item {
  display: block;
  padding: 3px 6px;
  border-radius: 6px;
  cursor: pointer;
  color: var(--text-1, #1f2329);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.wv-toc-item:hover {
  background: var(--bg-2, #f2f4f7);
}
.wv-toc-item.d1 { font-weight: 600; }
.wv-toc-item.d2 { padding-left: 16px; }
.wv-toc-item.d3 { padding-left: 26px; }
.wv-toc-item.d4, .wv-toc-item.d5, .wv-toc-item.d6 { padding-left: 36px; color: var(--text-3, #8a919f); }
.wv-uploading {
  font-size: 12px;
  color: var(--primary, #4f7cff);
}
.wv-cover-row {
  display: flex;
  align-items: center;
  gap: 8px;
}
.wv-cover-row .wv-input {
  flex: 1;
}
.wv-cover-up {
  position: relative;
  white-space: nowrap;
}
.wv-cover-thumb {
  width: 56px;
  height: 36px;
  object-fit: cover;
  border-radius: 4px;
  border: 1px solid var(--border, #e3e6eb);
}
/* plain 编辑器宿主：textarea 在 PlainEditor 内部，这里管外框与撑满 */
.wv-plain-host {
  display: block;
  border: 1px solid var(--border, #e3e6eb);
  border-radius: 10px;
  background: var(--bg-1, #fff);
  overflow: hidden;
}
.wv-plain-host:focus-within {
  border-color: var(--primary, #4f7cff);
}
/* 非 plain 编辑器（Vditor 等）整栏渲染 */
.wv-panes:not(.has-toc) .wv-host-editor {
  grid-column: 1 / -1;
}
.wv-preview {
  border: 1px solid var(--border, #e3e6eb);
  border-radius: 10px;
  padding: 14px 18px;
  overflow: auto;
  background: var(--bg-1, #fff);
  font-size: 14px;
  line-height: 1.8;
}
.wv-seo {
  border: 1px solid var(--border, #e3e6eb);
  border-radius: 10px;
  padding: 8px 12px;
  font-size: 13px;
}
.wv-seo summary {
  cursor: pointer;
  font-weight: 600;
  color: var(--text-2, #4a5568);
}
.wv-seo-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 8px 12px;
  margin-top: 8px;
}
.wv-seo-label {
  display: flex;
  flex-direction: column;
  gap: 4px;
  font-size: 12px;
  color: var(--text-3, #8a919f);
}
.wv-seo-hint {
  margin: 8px 0 0;
  font-size: 12px;
  color: var(--text-3, #8a919f);
}
.wv-ingest {
  border: 1px solid var(--border, #e3e6eb);
  border-radius: 10px;
  padding: 8px 12px;
  font-size: 13px;
}
.wv-ingest summary {
  cursor: pointer;
  font-weight: 600;
  color: var(--text-2, #4a5568);
}
.wv-ingest-bar {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 8px;
}
.wv-ingest-err {
  color: var(--danger, #e5484d);
  font-size: 12px;
}
.wv-ingest-empty {
  margin: 8px 0 0;
  font-size: 12px;
  color: var(--text-3, #8a919f);
}
.wv-ingest-list {
  margin-top: 8px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.wv-ingest-row {
  border: 1px solid var(--border, #e3e6eb);
  border-radius: 8px;
  padding: 8px 10px;
  background: var(--bg-2, #f7f8fa);
}
.wv-ingest-meta {
  font-size: 11px;
  color: var(--text-3, #8a919f);
}
.wv-ingest-body {
  margin: 4px 0 8px;
  white-space: pre-wrap;
  word-break: break-word;
  font-size: 13px;
  color: var(--text, #1f2329);
}
.wv-ingest-recs {
  margin-top: 10px;
  border-top: 1px dashed var(--border, #e3e6eb);
  padding-top: 8px;
}
.wv-ingest-sub {
  font-size: 12px;
  color: var(--text-3, #8a919f);
  margin-bottom: 6px;
}
.wv-ingest-rec {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
  margin-bottom: 4px;
}
.wv-ingest-status {
  padding: 1px 8px;
  border-radius: 999px;
  font-size: 11px;
}
.wv-ingest-status.accepted {
  background: rgba(46, 168, 122, 0.15);
  color: #2ea87a;
}
.wv-ingest-status.draft {
  background: rgba(245, 166, 35, 0.15);
  color: #d98e00;
}
.wv-ingest-status.reverted,
.wv-ingest-status.discarded {
  background: rgba(138, 145, 159, 0.18);
  color: #8a919f;
}
@media (max-width: 900px) {
  .wv-seo-grid {
    grid-template-columns: 1fr;
  }
}
@media (max-width: 900px) {
  .wv-panes,
  .wv-panes.has-toc {
    grid-template-columns: 1fr;
  }
  .wv-toc {
    max-height: 200px;
  }
}
</style>
