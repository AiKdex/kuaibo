<template>
  <div class="bf-post" :class="{ 'is-dark': dark }">
    <!-- 面包屑（站内跳转一律用 postUrl / 相对路径，不手拼 hash） -->
    <nav class="bf-crumb">
      <a :href="listHref">{{ $t('首页') }}</a>
      <span class="bf-crumb-sep">/</span>
      <template v-if="post.cat">
        <span class="bf-crumb-cat">{{ post.cat }}</span>
        <span class="bf-crumb-sep">/</span>
      </template>
      <span class="bf-crumb-now">{{ post.title }}</span>
    </nav>

    <div class="bf-post-layout">
      <main class="bf-post-main">
        <!-- 文章头部卡：紧凑版式（不做全屏 Banner），主色顶条 + 标题 + 元信息 -->
        <header class="bf-post-head">
          <span v-if="post.cat" class="bf-post-cat">{{ post.cat }}</span>
          <h1 class="bf-post-h1">{{ post.title }}</h1>
          <div class="bf-post-meta">
            <span class="bf-post-meta-i">
              <span class="bf-post-ava">{{ avatarText }}</span>
              {{ authorName }}
            </span>
            <span v-if="dateText" class="bf-post-meta-i">{{ dateText }}</span>
            <span v-if="sizeText" class="bf-post-meta-i">{{ sizeText }}</span>
          </div>
        </header>

        <!-- 正文（HTML 由系统渲染后下发，主题不负责正文渲染本身） -->
        <article ref="art" class="bf-article" v-html="post.html"></article>

        <!-- 版权卡：Butterfly 标志性组件 -->
        <div class="bf-copyright">
          <div class="bf-copyright-i"><b>{{ $t('文章作者') }}</b><span>{{ authorName }}</span></div>
          <div class="bf-copyright-i"><b>{{ $t('文章标题') }}</b><span>{{ post.title }}</span></div>
          <div class="bf-copyright-i"><b>{{ $t('文章链接') }}</b><span class="bf-copyright-url">{{ shareUrl }}</span></div>
          <div class="bf-copyright-i"><b>{{ $t('版权声明') }}</b><span>{{ $t("本博客所有文章除特别声明外，均采用「署名 - 非商业性使用 - 相同方式共享」许可协议。转载请注明出处。") }}</span></div>
        </div>

        <div class="bf-share">
          <button class="bf-share-btn" type="button" @click="copyLink">{{ $t('复制链接') }}</button>
          <button class="bf-share-btn" type="button" @click="shareWeibo">{{ $t('分享到微博') }}</button>
          <a class="bf-share-btn" :href="ssrHref" target="_blank" rel="noopener">{{ $t('打开静态页') }}</a>
        </div>

        <!-- 上下篇 -->
        <nav v-if="prevPost || nextPost" class="bf-pn">
          <a v-if="prevPost" class="bf-pn-card" :href="postHref(prevPost)">
            <span class="bf-pn-dir">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M15 6l-6 6 6 6" /></svg>
              {{ $t('上一篇') }}
            </span>
            <span class="bf-pn-ttl">{{ postTitle(prevPost) }}</span>
          </a>
          <span v-else class="bf-pn-empty">{{ $t('已是第一篇') }}</span>

          <a v-if="nextPost" class="bf-pn-card is-next" :href="postHref(nextPost)">
            <span class="bf-pn-dir">
              {{ $t('下一篇') }}
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9 6l6 6-6 6" /></svg>
            </span>
            <span class="bf-pn-ttl">{{ postTitle(nextPost) }}</span>
          </a>
          <span v-else class="bf-pn-empty">{{ $t('已是最后一篇') }}</span>
        </nav>

        <!-- 相关阅读：同分类优先 + 其余补齐（纯时间序，不表达热度） -->
        <section v-if="related.length" class="bf-related">
          <h3 class="bf-related-h">{{ $t('相关阅读') }}<span class="bf-related-note">{{ $t('同分类优先') }}</span></h3>
          <div class="bf-related-grid">
            <a v-for="p in related" :key="p.token || p.path" class="bf-related-card" :href="postHref(p)">
              <span class="bf-related-art" :class="catTone(catName(p))"></span>
              <span class="bf-related-body">
                <span class="bf-related-cat">{{ catName(p) }}</span>
                <span class="bf-related-ttl">{{ postTitle(p) }}</span>
                <span class="bf-related-date">{{ postDate(p) }}</span>
              </span>
            </a>
          </div>
        </section>

        <!-- 评论：宿主插件槽（真实后端） -->
        <section class="bf-comments">
          <BlogPluginSlot mount="post_bottom" :ctx="pluginCtx" />
        </section>
      </main>

      <aside class="bf-post-aside">
        <section v-if="tocList.length" class="bf-post-sb">
          <h3 class="bf-post-sb-h">{{ $t('本篇目录') }}</h3>
          <ul class="bf-post-toc">
            <li v-for="(h, i) in tocList" :key="i" class="bf-post-toc-i" :class="{ sub: h.level === 3 }">
              <!-- 目录跳转用 button + scrollIntoView（规范 §3.2） -->
              <button class="bf-post-toc-b" type="button" @click="scrollToHeading(i)">{{ h.text }}</button>
            </li>
          </ul>
        </section>

        <section class="bf-post-sb">
          <h3 class="bf-post-sb-h">{{ $t('文章信息') }}</h3>
          <ul class="bf-post-info">
            <li><span>{{ $t('分类') }}</span><b>{{ post.cat || $t("未分类") }}</b></li>
            <li v-if="dateText"><span>{{ $t('更新') }}</span><b>{{ postDate({ file: { updated_at: post.updatedAt } }) }}</b></li>
            <li v-if="sizeText"><span>{{ $t('篇幅') }}</span><b>{{ sizeText }}</b></li>
            <li><span>{{ $t('目录层级') }}</span><b>{{ tocList.length }} {{ $t('节') }}</b></li>
          </ul>
        </section>

        <section v-if="sameCatPosts.length" class="bf-post-sb">
          <h3 class="bf-post-sb-h">{{ $t('同类文章') }}</h3>
          <ul class="bf-post-toc">
            <li v-for="p in sameCatPosts" :key="p.token || p.path" class="bf-post-toc-i">
              <a class="bf-post-toc-b" :href="postHref(p)">{{ postTitle(p) }}</a>
            </li>
          </ul>
        </section>
      </aside>
    </div>

    <ButterflyFoot :site-name="siteName" :blog-url="listHref" :count="posts.length" />
    <ButterflyRight :dark="dark" :toc="tocList" @toggle-dark="toggleDark" @scroll-to="scrollToHeading" />
  </div>
    <AskWidget />
