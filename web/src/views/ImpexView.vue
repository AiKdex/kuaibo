<template>
  <div class="impex-view">
    <div class="iv-head">
      <div>
        <h2>{{  $t('导入导出中心')  }}</h2>
        <p class="iv-sub">{{  $t('统一入口 · 异步任务 · 进度可见 · 失败清单可重试')  }}</p>
      </div>
      <button class="btn" :disabled="loading" @click="loadAll">
        <AikIcon name="refresh" :size="14" />{{  $t('刷新')  }}
      </button>
    </div>

    <!-- 页签：导入 / 导出 -->
    <div class="iv-tabs">
      <button class="iv-tab" :class="{ active: tab === 'import' }" @click="tab = 'import'">
        <AikIcon name="upload" :size="15" />{{  $t('导入')  }}
      </button>
      <button class="iv-tab" :class="{ active: tab === 'export' }" @click="tab = 'export'">
        <AikIcon name="download" :size="15" />{{  $t('导出')  }}
      </button>
    </div>

    <!-- 目标目录（导入/文件夹导出共用） -->
    <div v-if="tab === 'import' || expSource === 'folder' || expSource === 'obsidian'" class="iv-field">
      <label>{{  $t('目标目录')  }}</label>
      <select v-model="parent">
        <option value="">{{  $t('根目录（我的空间）')  }}</option>
        <option v-for="d in dirOptions" :key="d.id" :value="d.id">{{  d.label  }}</option>
      </select>
    </div>

    <!-- 导入面板 -->
    <div v-if="tab === 'import'" class="iv-grid">
      <div class="iv-card">
        <div class="iv-card-h"><AikIcon name="fileArchive" :size="16" /><span>{{  $t('ZIP 批量导入')  }}</span></div>
        <p class="iv-card-p">{{  $t('上传 ZIP 包，自动解压并按目录结构递归入库')  }}</p>
        <div class="iv-row">
          <select v-model="zipConflict">
            <option value="rename">{{  $t('同名：自动重命名')  }}</option>
            <option value="skip">{{  $t('同名：跳过')  }}</option>
            <option value="overwrite">{{  $t('同名：覆盖')  }}</option>
          </select>
        </div>
        <label class="iv-file">
          <input type="file" accept=".zip" @change="onPickZip('zip', $event)" />
          <span>{{  $t('选择 ZIP 并导入')  }}</span>
        </label>
      </div>

      <div class="iv-card">
        <div class="iv-card-h"><AikIcon name="book" :size="16" /><span>{{  $t('Obsidian vault 导入')  }}</span></div>
        <p class="iv-card-p">{{  $t('解析')  }} <code>{{  $t('[[双链]]')  }}</code> {{  $t('与 frontmatter 标签 → 映射到标签')  }}</p>
        <div class="iv-row">
          <select v-model="obsConflict">
            <option value="rename">{{  $t('同名：自动重命名')  }}</option>
            <option value="skip">{{  $t('同名：跳过')  }}</option>
            <option value="overwrite">{{  $t('同名：覆盖')  }}</option>
          </select>
        </div>
        <label class="iv-file">
          <input type="file" accept=".zip" @change="onPickZip('obsidian', $event)" />
          <span>{{  $t('选择 vault ZIP 并导入')  }}</span>
        </label>
      </div>

      <div class="iv-card">
        <div class="iv-card-h"><AikIcon name="link" :size="16" /><span>{{  $t('URL 网页抓取')  }}</span></div>
        <p class="iv-card-p">{{  $t('抓取网页正文 → 转为 Markdown 入库')  }}</p>
        <div class="iv-row">
          <input v-model="url" placeholder="https://example.com/article" @keydown.enter="doImportURL" />
          <button class="btn btn-primary" :disabled="busy || !url.trim()" @click="doImportURL">{{  $t('抓取')  }}</button>
        </div>
      </div>

      <div class="iv-card">
        <div class="iv-card-h"><AikIcon name="clipboard" :size="16" /><span>{{  $t('剪贴板粘贴')  }}</span></div>
        <p class="iv-card-p">{{  $t('文本（支持图片，直接 Ctrl+V 粘贴到下方）')  }}</p>
        <div class="iv-paste" tabindex="0" @paste.prevent="onPaste" @click="$event.currentTarget.focus()">
          <template v-if="!clipText">
            <AikIcon name="clipboard" :size="20" />
            <span>{{  $t('点击后按 Ctrl+V 粘贴文本或图片')  }}</span>
          </template>
          <span v-else class="iv-paste-text">{{  clipText.slice(0, 160)  }}{{  clipText.length > 160 ? '…' : ''  }}</span>
        </div>
        <div class="iv-row">
          <input v-model="clipName" :placeholder="t('文件名（默认 剪贴板粘贴.md）')" />
          <button class="btn btn-primary" :disabled="busy || !clipText.trim()" @click="doImportClipboard">{{ $t('入库') }}</button>
        </div>
      </div>
    </div>

    <!-- 导出面板 -->
    <div v-else class="iv-grid">
      <div class="iv-card">
        <div class="iv-card-h"><AikIcon name="folder" :size="16" /><span>{{ $t('文件夹 ZIP 打包') }}</span></div>
        <p class="iv-card-p">{{ $t('选中目录 → 生成 ZIP（保留子目录结构）') }}</p>
        <div class="iv-row">
          <button class="btn btn-primary" :disabled="busy" @click="doExport('folder', parent, t('文件夹导出'))">
            <AikIcon name="download" :size="14" />{{ $t('打包下载') }}
          </button>
        </div>
      </div>

      <div class="iv-card">
        <div class="iv-card-h"><AikIcon name="book" :size="16" /><span>{{ $t('Obsidian vault 导出') }}</span></div>
        <p class="iv-card-p">{{ $t('生成') }} <code>{{ $t('[[双链]]') }}</code> {{ $t('+ frontmatter 标签 +') }} <code>.obsidian</code> {{ $t('配置') }}</p>
        <div class="iv-row">
          <button class="btn btn-primary" :disabled="busy" @click="doExport('obsidian', parent, t('vault 导出'))">
            <AikIcon name="download" :size="14" />{{ $t('导出 vault') }}
          </button>
        </div>
      </div>

      <div class="iv-card">
        <div class="iv-card-h"><AikIcon name="tag" :size="16" /><span>{{ $t('按标签批量导出') }}</span></div>
        <p class="iv-card-p">{{ $t('按标签筛选 → Markdown 批量打包') }}</p>
        <div class="iv-row">
          <select v-model="expTag">
            <option value="" disabled>{{ $t('选择标签…') }}</option>
            <option v-for="t in tags" :key="t.id" :value="t.id">{{  t.path  }}（{{  t.count  }}）</option>
          </select>
          <button class="btn btn-primary" :disabled="busy || !expTag" @click="doExport('tag', expTag, t('标签导出'))">{{ $t('导出') }}</button>
        </div>
      </div>

      <div class="iv-card">
        <div class="iv-card-h"><AikIcon name="grid" :size="16" /><span>{{ $t('按集合批量导出') }}</span></div>
        <p class="iv-card-p">{{ $t('按集合筛选 → Markdown 批量打包') }}</p>
        <div class="iv-row">
          <select v-model="expCollection">
            <option value="" disabled>{{ $t('选择集合…') }}</option>
            <option v-for="c in collections" :key="c.id" :value="c.id">{{  c.name  }}</option>
          </select>
          <button class="btn btn-primary" :disabled="busy || !expCollection" @click="doExport('collection', expCollection, t('集合导出'))">{{ $t('导出') }}</button>
        </div>
      </div>
    </div>

    <!-- 任务中心 -->
    <div class="iv-jobs">
      <div class="iv-jobs-h">
        <h3>{{ $t('任务中心') }}<span class="iv-count">{{  shownJobs.length  }}</span></h3>
      </div>

      <div v-if="loading && !shownJobs.length" class="iv-empty">{{ $t('加载中…') }}</div>

      <div v-else-if="!shownJobs.length" class="iv-empty">
        <AikIcon :name="tab === 'import' ? 'upload' : 'download'" :size="28" />
        <p>{{ $t('暂无') }}{{  tab === 'import' ? $t('导入') : $t('导出')  }}{{ $t('任务') }}</p>
        <p class="iv-muted">{{ $t('从上方选择一种方式开始') }}</p>
      </div>

      <div v-else class="iv-list">
        <div v-for="j in shownJobs" :key="j.id" class="iv-job">
          <div class="iv-job-main">
            <AikIcon :name="j.kind === 'import' ? 'upload' : 'download'" :size="16" class="iv-job-ico" />
            <div class="iv-job-info">
              <div class="iv-job-title">
                <span class="iv-job-src">{{  srcLabel(j)  }}</span>
                <span class="iv-badge" :class="'st-' + j.status">{{  statusText(j.status)  }}</span>
                <span class="iv-job-time">{{  fmtTime(j.created_at)  }}</span>
              </div>
              <div class="iv-bar"><div class="iv-bar-fill" :style="{ width: j.percent + '%' }"></div></div>
              <div class="iv-job-meta">
                <span>{{ $t('进度') }} {{  j.percent  }}%</span>
                <span v-if="j.total">{{ $t('· 共') }} {{  j.total  }} {{ $t('项') }}</span>
                <span v-if="j.succeeded">{{ $t('· 成功') }} {{  j.succeeded  }}</span>
                <span v-if="j.failed" class="iv-danger">{{ $t('· 失败') }} {{  j.failed  }}</span>
                <span v-if="j.last_error" class="iv-danger" :title="j.last_error">· {{  j.last_error.slice(0, 60)  }}</span>
              </div>
            </div>
            <div class="iv-job-ops">
              <a
                v-if="j.kind === 'export' && (j.status === 'done' || j.status === 'partial')"
                class="btn btn-primary btn-sm"
                @click.prevent="doDownload(j.id)"
              >{{ $t('下载') }}</a>
              <button v-if="j.failed" class="btn btn-sm" @click="toggleExpand(j.id)">
                {{  expanded === j.id ? $t('收起') : $t('失败清单')  }}
              </button>
              <button v-if="j.status === 'failed' || j.status === 'partial'" class="btn btn-sm" @click="doRetry(j)">
                {{ $t('重试') }}
              </button>
            </div>
          </div>
          <div v-if="expanded === j.id && j.fail_list && j.fail_list.length" class="iv-fails">
            <div v-for="(f, i) in j.fail_list" :key="i" class="iv-fail">
              <AikIcon name="close" :size="12" />
              <span class="iv-fail-item">{{  f.item  }}</span>
              <span class="iv-fail-err">{{  f.error  }}</span>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- 上传进度提示 -->
    <div v-if="uploadPct > 0 && uploadPct < 100" class="iv-uploading">{{ $t('上传中…') }} {{  uploadPct  }}%</div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import AikIcon from '@/components/AikIcon.vue'
