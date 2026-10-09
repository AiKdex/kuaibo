<template>
  <div class="au-root au-post">
    <AuroraHead />

    <main class="au-main">
      <nav class="au-crumb">
        <a class="au-crumb-link" :href="listHref">{{ $t('全部文章') }}</a>
        <span class="au-crumb-sep" aria-hidden="true">/</span>
        <template v-if="post.cat">
          <span class="au-crumb-mid">{{ post.cat }}</span>
          <span class="au-crumb-sep" aria-hidden="true">/</span>
        </template>
        <span class="au-crumb-now">{{ post.title }}</span>
      </nav>

      <header class="au-post-head">
        <span v-if="post.cat" class="au-post-cat">{{ post.cat }}</span>
        <h1 class="au-post-title">{{ post.title }}</h1>
        <div class="au-post-meta">
          <span v-for="(m, i) in meta" :key="i" class="au-post-meta-i">{{ m }}</span>
          <BlogPluginSlot mount="post_meta" :ctx="pluginCtx" />
        </div>
      </header>

      <div class="au-post-layout">
        <div class="au-post-main">
          <!-- 正文由系统渲染好（ctx.page.post.html），主题只负责排版容器 -->
          <article ref="art" class="au-article" v-html="post.html"></article>

          <div class="au-post-foot">
            <button class="au-btn" type="button" @click="copyLink">
              {{ copied ? $t("链接已复制") : $t("复制分享链接") }}
            </button>
            <a class="au-btn au-btn-ghost" :href="shareHref">{{ $t('打开静态页') }}</a>
          </div>

          <nav v-if="prevPost || nextPost" class="au-pn">
            <a v-if="prevPost" class="au-pn-item" :href="href(prevPost)">
              <span class="au-pn-dir">{{ $t('← 上一篇') }}</span>
              <span class="au-pn-t">{{ titleOf(prevPost) }}</span>
            </a>
            <a v-if="nextPost" class="au-pn-item au-pn-item-r" :href="href(nextPost)">
              <span class="au-pn-dir">{{ $t('下一篇 →') }}</span>
              <span class="au-pn-t">{{ titleOf(nextPost) }}</span>
            </a>
          </nav>

          <!-- 插件槽：评论 / 统计 / 相关推荐等由宿主插件决定 -->
          <section class="au-plugins">
            <BlogPluginSlot mount="post_bottom" :ctx="pluginCtx" />
          </section>
        </div>

        <aside class="au-side">
          <div v-if="toc.length" class="au-side-box">
            <div class="au-side-t">{{ $t('本文目录') }}</div>
            <ul class="au-toc">
              <li v-for="(h, i) in toc" :key="i" class="au-toc-li">
                <button
                  class="au-toc-btn"
                  :class="{ sub: h.level === 3 }"
                  type="button"
                  @click="jump(i)"
                >
                  {{ h.text }}
                </button>
              </li>
            </ul>
          </div>

          <div v-if="related.length" class="au-side-box">
            <div class="au-side-t">{{ $t('相关文章') }}</div>
            <ul class="au-side-list">
              <li v-for="(p, i) in related" :key="i" class="au-side-li">
                <a class="au-side-a" :href="href(p)">{{ titleOf(p) }}</a>
                <span class="au-side-d">{{ dateOf(p) }}</span>
              </li>
            </ul>
          </div>

          <div v-if="cats.length" class="au-side-box">
            <div class="au-side-t">{{ $t('分类') }}</div>
            <ul class="au-side-list">
              <li v-for="c in cats" :key="c.name" class="au-side-li">
                <span class="au-side-a">{{ c.name }}</span>
                <span class="au-side-d">{{ c.count }} {{ $t('篇') }}</span>
              </li>
            </ul>
          </div>
        </aside>
      </div>
    </main>

    <AuroraFoot />
  </div>
    <AskWidget />
</template>

<script setup>
import AskWidget from '../AskWidget.vue'
/**
 * AuroraPostView —— 极光主题文章内页（entries.post，一期交付项）。
 *
 * 数据全部来自 inject('themeContext').page.post：
 *   { token, path, slug, title, html, cat, category, author, updatedAt, size }
 * 其中 html 是系统已渲染好的正文 HTML，用 v-html 输出；主题不参与正文渲染。
 * 目录跳转一律 button + scrollIntoView —— 宿主是 hash 路由，绝不碰 location.hash。
 */
