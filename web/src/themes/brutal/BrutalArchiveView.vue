<template>
  <div class="brutal-root">
    <BrutalHead :site-name="siteName" :blog-url="blogUrl" :slogan="$t('ARCHIVE · 归档')" />

    <div v-if="loading" class="bt-wrap"><div class="bt-state">{{ $t('正在加载文章…') }}</div></div>
    <div v-else-if="error" class="bt-wrap"><div class="bt-state bt-state-err">{{ error }}</div></div>
    <div v-else-if="!posts.length" class="bt-wrap"><div class="bt-state">{{ $t('还没有公开文章') }}</div></div>

    <template v-else>
      <section class="bt-posts">
        <div class="bt-wrap">
          <div class="bt-sect">
            <h2>{{ $t('全部归档') }}</h2>
            <span class="bt-rule"></span>
            <span class="bt-sect-note">{{ posts.length }} {{ $t('篇 ·') }} {{ groups.length }} {{ $t('个年份') }}</span>
          </div>

          <div class="bt-bar">
            <span class="bt-bar-info">{{ $t('按更新时间倒序，年份由 file.updated_at 派生') }}</span>
            <a class="bt-btn bt-btn-sm" :href="blogUrl">{{ $t('‹ 返回全部文章') }}</a>
          </div>

          <div v-for="g in groups" :key="g.year" class="bt-arc-group">
            <div class="bt-arc-year">
              {{ g.year }}<i>{{ g.list.length }}</i>
            </div>
            <ul class="bt-arc-list">
              <li v-for="p in g.list" :key="p.token || p.path">
                <a class="bt-arc-a" :href="postHref(ctx, p)">
                  <span class="bt-arc-date">{{ postDate(p, 'short') }}</span>
                  <span class="bt-arc-title">{{ postTitle(p) }}</span>
                  <span class="bt-chip" :class="catColor(catName(p))">{{ catName(p) }}</span>
                  <span class="bt-meta bt-arc-size">{{ postSize(p) || '—' }}</span>
                </a>
              </li>
            </ul>
          </div>
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
    <AskWidget />
</template>

<script setup>
import AskWidget from '../AskWidget.vue'
import { computed } from 'vue'
import BrutalHead from './BrutalHead.vue'
import BrutalSide from './BrutalSide.vue'
import BrutalFoot from './BrutalFoot.vue'
import './style.css'
import {
  LIST_HREF,
  catColor,
  catName,
  postDate,
  postHref,
  postSize,
  postTitle,
  useBlog,
  yearGroups,
} from './helpers.js'

const ctx = useBlog()
const posts = computed(() => ctx.value.posts || [])
const loading = computed(() => !!ctx.value.loading)
const error = computed(() => ctx.value.error || '')
const siteName = computed(() => ctx.value.siteName || '')
const blogUrl = computed(() => ctx.value.blogUrl || LIST_HREF)

const groups = computed(() => yearGroups(posts.value))
</script>
