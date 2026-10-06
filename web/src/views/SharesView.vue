<template>
  <div class="shares-view">
    <!-- 顶栏 -->
    <div class="sv-topbar">
      <h2 class="sv-title">{{  $t('博客 / 分享')  }}</h2>
      <div class="sv-actions">
        <button class="btn btn-sm" :title="$t('在浏览器中打开对外博客首页')" @click="openBlogHome">
          <AikIcon name="link" :size="13" /><span>{{  $t('对外首页')  }}</span>
        </button>
        <button class="icon-btn" :title="$t('刷新')" @click="load"><AikIcon name="refresh" :size="15" /></button>
      </div>
    </div>

    <!-- 视图切换：博客首页 / 管理 -->
    <div class="sv-tabs">
      <button class="tab" :class="{ active: view === 'home' }" @click="view = 'home'">
        <AikIcon name="book" :size="14" /><span>{{  $t('博客首页')  }}</span>
      </button>
      <button class="tab" :class="{ active: view === 'manage' }" @click="view = 'manage'">
        <AikIcon name="grid" :size="14" /><span>{{  $t('管理')  }}</span>
      </button>
      <span v-if="view === 'home' && activeItems.length" class="tab-count">{{  activeItems.length  }} {{  $t('篇已发布')  }}</span>
    </div>

    <!-- 错误/加载 -->
    <div v-if="error" class="error-banner">
      <AikIcon name="info" :size="14" />
      <span>{{  error  }}</span>
      <button class="btn-link" @click="load">{{  $t('重试')  }}</button>
    </div>
    <div v-if="loading" class="loading-state"><div class="spinner"></div><span>{{  $t('加载中…')  }}</span></div>

    <!-- ============ 博客首页（阅读视角） ============ -->
    <template v-else-if="view === 'home'">
      <div v-if="!activeItems.length" class="empty-state">
        <div class="empty-icon"><AikIcon name="book" :size="28" /></div>
        <p class="empty-title">{{  $t('博客还没有文章')  }}</p>
        <p class="empty-desc">{{  $t('在文件列表中右键 → 「分享 / 发布」，即可把文档发布为博客文章')  }}</p>
      </div>
      <div v-else class="blog-list">
        <article
          v-for="it in activePosts"
          :key="it.token + '/' + (it.path || '')"
          class="blog-post"
          @click="openPage(it)"
        >
          <div class="bp-icon">
            <AikIcon :name="kindIcon(it)" :size="20" />
          </div>
          <div class="bp-body">
            <h3 class="bp-title" :title="it.file?.name">{{  it.file?.name || $t('（文件已删除）')  }}
              <span v-if="it.scope === 'dir'" class="dir-tag" :title="$t('来自文件夹整体分享')">{{  $t('文件夹')  }}</span>
            </h3>
            <div class="bp-meta">
              <span>{{  formatTime(it.created_at)  }}</span>
              <span class="dot">·</span>
              <span>{{  it.file?.size ? formatSize(it.file.size) : '—'  }}</span>
              <span class="dot">·</span>
              <span>{{  typeLabel(it.file)  }}</span>
              <template v-if="it.path">
                <span class="dot">·</span>
                <span class="bp-path" :title="it.path">{{  it.path  }}</span>
              </template>
            </div>
          </div>
          <span class="bp-read">{{ $t('阅读') }}<AikIcon name="chevronRight" :size="12" /></span>
        </article>
      </div>
    </template>

    <!-- ============ 管理（表格视角） ============ -->
    <template v-else>
      <div v-if="!items.length" class="empty-state">
        <div class="empty-icon"><AikIcon name="fileText" :size="28" /></div>
        <p class="empty-title">{{ $t('还没有发布任何内容') }}</p>
        <p class="empty-desc">{{ $t('在文件列表中右键 → 「分享 / 发布」，即可把文档作为博客文章发布') }}</p>
      </div>
      <div v-else class="sv-list">
        <div class="sv-head">
          <span class="col-name">{{ $t('文档') }}</span>
          <span class="col-meta">{{ $t('类型') }}</span>
          <span class="col-meta">{{ $t('大小') }}</span>
          <span class="col-meta">{{ $t('发布时间') }}</span>
          <span class="col-status">{{ $t('状态') }}</span>
          <span class="col-actions">{{ $t('操作') }}</span>
        </div>
        <div v-for="it in items" :key="it.token" class="sv-row" :class="'st-' + it.status">
          <span class="col-name">
            <AikIcon :name="kindIcon(it)" :size="16" />
            <span class="sv-file-name" :title="it.file?.name || t('（文件已删除）')">{{  it.file?.name || $t('（文件已删除）')  }}</span>
          </span>
          <span class="col-meta">{{  it.scope === 'dir' ? $t('文件夹·整体') : typeLabel(it.file)  }}</span>
          <span class="col-meta">{{  it.file?.size ? formatSize(it.file.size) : '—'  }}</span>
          <span class="col-meta">{{  formatTime(it.created_at)  }}</span>
          <span class="col-status">
            <span class="st-badge" :class="'st-' + it.status">{{  statusLabel(it.status)  }}</span>
          </span>
          <span class="col-actions">
            <template v-if="it.status === 'active'">
              <button class="btn btn-sm" @click="copyUrl(it)">{{ $t('复制链接') }}</button>
              <button class="btn btn-sm" @click="openPage(it)">{{ $t('预览') }}</button>
              <template v-if="it.token === 'blog'">
                <span class="st-locked" :title="$t('博客分享为系统内置：关闭/开启博客请到博客管理页使用博客开关')">{{ $t('内置') }}</span>
              </template>
              <template v-else>
                <button class="btn btn-sm btn-danger-ghost" @click="revoke(it)">{{ $t('撤销') }}</button>
              </template>
            </template>
            <template v-else>
              <span class="st-gone">{{  it.status === 'revoked' ? $t('已撤销') : it.status === 'expired' ? $t('已过期') : $t('文件缺失')  }}</span>
            </template>
          </span>
        </div>
      </div>
    </template>

    <!-- 提示条 -->
    <p class="sv-hint">{{ $t('发布后任何人凭链接即可阅读；撤销后链接立即失效。限时分享、访问统计将随阶段开放。') }}</p>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import AikIcon from '@/components/AikIcon.vue'
