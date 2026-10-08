<template>
  <div class="eforum-root">
    <EmforumHead :site-name="siteName" :site-desc="siteDesc" :blog-url="blogUrl" slogan="AUTHOR · 作者专栏" />

    <div v-if="loading" class="ef-state">正在加载文章…</div>
    <div v-else-if="error" class="ef-state ef-state-err">{{ error }}</div>

    <div v-else class="ef-wrap">
      <div class="ef-board-info">
        <div>
          <div class="ef-board-name">✍ 作者：{{ author || '未知作者' }}</div>
          <div class="ef-board-desc">共 {{ list.length }} 篇公开文章</div>
        </div>
        <div class="ef-board-actions">
          <a class="ef-ghost-btn" :href="blogUrl">‹ 返回全部版块</a>
        </div>
      </div>

      <div class="ef-layout">
        <main class="ef-list">
          <div class="ef-list-head">
            <span class="ef-feed">该作者的全部主题</span>
          </div>
          <EmforumThread v-for="p in list" :key="p.token || p.path" :post="p" />
          <p v-if="!list.length" class="ef-empty">
            {{ authorHint }}
          </p>
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
import EmforumThread from './EmforumThread.vue'
import './style.css'
import { readPageParam, useBlog, LIST_HREF } from './helpers.js'

/**
 * 作者页 —— 契约见规范 §4.5：数据为 ctx.page.author + ctx.posts 按作者过滤。
 * 现状：posts 尚未透出 file.author（系统侧待补，规范 §10 第 10 项），
 * 因此此处做优雅降级：无作者字段时给出明确提示，不伪造作者。
 */
const ctx = useBlog()
const posts = computed(() => ctx.value.posts || [])
const loading = computed(() => !!ctx.value.loading)
const error = computed(() => ctx.value.error || '')
const siteName = computed(() => ctx.value.siteName || '')
const siteDesc = computed(() => ctx.value.siteDesc || '')
const blogUrl = computed(() => ctx.value.blogUrl || LIST_HREF)

const author = computed(() =>
  readPageParam(ctx.value, ['author', 'username'], [/\/author\/([^/?#]+)/, /[?&]author=([^&#]+)/]),
)

function authorOf(p) {
  const a = p.file?.author
  if (!a) return ''
  return typeof a === 'string' ? a : a.name || a.username || ''
}

const hasAuthorField = computed(() => posts.value.some((p) => !!authorOf(p)))
const list = computed(() =>
  author.value ? posts.value.filter((p) => authorOf(p) === author.value) : [],
)
const authorHint = computed(() =>
  hasAuthorField.value ? '该作者下暂无公开文章' : '作者数据尚未由系统侧透出（规范 §10-10），敬请期待',
)
</script>
