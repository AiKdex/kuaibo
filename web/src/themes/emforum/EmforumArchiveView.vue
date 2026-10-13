<template>
  <div class="eforum-root">
    <EmforumHead :site-name="siteName" :site-desc="siteDesc" :blog-url="blogUrl" slogan="ARCHIVE · 时间归档" />

    <div v-if="loading" class="ef-state">正在加载文章…</div>
    <div v-else-if="error" class="ef-state ef-state-err">{{ error }}</div>

    <div v-else class="ef-wrap">
      <div class="ef-board-info">
        <div>
          <div class="ef-board-name">◈ 归档</div>
          <div class="ef-board-desc">
            按更新时间分组 · 共 {{ posts.length }} 篇 / {{ groups.length }} 个月
          </div>
        </div>
        <div class="ef-board-actions">
          <a class="ef-ghost-btn" :href="blogUrl">‹ 返回全部版块</a>
        </div>
      </div>

      <div class="ef-layout">
        <main class="ef-archive">
          <!-- 年份锚点：用 JS 平滑滚动，不写 href="#y-2026"
               —— 宿主为 hash 路由（#/blog?...），写入 hash 会被路由误判为一次页面跳转 -->
          <nav v-if="years.length > 1" class="ef-chips">
            <button v-for="y in years" :key="y" type="button" class="ef-chip" @click="jumpTo(y)">
              {{ y }} 年
            </button>
          </nav>

          <section v-for="y in years" :id="'y-' + y" :key="y" class="ef-panel ef-year">
            <div class="ef-panel-head">
              <h3>{{ y }} 年</h3>
              <span class="ef-count">{{ yearCount(y) }} 篇</span>
            </div>

            <div v-for="g in groupsByYear(y)" :key="g.month" class="ef-month">
              <div class="ef-month-head">
                <span class="ef-month-name">{{ g.month }} 月</span>
                <span class="ef-month-line"></span>
                <span class="ef-month-num">{{ g.items.length }} 篇</span>
              </div>
              <a
                v-for="p in g.items"
                :key="p.token || p.path"
                class="ef-arc-row"
                :href="postHref(ctx, p)"
              >
                <span class="ef-arc-day">{{ postDate(p).slice(-2) }}</span>
                <span class="ef-arc-title">{{ postTitle(p) }}</span>
                <span v-if="postCat(p)" class="ef-arc-cat">{{ postCat(p) }}</span>
              </a>
            </div>
          </section>

          <p v-if="!groups.length" class="ef-empty">还没有公开文章</p>
        </main>

        <EmforumSide />
      </div>
    </div>

    <EmforumFoot :site-name="siteName" :blog-url="blogUrl" />
  </div>
</template>

<script setup>
import { computed } from 'vue'
import EmforumHead from './EmforumHead.vue'
import EmforumSide from './EmforumSide.vue'
import EmforumFoot from './EmforumFoot.vue'
import './style.css'
import { postCat, postDate, postHref, postTitle, useBlog, LIST_HREF } from './helpers.js'

/** 归档页 —— 从 posts 自行派生（规范 §4.7 / §7.3），不依赖额外接口 */
const ctx = useBlog()
const posts = computed(() => ctx.value.posts || [])
const loading = computed(() => !!ctx.value.loading)
const error = computed(() => ctx.value.error || '')
const siteName = computed(() => ctx.value.siteName || '')
const siteDesc = computed(() => ctx.value.siteDesc || '')
const blogUrl = computed(() => ctx.value.blogUrl || LIST_HREF)

const groups = computed(() => {
  const map = new Map()
  for (const p of posts.value) {
    const ym = postDate(p, 'month')
    if (!ym) continue
    if (!map.has(ym)) map.set(ym, [])
    map.get(ym).push(p)
  }
  return [...map.entries()]
    .sort((a, b) => (a[0] < b[0] ? 1 : -1))
    .map(([ym, items]) => {
      const [y, m] = ym.split('-')
      return {
        ym,
        year: y,
        month: Number(m),
        items: items.sort((a, b) => (b.file?.updated_at || 0) - (a.file?.updated_at || 0)),
      }
    })
})

const years = computed(() =>
  [...new Set(groups.value.map((g) => g.year))].sort((a, b) => Number(b) - Number(a)),
)
const groupsByYear = (y) => groups.value.filter((g) => g.year === y)
const yearCount = (y) => groupsByYear(y).reduce((n, g) => n + g.items.length, 0)

/* 年份跳转：滚动到对应 section，不改动 location.hash */
function jumpTo(y) {
  const el = document.getElementById('y-' + y)
  if (el) el.scrollIntoView({ behavior: 'smooth', block: 'start' })
}
</script>
