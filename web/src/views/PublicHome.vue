<template>
  <div class="ph-view">
    <!-- 博客插件挂载点：head（SEO meta 注入，隐藏容器） -->
    <BlogPluginSlot mount="head" :ctx="pageCtx" />

    <!-- 极简顶栏：品牌 + 入口 -->
    <header class="pub-bar">
      <a class="pub-brand" href="#/blog?view=public" :title="$t('爱库录公开博客')">
        <span class="pub-logo">{{  $t('录')  }}</span>
        <span class="pub-name">{{  site.title || $t('爱库录')  }}</span>
        <span class="pub-sub">AiKlog</span>
      </a>
      <LangSwitch mode="inline" />
      <GuestEntry variant="bar" />
    </header>

    <!-- 头部：博客标题（站点设置 blog.title/description 同源） -->
    <div class="ph-head">
      <h1 class="ph-title">{{  site.title || $t('爱库录')  }}</h1>
      <p class="ph-desc">{{  site.description || $t('爱库录 · AI 知识库博客：目录即站点，文件即文章')  }}</p>
    </div>

    <!-- 加载/错误 -->
    <div v-if="loading" class="pub-center"><div class="spinner"></div><span>{{  $t('加载中…')  }}</span></div>
    <div v-else-if="error" class="pub-center">
      <AikIcon name="info" :size="20" />
      <p>{{  error  }}</p>
    </div>

    <!-- 空态 -->
    <div v-else-if="!items.length" class="pub-center">
      <AikIcon name="book" :size="40" />
      <p>{{  $t('这里还没有公开文章')  }}</p>
    </div>

    <!-- 文章列表（按目录分类分组：子目录=分类） -->
    <div v-else class="ph-groups">
      <!-- 博客根目录直放文件（未分类） -->
      <section v-if="loose.length" class="ph-group">
        <div class="ph-group-head"><span class="ph-group-cat">{{  $t('未分类')  }}</span><span class="ph-group-count">{{  loose.length  }}</span></div>
        <div class="ph-list">
          <article v-for="p in loose" :key="p.token + '/' + (p.path || '')" class="ph-post">
            <a class="ph-post-link" :href="postUrl(p)">
              <h2 class="ph-post-title">{{  p.file?.name || $t('（未命名）')  }}
                <span v-if="p.scope === 'dir' && p.token !== 'blog'" class="dir-tag" :title="$t('来自文件夹整体分享')">{{  $t('文件夹')  }}</span>
              </h2>
              <div class="ph-post-meta">
                <span>{{  formatTime(p.file?.updated_at || p.created_at)  }}</span>
                <span class="dot">·</span>
                <span>{{  p.file?.size ? formatSize(p.file.size) : '—'  }}</span>
                <span v-if="typeLabel(p.file)" class="dot">·</span>
                <span v-if="typeLabel(p.file)">{{  typeLabel(p.file)  }}</span>
              </div>
              <p v-if="cleanPreview(p.preview)" class="ph-post-excerpt">{{  cleanPreview(p.preview)  }}</p>
            </a>
            <!-- 博客插件挂载点：列表项（list_item） -->
            <BlogPluginSlot mount="list_item" :ctx="{ post: p }" />
          </article>
        </div>
      </section>

      <!-- 分类组（博客目录子目录） -->
      <section v-for="g in groups" :key="g.cat" class="ph-group">
        <div class="ph-group-head">
          <span class="ph-group-cat">{{  g.cat  }}</span>
          <span class="ph-group-count">{{  g.posts.length  }}</span>
        </div>
        <div class="ph-list">
          <article v-for="p in g.posts" :key="p.token + '/' + (p.path || '')" class="ph-post">
            <a class="ph-post-link" :href="postUrl(p)">
              <h2 class="ph-post-title">{{  p.file?.name || $t('（未命名）')  }}
                <span v-if="p.scope === 'dir' && p.token !== 'blog'" class="dir-tag" :title="$t('来自文件夹整体分享')">{{  $t('文件夹')  }}</span>
              </h2>
              <div class="ph-post-meta">
                <span>{{  formatTime(p.file?.updated_at || p.created_at)  }}</span>
                <span class="dot">·</span>
                <span>{{  p.file?.size ? formatSize(p.file.size) : '—'  }}</span>
                <span v-if="typeLabel(p.file)" class="dot">·</span>
                <span v-if="typeLabel(p.file)">{{  typeLabel(p.file)  }}</span>
                <template v-if="p.path">
                  <span class="dot">·</span>
                  <span class="ph-post-path" :title="p.path">{{  p.path  }}</span>
                </template>
              </div>
              <p v-if="cleanPreview(p.preview)" class="ph-post-excerpt">{{  cleanPreview(p.preview)  }}</p>
            </a>
            <!-- 博客插件挂载点：列表项（list_item） -->
            <BlogPluginSlot mount="list_item" :ctx="{ post: p }" />
          </article>
        </div>
      </section>
    </div>

    <!-- 页脚（站点设置 blog.footer 同源） -->
    <footer class="pub-foot" v-html="footerHtml"></footer>
  </div>
