<template>
  <div class="search-view">
    <div class="search-head">
      <h1>{{  $t('智能检索')  }}</h1>
      <p class="muted">{{  $t('全库语义检索：关键词 + 向量融合（RRF），支持文件名、内容与语义匹配。')  }}</p>
    </div>

    <div class="search-input-row">
      <AikIcon name="search" :size="17" />
      <input
        v-model="q"
        :placeholder="$t('输入关键词，检索整个空间（如：歌曲、合同、向量检索…）')"
        class="search-input"
        autofocus
        @input="onInput"
        @keyup.enter="doSearch"
      />
      <button v-if="loading" class="search-spin" disabled></button>
    </div>

    <!-- 搜索范围（博客目录在存储层隐藏后的配套：文件/博客/全部） -->
    <div class="search-scope-row">
      <span class="scope-label">{{  $t('范围')  }}</span>
      <button
        v-for="s in scopes"
        :key="s.v"
        class="scope-btn"
        :class="{ active: scope === s.v }"
        @click="setScope(s.v)"
      >{{  s.label  }}</button>
    </div>

    <!-- 结果 -->
    <div v-if="searched" class="search-results">
      <div v-if="loading" class="empty-hint">{{  $t('检索中…')  }}</div>
      <div v-else-if="err" class="empty-hint">{{  err  }}</div>
      <div v-else-if="!results.length" class="empty-hint">{{  $t('没有匹配的内容，换个关键词试试')  }}</div>
      <template v-else>
        <div class="result-meta muted-3">{{  $t('共')  }} {{  results.length  }} {{  $t('条结果')  }}</div>
        <div
          v-for="f in results"
          :key="f.id"
          class="result-row"
          @dblclick="openFile(f)"
        >
          <AikIcon :name="kindIcon(f)" :size="17" />
          <div class="result-main">
            <div class="result-line">
              <span class="result-name" :title="f.name">{{  f.name  }}</span>
              <span class="hit-tag" :class="'hit-' + f.hit_in">{{  hitLabel(f.hit_in)  }}</span>
              <span class="result-path muted-3" :title="f.path">{{  f.path  }}</span>
              <span class="result-size muted-3">{{  f.kind === 'dir' ? $t('文件夹') : formatSize(f.size)  }}</span>
            </div>
            <!-- 溯源：命中块片段 + 定位原文 -->
            <div v-if="f.chunk" class="result-snippet">
              <span class="snippet-text" :title="f.chunk.content">{{  f.chunk.content  }}</span>
              <button
                class="btn btn-sm locate-btn"
                :title="$t('定位到原文段落')"
                @click.stop="locate(f)"
              >{{  $t('定位原文')  }}</button>
            </div>
          </div>
        </div>
      </template>
    </div>
  </div>
</template>

<script setup>
import { ref, watch } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import AikIcon from '@/components/AikIcon.vue'
import { useFilesStore } from '@/stores/files'
import { searchFiles } from '@/api'
import { t } from '@/i18n'

const router = useRouter()
const route = useRoute()
const files = useFilesStore()

const q = ref('')
const scope = ref('all')
const scopes = [
  { v: 'all', label: t('全部') },
  { v: 'file', label: t('文件') },
  { v: 'blog', label: t('博客') },
]
const results = ref([])
const loading = ref(false)
const searched = ref(false)
const err = ref('')
let timer = null

// 与顶部搜索联动：URL ?q= 变化 → 自动填入并搜索（顶部搜索 Enter 跳转后直接出结果）
watch(
  () => route.query.q,
  (v) => {
    if (v && typeof v === 'string') {
      q.value = v
      doSearch()
    }
  },
  { immediate: true }
)

function onInput() {
  clearTimeout(timer)
  timer = setTimeout(doSearch, 350)
}

function setScope(v) {
  if (scope.value === v) return
  scope.value = v
  doSearch()
}

async function doSearch() {
  const kw = q.value.trim()
  if (!kw) {
    searched.value = false
    results.value = []
    err.value = ''
    return
  }
  loading.value = true
  err.value = ''
  try {
    results.value = await searchFiles(kw, 30, scope.value)
    searched.value = true
  } catch (e) {
    err.value = t('检索失败：') + (e.message || e)
    results.value = []
    searched.value = true
  } finally {
    loading.value = false
  }
}

function hitLabel(hitIn) {
  if (hitIn === 'name') return t('文件名')
  if (hitIn === 'content') return t('内容')
  if (hitIn === 'vector') return t('语义')
  return t('匹配')
}

function ext(name) {
  const i = name.lastIndexOf('.')
  return i >= 0 ? name.slice(i + 1).toLowerCase() : ''
}

