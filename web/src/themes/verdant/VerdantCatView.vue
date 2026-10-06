<template>
  <div class="vd-root">
    <VerdantHead :site-name="siteName" :blog-url="blogUrl" :slogan="$t('CATEGORY · 分类')" />

    <div v-if="loading" class="vd-wrap"><div class="vd-state">{{ $t('正在加载文章…') }}</div></div>
    <div v-else-if="error" class="vd-wrap"><div class="vd-state vd-state-err">{{ error }}</div></div>

    <template v-else>
      <section class="vd-sub-hero">
        <div class="vd-wrap">
          <div class="vd-sect">
            <h2 class="vd-sect-t">{{ cat || $t("未分类") }}</h2>
            <span class="vd-rule"></span>
            <span class="vd-sect-note">{{ list.length }} {{ $t('篇') }}</span>
          </div>
          <p class="vd-sub-d">{{ $t('分类由文件路径首段决定：') }}<code>{{ cat || $t("（根目录）") }}</code></p>
          <div class="vd-filters">
            <button type="button" class="vd-chipbtn" @click="pickCat('')">{{ $t('全部') }}</button>
            <button
              v-for="c in cats"
              :key="c.name"
              type="button"
              class="vd-chipbtn"
              :class="{ on: c.name === (cat || $t('未分类')) }"
              @click="pickCat(c.name)"
            >
              {{ c.name }}<i>{{ c.count }}</i>
            </button>
          </div>
        </div>
      </section>

      <section class="vd-body">
        <div class="vd-wrap">
          <div class="vd-bar">
            <span class="vd-bar-info">{{ $t('共') }} {{ list.length }} {{ $t('篇文章') }}</span>
            <a class="vd-btn vd-btn-g vd-btn-sm" :href="blogUrl">{{ $t('‹ 返回全部文章') }}</a>
          </div>

          <div v-if="list.length" class="vd-grid">
            <VerdantCard v-for="(p, i) in list" :key="p.token || p.path" :post="p" :index="i" :show-cat="false" />
          </div>
          <div v-else class="vd-state">{{ $t('该分类下暂无文章') }}</div>
        </div>
      </section>
    </template>

    <VerdantFoot :site-name="siteName" :blog-url="blogUrl" :posts="posts" />
  </div>
    <AskWidget />
</template>

<script setup>
import AskWidget from '../AskWidget.vue'
import { computed, ref } from 'vue'
import VerdantHead from './VerdantHead.vue'
import VerdantCard from './VerdantCard.vue'
import VerdantFoot from './VerdantFoot.vue'
import './style.css'
import { LIST_HREF, catCounts, catName, readPageParam, useBlog } from './helpers.js'

const ctx = useBlog()
const posts = computed(() => ctx.value.posts || [])
const loading = computed(() => !!ctx.value.loading)
const error = computed(() => ctx.value.error || '')
const siteName = computed(() => ctx.value.siteName || '')
const blogUrl = computed(() => ctx.value.blogUrl || LIST_HREF)

const cats = computed(() => catCounts(posts.value))

/* 当前分类：优先 ctx.page.cat（系统注入），回退解析 hash / ?cat= */
const pageParam = computed(() =>
  readPageParam(ctx.value, ['cat', 'category'], [/\/cat\/([^/?#]+)/, /[?&]cat=([^&#]+)/]),
)
const picked = ref('')
const cat = computed(() => picked.value || pageParam.value)

const list = computed(() => posts.value.filter((p) => catName(p) === (cat.value || '未分类')))

function pickCat(name) {
  picked.value = name
}
</script>