</template>

<script setup>
import { t as i18t } from '@/i18n'
import { ref, computed, onMounted } from 'vue'
import AikIcon from '@/components/AikIcon.vue'
import { publicPosts, publicSite } from '@/api'
import DOMPurify from 'dompurify'
import BlogPluginSlot from '@/components/BlogPluginSlot.vue'
import GuestEntry from '@/components/GuestEntry.vue'
import LangSwitch from '@/components/LangSwitch.vue'
import '@/blogPlugins/builtin' // 注册内置博客插件
import { loadEnabledPlugins } from '@/blogPlugins'
import { t } from '@/i18n'

const items = ref([])
const loading = ref(true)
const error = ref('')
const site = ref({ title: '', description: '', footer: '' })

// head 挂载点上下文（公开首页；SEO 默认描述 → 站点简介 → 固定文案）
const pageCtx = computed(() => ({
  title: site.value.title || t('爱库录博客'),
  description: site.value.description || t('由爱库录知识库发布的公开文章'),
}))

// 页脚：站点设置文案（含链接），无则默认。
// H5 修复：v-html 渲染前必须 DOMPurify 消毒 —— blog.footer 可由后台写入，
// 未消毒时存储型 XSS 会在每个访客的浏览器里执行（与 utils/markdown.js 的消毒策略对齐）。
const footerHtml = computed(() => {
  const raw = site.value.footer ? site.value.footer : t('由 <a href="#/files">爱库录</a> 发布 · 数据主权在发布者')
  return DOMPurify.sanitize(raw)
})

// 分类分组：博客目录子目录 = 分类（path 首段）；根目录直放文件归"未分类"
// 分组顺序（A1 分类排序底座）：手动 sort_order（>=0）按值升序在前；未设置(-1/缺失)在后按组内最新文章时间动态排。
const loose = computed(() => items.value.filter((p) => !p.path))
const groups = computed(() => {
  const m = new Map()
  for (const p of items.value) {
    if (!p.path) continue
    const cat = p.path.split('/')[0]
    if (!m.has(cat)) m.set(cat, [])
    m.get(cat).push(p)
  }
  const arr = [...m.entries()].map(([cat, posts]) => {
    const soRaw = posts[0]?.file?.cat_sort_order
    const so = typeof soRaw === 'number' && soRaw >= 0 ? soRaw : -1
    // 动态基准：组内最新文章更新时间
    let last = 0
    for (const p of posts) {
      const t = Number(p.file?.updated_at || p.file?.created_at || 0)
      if (t > last) last = t
    }
    // 2.1 置顶底座：组内 = 全局置顶(global) > 分类置顶(category，pin_order 升序) > 其余按时间倒序
    posts = posts.slice().sort((x, y) => {
      const rank = (p) => (p.file?.pin_scope === 'global' ? 2 : (p.file?.pin_scope === 'category' ? 1 : 0))
      const rx = rank(x), ry = rank(y)
      if (rx !== ry) return rx - ry
      const xo = x.file?.pin_order, yo = y.file?.pin_order
      if (typeof xo === 'number' && typeof yo === 'number' && xo !== yo) return xo - yo
      if ((typeof xo === 'number') !== (typeof yo === 'number')) return typeof xo === 'number' ? -1 : 1
      return (Number(y.file?.updated_at || 0)) - (Number(x.file?.updated_at || 0))
    })
    return { cat, posts, sort: so, last }
  })
  arr.sort((x, y) => {
    if (x.sort !== y.sort) return (x.sort === -1 ? 1 : 0) - (y.sort === -1 ? 1 : 0) || x.sort - y.sort
    return y.last - x.last
  })
  return arr
})

function postUrl(p) {
  // 目录分享展开的文件：优先用公开稳定链接 slug（改名不碎链），无 slug 回退 path
  const f = p.file || {}
  if (p.path && f.slug) return `#/p/${p.token}?slug=${encodeURIComponent(f.slug)}`
  return p.path ? `#/p/${p.token}?path=${encodeURIComponent(p.path)}` : `#/p/${p.token}`
}

function typeLabel(f) {
  if (!f) return ''
  if (f.kind === 'dir') return i18t('文件夹')
  const e = (f.name || '').split('.').pop().toLowerCase()
  return (e || '').toUpperCase()
}

function formatSize(n) {
  if (!n) return '—'
  if (n < 1024) return n + ' B'
  if (n < 1024 * 1024) return (n / 1024).toFixed(1) + ' KB'
  if (n < 1024 * 1024 * 1024) return (n / 1024 / 1024).toFixed(1) + ' MB'
  return (n / 1024 / 1024 / 1024).toFixed(2) + ' GB'
}