import { computed, ref } from 'vue'
import BlogPluginSlot from '@/components/BlogPluginSlot.vue'
import AuroraHead from './AuroraHead.vue'
import AuroraFoot from './AuroraFoot.vue'
import './post.css'
import {
  useBlog,
  postTitle,
  postDate,
  postSize,
  postHref,
  postShareHref,
  catCounts,
  catName,
  LIST_HREF,
  SSR_LIST_HREF,
} from './helpers.js'

const ctx = useBlog()
const post = computed(() => ctx.value?.page?.post || {})
const posts = computed(() => ctx.value?.posts || [])

const listHref = LIST_HREF

/* 当前文章唯一键：博客为单分享，token 恒为 blog，用 path / slug 区分 */
const keyOf = (p) => p?.path || p?.file?.slug || ''
const curKey = computed(() => post.value.path || post.value.slug || '')
const idx = computed(() => posts.value.findIndex((p) => keyOf(p) === curKey.value))
const prevPost = computed(() => (idx.value > 0 ? posts.value[idx.value - 1] : null))
const nextPost = computed(() =>
  idx.value >= 0 && idx.value < posts.value.length - 1 ? posts.value[idx.value + 1] : null
)

/* 相关文章：同分类优先，其余按系统顺序补足 3 篇 */
const related = computed(() => {
  const cur = curKey.value
  const rest = posts.value.filter((p) => keyOf(p) !== cur)
  const same = rest.filter((p) => catName(p) === catName(post.value))
  const other = rest.filter((p) => catName(p) !== catName(post.value))
  return [...same, ...other].slice(0, 3)
})

const cats = computed(() => catCounts(posts.value))

const meta = computed(() => {
  const arr = []
  const d = postDate({ file: { updated_at: post.value.updatedAt } }, 'ymd')
  if (d) arr.push(`${d} 更新`)
  if (post.value.author) arr.push(String(post.value.author))
  if (post.value.cat) arr.push(post.value.cat)
  const s = postSize({ file: { size: post.value.size } })
  if (s) arr.push(s)
  return arr
})

/* 分享链接：SSR 静态页（canonical / 分享快照场景） */
const shareHref = computed(() => {
  const raw = postShareHref(ctx.value, { token: post.value.token, path: post.value.path, file: { slug: post.value.slug } })
  if (raw) return raw
  return post.value.slug ? `/${encodeURIComponent(post.value.slug)}` : SSR_LIST_HREF
})

const copied = ref(false)
async function copyLink() {
  const rel = shareHref.value
  const abs =
    typeof window !== 'undefined'
      ? new URL(rel, window.location.origin).href
      : rel
  try {
    await navigator.clipboard.writeText(abs)
    copied.value = true
    setTimeout(() => {
      copied.value = false
    }, 2000)
  } catch {
    copied.value = false
  }
}

/* 目录：从已渲染正文里解析 h2 / h3，点击平滑滚动（不写 hash） */
const art = ref(null)
const toc = computed(() => {
  const html = post.value.html || ''
  const re = /<h([23])[^>]*>([\s\S]*?)<\/h\1>/gi
  const out = []
  let m
  while ((m = re.exec(html))) {
    const text = m[2].replace(/<[^>]+>/g, '').trim()
    if (text) out.push({ level: Number(m[1]), text })
  }
  return out
})
function jump(i) {
  const els = art.value?.querySelectorAll('h2, h3')
  if (els && els[i]) els[i].scrollIntoView({ behavior: 'smooth', block: 'start' })
}

const pluginCtx = computed(() => ({
  title: post.value.title || '',
  path: post.value.path || '',
  slug: post.value.slug || '',
  category: post.value.cat || '',
  updatedAt: post.value.updatedAt || 0,
  size: post.value.size || 0,
  token: post.value.token || '',
}))

function href(p) {
  return postHref(ctx.value, p)
}
function titleOf(p) {
  return postTitle(p)
}
function dateOf(p) {
  return postDate(p, 'ymd')
}
</script>
