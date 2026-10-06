<template>
  <div class="ib-view">
    <div class="ib-head">
      <div>
        <h2>{{  $t('收件箱')  }}<span v-if="unread" class="ib-dot">{{  unread  }}</span></h2>
        <p class="ib-sub">
          {{  $t('采集聚合区：第三方引擎 / 网页剪藏 / 导入落进「')  }}{{  inboxDir  }}{{  $t('」目录的文件都先到这里， 处理完点「归档」标记（只打标记，原文件仍在文件库）。')  }}
        </p>
      </div>
      <div class="ib-tools">
        <button class="btn" :disabled="loading" @click="loadAll"><AikIcon name="refresh" :size="14" />{{  $t('刷新')  }}</button>
        <button class="btn" :disabled="archivingAll || !unread" @click="doArchiveAll">
          <AikIcon name="check" :size="14" />{{  archivingAll ? $t('归档中…') : $t('全部归档')  }}
        </button>
      </div>
    </div>

    <div class="ib-bar">
      <div class="ib-tabs">
        <button class="ib-tab" :class="{ on: filter === 'unread' }" @click="setFilter('unread')">
          {{  $t('未读')  }} <span class="ib-n">{{  unread  }}</span>
        </button>
        <button class="ib-tab" :class="{ on: filter === 'all' }" @click="setFilter('all')">{{  $t('全部')  }}</button>
      </div>
      <span class="ib-count">{{  items.length  }} {{  $t('条')  }}</span>
    </div>

    <div class="ib-list">
      <div v-if="loading" class="ib-empty">{{  $t('加载中…')  }}</div>
      <div v-else-if="!items.length" class="ib-empty">
        <AikIcon name="bell" :size="28" />
        <p>{{  filter === 'unread' ? $t('没有未读内容') : $t('收件箱是空的')  }}</p>
        <p class="ib-hint">{{  $t('采集源产出、网页剪藏与 WebDAV 导入都会汇总到这里。')  }}</p>
      </div>
      <div v-for="it in items" :key="it.id" class="ib-row" :class="{ archived: it.inbox_state === 1 }">
        <span class="ib-ico"><AikIcon :name="iconFor(it.mime)" :size="16" /></span>
        <div class="ib-main">
          <div class="ib-title">
            <a class="ib-name" :title="it.name" @click.prevent="open(it)">{{  it.name  }}</a>
            <span v-if="it.inbox_state === 0" class="ib-badge st-new">{{  $t('未读')  }}</span>
            <span v-else class="ib-badge st-done">{{  $t('已归档')  }}</span>
          </div>
          <div class="ib-meta">
            <span>{{  it.mime || $t('未知类型')  }}</span>
            <span>·</span>
            <span>{{  fmtSize(it.size)  }}</span>
            <span>·</span>
            <span>{{  fmtTime(it.created_at)  }}</span>
            <span v-if="it.parent_name">· {{  it.parent_name  }}</span>
          </div>
        </div>
        <div class="ib-ops">
          <button class="btn btn-sm" @click="open(it)"><AikIcon name="eye" :size="13" />{{  $t('查看')  }}</button>
          <button
            v-if="it.inbox_state === 0"
            class="btn btn-sm"
            :disabled="archiving === it.id"
            @click="doArchive(it)"
          ><AikIcon name="check" :size="13" />{{  $t('归档')  }}</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import AikIcon from '@/components/AikIcon.vue'
import * as api from '@/api'
import { useToastStore } from '@/stores/toast'
import { t } from '@/i18n'

const toast = useToastStore()
const router = useRouter()

const items = ref([])
const unread = ref(0)
const filter = ref('unread')
const loading = ref(false)
const archiving = ref('')
const archivingAll = ref(false)
// 收件箱目录名由后端 inbox.dir 决定（默认「采集」）；前端只做文案展示。
// 用 computed 而非 const：t() 在 setup 期求值一次就固定了，切换语言不会重算，
// 会让这行文案成为全站唯一不跟随语言切换的残留。
const inboxDir = computed(() => t('采集'))

async function loadAll() {
  loading.value = true
  try {
    const [d, u] = await Promise.all([api.inboxList(filter.value, 100), api.inboxUnread()])
    items.value = d.items || []
    unread.value = u.count || 0
  } catch (e) {
    toast.push(t('加载收件箱失败：') + (e.message || e))
  } finally {
    loading.value = false
  }
}

async function setFilter(f) {
  if (filter.value === f) return
  filter.value = f
  await loadAll()
}

async function doArchive(it) {
  archiving.value = it.id
  try {
    await api.inboxArchive(it.id)
    toast.push(t('已归档'))
    await loadAll()
  } catch (e) {
    toast.push(t('归档失败：') + (e.message || e))
  } finally {
    archiving.value = ''
  }
}

