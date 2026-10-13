<template>
  <div class="brutal-root">
    <BrutalHead :site-name="siteName" :blog-url="blogUrl" slogan="AUTHOR · 作者" />

    <div v-if="loading" class="bt-wrap"><div class="bt-state">正在加载文章…</div></div>
    <div v-else-if="error" class="bt-wrap"><div class="bt-state bt-state-err">{{ error }}</div></div>

    <template v-else>
      <section class="bt-posts">
        <div class="bt-wrap">
          <div class="bt-sect">
            <h2>{{ author || '作者主页' }}</h2>
            <span class="bt-rule"></span>
            <span class="bt-sect-note">{{ list.length }} 篇</span>
          </div>

          <!-- 契约未透出 file.author：明确说明而非伪造数据（规范 §2.2 / §4.4） -->
          <div class="bt-notice">
            <b>作者信息未开放</b>
            <p>
              当前博客数据契约未包含文章作者字段，因此无法按作者筛选。
              下方展示站点全部文章，待系统透出 <code>file.author</code> 后本页将自动按作者聚合。
            </p>
          </div>

          <div class="bt-bar">
            <span class="bt-bar-info">共 {{ list.length }} 篇文章</span>
            <a class="bt-btn bt-btn-sm" :href="blogUrl">‹ 返回全部文章</a>
          </div>

          <div v-if="list.length" class="bt-grid">
            <BrutalCard v-for="(p, i) in list" :key="p.token || p.path" :post="p" :index="i" />
          </div>
          <div v-else class="bt-state">暂无文章</div>
        </div>
      </section>

      <section class="bt-sidewrap">
        <div class="bt-wrap">
          <BrutalSide />
        </div>
      </section>
    </template>

    <BrutalFoot :site-name="siteName" :blog-url="blogUrl" />
  </div>
</template>

<script setup>
import { computed } from 'vue'
import BrutalHead from './BrutalHead.vue'
import BrutalCard from './BrutalCard.vue'
import BrutalSide from './BrutalSide.vue'
import BrutalFoot from './BrutalFoot.vue'
import './style.css'
import { LIST_HREF, readPageParam, useBlog } from './helpers.js'

const ctx = useBlog()
const posts = computed(() => ctx.value.posts || [])
const loading = computed(() => !!ctx.value.loading)
const error = computed(() => ctx.value.error || '')
const siteName = computed(() => ctx.value.siteName || '')
const blogUrl = computed(() => ctx.value.blogUrl || LIST_HREF)

/* 优先 ctx.page.author，回退解析 hash / ?author= */
const author = computed(() =>
  readPageParam(ctx.value, ['author'], [/\/author\/([^/?#]+)/, /[?&]author=([^&#]+)/]),
)

/* 数据层无作者字段 → 保持全量列表（不臆造来源） */
const list = computed(() => posts.value)
</script>
