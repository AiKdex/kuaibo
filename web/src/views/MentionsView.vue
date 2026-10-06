<template>
  <div class="mt-view">
    <div class="mt-head">
      <div>
        <h2>{{  $t('@我')  }}<span v-if="unread" class="mt-dot">{{  unread  }}</span></h2>
        <p class="mt-sub">
          {{  $t('评论或文章里 @ 到你的时候会落在这里。写')  }} <code>{{  $t('@[显示名](用户id)')  }}</code> {{  $t('是精确写法， 直接写')  }} <code>{{  $t('@用户名')  }}</code> {{  $t('也能识别（按用户名/显示名反查）。')  }}
        </p>
      </div>
      <div class="mt-tools">
        <button class="btn" :disabled="loading" @click="load"><AikIcon name="refresh" :size="14" />{{  $t('刷新')  }}</button>
        <button class="btn" :disabled="!unread" @click="readAll"><AikIcon name="check" :size="14" />{{  $t('全部已读')  }}</button>
      </div>
    </div>

    <div class="mt-bar">
      <div class="mt-tabs">
        <button class="mt-tab" :class="{ on: filter === 'unread' }" @click="filter = 'unread'">
          {{  $t('未读')  }} <span class="mt-n">{{  unread  }}</span>
        </button>
        <button class="mt-tab" :class="{ on: filter === 'all' }" @click="filter = 'all'">{{  $t('全部')  }}</button>
      </div>
      <span class="mt-count">{{  shown.length  }} {{  $t('条')  }}</span>
    </div>

    <div class="mt-list">
      <div v-if="loading" class="mt-empty">{{  $t('加载中…')  }}</div>
      <div v-else-if="!shown.length" class="mt-empty">
        <AikIcon name="user" :size="28" />
        <p>{{  filter === 'unread' ? $t('没有未读的提及') : $t('还没有人提到你')  }}</p>
        <p class="mt-hint">{{  $t('把文章分享给别人，或在评论区互相 @ 一下试试。')  }}</p>
      </div>
      <div v-for="it in shown" :key="it.id" class="mt-row" :class="{ done: it.read_at }">
        <span class="mt-ico"><AikIcon name="user" :size="16" /></span>
        <div class="mt-main">
          <div class="mt-title">
            <b>{{  it.from_name || $t('有人')  }}</b>
            <span>{{  it.source_type === 'comment' ? $t('在评论里') : $t('在文章里')  }}{{  $t('提到了你')  }}</span>
            <span v-if="!it.read_at" class="mt-badge">{{  $t('未读')  }}</span>
          </div>
          <div class="mt-meta">
            <span v-if="it.source_name">《{{  it.source_name  }}》</span>
            <span v-if="it.source_name">·</span>
            <span>{{  fmtTime(it.created_at)  }}</span>
          </div>
        </div>
        <div class="mt-ops">
          <button v-if="it.link" class="btn btn-sm" @click="open(it)"><AikIcon name="eye" :size="13" />{{  $t('查看')  }}</button>
          <span v-else class="mt-gone">{{  $t('源内容已删除')  }}</span>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import AikIcon from '@/components/AikIcon.vue'
import { listMentions, mentionsReadAll } from '@/api'
import { t } from '@/i18n'

const router = useRouter()
const items = ref([])
const unread = ref(0)
const loading = ref(false)
const filter = ref('unread')

const shown = computed(() => (filter.value === 'unread' ? items.value.filter((i) => !i.read_at) : items.value))

async function load() {
  loading.value = true
  try {
    const d = await listMentions(100)
    items.value = d.items || []
    unread.value = d.unread_count || 0
  } catch (_) {
    items.value = []
    unread.value = 0
  } finally {
    loading.value = false
  }
}

async function readAll() {
  try {
    await mentionsReadAll()
    items.value.forEach((i) => { i.read_at = i.read_at || Math.floor(Date.now() / 1000) })
    unread.value = 0
  } catch (_) { /* ignore */ }
}

function open(it) {
  // link 形如 /read/<fileId>#comment-<cid>；走 SPA 路由，锚点交给阅读页滚动定位
  const [path, hash] = String(it.link || '').split('#')
  if (!path) return
  router.push(hash ? { path, hash: '#' + hash } : path)
}

function fmtTime(ts) {
  if (!ts) return ''
  const d = new Date(ts * 1000)
  const diff = Math.floor((Date.now() - d.getTime()) / 60000)
  if (diff < 1) return t('刚刚')
  if (diff < 60) return diff + t(' 分钟前')
  if (diff < 1440) return Math.floor(diff / 60) + t(' 小时前')
  if (diff < 43200) return Math.floor(diff / 1440) + t(' 天前')
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}

onMounted(load)
</script>

<style scoped>
.mt-view { max-width: 1200px; margin: 0 auto; padding: 20px 24px 40px; }
.mt-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; }
.mt-head h2 { margin: 0 0 6px; font-size: 18px; display: flex; align-items: center; gap: 8px; }
.mt-dot { background: var(--danger, #d9534f); color: #fff; border-radius: 10px; padding: 1px 7px; font-size: 12px; font-weight: 600; }
.mt-sub { margin: 0; color: var(--text-muted, #888); font-size: 13px; line-height: 1.7; max-width: 640px; }
.mt-sub code { background: var(--bg-soft, #f3f3f3); padding: 1px 5px; border-radius: 4px; font-size: 12px; }
.mt-tools { display: flex; gap: 8px; flex-shrink: 0; }

.mt-bar { display: flex; align-items: center; justify-content: space-between; margin: 18px 0 10px; }
.mt-tabs { display: flex; gap: 4px; }
.mt-tab { border: 1px solid var(--border, #e2e2e2); background: transparent; border-radius: 6px; padding: 5px 12px; font-size: 13px; cursor: pointer; color: var(--text-muted, #888); }
.mt-tab.on { background: var(--accent-soft, #eef4ff); border-color: var(--accent, #3b6ef5); color: var(--accent, #3b6ef5); }
.mt-n { font-weight: 600; }
.mt-count { font-size: 12px; color: var(--text-muted, #888); }

.mt-list { display: flex; flex-direction: column; gap: 6px; }
.mt-empty { text-align: center; color: var(--text-muted, #888); padding: 48px 0; font-size: 13px; }
.mt-empty p { margin: 8px 0 0; }
.mt-hint { font-size: 12px; opacity: .8; }
.mt-row { display: flex; gap: 12px; align-items: center; padding: 12px 14px; border: 1px solid var(--border, #ececec); border-radius: 8px; background: var(--bg-panel, #fff); }
.mt-row.done { opacity: .62; }
.mt-ico { width: 28px; height: 28px; display: grid; place-items: center; border-radius: 6px; background: var(--bg-soft, #f5f7fb); color: var(--accent, #3b6ef5); flex-shrink: 0; }
.mt-main { flex: 1; min-width: 0; }
.mt-title { font-size: 13.5px; display: flex; align-items: center; gap: 6px; flex-wrap: wrap; }
.mt-badge { background: var(--accent-soft, #eef4ff); color: var(--accent, #3b6ef5); border-radius: 4px; padding: 0 6px; font-size: 11px; line-height: 17px; }
.mt-meta { margin-top: 3px; font-size: 12px; color: var(--text-muted, #999); display: flex; gap: 6px; }
.mt-ops { flex-shrink: 0; }
.mt-gone { font-size: 12px; color: var(--text-muted, #aaa); }
</style>