async function doArchiveAll() {
  archivingAll.value = true
  try {
    const d = await api.inboxArchiveAll()
    toast.push(`已归档 ${d.archived || 0} 条`)
    await loadAll()
  } catch (e) {
    toast.push(t('全部归档失败：') + (e.message || e))
  } finally {
    archivingAll.value = false
  }
}

function open(it) {
  router.push(`/read/${it.id}`)
}

function iconFor(mime) {
  const m = (mime || '').toLowerCase()
  if (m.startsWith('image/')) return 'fileImage'
  if (m.startsWith('audio/')) return 'fileAudio'
  if (m.startsWith('video/')) return 'fileVideo'
  if (m.includes('pdf')) return 'filePdf'
  if (m.includes('zip') || m.includes('compressed')) return 'fileArchive'
  if (m.includes('json') || m.includes('javascript') || m.includes('xml')) return 'fileCode'
  if (m.startsWith('text/')) return 'fileText'
  return 'file'
}

function fmtSize(n) {
  if (!n) return '0 B'
  if (n < 1024) return n + ' B'
  if (n < 1024 * 1024) return (n / 1024).toFixed(1) + ' KB'
  return (n / 1024 / 1024).toFixed(1) + ' MB'
}

function fmtTime(ts) {
  if (!ts) return '—'
  const d = new Date(ts < 1e12 ? ts * 1000 : ts)
  const p = (x) => String(x).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`
}

onMounted(loadAll)
</script>

<style scoped>
.ib-view { max-width: 1200px; margin: 0 auto; padding: 20px 24px 40px; }
.ib-head { display: flex; justify-content: space-between; align-items: flex-start; gap: 12px; margin-bottom: 14px; }
.ib-head h2 { font-size: 20px; margin: 0 0 4px; display: inline-flex; align-items: center; gap: 8px; }
.ib-dot { font-size: 12px; font-weight: 600; padding: 1px 8px; border-radius: 10px; background: rgba(224,62,62,.12); color: #e03e3e; }
.ib-sub { color: var(--text-3, #8a919f); font-size: 13px; margin: 0; max-width: 720px; line-height: 1.6; }
.ib-tools { display: inline-flex; gap: 8px; flex-shrink: 0; }

.ib-bar { display: flex; align-items: center; justify-content: space-between; margin-bottom: 10px; }
.ib-tabs { display: inline-flex; gap: 4px; padding: 3px; border-radius: 9px; background: var(--surface-2, #f7f8fa); }
.ib-tab { border: 0; background: transparent; padding: 6px 14px; border-radius: 7px; font-size: 13px; color: var(--text-2, #4a5164); cursor: pointer; }
.ib-tab.on { background: var(--bg-1, #fff); color: var(--primary, #2f6bff); font-weight: 600; box-shadow: 0 1px 3px rgba(22,24,43,.08); }
.ib-n { font-size: 11px; color: var(--text-3, #8a919f); }
.ib-count { font-size: 12.5px; color: var(--text-3, #8a919f); }

.ib-list { display: flex; flex-direction: column; gap: 6px; }
.ib-row { display: flex; align-items: center; gap: 12px; padding: 11px 14px; border: 1px solid var(--border, #e3e6ee); border-radius: 10px; background: var(--bg-2, #fff); transition: border-color .14s; }
.ib-row:hover { border-color: var(--primary, #2f6bff); }
.ib-row.archived { opacity: .62; }
.ib-ico { flex-shrink: 0; color: var(--text-3, #8a919f); display: inline-flex; }
.ib-main { flex: 1; min-width: 0; }
.ib-title { display: flex; align-items: center; gap: 8px; }
.ib-name { font-size: 14px; color: var(--text, #20242c); text-decoration: none; cursor: pointer; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.ib-name:hover { color: var(--primary, #2f6bff); }
.ib-badge { font-size: 11px; padding: 1px 7px; border-radius: 8px; flex-shrink: 0; }
.ib-badge.st-new { background: rgba(224,62,62,.12); color: #e03e3e; }
.ib-badge.st-done { background: var(--bg-3, #f0f1f4); color: var(--text-3, #8a919f); }
.ib-meta { display: flex; gap: 6px; flex-wrap: wrap; font-size: 12px; color: var(--text-3, #8a919f); margin-top: 4px; }
.ib-ops { display: inline-flex; gap: 6px; flex-shrink: 0; }

.ib-empty { display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 6px; color: var(--text-3, #8a919f); padding: 56px 0; font-size: 13px; border: 1px dashed var(--border, #e3e6ee); border-radius: 10px; }
.ib-empty p { margin: 0; }
.ib-hint { font-size: 12px; color: var(--text-3, #a0a6b2); }
</style>
