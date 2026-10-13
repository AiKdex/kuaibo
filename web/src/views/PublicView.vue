<template>
  <div class="pub-view">
    <!-- 博客插件挂载点：head（SEO meta 注入，隐藏容器） -->
    <BlogPluginSlot mount="head" :ctx="postCtx" />

    <!-- 极简顶栏：品牌 + 入口 -->
    <header class="pub-bar">
      <a class="pub-brand" href="/app#/blog?view=public" title="爱库录公开博客">
        <span class="pub-logo">录</span>
        <span class="pub-name">爱库录</span>
        <span class="pub-sub">AiKlog</span>
      </a>
      <GuestEntry variant="bar" />
      <a class="pub-home btn btn-sm" href="/app#/blog?view=public">返回列表</a>
    </header>

    <!-- 加载/错误 -->
    <div v-if="loading" class="pub-center"><div class="spinner"></div><span>加载中…</span></div>
    <div v-else-if="error" class="pub-center">
      <AikIcon name="info" :size="20" />
      <p>{{ error }}</p>
    </div>

    <!-- 正文：文本类 → 博客文章；媒体类 → 播放器/预览 -->
    <template v-else-if="!dirList">
      <div class="pub-shell">
        <MarkdownArticle
          v-if="isText"
          :title="file?.name || ''"
          :html="html"
          :meta="meta"
          footer="本文由爱库录发布 · 目录即站点，文件即文章"
        />
        <!-- 正文下方：侧栏类插件（信息卡）再挂 post_bottom（相关推荐/评论/问答/统计） -->
        <BlogPluginSlot v-if="isText" mount="sidebar" :ctx="postCtx" class="pub-plugins-side" />
        <BlogPluginSlot v-if="isText" mount="post_bottom" :ctx="postCtx" class="pub-plugins-bottom" />

        <!-- 图片 -->
        <div v-else-if="isImage" class="pub-media">
          <img :src="contentUrl" :alt="file?.name" class="pub-img" />
          <p class="pub-media-meta">{{ file?.name }}</p>
        </div>

        <!-- 视频 -->
        <div v-else-if="isVideo" class="pub-media">
          <video :src="contentUrl" controls class="pub-video"></video>
          <p class="pub-media-meta">{{ file?.name }}</p>
        </div>

        <!-- 音频 -->
        <div v-else-if="isAudio" class="pub-media">
          <audio :src="contentUrl" controls class="pub-audio"></audio>
          <p class="pub-media-meta">{{ file?.name }}</p>
        </div>

        <!-- PDF -->
        <div v-else-if="isPdf" class="pub-media">
          <iframe :src="contentUrl" class="pub-pdf" title="PDF 预览"></iframe>
        </div>

        <!-- 其他：下载 -->
        <div v-else class="pub-center">
          <AikIcon name="file" :size="40" />
          <p>{{ file?.name }}</p>
          <a class="btn btn-primary" :href="contentUrl + '&download=1'" download>下载文件</a>
        </div>
      </div>
    </template>

    <!-- 目录整体分享：公开目录文件列表 -->
    <div v-else class="pub-dir">
      <div class="pub-dir-head">
        <AikIcon name="folder" :size="22" />
        <h1 class="pub-dir-title">{{ dirData?.dir?.name || '公开目录' }}</h1>
        <span class="pub-dir-count">{{ dirData?.files?.length || 0 }} 个文件</span>
      </div>
      <p class="pub-dir-desc">此目录由发布者整体公开（文件夹分享）。目录内新增文件会自动出现在这里。</p>
      <div v-if="!dirData?.files?.length" class="pub-center">
        <AikIcon name="folder" :size="40" />
        <p>目录内还没有可公开的文件</p>
      </div>
      <div v-else class="pub-dir-list">
        <a
          v-for="f in dirData.files"
          :key="f.path"
          class="pub-dir-row"
          :href="`#/p/${route.params.token}?path=${encodeURIComponent(f.path)}`"
        >
          <AikIcon :name="fileIcon(f)" :size="16" />
          <span class="pub-dir-fname" :title="f.path">{{ f.path }}</span>
          <span class="pub-dir-fsize">{{ f.size ? formatSize(f.size) : '—' }}</span>
        </a>
      </div>
    </div>

    <!-- 页脚 -->
    <footer class="pub-foot">
      <span>爱库录 AiKlog · 数据主权在发布者 · <a href="/app#/blog?view=public">交互版</a> · <a href="/blog">静态页</a></span>
    </footer>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, watch } from 'vue'
