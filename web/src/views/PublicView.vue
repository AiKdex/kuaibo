<template>
  <div class="pub-view">
    <!-- 博客插件挂载点：head（SEO meta 注入，隐藏容器） -->
    <BlogPluginSlot mount="head" :ctx="postCtx" />

    <!-- 极简顶栏：品牌 + 入口 -->
    <header class="pub-bar">
      <a class="pub-brand" href="/app#/blog?view=public" :title="$t('爱库录公开博客')">
        <span class="pub-logo">{{  $t('录')  }}</span>
        <span class="pub-name">{{  $t('爱库录')  }}</span>
        <span class="pub-sub">AiKlog</span>
      </a>
      <LangSwitch mode="inline" />
      <GuestEntry variant="bar" />
      <a class="pub-home btn btn-sm" href="/app#/blog?view=public">{{  $t('返回列表')  }}</a>
    </header>

    <!-- 加载/错误 -->
    <div v-if="loading" class="pub-center"><div class="spinner"></div><span>{{  $t('加载中…')  }}</span></div>
    <div v-else-if="error" class="pub-center">
      <AikIcon name="info" :size="20" />
      <p>{{  error  }}</p>
    </div>

    <!-- 正文：文本类 → 博客文章；媒体类 → 播放器/预览 -->
    <template v-else-if="!dirList">
      <!-- 付费/加密闸门：密码保护(2.2) 或标记为付费内容（复用 article_access） -->
      <div v-if="locked" class="pub-lock">
        <AikIcon :name="'lock'" :size="30" />
        <h2 class="pub-lock-title">{{  lockKind === 'paid' ? $t('付费内容') : $t('加密内容')  }}</h2>
        <p v-if="paidPreview" class="pub-lock-preview">{{  paidPreview  }}</p>
        <p class="pub-lock-desc">
          {{  lockKind === 'paid' ? $t('该内容需购买/解锁后阅读') : $t('该内容已加密，请输入访问密码')  }}
        </p>
        <template v-if="lockKind === 'password'">
          <div class="pub-lock-form">
            <input
              v-model="pwdInput"
              type="password"
              class="pub-lock-input"
              :placeholder="$t('访问密码')"
              @keyup.enter="submitUnlock"
            />
            <button class="btn btn-primary" :disabled="trying" @click="submitUnlock">
              {{  trying ? $t('验证中…') : $t('解锁')  }}
            </button>
          </div>
          <p v-if="unlockErr" class="pub-lock-err">{{  unlockErr  }}</p>
        </template>
        <p v-else class="pub-lock-note">{{ $t('在线支付通道待接入，请联系站长获取访问权限。') }}</p>
      </div>

      <div v-else class="pub-shell">
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
          <p class="pub-media-meta">{{  file?.name  }}</p>
        </div>

        <!-- 视频 -->
        <div v-else-if="isVideo" class="pub-media">
          <video :src="contentUrl" controls class="pub-video"></video>
          <p class="pub-media-meta">{{  file?.name  }}</p>
        </div>

        <!-- 音频 -->
        <div v-else-if="isAudio" class="pub-media">
          <audio :src="contentUrl" controls class="pub-audio"></audio>
          <p class="pub-media-meta">{{  file?.name  }}</p>
        </div>

        <!-- PDF -->
        <div v-else-if="isPdf" class="pub-media">
          <iframe :src="contentUrl" class="pub-pdf" :title="$t('PDF 预览')"></iframe>
        </div>

        <!-- 其他：下载 -->
        <div v-else class="pub-center">
          <AikIcon name="file" :size="40" />
          <p>{{  file?.name  }}</p>
          <a class="btn btn-primary" :href="contentUrl + '&download=1'" download>{{ $t('下载文件') }}</a>
        </div>

        <!-- 内容类型差异化渲染（node_type / fields）：资源下载区 / 外链卡 / 信息卡 -->
        <ContentStatePanel
          :file="file"
          :token="route.params.token"
          :path="route.query.path"
          :download-url="downloadUrl"
        />
      </div>
    </template>

    <!-- 目录整体分享：公开目录文件列表 -->
    <div v-else class="pub-dir">
      <div class="pub-dir-head">
        <AikIcon name="folder" :size="22" />
        <h1 class="pub-dir-title">{{  dirData?.dir?.name || $t('公开目录')  }}</h1>
        <span class="pub-dir-count">{{  dirData?.files?.length || 0  }} {{ $t('个文件') }}</span>
      </div>
      <p class="pub-dir-desc">{{ $t('此目录由发布者整体公开（文件夹分享）。目录内新增文件会自动出现在这里。') }}</p>
      <div v-if="!dirData?.files?.length" class="pub-center">
        <AikIcon name="folder" :size="40" />
        <p>{{ $t('目录内还没有可公开的文件') }}</p>
      </div>
      <div v-else class="pub-dir-list">
        <a
          v-for="f in dirData.files"
          :key="f.path"
          class="pub-dir-row"
          :href="`#/p/${route.params.token}?path=${encodeURIComponent(f.path)}`"
        >
          <span class="pub-dir-thumb">
            <img
              v-if="isThumbable(f) && !thumbFails[f.path]"
              :src="thumbUrl(f)"
              :alt="f.path"
              loading="lazy"
              @error="onThumbError(f.path)"
            />
            <AikIcon v-else :name="fileIcon(f)" :size="16" />
          </span>
          <span class="pub-dir-fname" :title="f.path">{{  f.path  }}</span>
          <span class="pub-dir-fsize">{{  f.size ? formatSize(f.size) : '—'  }}</span>
        </a>
      </div>
    </div>

    <!-- 页脚 -->
    <footer class="pub-foot">
      <span>{{ $t('爱库录 AiKlog · 数据主权在发布者 ·') }} <a href="/app#/blog?view=public">{{ $t('交互版') }}</a> · <a href="/blog">{{ $t('静态页') }}</a> · <a href="/api/v1/blog/sitemap.xml">{{ $t('站点地图') }}</a></span>
    </footer>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, watch } from 'vue'
