<template>
  <div class="trash-view">
    <div class="tv-head">
      <div>
        <h2>{{  $t('回收站')  }}</h2>
        <p class="tv-sub">{{  $t('已删除的内容暂存于此；恢复回到原位置，彻底删除不可恢复')  }}</p>
      </div>
      <div class="tv-tools">
        <button class="btn" @click="load" :disabled="loading">
          <AikIcon name="refresh" :size="14" />{{  $t('刷新')  }}
        </button>
        <button v-if="items.length" class="btn btn-danger" @click="confirmEmpty = true">{{  $t('清空回收站')  }}</button>
      </div>
    </div>

    <div v-if="loading" class="tv-empty">{{  $t('加载中…')  }}</div>

    <div v-else-if="!items.length" class="tv-empty">
      <AikIcon name="trash" :size="30" />
      <p>{{  $t('回收站是空的')  }}</p>
      <p class="muted">{{  $t('删除的文件会先到这里，可以恢复；彻底删除后无法找回')  }}</p>
    </div>

    <div v-else class="tv-list">
      <div v-for="f in items" :key="f.id" class="tv-row">
        <AikIcon :name="f.kind === 'dir' ? 'folder' : 'file'" :size="16" class="tv-ico" />
        <button class="tv-name" :title="f.name" @click="openFile(f)">{{  f.name  }}</button>
        <span class="tv-meta tv-loc">{{  parentName(f)  }}</span>
        <span class="tv-meta">{{  f.kind === 'dir' ? $t('文件夹') : fmtSize(f.size)  }}</span>
        <span class="tv-meta">{{  fmtTime(f.deleted_at)  }}</span>
        <div class="tv-ops">
          <button class="btn btn-sm" :title="$t('恢复到原位置（同名自动改名）')" @click="restore(f)">{{  $t('恢复')  }}</button>
          <button class="btn btn-sm btn-danger" :title="$t('彻底删除，不可恢复')" @click="confirmPurge = f">{{  $t('彻底删除')  }}</button>
        </div>
      </div>
    </div>

    <!-- 应用内确认弹窗（替代系统级 confirm） -->
    <ConfirmDialog
      v-if="confirmPurge"
      :title="$t('彻底删除')"
      :message="$t('彻底删除「{v0}」？此操作不可恢复。', { v0: confirmPurge.name })"
      confirm-text="彻底删除"
      danger
      @confirm="doPurge(confirmPurge)"
      @cancel="confirmPurge = null"
    />
    <ConfirmDialog
      v-if="confirmEmpty"
      :title="$t('清空回收站')"
      :message="$t('清空回收站（{v0} 项）？全部彻底删除，不可恢复。', { v0: items.length })"
      confirm-text="清空"
      danger
      @confirm="doEmptyAll"
      @cancel="confirmEmpty = false"
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
    items.value = await api.listTrash()
  } catch (e) {
    toast.push(t('加载回收站失败：') + (e.message || e))
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
  // 目录不可打开（已删除）；文件可直接阅读
  if (f.kind !== 'dir') router.push('/read/' + f.id)
}

async function restore(f) {
  try {
    const nf = await api.restoreFile(f.id)
    toast.push(`已恢复「${nf.name}」`)
    items.value = items.value.filter((x) => x.id !== f.id)
  } catch (e) {
    toast.push(t('恢复失败：') + (e.message || e))
  }
}

const confirmPurge = ref(null)
const confirmEmpty = ref(false)

function doPurge(f) {
  confirmPurge.value = null
  api
    .purgeFile(f.id)
    .then(() => {
      toast.push(t('已彻底删除'))
      items.value = items.value.filter((x) => x.id !== f.id)
    })
    .catch((e) => toast.push(t('彻底删除失败：') + (e.message || e)))
}

function doEmptyAll() {
  confirmEmpty.value = false
  api
    .emptyTrash()
    .then((d) => {
      toast.push(`已清空 ${d.purged} 项`)
      items.value = []
    })
    .catch((e) => toast.push(t('清空失败：') + (e.message || e)))
}

function fmtSize(n) {
  if (!n) return '—'
  if (n < 1024) return n + ' B'
  if (n < 1048576) return (n / 1024).toFixed(1) + ' KB'
  return (n / 1048576).toFixed(1) + ' MB'
}
function fmtTime(ts) {
  if (!ts) return ''
  const d = new Date(Number(ts) * 1000)
  const p = (x) => String(x).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`
}
</script>

<style scoped>
.trash-view {
  max-width: 1200px;
  margin: 0 auto;
  padding: 20px 24px 40px;
}
.tv-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  margin-bottom: 18px;
}
.tv-head h2 {
  font-size: 18px;
  font-weight: 600;
  color: var(--text);
}
.tv-sub {
  font-size: 12.5px;
  color: var(--text-3);
  margin-top: 4px;
}
.tv-tools {
  display: flex;
  gap: 8px;
}
.tv-list {
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: var(--surface);
  overflow: hidden;
}
.tv-row {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 11px 14px;
  border-bottom: 1px solid var(--border);
  transition: background 0.12s ease;
}
.tv-row:last-child {
  border-bottom: none;
}
.tv-row:hover {
  background: var(--surface-2);
}
.tv-ico {
  color: var(--text-3);
  flex-shrink: 0;
}
.tv-name {
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
.tv-name:hover {
  color: var(--primary);
}
.tv-meta {
  flex-shrink: 0;
  font-size: 12px;
  color: var(--text-3);
  width: 120px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.tv-meta.tv-loc {
  width: 140px;
}
.tv-ops {
  display: flex;
  gap: 6px;
  flex-shrink: 0;
}
.tv-empty {
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