import * as api from '@/api'
import { useToastStore } from '@/stores/toast'
import { t } from '@/i18n'

const toast = useToastStore()

const tab = ref('import')
const loading = ref(false)
const busy = ref(false)
const uploadPct = ref(0)
const expanded = ref('')

const imports = ref([])
const exports = ref([])
const tree = ref([])
const tags = ref([])
const collections = ref([])

const parent = ref('')
const url = ref('')
const clipText = ref('')
const clipName = ref('')
const zipConflict = ref('rename')
const obsConflict = ref('rename')
const expSource = ref('folder')
const expTag = ref('')
const expCollection = ref('')

const dirOptions = computed(() => {
  const byId = {}
  for (const n of tree.value) byId[n.id] = n
  const label = (n) => {
    const parts = [n.name]
    let p = n.parent_id
    while (p && byId[p]) {
      parts.unshift(byId[p].name)
      p = byId[p].parent_id
    }
    return parts.join(' / ')
  }
  return tree.value
    .filter((n) => n.kind === 'dir')
    .map((n) => ({ id: n.id, label: label(n) }))
    .sort((a, b) => a.label.localeCompare(b.label, 'zh'))
})

const shownJobs = computed(() => (tab.value === 'import' ? imports.value : exports.value))

const SRC = {
  zip: t('ZIP 批量导入'),
  clipboard: t('剪贴板粘贴'),
  url: t('URL 网页抓取'),
  folder: t('文件夹打包'),
  tag: t('按标签导出'),
  collection: t('按集合导出')
}
function srcLabel(j) {
  if (j.source === 'obsidian') return j.kind === 'export' ? t('Obsidian 导出') : t('Obsidian 导入')
  return SRC[j.source] || j.source
}