</template>

<script setup>
import AskWidget from '../AskWidget.vue'
/**
 * ButterflyPostView —— 蝶语主题「文章内页」（entries.post）。
 * 宿主 BlogPostView 在主题声明 entries.post 时整页交给本组件渲染，
 * 数据来自 inject('themeContext').page.post（标题 / 正文 HTML / 分类 / 作者 / 日期）。
 * ⚠️ 文章专属样式全部在 post.css（构建插件加 .th-butterfly.th-view-post 作用域，零泄漏）。
 */
import { computed, ref } from 'vue'
import BlogPluginSlot from '@/components/BlogPluginSlot.vue'
import ButterflyFoot from './ButterflyFoot.vue'
import ButterflyRight from './ButterflyRight.vue'
import {
  LIST_HREF, catName, catTone, postDate, postTitle, useBlog, useDark,
} from './helpers.js'

const ctx = useBlog()
const { dark, toggleDark } = useDark()

const post = computed(() => ctx.value?.page?.post || {})
const posts = computed(() => ctx.value?.posts || [])
const siteName = computed(() => ctx.value?.siteName || '爱库录')
const postUrlFn = computed(() => ctx.value?.postUrl)
const ssrUrlFn = computed(() => ctx.value?.postSsrUrl)

const listHref = LIST_HREF

const authorName = computed(() => post.value.author || siteName.value)
const avatarText = computed(() => (authorName.value || '?').slice(0, 1))
const dateText = computed(() => {
  const d = postDate({ file: { updated_at: post.value.updatedAt } }, 'ymd')
  return d ? d + ' 更新' : ''
})
const sizeText = computed(() => {
  const s = post.value.size || 0
  if (!s) return ''
  return s > 1024 * 1024 ? (s / 1024 / 1024).toFixed(1) + ' MB' : Math.max(1, Math.round(s / 1024)) + ' KB'
})
/** 分享 / SEO 场景用 SSR 静态页地址（规范 §2.3） */
const ssrHref = computed(() => (ssrUrlFn.value ? ssrUrlFn.value(post.value) : '/blog'))
const shareUrl = computed(() => (typeof window !== 'undefined' ? window.location.href : ''))

/* 当前文章唯一键：token 可能相同，用 path/slug 区分 */
const curKey = computed(() => post.value.path || post.value.slug || '')
const idx = computed(() => posts.value.findIndex((p) => (p.path || p.file?.slug || '') === curKey.value))
const prevPost = computed(() => (idx.value > 0 ? posts.value[idx.value - 1] : null))
const nextPost = computed(() =>
  idx.value >= 0 && idx.value < posts.value.length - 1 ? posts.value[idx.value + 1] : null)

const sameCatPosts = computed(() =>
  posts.value
    .filter((p) => (p.path || p.file?.slug || '') !== curKey.value && catName(p) === post.value.cat)
    .slice(0, 5))

const related = computed(() => {
  const cur = curKey.value
  const rest = posts.value.filter((p) => (p.path || p.file?.slug || '') !== cur)
  const sameCat = rest.filter((p) => catName(p) === post.value.cat)
  const others = rest.filter((p) => catName(p) !== post.value.cat)
  return [...sameCat, ...others].slice(0, 3)
})

function postHref(p) {
  return postUrlFn.value ? postUrlFn.value(p) : '#'
}

/* 目录：从正文 HTML 解析 h2/h3，点击平滑滚动（不碰 location.hash） */
const art = ref(null)
const tocList = computed(() => {
  const html = post.value.html || ''
  const re = /<h([23])[^>]*>([\s\S]*?)<\/h\1>/gi
  const out = []
  let m
  while ((m = re.exec(html))) {
    const text = m[2].replace(/<[^>]+>/g, '').trim()
    if (text) out.push({ level: +m[1], text })
  }
  return out
})
function scrollToHeading(i) {
  const els = art.value?.querySelectorAll('h2, h3')
  if (els && els[i]) els[i].scrollIntoView({ behavior: 'smooth', block: 'start' })
}

function copyLink() {
  try {
    if (navigator.clipboard) navigator.clipboard.writeText(window.location.href)
  } catch (e) { /* 剪贴板不可用时静默忽略 */ }
}
function shareWeibo() {
  const u = encodeURIComponent(window.location.href)
  const t = encodeURIComponent(post.value.title || '')
  window.open(`http://service.weibo.com/share/share.php?url=${u}&title=${t}`, '_blank')
}

/* 评论插件上下文（与宿主一致） */
const pluginCtx = computed(() => ({
  title: post.value.title,
  path: post.value.path,
  slug: post.value.slug,
  category: post.value.cat,
  updatedAt: post.value.updatedAt,
  size: post.value.size,
  token: post.value.token,
}))
</script>