function kindIcon(f) {
  if (f.kind === 'dir') return 'folder'
  const map = {
    md: 'fileText', txt: 'fileText', log: 'fileText',
    png: 'fileImage', jpg: 'fileImage', jpeg: 'fileImage', gif: 'fileImage', webp: 'fileImage', svg: 'fileImage',
    mp3: 'fileAudio', wav: 'fileAudio',
    mp4: 'fileVideo', mov: 'fileVideo',
    pdf: 'filePdf',
    zip: 'fileArchive', rar: 'fileArchive'
  }
  return map[ext(f.name)] || 'file'
}

function formatSize(n) {
  if (n < 1024) return n + ' B'
  if (n < 1024 * 1024) return (n / 1024).toFixed(1) + ' KB'
  if (n < 1024 * 1024 * 1024) return (n / 1024 / 1024).toFixed(1) + ' MB'
  return (n / 1024 / 1024 / 1024).toFixed(2) + ' GB'
}

function openFile(f) {
  if (f.kind === 'dir') {
    files.openDir(f.id, f)
    router.push('/files')
    return
  }
  router.push({ path: `/read/${f.id}` })
}

// 溯源定位：打开阅读页并高亮命中段落（带搜索词 q，优先精确命中处）
function locate(f) {
  if (!f.chunk) return
  const query = { hl: f.chunk.id }
  if (q.value) query.q = q.value
  router.push({ path: `/read/${f.id}`, query })
}
</script>

<style scoped>
.search-view {
  max-width: 1200px;
  margin: 0 auto;
  padding: 20px 24px 40px;
}

.search-head h1 {
  font-size: var(--fs-xlarge);
  font-weight: 700;
  margin-bottom: 6px;
}

.search-head p {
  font-size: var(--fs-small);
  margin-bottom: 24px;
}

.search-input-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 12px 16px;
  border-radius: var(--radius);
  background: var(--surface);
  border: 1.5px solid var(--border);
  color: var(--text-3);
  margin-bottom: 12px;
  transition: border-color 0.15s ease;
}

/* 搜索范围：全部 / 文件 / 博客 */
.search-scope-row {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 20px;
}

.scope-label {
  font-size: 13px;
  color: var(--muted, #8a919f);
  margin-right: 4px;
}

.scope-btn {
  font-size: 13px;
  padding: 4px 14px;
  border-radius: 999px;
  border: 1px solid var(--border, #e3e6eb);
  background: var(--surface, #fff);
  color: var(--text-3, #6b7280);
  cursor: pointer;
  transition: all 0.15s ease;
}

.scope-btn:hover {
  border-color: var(--accent, #4c7df0);
  color: var(--accent, #4c7df0);
}

.scope-btn.active {
  background: var(--accent, #4c7df0);
  border-color: var(--accent, #4c7df0);
  color: #fff;
}

.search-input-row:focus-within {
  border-color: var(--primary);
}

.search-input {
  flex: 1;
  border: none;
  outline: none;
  background: none;
  color: var(--text);
  font-size: var(--fs-medium);
}

.search-spin {
  width: 14px;
  height: 14px;
  border: 2px solid var(--border-strong);
  border-top-color: var(--primary);
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
  background: none;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

.search-results {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.result-meta {
  font-size: var(--fs-small);
  margin-bottom: 6px;
  padding: 0 14px;
}

.result-row {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  padding: 11px 14px;
  border-radius: var(--radius-sm);
  cursor: pointer;
  transition: background 0.12s ease;
}

.result-row:hover {
  background: var(--surface);
}

.result-main {
  flex: 1;
  min-width: 0;
}

.result-line {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

.result-snippet {
  margin-top: 5px;
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

.snippet-text {
  flex: 1;
  min-width: 0;
  font-size: 12px;
  line-height: 1.55;
  color: var(--text-2, #555);
  background: var(--surface-2, rgba(0, 0, 0, 0.03));
  border-radius: 6px;
  padding: 5px 9px;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
  cursor: default;
}

.locate-btn {
  flex: none;
  padding: 2px 10px;
  font-size: 12px;
}

.result-name {
  font-size: var(--fs-base);
  color: var(--text);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  min-width: 0;
  max-width: 40%;
}

.hit-tag {
  flex: none;
  font-size: 11px;
  line-height: 1;
  padding: 3px 7px;
  border-radius: 999px;
  color: var(--text-3);
  background: var(--surface-2, rgba(0, 0, 0, 0.04));
}

.hit-name {
  color: #1f7a3d;
  background: rgba(31, 122, 61, 0.12);
}

.hit-content {
  color: #2b5fd9;
  background: rgba(43, 95, 217, 0.12);
}

.hit-vector {
  color: #8a3dd9;
  background: rgba(138, 61, 217, 0.12);
}

.result-path {
  margin-left: auto;
  font-size: var(--fs-small);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 34%;
}

.result-size {
  font-size: var(--fs-small);
  width: 72px;
  text-align: right;
  flex: none;
}

.empty-hint {
  padding: 30px;
  text-align: center;
  color: var(--text-3);
  font-size: var(--fs-small);
}
</style>