const ST = { pending: t('等待中'), running: t('进行中'), done: t('完成'), partial: t('部分完成'), failed: t('失败') }
function statusText(s) {
  return ST[s] || s
}

function fmtTime(ts) {
  if (!ts) return ''
  const d = new Date(Number(ts) * 1000)
  const p = (x) => String(x).padStart(2, '0')
  return `${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`
}

function downloadUrl(id) {
  return api.impexDownloadUrl(id)
}
async function doDownload(id) {
  try {
    await api.impexDownload(id)
  } catch (err) {
    showToast(err.message || t('下载失败'))
  }
}

// ---- 数据加载 ----
async function loadJobs() {
  const [im, ex] = await Promise.all([api.impexList('import'), api.impexList('export')])
  imports.value = im
  exports.value = ex
  ensurePolling()
}

async function loadAll() {
  loading.value = true
  try {
    const [im, ex, t, tg, col] = await Promise.all([
      api.impexList('import'),
      api.impexList('export'),
      api.getTree().catch(() => []),
      api.listTags().catch(() => []),
      api.listCollections().catch(() => [])
    ])
    imports.value = im
    exports.value = ex
    tree.value = t
    tags.value = tg
    collections.value = col
    ensurePolling()
  } catch (e) {
    toast.push(t('加载失败：') + (e.message || e))
  } finally {
    loading.value = false
  }
}

