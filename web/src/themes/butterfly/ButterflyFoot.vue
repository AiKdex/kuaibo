<template>
  <footer class="bf-foot">
    <!-- 顶部波浪：Butterfly 页脚的标志性过渡 -->
    <svg class="bf-foot-wave" viewBox="0 0 1440 60" preserveAspectRatio="none" aria-hidden="true">
      <path d="M0 34c180-26 340 14 520 14s340-42 520-30 260 26 400 18v24H0z" />
    </svg>

    <div class="bf-foot-in">
      <div class="bf-foot-brand">
        <span class="bf-foot-mark" aria-hidden="true">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round">
            <path d="M12 12c-2.2-3.4-5.2-5-8-4.6.4 3 2.4 5.6 5.4 6.6" />
            <path d="M12 12c2.2-3.4 5.2-5 8-4.6-.4 3-2.4 5.6-5.4 6.6" />
            <path d="M12 12c-.6 3.2-.2 6 1.2 8.4 1.6-2.2 2.2-5 1.6-8.4" />
          </svg>
        </span>
        <span class="bf-foot-name">{{ siteName || '爱库录' }}</span>
      </div>

      <nav class="bf-foot-nav">
        <a :href="blogUrl">{{ $t('全部文章') }}</a>
        <button type="button" @click="$emit('go-archive')">{{ $t('归档') }}</button>
        <button type="button" @click="$emit('go-search')">{{ $t('检索') }}</button>
        <a :href="rssHref" target="_blank" rel="noopener">RSS</a>
        <a href="/blog">{{ $t('静态页') }}</a>
      </nav>

      <p class="bf-foot-meta">
        <span>{{ count }} {{ $t('篇文章') }}</span>
        <span class="bf-foot-sep">·</span>
        <span>{{ $t('目录即站点，文件即文章') }}</span>
      </p>
    </div>
  </footer>
</template>

<script setup>
import { LIST_HREF, RSS_HREF } from './helpers.js'

defineProps({
  siteName: { type: String, default: '' },
  blogUrl: { type: String, default: LIST_HREF },
  count: { type: Number, default: 0 },
})
defineEmits(['go-archive', 'go-search'])

const rssHref = RSS_HREF
</script>
