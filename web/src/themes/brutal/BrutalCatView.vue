<template>
  <div class="brutal-root">
    <BrutalHead :site-name="siteName" :blog-url="blogUrl" :slogan="$t('CATEGORY · 分类')" />

    <div v-if="loading" class="bt-wrap"><div class="bt-state">{{ $t('正在加载文章…') }}</div></div>
    <div v-else-if="error" class="bt-wrap"><div class="bt-state bt-state-err">{{ error }}</div></div>

    <template v-else>
      <section class="bt-posts">
        <div class="bt-wrap">
          <div class="bt-sect">
            <h2>{{ cat || $t("未分类") }}</h2>
            <span class="bt-rule"></span>
            <span class="bt-sect-note">{{ list.length }} {{ $t('篇') }}</span>
          </div>

          <p class="bt-lead">{{ $t('分类由文件路径首段决定：') }}<code>{{ cat || $t("（根目录）") }}</code></p>

          <!-- 分类切换 -->
          <div v-if="cats.length > 1" class="bt-filters">
            <button type="button" class="bt-chipbtn" @click="pickCat('')">{{ $t('全部') }}</button>
            <button
              v-for="c in cats"
              :key="c.name"
              type="button"
              class="bt-chipbtn"
              :class="{ on: c.name === (cat || $t('未分类')) }"
              @click="pickCat(c.name)"
            >
              {{ c.name }}<i>{{ c.count }}</i>
            </button>
          </div>

          <div class="bt-bar">
            <span class="bt-bar-info">{{ $t('共') }} {{ list.length }} {{ $t('篇文章') }}</span>
            <a class="bt-btn bt-btn-sm" :href="blogUrl">{{ $t('‹ 返回全部文章') }}</a>
          </div>

          <div v-if="list.length" class="bt-grid">
            <BrutalCard v-for="(p, i) in list" :key="p.token || p.path" :post="p" :index="i" :show-cat="false" />
          </div>
          <div v-else class="bt-state">{{ $t('该分类下暂无文章') }}</div>
        </div>
      </section>

      <section class="bt-sidewrap">
        <div class="bt-wrap">
          <BrutalSide @pick-cat="pickCat" />
        </div>
      </section>
    </template>

    <BrutalFoot :site-name="siteName" :blog-url="blogUrl" />
  </div>
    <AskWidget />
</template>

<script setup>
import AskWidget from '../AskWidget.vue'
import { computed, ref } from 'vue'
import BrutalHead from './BrutalHead.vue'
import BrutalCard from './BrutalCard.vue'
import BrutalSide from './BrutalSide.vue'
import BrutalFoot from './BrutalFoot.vue'
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