// ---- 轮询：有进行中的任务时刷新 ----
let timer = null
function ensurePolling() {
  const active = [...imports.value, ...exports.value].some((j) => j.status === 'pending' || j.status === 'running')
  if (active && !timer) {
    timer = setInterval(async () => {
      try {
        await loadJobs()
      } catch (_) {
        /* 静默 */
      }
      if (![...imports.value, ...exports.value].some((j) => j.status === 'pending' || j.status === 'running')) {
        clearInterval(timer)
        timer = null
      }
    }, 1500)
  }
}
onUnmounted(() => {
  if (timer) clearInterval(timer)
})

// ---- 导入 ----
async function onPickZip(source, e) {
  const file = e.target.files && e.target.files[0]
  e.target.value = ''
  if (!file) return
  busy.value = true
  try {
    const job = await api.impexImportZip(file, {
      parent: parent.value,
      source,
      onConflict: source === 'obsidian' ? obsConflict.value : zipConflict.value,
      onProgress: (p) => (uploadPct.value = p)
    })
    toast.push(source === 'obsidian' ? t('Obsidian 导入任务已创建') : t('ZIP 导入任务已创建'))
    await loadJobs()
    if (job && job.id) expanded.value = ''
  } catch (err) {
    toast.push(t('导入失败：') + (err.message || err))
  } finally {
    uploadPct.value = 0
    busy.value = false
  }
}

async function doImportURL() {
  const u = url.value.trim()
  if (!u) return
  busy.value = true
  try {
    await api.impexImportURL(u, { parent: parent.value })
    toast.push(t('URL 抓取任务已创建'))
    url.value = ''
    await loadJobs()
  } catch (e) {
    toast.push(t('创建失败：') + (e.message || e))
  } finally {
    busy.value = false
  }
}