import { useRoute } from 'vue-router'
import AikIcon from '@/components/AikIcon.vue'
import MarkdownArticle from '@/components/MarkdownArticle.vue'
import { getShare, fetchShareContent, shareContentUrl, fetchDirShareContent, dirShareContentUrl } from '@/api'
import BlogPluginSlot from '@/components/BlogPluginSlot.vue'
import GuestEntry from '@/components/GuestEntry.vue'
import '@/blogPlugins/builtin' // 注册内置博客插件
import { loadEnabledPlugins } from '@/blogPlugins'
import { renderMarkdown } from '@/utils/markdown'

const route = useRoute()
const file = ref(null)
const html = ref('')
const loading = ref(true)
const error = ref('')
const dirData = ref(null)   // 目录分享：{ dir, files }
const dirList = ref(false)  // 目录列表视图（无 ?path=）

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

const contentUrl = computed(() => {
  if (!file.value) return ''
  if (dirData.value && route.query.path) return dirShareContentUrl(route.params.token, route.query.path)
  return shareContentUrl(route.params.token)
})

function fileIcon(f) {
  const e = (f.name || '').split('.').pop().toLowerCase()
  if (['png', 'jpg', 'jpeg', 'gif', 'webp', 'svg', 'bmp', 'avif'].includes(e)) return 'image'
  if (['mp4', 'mov', 'mkv', 'webm', 'avi'].includes(e)) return 'video'
  if (['mp3', 'wav', 'm4a', 'flac', 'ogg'].includes(e)) return 'audio'
  if (['md', 'markdown', 'txt', 'log', 'json', 'html', 'htm', 'css', 'js', 'ts', 'py', 'go', 'vue', 'sql', 'sh'].includes(e)) return 'fileText'
  return 'file'
}

function formatSize(n) {
  if (n < 1024) return n + ' B'
  if (n < 1024 * 1024) return (n / 1024).toFixed(1) + ' KB'
  if (n < 1024 * 1024 * 1024) return (n / 1024 / 1024).toFixed(1) + ' MB'
  return (n / 1024 / 1024 / 1024).toFixed(2) + ' GB'
}

