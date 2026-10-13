<template>
  <div class="wv-view">
    <div class="wv-head">
      <h2 class="wv-title">{{ editingId ? '编辑文章' : '写作工作台' }}</h2>
      <span class="wv-draft-hint" v-if="hasLocalDraft">本地草稿已自动保存</span>
      <span class="wv-spacer"></span>
      <select v-if="!editingId" v-model="cat" class="wv-input wv-cat">
        <option value="">未分类</option>
        <option v-for="c in cats" :key="c.id" :value="c.name">{{ c.name }}</option>
      </select>
      <button class="btn btn-sm" :class="{ 'bm-primary': showToc }" @click="showToc = !showToc" title="显示/隐藏正文大纲">☰ 大纲</button>
      <button class="btn btn-sm" :disabled="busy || !canSave" @click="savePost">{{ editingId ? '保存修改' : '发布文章' }}</button>
    </div>

    <div class="wv-meta">
      <input v-model="title" class="wv-input wv-title-in" placeholder="文章标题" maxlength="200" :disabled="!!editingId" />
      <select v-if="editingId" class="wv-input wv-load" @change="loadPost($event.target.value)">
        <option value="">载入已有文章编辑…</option>
        <option v-for="p in posts" :key="p.id" :value="p.id">{{ p.name }}</option>
      </select>
      <span v-else class="wv-hint-new">发布后将自动进入编辑模式</span>
    </div>

    <!-- AI 工具栏 -->
    <div class="wv-ai-bar">
      <button class="btn btn-sm" :disabled="aiBusy" @click="aiOutline" title="基于标题与知识库素材生成大纲">✦ 大纲</button>
      <button class="btn btn-sm" :disabled="aiBusy || !content" @click="aiContinue" title="从结尾自然续写">✦ 续写</button>
      <button class="btn btn-sm" :disabled="aiBusy || !content" @click="aiPolish('polish')" title="优化表达不改原意">✦ 润色</button>
      <button class="btn btn-sm" :disabled="aiBusy || !content" @click="aiPolish('expand')" title="补充细节，篇幅 1.5-2 倍">✦ 扩写</button>
      <button class="btn btn-sm" :disabled="aiBusy || !content" @click="aiPolish('shorten')" title="精炼到一半篇幅">✦ 缩写</button>
      <button class="btn btn-sm" :disabled="aiBusy || (!title && !content)" @click="aiTags" title="推荐 3-6 个标签">✦ 标签建议</button>
      <span v-if="aiBusy" class="wv-ai-busy">AI 生成中…</span>
      <span v-if="aiErr" class="wv-ai-err">{{ aiErr }}</span>
    </div>

    <!-- AI 结果面板 -->
    <div v-if="aiText" class="wv-ai-result">
      <div class="wv-ai-result-head">
        <span>AI 输出</span>
        <span class="wv-ai-tags" v-if="aiTagsList.length">
          <button v-for="t in aiTagsList" :key="t" class="wv-tag-chip" @click="appendTag(t)" title="点击追加到文末">#{{ t }}</button>
        </span>
        <span class="wv-spacer"></span>
        <button class="btn btn-sm" @click="insertAtEnd(aiText)" v-if="aiMode !== 'polish'">插入到文末</button>
        <button class="btn btn-sm" @click="copyAI">复制</button>
        <button class="btn btn-sm bm-primary" @click="replaceContent(aiText)" v-if="aiMode === 'polish'">替换正文</button>
        <button class="btn btn-sm" @click="aiText = ''; aiTagsList = []">关闭</button>
      </div>
      <div class="wv-ai-out" v-html="aiHtml"></div>
    </div>

    <!-- 素材检索 -->
    <div class="wv-mat">
      <input v-model="matQ" class="wv-input" placeholder="素材检索：输入关键词，从本站知识库找素材（回车检索）" @keyup.enter="doSearch" />
      <div v-if="mats.length" class="wv-mat-list">
        <div v-for="m in mats" :key="m.path" class="wv-mat-item" @click="insertAtEnd('[' + m.title + '](#/p/blog?path=' + encodeURIComponent(m.path) + ')')" :title="'点击插入内链：' + m.path">
          <span class="wv-mat-title">{{ m.title }}</span>
          <span class="wv-mat-preview">{{ m.preview }}</span>
        </div>
      </div>
    </div>

    <!-- SEO 与封面（编辑已有文章时可设置；新文章发布后再载入设置） -->
    <details v-if="editingId" class="wv-seo">
      <summary>封面图 / 摘要 / SEO</summary>
      <div class="wv-seo-grid">
        <label class="wv-seo-label">封面图 URL
          <div class="wv-cover-row">
            <input v-model="cover" class="wv-input" placeholder="https://… 或站内可公开访问的图片地址（留空不设）" />
            <label class="btn btn-sm wv-cover-up">
              {{ uploading ? '上传中…' : '上传封面' }}
              <input type="file" accept="image/*" hidden @change="uploadCover" />
            </label>
            <img v-if="cover" :src="cover" class="wv-cover-thumb" alt="封面预览" @error="$event.target.style.display='none'" />
          </div>
        </label>
        <label class="wv-seo-label">自定义摘要<textarea v-model="excerpt" class="wv-input" rows="2" maxlength="600" placeholder="留空则自动取正文开头 / AI 摘要"></textarea></label>
        <label class="wv-seo-label">SEO 标题<input v-model="seoTitle" class="wv-input" maxlength="200" placeholder="留空则用文章标题" /></label>
        <label class="wv-seo-label">SEO 描述<textarea v-model="seoDesc" class="wv-input" rows="2" maxlength="400" placeholder="留空则用摘要（搜索引擎结果页描述）"></textarea></label>
      </div>
      <p class="wv-seo-hint">保存文章时一并保存本面板；SEO 字段用于 SSR 页 title/description/OG，缺省自动回退。</p>
    </details>

    <!-- 双栏：编辑 / 预览（左侧可选大纲） -->
    <div class="wv-panes" :class="{ 'has-toc': showToc }">
      <aside v-if="showToc" class="wv-toc">
        <p class="wv-toc-title">大纲</p>
        <p v-if="!toc.length" class="wv-toc-empty">用 # 标题生成大纲</p>
        <a
          v-for="t in toc"
          :key="t.idx"
          class="wv-toc-item"
          :class="'d' + t.depth"
          :title="t.text"
          @click="jumpToc(t)"
        >{{ t.text }}</a>
      </aside>
      <textarea
        ref="editorEl"
        v-model="content"
        class="wv-editor"
        placeholder="Markdown 正文…（左侧编辑，右侧实时预览；可直接粘贴/拖入图片）"
        spellcheck="false"
        @paste="onPaste"
        @drop.prevent="onDrop"
        @dragover.prevent
      ></textarea>
      <div ref="previewEl" class="wv-preview prose" v-html="previewHtml"></div>
    </div>
    <span v-if="uploading" class="wv-uploading">图片上传中…（{{ uploading }}）</span>
  </div>
