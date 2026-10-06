<template>
  <div class="drafts-view">
    <div class="dv-head">
      <div>
        <h2>{{  $t('草稿箱')  }}</h2>
        <p class="dv-sub">{{  $t('未发布的内容（新建文档默认进入；发布后转正，不再出现在这里）')  }}</p>
      </div>
      <button class="btn" @click="load" :disabled="loading">
        <AikIcon name="refresh" :size="14" />{{  $t('刷新')  }}
      </button>
    </div>

    <div v-if="loading" class="dv-empty">{{  $t('加载中…')  }}</div>

    <div v-else-if="!items.length" class="dv-empty">
      <AikIcon name="fileText" :size="30" />
      <p>{{  $t('草稿箱是空的')  }}</p>
      <p class="muted">{{  $t('在文件库新建文档并保存，会作为草稿出现在这里；发布后移出')  }}</p>
    </div>

    <div v-else class="dv-list">
      <div v-for="f in items" :key="f.id" class="dv-row" @dblclick="openFile(f)">
        <AikIcon name="fileText" :size="16" class="dv-ico" />
        <button class="dv-name" :title="f.name" @click="openFile(f)">{{  f.name  }}</button>
        <span class="dv-meta">{{  parentName(f)  }}</span>
        <span class="dv-meta">{{  fmtSize(f.size)  }}</span>
        <span class="dv-meta">{{  fmtTime(f.updated_at)  }}</span>
        <div class="dv-ops">
          <button class="btn btn-sm" :title="$t('发布（转为正式内容，移出草稿箱）')" @click="publish(f)">{{  $t('发布')  }}</button>
          <button class="btn btn-sm btn-danger" :title="$t('删除草稿')" @click="confirmRemove = f">{{  $t('删除')  }}</button>
        </div>
      </div>
    </div>

    <!-- 应用内确认弹窗（替代系统级 confirm） -->
    <ConfirmDialog
      v-if="confirmRemove"
      :title="$t('删除草稿')"
      :message="$t('删除草稿「{v0}」？删除后可在回收站恢复。', { v0: confirmRemove.name })"
      confirm-text="删除"
      danger
      @confirm="doRemove(confirmRemove)"
      @cancel="confirmRemove = null"
    />
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import AikIcon from '@/components/AikIcon.vue'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import * as api from '@/api'
import { useToastStore } from '@/stores/toast'
import { useFilesStore } from '@/stores/files'
import { t } from '@/i18n'

const router = useRouter()
const toast = useToastStore()
const filesStore = useFilesStore()

const items = ref([])
const loading = ref(false)

async function load() {
  loading.value = true
  try {
    items.value = await api.listDrafts()
  } catch (e) {
    toast.push(t('加载草稿失败：') + (e.message || e))
  } finally {
    loading.value = false
  }
}
onMounted(load)

function parentName(f) {
  if (!f.parent_id) return t(t('我的空间'))
  const d = (filesStore.tree || []).find((x) => x.id === f.parent_id)
  return d ? d.name : '…'
}

function openFile(f) {
  router.push('/read/' + f.id)
}

async function publish(f) {
  try {
    await api.setFileStatus(f.id, 'published')
    toast.push(`已发布「${f.name}」`)
    items.value = items.value.filter((x) => x.id !== f.id)
  } catch (e) {
    toast.push(t('发布失败：') + (e.message || e))
  }
}

const confirmRemove = ref(null)

function doRemove(f) {
  confirmRemove.value = null
  api
    .deleteFile(f.id)
    .then(() => {
      toast.push(t('已删除草稿'))
      items.value = items.value.filter((x) => x.id !== f.id)
    })
    .catch((e) => toast.push(t('删除失败：') + (e.message || e)))
}

function fmtSize(n) {
  if (!n) return '—'
  if (n < 1024) return n + ' B'
  if (n < 1048576) return (n / 1024).toFixed(1) + ' KB'
  return (n / 1048576).toFixed(1) + ' MB'
}
function fmtTime(ts) {
  if (!ts) return ''
  // API 返回 Unix 秒，前端需转毫秒
  const d = new Date(Number(ts) * 1000)
  const p = (x) => String(x).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`
}
</script>

<style scoped>
.drafts-view {
  max-width: 1200px;
  margin: 0 auto;
  padding: 20px 24px 40px;
}
.dv-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  margin-bottom: 18px;
}
.dv-head h2 {
  font-size: 18px;
  font-weight: 600;
  color: var(--text);
}
.dv-sub {
  font-size: 12.5px;
  color: var(--text-3);
  margin-top: 4px;
}
.dv-list {
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: var(--surface);
  overflow: hidden;
}
.dv-row {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 11px 14px;
  border-bottom: 1px solid var(--border);
  transition: background 0.12s ease;
}
.dv-row:last-child {
  border-bottom: none;
}
.dv-row:hover {
  background: var(--surface-2);
}
.dv-ico {
  color: var(--text-3);
  flex-shrink: 0;
}
.dv-name {
  flex: 1;
  min-width: 0;
  text-align: left;
  font-size: 13.5px;
  font-weight: 500;
  color: var(--text);
  cursor: pointer;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.dv-name:hover {
  color: var(--primary);
}
.dv-meta {
  flex-shrink: 0;
  font-size: 12px;
  color: var(--text-3);
  width: 140px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.dv-meta:nth-of-type(1) {
  width: 130px;
}
.dv-meta:nth-of-type(2) {
  width: 70px;
}
.dv-meta:nth-of-type(3) {
  width: 130px;
}
.dv-ops {
  display: flex;
  gap: 6px;
  flex-shrink: 0;
}
.dv-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  padding: 60px 20px;
  color: var(--text-3);
  font-size: 13px;
  border: 1px dashed var(--border);
  border-radius: var(--radius);
}
.muted {
  font-size: 12px;
  color: var(--text-3);
  opacity: 0.8;
}
</style>