function formatTime(ms) {
  if (!ms) return ''
  // 后端时间戳为 Unix 毫秒（sharesGet/collectDirFiles 返回 updated_at 均为毫秒）
  const d = new Date(ms)
  if (isNaN(d.getTime())) return ''
  const p = (n) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`
}

const meta = computed(() => {
  if (!file.value) return []
  const arr = []
  if (file.value.size > 0) arr.push(formatSize(file.value.size))
  if (file.value.updated_at) arr.push(formatTime(file.value.updated_at) + ' 更新')
  arr.push('由爱库录 AiKlog 发布')
  return arr
})

async function load() {
  const token = route.params.token
  if (!token) {
    error.value = '分享链接无效'
    loading.value = false
    return
  }
  loading.value = true
  error.value = ''
  dirList.value = false
  dirData.value = null
  file.value = null
  html.value = ''
  try {
    const data = await getShare(token)
    // 文件夹整体分享
    if (data.share?.scope === 'dir') {
      dirData.value = { dir: data.dir, files: data.files || [] }
      const p = route.query.path
      const slug = route.query.slug
      // 优先 slug 定位（公开稳定链接：文件名/分类改名不碎链），回退 path
      let hit = null
      if (slug) hit = dirData.value.files.find((f) => f.slug === slug)
      if (!hit && p) hit = dirData.value.files.find((f) => f.path === p)
      if (!p && !slug) {
        dirList.value = true
        loading.value = false
        return
      }
      if (!hit) {
        error.value = '目录中没有找到该文件（可能已被移除）'
        loading.value = false
        return
      }
      dirList.value = false
      file.value = hit
      if (kind.value === 'text' && ext(file.value.name) !== 'html') {
        html.value = renderMarkdown(await fetchDirShareContent(token, hit.path))
      } else if (ext(file.value.name) === 'html') {
        html.value = await fetchDirShareContent(token, hit.path)
      }
      loading.value = false
      return
    }
    file.value = data.file
    if (kind.value === 'text' && ext(file.value.name) !== 'html') {
      html.value = renderMarkdown(await fetchShareContent(token))
    } else if (ext(file.value.name) === 'html') {
      html.value = await fetchShareContent(token)
    }
  } catch (e) {
    error.value = e.message || '分享已失效或不存在'
  } finally {
    loading.value = false
  }
}

onMounted(load)

// 同页 hash 导航（目录列表点文件 / 分享链接带 path）：组件不重挂载，需监听路由变化
watch(
  () => [route.params.token, route.query.path],
  () => { load() }
)
// 博客插件上下文（post_bottom/sidebar/head）：文章页数据契约
const postCtx = computed(() => {
  const p = (route.query.path || '')
  const slug = route.query.slug || ''
  const f = file.value || {}
  const cat = (p || (f.path || '')).includes('/') ? (p || f.path).split('/')[0] : ''
  return {
    title: f.name || '',
    path: p || f.path || '',
    slug: slug || f.slug || '',
    category: cat,
    updatedAt: f.updated_at || 0,
    size: f.size || 0,
    token: route.params.token || '',
  }
})
onMounted(loadEnabledPlugins)

</script>

<style scoped>
.pub-view {
  min-height: 100vh;
  display: flex;
  flex-direction: column;
  background: var(--app-bg, #f5f6f8);
}

/* 极简顶栏 */
.pub-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 24px;
  height: 56px;
  background: var(--surface);
  border-bottom: 1px solid var(--border);
  position: sticky;
  top: 0;
  z-index: 10;
}

.pub-brand {
  display: flex;
  align-items: center;
  gap: 10px;
  text-decoration: none;
}

.pub-logo {
  width: 28px;
  height: 28px;
  border-radius: 8px;
  background: linear-gradient(135deg, var(--primary), #6366f1);
  color: #fff;
  font-size: 11px;
  font-weight: 700;
  display: flex;
  align-items: center;
  justify-content: center;
  letter-spacing: 0.5px;
}

.pub-name {
  font-weight: 600;
  font-size: 15px;
  color: var(--text);
}

.pub-sub {
  font-size: 12px;
  color: var(--text-3);
}

.pub-home {
  text-decoration: none;
}

/* 中间状态 */
.pub-center {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 14px;
  padding: 80px 24px;
  color: var(--text-3);
  font-size: 14px;
}

/* 媒体 */
.pub-media {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 40px 24px;
  gap: 16px;
}

.pub-img {
  max-width: min(100%, 1100px);
  max-height: 80vh;
  border-radius: var(--radius);
  object-fit: contain;
}

.pub-video {
  max-width: min(100%, 1100px);
  border-radius: var(--radius);
}

.pub-audio {
  width: min(100%, 640px);
}

.pub-pdf {
  width: min(100%, 1100px);
  height: 80vh;
  border: 1px solid var(--border);
  border-radius: var(--radius);
}

.pub-media-meta {
  font-size: 13px;
  color: var(--text-3);
}

/* 公开目录（文件夹整体分享） */
.pub-dir {
  flex: 1;
  width: min(100%, 860px);
  margin: 0 auto;
  padding: 36px 24px 60px;
}
.pub-dir-head {
  display: flex;
  align-items: center;
  gap: 10px;
  color: var(--text);
}
.pub-dir-title {
  margin: 0;
  font-size: 22px;
  font-weight: 700;
}
.pub-dir-count {
  font-size: 13px;
  color: var(--text-3);
}
.pub-dir-desc {
  margin: 8px 0 20px;
  font-size: 13px;
  color: var(--text-3);
}
.pub-dir-list {
  display: flex;
  flex-direction: column;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  overflow: hidden;
  background: var(--surface);
}
.pub-dir-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 11px 14px;
  border-bottom: 1px solid var(--border);
  color: var(--text);
  text-decoration: none;
  font-size: 14px;
}
.pub-dir-row:last-child { border-bottom: none; }
.pub-dir-row:hover { background: color-mix(in srgb, var(--primary) 6%, transparent); }
.pub-dir-fname {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.pub-dir-fsize {
  flex: 0 0 auto;
  font-size: 12px;
  color: var(--text-3);
}

/* 页脚 */
.pub-foot {
  padding: 20px 24px 28px;
  text-align: center;
  font-size: 13px;
  color: var(--text-3);
  border-top: 1px solid var(--border);
  background: var(--surface);
}

/* 侧栏插件区（sidebar 挂载点容器） — 与正文同宽，避免满宽/贴边 */
.pub-shell {
  width: min(100%, 860px);
  margin: 0 auto;
  padding: 0 20px 32px;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.pub-shell :deep(.art-wrap),
.pub-shell :deep(.md-article),
.pub-shell :deep(.art-body) {
  max-width: 100%;
}

.pub-shell :deep(.md-article) {
  --reading-maxw: 100%;
  padding: 28px 0 8px;
  background: transparent;
}

.pub-shell :deep(.blog-plugin-slot) {
  width: 100%;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.pub-plugins-side:empty,
.pub-plugins-bottom:empty {
  display: none;
}

.pub-sidebar {
  width: min(100%, 860px);
  margin: 0 auto;
  padding: 0 24px 16px;
  display: flex;
  flex-direction: column;
  align-items: stretch;
}

.pub-sidebar:empty {
  display: none;
}

.pub-foot a {
  color: var(--primary);
  text-decoration: none;
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

@media (max-width: 767px) {
  .pub-bar {
    padding: 0 14px;
  }
  .pub-sub {
    display: none;
  }
}
</style>