// 分享时间戳为 Unix 毫秒
function formatTime(ms) {
  if (!ms) return ''
  const d = new Date(ms > 1e12 ? ms : ms * 1000)
  const p = (n) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`
}

// 列表摘要：去掉降级前缀、标题行、图片语法
function cleanPreview(s) {
  return String(s || '')
    .replace(/^（降级摘要）文件名：[^\n#]+/m, '')
    .replace(/!\[[^\]]*\]\([^)]*\)/g, '')
    .replace(/^#{1,6}\s+[^\n]+$/gm, '')
    .replace(/\s+/g, ' ')
    .trim()
    .slice(0, 160)
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const d = await publicPosts()
    items.value = d.items || []
  } catch (e) {
    error.value = e.message || t('加载失败')
  } finally {
    loading.value = false
  }
}

async function loadSiteCfg() {
  try {
    const d = await publicSite()
    if (d?.site) {
      site.value = {
        title: d.site.title || '',
        description: d.site.description || '',
        footer: d.site.footer || '',
      }
      // 2.7 站点自定义 CSS/JS：注入 <style>/<script>（全主题生效）
      if (d.site.custom_css) injectStyle('ph-custom-css', d.site.custom_css)
      if (d.site.custom_js) injectScript('ph-custom-js', d.site.custom_js)
    }
  } catch (_) { /* 离线时保持默认 */ }
}

// 注入自定义 CSS（幂等：同 id 只注入一次，内容更新时替换）
function injectStyle(id, css) {
  let el = document.getElementById(id)
  if (!el) {
    el = document.createElement('style')
    el.id = id
    document.head.appendChild(el)
  }
  el.textContent = css
}

// 注入自定义 JS（幂等：同 id 已存在则跳过，避免重复执行）
function injectScript(id, js) {
  if (document.getElementById(id)) return
  const el = document.createElement('script')
  el.id = id
  el.textContent = js
  document.head.appendChild(el)
}

onMounted(() => {
  load()
  loadSiteCfg()
  loadEnabledPlugins()
})
</script>

<style scoped>
.ph-view {
  min-height: 100vh;
  display: flex;
  flex-direction: column;
  background: var(--app-bg, #f5f6f8);
}

/* 极简顶栏（与公开单篇页一致） */
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

/* 博客头 */
.ph-head {
  padding: 44px 24px 10px;
  text-align: center;
}

.ph-title {
  margin: 0 0 6px;
  font-size: 30px;
  font-weight: 700;
  color: var(--text);
}

.ph-desc {
  margin: 0;
  font-size: 13px;
  color: var(--text-3);
}

/* 分组容器（子目录=分类） */
.ph-groups {
  width: 100%;
  max-width: 760px;
  margin: 0 auto;
  padding: 24px 24px 12px;
  display: flex;
  flex-direction: column;
  gap: 30px;
}

.ph-group-head {
  display: flex;
  align-items: center;
  gap: 10px;
  padding-bottom: 10px;
  border-bottom: 2px solid var(--border, #e3e6eb);
}

.ph-group-cat {
  font-size: 17px;
  font-weight: 700;
  color: var(--text, #1f2329);
}

.ph-group-count {
  font-size: 12px;
  color: var(--muted, #8a919f);
  background: var(--border, #e3e6eb);
  border-radius: 10px;
  padding: 1px 9px;
}

/* 文章列表 */
.ph-list {
  flex: 1;
  width: 100%;
  max-width: 760px;
  margin: 0 auto;
  padding: 0 0 8px;
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.ph-post {
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: var(--surface);
  transition: all 0.16s ease;
}

.ph-post:hover {
  border-color: var(--primary);
  box-shadow: var(--shadow-sm, 0 2px 10px rgba(0, 0, 0, 0.06));
  transform: translateY(-1px);
}

.ph-post-link {
  display: block;
  padding: 18px 22px;
  text-decoration: none;
  color: inherit;
}

.ph-post-title {
  margin: 0 0 8px;
  font-size: 18px;
  font-weight: 600;
  color: var(--text);
}

.dir-tag {
  display: inline-block;
  margin-left: 8px;
  padding: 0 7px;
  border-radius: 4px;
  font-size: 12px;
  font-weight: 500;
  vertical-align: 2px;
  color: var(--warn);
  border: 1px solid color-mix(in srgb, var(--warn) 50%, transparent);
  background: color-mix(in srgb, var(--warn) 12%, transparent);
}

.ph-post-path {
  max-width: 260px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ph-post:hover .ph-post-title {
  color: var(--primary);
}

.ph-post-meta {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--text-3);
}

.ph-post-meta .dot {
  opacity: 0.6;
}

.ph-post-excerpt {
  margin: 10px 0 0;
  font-size: 13.5px;
  line-height: 1.7;
  color: var(--text-2);
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
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

/* 页脚 */
.pub-foot {
  padding: 20px 24px 28px;
  text-align: center;
  font-size: 13px;
  color: var(--text-3);
  border-top: 1px solid var(--border);
  background: var(--surface);
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
  .ph-list {
    padding: 16px 14px 32px;
  }
}
</style>