import { useRoute } from 'vue-router'
import AikIcon from '@/components/AikIcon.vue'
import MarkdownArticle from '@/components/MarkdownArticle.vue'
import { getShare, shareContentUrl, dirShareContentUrl, fetchShareContentRaw, fetchDirShareContentRaw, publicUnlock, shareThumbUrl } from '@/api'
import BlogPluginSlot from '@/components/BlogPluginSlot.vue'
import GuestEntry from '@/components/GuestEntry.vue'
import LangSwitch from '@/components/LangSwitch.vue'
import '@/blogPlugins/builtin' // 注册内置博客插件
import { loadEnabledPlugins } from '@/blogPlugins'
import { renderMarkdown } from '@/utils/markdown'
import DOMPurify from 'dompurify'
import ContentStatePanel from '@/components/ContentStatePanel.vue'
import { parseContentState } from '@/utils/contentState'
import { t } from '@/i18n'

const route = useRoute()
const file = ref(null)
const html = ref('')
const loading = ref(true)
const error = ref('')
const dirData = ref(null)   // 目录分享：{ dir, files }
const dirList = ref(false)  // 目录列表视图（无 ?path=）

// 访问闸门（密码保护 2.2 / 付费标记）：401 进入解锁流程
const locked = ref(false)
const lockKind = ref('password') // 'password' | 'paid'
const paidPreview = ref('')
const pwdInput = ref('')
const trying = ref(false)
const unlockErr = ref('')
const unlockToken = ref('')

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
  let base
  if (dirData.value && route.query.path) base = dirShareContentUrl(route.params.token, route.query.path)
  else base = shareContentUrl(route.params.token)
  if (unlockToken.value) base += (base.includes('?') ? '&' : '?') + 'unlock=' + encodeURIComponent(unlockToken.value)
  return base
})

// 资源下载直链（带解锁令牌，受密码/付费闸门保护）
const downloadUrl = computed(() => {
  if (!file.value) return ''
  let base
  if (dirData.value && route.query.path) base = dirShareContentUrl(route.params.token, route.query.path, true)
  else base = shareContentUrl(route.params.token, true)
  if (unlockToken.value) base += (base.includes('?') ? '&' : '?') + 'unlock=' + encodeURIComponent(unlockToken.value)
  return base
})

function fileIcon(f) {
  const e = (f.name || '').split('.').pop().toLowerCase()
  if (['png', 'jpg', 'jpeg', 'gif', 'webp', 'svg', 'bmp', 'avif'].includes(e)) return 'image'
  if (['mp4', 'mov', 'mkv', 'webm', 'avi'].includes(e)) return 'video'
  if (['mp3', 'wav', 'm4a', 'flac', 'ogg'].includes(e)) return 'audio'
  if (['md', 'markdown', 'txt', 'log', 'json', 'html', 'htm', 'css', 'js', 'ts', 'py', 'go', 'vue', 'sql', 'sh'].includes(e)) return 'fileText'
  return 'file'
}

// B19：判断目录分享中的文件能否显示缩略图（与后端 thumbMimeOf 同口径：image/*|video/*，svg 排除）。
function isThumbable(f) {
  const m = (f.mime || '').toLowerCase()
  if (m.startsWith('image/')) return m !== 'image/svg+xml'
  if (m.startsWith('video/')) return true
  const e = ext(f.name || '')
  return EXT.image.includes(e) || EXT.video.includes(e)
}