function onPaste(e) {
  const dt = e.clipboardData
  if (!dt) return
  const img = Array.from(dt.items || []).find((i) => i.type.startsWith('image/'))
  if (img) {
    const f = img.getAsFile()
    const reader = new FileReader()
    reader.onload = async () => {
      busy.value = true
      try {
        await api.impexImportClipboard({ parent: parent.value, name: clipName.value, image: String(reader.result) })
        toast.push(t('图片粘贴任务已创建'))
        clipText.value = ''
        await loadJobs()
      } catch (err) {
        toast.push(t('创建失败：') + (err.message || err))
      } finally {
        busy.value = false
      }
    }
    reader.readAsDataURL(f)
    return
  }
  const text = dt.getData('text')
  if (text) clipText.value = text
}

async function doImportClipboard() {
  if (!clipText.value.trim()) return
  busy.value = true
  try {
    await api.impexImportClipboard({ parent: parent.value, name: clipName.value, text: clipText.value })
    toast.push(t('粘贴任务已创建'))
    clipText.value = ''
    clipName.value = ''
    await loadJobs()
  } catch (e) {
    toast.push(t('创建失败：') + (e.message || e))
  } finally {
    busy.value = false
  }
}

// ---- 导出 ----
async function doExport(source, targetId, name) {
  busy.value = true
  expSource.value = source
  try {
    await api.impexCreateExport(source, targetId, name)
    toast.push(t('导出任务已创建'))
    tab.value = 'export'
    await loadJobs()
  } catch (e) {
    toast.push(t('创建失败：') + (e.message || e))
  } finally {
    busy.value = false
  }
}

async function doRetry(j) {
  try {
    await api.impexRetry(j.kind, j.id)
    toast.push(t('已重试失败项'))
    await loadJobs()
  } catch (e) {
    toast.push(t('重试失败：') + (e.message || e))
  }
}

function toggleExpand(id) {
  expanded.value = expanded.value === id ? '' : id
}

onMounted(loadAll)
</script>

