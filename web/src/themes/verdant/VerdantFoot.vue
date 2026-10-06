<template>
  <footer class="vd-foot">
    <div class="vd-wrap vd-foot-in">
      <div class="vd-foot-col">
        <div class="vd-foot-brand">
          <span class="vd-brand-mark" aria-hidden="true"></span>
          <span class="vd-foot-name">{{ siteName || '爱库录' }}</span>
        </div>
        <p class="vd-foot-desc">这里是青野的博客：记录产品、工程与可持续上的真实进展。</p>
      </div>

      <div class="vd-foot-col">
        <div class="vd-foot-h">{{ $t('开始阅读') }}</div>
        <a class="vd-foot-link" :href="blogUrl">{{ $t('全部文章') }}</a>
        <a class="vd-foot-link" :href="rssHref">{{ $t('RSS 订阅') }}</a>
        <a class="vd-foot-link" href="/blog">{{ $t('静态页（SEO）') }}</a>
      </div>

      <div class="vd-foot-col">
        <div class="vd-foot-h">{{ $t('分类') }}</div>
        <button
          v-for="c in cats.slice(0, 4)"
          :key="c.name"
          type="button"
          class="vd-foot-link"
          @click="$emit('pick-cat', c.name)"
        >
          {{ c.name }}（{{ c.count }}）
        </button>
        <div v-if="!cats.length" class="vd-foot-desc">{{ $t('暂无分类') }}</div>
      </div>
    </div>

    <div class="vd-wrap vd-foot-bottom">
      <span class="vd-foot-copy">© {{ year }} {{ siteName || '爱库录' }}</span>
      <span class="vd-foot-copy">{{ $t('共') }} {{ total }} {{ $t('篇文章 · 由 AiKlog 驱动') }}</span>
    </div>
  </footer>
</template>

<script setup>
import { computed } from 'vue'
import { LIST_HREF, RSS_HREF, catCounts } from './helpers.js'

const props = defineProps({
  siteName: { type: String, default: '' },
  blogUrl: { type: String, default: LIST_HREF },
  posts: { type: Array, default: () => [] },
})

defineEmits(['pick-cat'])

const rssHref = RSS_HREF
const year = new Date().getFullYear()
const cats = computed(() => catCounts(props.posts))
const total = computed(() => props.posts.length)
</script>