import * as api from '@/api'
import { useToastStore } from '@/stores/toast'
import { tryCopy } from '@/utils/clipboard'
import { t } from '@/i18n'
const toastStore = useToastStore()

const view = ref('home')
const items = ref([])
const posts = ref([])
const loading = ref(true)
const error = ref('')

const activeItems = computed(() => items.value.filter((it) => it.status === 'active'))
const activePosts = computed(() => posts.value.filter((it) => it.scope !== 'dir' || it.file))

async function load() {
  loading.value = true
  error.value = ''
  try {
    const [si, pp] = await Promise.all([api.sharesList(), api.publicPosts().catch(() => null)])
    items.value = si
    posts.value = (pp && pp.items) || si.filter((it) => it.status === 'active').map((it) => ({ ...it, scope: it.scope || 'file' }))
  } catch (e) {
    error.value = e.message
  } finally {
    loading.value = false
  }
}

function shareUrl(token) {
  return `${location.protocol}//${location.host}${location.pathname}#/p/${token}`
}

function openBlogHome() {
  window.open(`${location.protocol}//${location.host}${location.pathname}#/blog`, '_blank')
}

async function copyUrl(it) {
  const url = shareUrl(it.token)
  if (await tryCopy(url)) {
    toastStore.push(t('链接已复制'))
  } else {
    toastStore.push(t('浏览器限制了自动复制，请手动复制链接'))
  }
}

function openPage(it) {
  // 目录分享展开的文件：公开页直接定位到该文件
  window.open(it.path ? shareUrl(it.token) + '?path=' + encodeURIComponent(it.path) : shareUrl(it.token), '_blank')
}

async function revoke(it) {
  try {
    await api.revokeShare(it.token)
    toastStore.push(t('已撤销，链接已失效'))
    await load()
  } catch (e) {
    error.value = e.message
  }
}

function kindIcon(it) {
  const f = it.file
  if (!f) return 'file'
  if (f.kind === 'dir') return 'folder'
  const e = (f.name || '').split('.').pop().toLowerCase()
  if (['png', 'jpg', 'jpeg', 'gif', 'webp', 'svg', 'bmp'].includes(e)) return 'image'
  if (['mp4', 'mov', 'mkv', 'webm'].includes(e)) return 'video'
  if (['mp3', 'wav', 'm4a', 'flac', 'ogg'].includes(e)) return 'audio'
  if (['md', 'markdown', 'txt', 'log', 'html', 'json'].includes(e)) return 'fileText'
  return 'file'
}

function typeLabel(f) {
  if (!f) return '—'
  if (f.kind === 'dir') return t('文件夹')
  const e = (f.name || '').split('.').pop().toLowerCase()
  return (e || t('文件')).toUpperCase()
}

function statusLabel(s) {
  return { active: t('已发布'), revoked: t('已撤销'), expired: t('已过期'), missing: t('文件缺失') }[s] || s
}

function formatSize(n) {
  if (!n) return '—'
  if (n < 1024) return n + ' B'
  if (n < 1024 * 1024) return (n / 1024).toFixed(1) + ' KB'
  return (n / 1024 / 1024).toFixed(1) + ' MB'
}