<style scoped>
.impex-view {
  max-width: 1200px;
  margin: 0 auto;
  padding: 20px 24px 40px;
}
.iv-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  margin-bottom: 14px;
}
.iv-head h2 {
  font-size: 18px;
  font-weight: 600;
  color: var(--text);
}
.iv-sub {
  font-size: 12.5px;
  color: var(--text-3);
  margin-top: 4px;
}
.iv-tabs {
  display: flex;
  gap: 6px;
  margin-bottom: 16px;
  border-bottom: 1px solid var(--border);
}
.iv-tab {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 9px 14px;
  font-size: 13.5px;
  color: var(--text-2);
  border-bottom: 2px solid transparent;
  margin-bottom: -1px;
}
.iv-tab.active {
  color: var(--primary);
  border-bottom-color: var(--primary);
  font-weight: 500;
}
.iv-field {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 14px;
}
.iv-field label {
  font-size: 12.5px;
  color: var(--text-2);
  flex-shrink: 0;
}
select,
input {
  height: 32px;
  padding: 0 10px;
  border-radius: var(--radius-sm);
  border: 1px solid var(--border);
  background: var(--surface);
  color: var(--text);
  font-size: 12.5px;
  outline: none;
}
select:focus,
input:focus {
  border-color: var(--primary);
}
.iv-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: 14px;
  margin-bottom: 22px;
}
.iv-card {
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: var(--surface);
  padding: 14px 16px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.iv-card-h {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13.5px;
  font-weight: 600;
  color: var(--text);
}
.iv-card-p {
  font-size: 12px;
  color: var(--text-3);
  line-height: 1.6;
}
.iv-card-p code {
  background: var(--surface-2);
  padding: 1px 4px;
  border-radius: 4px;
  font-size: 11px;
}
.iv-row {
  display: flex;
  gap: 8px;
  align-items: center;
}
.iv-row input,
.iv-row select {
  flex: 1;
  min-width: 0;
}
.iv-file {
  display: inline-flex;
}
.iv-file input {
  display: none;
}
.iv-file span {
  display: inline-flex;
  align-items: center;
  height: 32px;
  padding: 0 14px;
  border-radius: var(--radius-sm);
  background: var(--primary);
  color: #fff;
  font-size: 12.5px;
  font-weight: 500;
  cursor: pointer;
}
.iv-file span:hover {
  opacity: 0.92;
}
.iv-paste {
  border: 1px dashed var(--border);
  border-radius: var(--radius-sm);
  padding: 14px;
  min-height: 64px;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  color: var(--text-3);
  font-size: 12px;
  cursor: text;
  outline: none;
}
.iv-paste:focus {
  border-color: var(--primary);
}
.iv-paste-text {
  color: var(--text-2);
  text-align: left;
  width: 100%;
}
.btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 32px;
  padding: 0 12px;
  border-radius: var(--radius-sm);
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
  background: var(--surface-2);
  border: 1px solid var(--border);
  color: var(--text-2);
  white-space: nowrap;
}
.btn:hover:not(:disabled) {
  color: var(--text);
  border-color: var(--text-3);
}
.btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
.btn-primary {
  background: var(--primary);
  border-color: var(--primary);
  color: #fff;
}
.btn-primary:hover:not(:disabled) {
  color: #fff;
  opacity: 0.92;
}
.btn-sm {
  height: 28px;
  padding: 0 10px;
  font-size: 12px;
}
.iv-jobs {
  margin-top: 8px;
}
.iv-jobs-h h3 {
  font-size: 14px;
  font-weight: 600;
  color: var(--text);
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
}
.iv-count {
  font-size: 11px;
  color: var(--text-3);
  background: var(--surface-2);
  border-radius: 10px;
  padding: 1px 8px;
}
.iv-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  padding: 48px 20px;
  color: var(--text-3);
  font-size: 13px;
  border: 1px dashed var(--border);
  border-radius: var(--radius);
}
.iv-muted {
  font-size: 12px;
  color: var(--text-3);
  opacity: 0.8;
}
.iv-list {
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: var(--surface);
  overflow: hidden;
}
.iv-job {
  border-bottom: 1px solid var(--border);
}
.iv-job:last-child {
  border-bottom: none;
}
.iv-job-main {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 14px;
}
.iv-job-ico {
  color: var(--text-3);
  flex-shrink: 0;
}
.iv-job-info {
  flex: 1;
  min-width: 0;
}
.iv-job-title {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 6px;
}
.iv-job-src {
  font-size: 13px;
  font-weight: 500;
  color: var(--text);
}
.iv-job-time {
  font-size: 11.5px;
  color: var(--text-3);
  margin-left: auto;
}
.iv-badge {
  font-size: 11px;
  padding: 1px 8px;
  border-radius: 10px;
}
.st-pending {
  background: var(--surface-2);
  color: var(--text-3);
}
.st-running {
  background: var(--primary-soft);
  color: var(--primary);
}
.st-done {
  background: rgba(16, 185, 129, 0.12);
  color: var(--success);
}
.st-partial {
  background: rgba(245, 158, 11, 0.14);
  color: var(--warning);
}
.st-failed {
  background: var(--danger-soft);
  color: var(--danger);
}
.iv-bar {
  height: 5px;
  border-radius: 3px;
  background: var(--surface-2);
  overflow: hidden;
}
.iv-bar-fill {
  height: 100%;
  background: var(--primary);
  transition: width 0.3s ease;
}
.iv-job-meta {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
  font-size: 11.5px;
  color: var(--text-3);
  margin-top: 6px;
}
.iv-danger {
  color: var(--danger);
}
.iv-job-ops {
  display: flex;
  gap: 6px;
  flex-shrink: 0;
}
.iv-fails {
  border-top: 1px dashed var(--border);
  padding: 8px 14px 12px;
  background: var(--surface-2);
  display: flex;
  flex-direction: column;
  gap: 6px;
  max-height: 200px;
  overflow-y: auto;
}
.iv-fail {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
}
.iv-fail-item {
  color: var(--text);
  flex-shrink: 0;
  max-width: 40%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.iv-fail-err {
  color: var(--danger);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.iv-uploading {
  position: fixed;
  right: 24px;
  bottom: 44px;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  padding: 8px 14px;
  font-size: 12.5px;
  color: var(--primary);
  box-shadow: var(--shadow-lg);
  z-index: var(--z-toast);
}
</style>