</template>

<script setup>
import { ref, computed, nextTick, onMounted, onUnmounted, watch } from 'vue'
import { useToastStore } from '@/stores/toast'
import { requestJSON, fetchFileText, uploadFiles, mkdir } from '@/api'
import { renderMarkdown, extractToc, enhanceMermaid } from '@/utils/markdown'

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
const editorEl = ref(null)
const previewEl = ref(null)
const showToc = ref(true)
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
  const d = await mkdir('媒体', BLOG_DIR)
  mediaDirId = d?.id || ''
  return mediaDirId
}
function insertAtCursor(text) {
  const ta = editorEl.value
  if (!ta) {
    content.value = (content.value || '') + text
    return
  }
  const s = ta.selectionStart ?? (content.value || '').length
  const e = ta.selectionEnd ?? s
  content.value = (content.value || '').slice(0, s) + text + (content.value || '').slice(e)
  nextTick(() => {
    ta.selectionStart = ta.selectionEnd = s + text.length
    ta.focus()
  })
}
async function uploadImageFile(file, setCover = false) {
  if (!file || !/^image\//.test(file.type || '')) return false
  uploading.value++
  try {
    const parent = await ensureMediaDir()
    if (!parent) throw new Error('媒体目录不可用')
    const rs = await uploadFiles([{ file, path: file.name }], parent)
    const f = rs?.[0]?.file
    if (!f?.id) throw new Error('上传无结果')
    if (setCover) {
      cover.value = '/api/v1/public/media/' + f.id
      toast.success('封面已上传')
    } else {
      insertAtCursor(`![](/api/v1/public/media/${f.id})\n`)
      toast.success('图片已插入')
    }
    return true
  } catch (e) {
    toast.error('图片上传失败：' + (e.message || e))
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
    toast.success('已载入「' + title.value + '」，编辑后点"保存修改"')
  } catch (e) {
    toast.error('载入失败：' + (e.message || e))
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
        toast.error('正文已保存，但封面/SEO 保存失败：' + (me.message || me))
      }
      toast.success('文章已保存')
    } else {
      const d = await requestJSON('/blog/posts', {
        method: 'POST',
        body: JSON.stringify({ title: title.value.trim(), category: cat.value, content: content.value }),
      })
      toast.success('已发布：' + (d?.path || title.value))
      editingId.value = d?.id || ''
      try { localStorage.removeItem(DRAFT_KEY) } catch (_) {}
      hasLocalDraft.value = false
    }
    loadManage()
  } catch (e) {
    toast.error('保存失败：' + (e.message || e))
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
    aiErr.value = 'AI 调用失败：' + (e.message || e)
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

async function aiOutline() {
  const topic = title.value.trim()
  if (!topic) {
    aiErr.value = '请先填写文章标题（作为大纲主题）'
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
    if (!d.tags.length) aiErr.value = 'AI 未给出标签建议'
  }
}

function insertAtEnd(text) {
  content.value = (content.value || '') + (content.value ? '\n\n' : '') + text
  toast.success('已插入到文末')
}

function replaceContent(text) {
  content.value = text
  toast.success('已替换正文（原内容可用浏览器撤销 Ctrl+Z 找回）')
}

function appendTag(t) {
  content.value = (content.value || '') + (content.value ? '\n\n' : '') + '标签：' + t
}

function copyAI() {
  navigator.clipboard?.writeText(aiText.value || '').then(
    () => toast.success('已复制'),
    () => toast.error('复制失败')
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
  padding: 16px 18px;
  max-width: 1280px;
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
.wv-editor {
  resize: none;
  border: 1px solid var(--border, #e3e6eb);
  border-radius: 10px;
  padding: 14px;
  font-family: var(--mono, ui-monospace, monospace);
  font-size: 13px;
  line-height: 1.7;
  background: var(--bg-1, #fff);
  color: var(--text-1, #1f2329);
  outline: none;
}
.wv-editor:focus {
  border-color: var(--primary, #4f7cff);
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
