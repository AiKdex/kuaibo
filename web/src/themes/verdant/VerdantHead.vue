<template>
  <header class="vd-topbar">
    <div class="vd-wrap vd-topbar-in">
      <a class="vd-brand" :href="blogUrl">
        <span class="vd-brand-mark" aria-hidden="true"></span>
        <span class="vd-brand-txt">
          <!-- 站名用 div/span，不占 h1（规范 §5.3） -->
          <span class="vd-brand-name">{{ siteName || '爱库录' }}</span>
          <span class="vd-brand-sub">{{ slogan }}</span>
        </span>
      </a>

      <div class="vd-head-right">
        <div v-if="$slots.stats" class="vd-stats"><slot name="stats" /></div>
        <nav class="vd-nav">
          <ThemeSwitch />
          <a :href="blogUrl">{{ $t('全部文章') }}</a>
          <a :href="rssHref">RSS</a>
        </nav>
      </div>
    </div>
    <slot name="subbar" />
  </header>
</template>

<script setup>
import ThemeSwitch from '@/components/ThemeSwitch.vue'
import { LIST_HREF, RSS_HREF } from './helpers.js'

defineProps({
  siteName: { type: String, default: '' },
  slogan: { type: String, default: 'VERDANT · 把阳光变成日常' },
  blogUrl: { type: String, default: LIST_HREF },
})

const rssHref = RSS_HREF
</script>