// B19：目录分享缩略图地址（失败回退图标，见 onThumbError）。
function thumbUrl(f) {
  return shareThumbUrl(route.params.token, f.path)
}

// B19：缩略图加载失败时回退到文件图标（按 path 标记）。
const thumbFails = ref({})
function onThumbError(path) {
  thumbFails.value[path] = true
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
  if (file.value.updated_at) arr.push(formatTime(file.value.updated_at) + t(' 更新'))
  arr.push(t('由爱库录 AiKlog 发布'))
  return arr
})

async function load() {
  const token = route.params.token
  if (!token) {
    error.value = t('分享链接无效')
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
        error.value = t('目录中没有找到该文件（可能已被移除）')
        loading.value = false
        return
      }
      dirList.value = false
      file.value = hit
      await loadContent(token, hit.path, '')
      loading.value = false
      return
    }
    file.value = data.file
    await loadContent(token, '', '')
  } catch (e) {
    error.value = e.message || t('分享已失效或不存在')
  } finally {
    loading.value = false
  }
}

// 加载分享正文；捕获 401 密码/付费闸门，进入解锁流程
async function loadContent(token, path, unlock) {
  unlockToken.value = unlock || ''
  const r = path
    ? await fetchDirShareContentRaw(token, path, unlock)
    : await fetchShareContentRaw(token, { unlock })
  if (r.status === 401) {
    locked.value = true
    const c = parseContentState(file.value)
    lockKind.value = c.fields && c.fields.access === 'paid' ? 'paid' : 'password'
    paidPreview.value = (c.fields && c.fields.paid_preview) || ''
    html.value = ''
    return
  }
  if (!r.ok) {
    error.value = t('内容加载失败（HTTP ') + r.status + '）'
    return
  }
  locked.value = false
  const text = r.text
  // H5b 修复：.html 文件此前把原文直接注入 v-html —— 上传一个含 <script> 的 .html
  // 再分享出去，访客打开 /p/{token} 即执行。所有非 markdown 原文一律先 DOMPurify 消毒。
  html.value = kind.value === 'text' && ext(file.value.name) !== 'html'
    ? renderMarkdown(text)
    : DOMPurify.sanitize(text)
}

// 提交访问密码 → 解锁（复用现有 article_access 密码闸门）
async function submitUnlock() {
  if (!pwdInput.value || trying.value) return
  trying.value = true
  unlockErr.value = ''
  try {
    const d = await publicUnlock(file.value.id, pwdInput.value)
    if (d && d.ok && d.unlock_token) {
      locked.value = false
      await loadContent(route.params.token, route.query.path, d.unlock_token)
    } else {
      unlockErr.value = t('解锁失败，请重试')
    }
  } catch (e) {
    unlockErr.value = e.message || t('密码错误')
  } finally {
    trying.value = false
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
.pub-dir-thumb {
  flex: 0 0 auto;
  width: 28px;
  height: 28px;
  border-radius: 6px;
  overflow: hidden;
  display: flex;
  align-items: center;
  justify-content: center;
  background: color-mix(in srgb, var(--text) 6%, transparent);
}
.pub-dir-thumb img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
}
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

/* 付费/加密闸门 */
.pub-lock {
  width: min(100%, 560px);
  margin: 80px auto;
  padding: 36px 28px;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
  text-align: center;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--radius, 12px);
  color: var(--text);
}
.pub-lock-title {
  margin: 4px 0 0;
  font-size: 20px;
  font-weight: 700;
}
.pub-lock-preview {
  margin: 0;
  font-size: 14px;
  color: var(--text-2, #555);
  line-height: 1.7;
  max-height: 9.6em;
  overflow: hidden;
  -webkit-mask-image: linear-gradient(180deg, #000 60%, transparent);
  mask-image: linear-gradient(180deg, #000 60%, transparent);
}
.pub-lock-desc {
  margin: 0;
  font-size: 13px;
  color: var(--text-3, #888);
}
.pub-lock-form {
  display: flex;
  gap: 8px;
  margin-top: 6px;
}
.pub-lock-input {
  width: 220px;
  padding: 9px 12px;
  border: 1px solid var(--border);
  border-radius: 8px;
  font-size: 14px;
  background: var(--app-bg, #fff);
  color: var(--text);
}
.pub-lock-input:focus {
  outline: none;
  border-color: var(--primary);
}
.pub-lock-err {
  margin: 0;
  font-size: 13px;
  color: #dc2626;
}
.pub-lock-note {
  margin: 4px 0 0;
  font-size: 13px;
  color: var(--text-3, #888);
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
