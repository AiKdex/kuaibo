<template>
  <div class="vd-root">
    <VerdantHead :site-name="siteName" :blog-url="blogUrl" :slogan="$t('ARCHIVE · 归档')" />

    <div v-if="loading" class="vd-wrap"><div class="vd-state">{{ $t('正在加载文章…') }}</div></div>
    <div v-else-if="error" class="vd-wrap"><div class="vd-state vd-state-err">{{ error }}</div></div>

    <template v-else>
      <section class="vd-sub-hero">
        <div class="vd-wrap">
          <div class="vd-sect">
            <h2 class="vd-sect-t">{{ $t('全部归档') }}</h2>
            <span class="vd-rule"></span>
            <span class="vd-sect-note">{{ posts.length }} {{ $t('篇 ·') }} {{ groups.length }} {{ $t('年') }}</span>
          </div>
          <p class="vd-sub-d">{{ $t('按更新时间（') }}<code>file.updated_at</code>{{ $t("）分组，由前端从公开文章列表派生。") }}</p>

          <!-- 年份跳转一律 button + scrollIntoView，绝不写进 href / location.hash（规范 §9.3） -->
          <div v-if="groups.length > 1" class="vd-filters">
            <button
              v-for="g in groups"
              :key="g.year"
              type="button"
              class="vd-chipbtn"
              @click="jumpTo(g.year)"
            >
              {{ g.year }}<i>{{ g.list.length }}</i>
            </button>
          </div>
        </div>
      </section>

      <section class="vd-body">
        <div class="vd-wrap">
          <div v-if="!groups.length" class="vd-state">{{ $t('还没有公开文章') }}</div>

          <section v-for="g in groups" :id="'vd-y-' + g.year" :key="g.year" class="vd-archive-year">
            <div class="vd-year-head">
              <span class="vd-year-n">{{ g.year }}</span>
              <span class="vd-year-c">{{ g.list.length }} {{ $t('篇') }}</span>
            </div>
            <ul class="vd-year-list">
              <li v-for="p in g.list" :key="p.token || p.path" class="vd-year-item">
                <span class="vd-year-date">{{ postDate(p, 'short') }}</span>
                <a class="vd-year-title" :href="postHref(ctx, p)">{{ postTitle(p) }}</a>
                <span class="vd-year-cat">{{ catName(p) }}</span>
              </li>
            </ul>
          </section>
        </div>
      </section>
    </template>

    <VerdantFoot :site-name="siteName" :blog-url="blogUrl" :posts="posts" />
  </div>
    <AskWidget />
</template>

<script setup>
import AskWidget from '../AskWidget.vue'
import { computed } from 'vue'
import VerdantHead from './VerdantHead.vue'
import VerdantFoot from './VerdantFoot.vue'
import './style.css'
import { LIST_HREF, catName, postDate, postHref, postTitle, useBlog, yearGroups } from './helpers.js'

const ctx = useBlog()
const posts = computed(() => ctx.value.posts || [])
const loading = computed(() => !!ctx.value.loading)
const error = computed(() => ctx.value.error || '')
const siteName = computed(() => ctx.value.siteName || '')
const blogUrl = computed(() => ctx.value.blogUrl || LIST_HREF)

const groups = computed(() => yearGroups(posts.value))

/* 年份锚点跳转：只滚动，不触碰 location.hash */
function jumpTo(year) {
  const el = document.getElementById('vd-y-' + year)
  if (el) el.scrollIntoView({ behavior: 'smooth', block: 'start' })
}
</script>