// shares 表时间戳为 Unix 毫秒
function formatTime(ms) {
  if (!ms) return '—'
  const d = new Date(ms)
  const p = (n) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`
}

onMounted(load)
</script>

<style scoped>
.shares-view {
  max-width: 1200px;
  margin: 0 auto;
  padding: 20px 24px 40px;
}

.sv-topbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 14px;
}

.sv-title {
  font-size: 17px;
  font-weight: 600;
  margin: 0;
}

/* 视图切换 */
.sv-tabs {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 16px;
}

.sv-tabs .tab {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 6px 14px;
  border: 1px solid var(--border);
  border-radius: 999px;
  background: var(--surface);
  color: var(--text-2);
  font-size: 13px;
  cursor: pointer;
  transition: all 0.15s ease;
}

.sv-tabs .tab:hover {
  border-color: var(--primary);
  color: var(--primary);
}

.sv-tabs .tab.active {
  background: var(--primary-soft);
  border-color: var(--primary);
  color: var(--primary);
  font-weight: 600;
}

.tab-count {
  margin-left: 10px;
  font-size: 12px;
  color: var(--text-3);
}

/* ===== 博客首页 ===== */
.blog-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.blog-post {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 14px 18px;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: var(--surface);
  cursor: pointer;
  transition: all 0.16s ease;
}

.blog-post:hover {
  border-color: var(--primary);
  box-shadow: var(--shadow-sm, 0 2px 10px rgba(0, 0, 0, 0.06));
  transform: translateY(-1px);
}

.bp-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 40px;
  height: 40px;
  border-radius: 10px;
  background: var(--primary-soft);
  color: var(--primary);
  flex-shrink: 0;
}

.bp-body {
  flex: 1;
  min-width: 0;
}

.bp-title {
  margin: 0 0 4px;
  font-size: 15px;
  font-weight: 600;
  color: var(--text);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.dir-tag {
  display: inline-block;
  margin-left: 6px;
  padding: 0 6px;
  border-radius: 4px;
  font-size: 11px;
  font-weight: 500;
  color: var(--warn);
  border: 1px solid color-mix(in srgb, var(--warn) 50%, transparent);
  background: color-mix(in srgb, var(--warn) 12%, transparent);
  vertical-align: 1px;
}

.bp-path {
  color: var(--text-3);
  font-size: 12px;
  max-width: 220px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.bp-meta {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--text-3);
}

.bp-meta .dot {
  opacity: 0.6;
}

.bp-read {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  font-size: 13px;
  color: var(--text-3);
  flex-shrink: 0;
  transition: color 0.15s ease;
}

.blog-post:hover .bp-read {
  color: var(--primary);
}

/* ===== 管理表格 ===== */
.sv-list {
  border: 1px solid var(--border);
  border-radius: var(--radius);
  overflow: hidden;
}

.sv-head,
.sv-row {
  display: grid;
  grid-template-columns: minmax(200px, 1fr) 90px 80px 150px 90px 220px;
  align-items: center;
  gap: 12px;
  padding: 0 16px;
}

.sv-head {
  background: var(--surface-2);
  font-size: 12px;
  color: var(--text-3);
  padding-top: 9px;
  padding-bottom: 9px;
  border-bottom: 1px solid var(--border);
}

.sv-row {
  padding-top: 10px;
  padding-bottom: 10px;
  border-bottom: 1px solid var(--border);
  font-size: 13px;
}

.sv-row:last-child {
  border-bottom: none;
}

.sv-row:hover {
  background: var(--surface-2);
}

.col-name {
  display: flex;
  align-items: center;
  gap: 8px;
  color: var(--text);
}

.sv-file-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.col-meta {
  color: var(--text-3);
  font-variant-numeric: tabular-nums;
}

.st-badge {
  display: inline-block;
  padding: 2px 8px;
  border-radius: 999px;
  font-size: 12px;
}

.st-badge.st-active {
  background: var(--success-soft, rgba(22, 163, 74, 0.12));
  color: var(--success, #16a34a);
}

.st-badge.st-revoked,
.st-badge.st-expired {
  background: var(--surface-2);
  color: var(--text-3);
}

.st-badge.st-missing {
  background: var(--danger-soft);
  color: var(--danger);
}

.st-gone {
  font-size: 12px;
  color: var(--text-3);
}

.st-locked {
  font-size: 12px;
  color: var(--warn, #f59e0b);
  border: 1px solid color-mix(in srgb, var(--warn, #f59e0b) 45%, transparent);
  border-radius: 4px;
  padding: 2px 8px;
  cursor: default;
}

.col-actions {
  display: flex;
  gap: 6px;
}

.btn-danger-ghost {
  color: var(--danger);
  border-color: var(--danger);
  background: transparent;
}

.btn-danger-ghost:hover {
  background: var(--danger-soft, rgba(220, 38, 38, 0.1));
}

.sv-hint {
  margin-top: 16px;
  font-size: 12px;
  color: var(--text-3);
}

.error-banner {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 14px;
  margin-bottom: 12px;
  border-radius: var(--radius-sm);
  background: var(--danger-soft);
  color: var(--danger);
  font-size: 13px;
}

.btn-link {
  color: var(--danger);
  text-decoration: underline;
  margin-left: 4px;
}

.loading-state,
.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
  padding: 90px 20px;
  color: var(--text-3);
  font-size: 14px;
}

.empty-icon {
  opacity: 0.5;
}

.empty-title {
  font-size: 15px;
  font-weight: 600;
  color: var(--text-2);
  margin: 0;
}

.empty-desc {
  margin: 0;
  font-size: 13px;
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

@media (max-width: 900px) {
  .sv-head,
  .sv-row {
    grid-template-columns: minmax(160px, 1fr) 80px 130px 180px;
  }
  .col-meta:nth-of-type(2) {
    display: none;
  }
}
</style>
