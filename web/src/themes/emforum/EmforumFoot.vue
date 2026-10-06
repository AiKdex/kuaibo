<template>
  <!-- 页脚（各页共用，保证换页不割裂） -->
  <footer class="ef-footer">
    <div class="ef-wrap">
      <div class="ef-foot-links">
        <a :href="blogUrl">{{ $t('文章') }}</a>
        <a :href="rssHref">RSS</a>
        <a href="/api/v1/blog/sitemap.xml">Sitemap</a>
      </div>
      <p>© {{ year }} {{ siteName || '爱库录' }} · Powered by AiKlog · Theme EmForum</p>
    </div>
  </footer>
</template>

<script setup>
import { LIST_HREF, RSS_HREF } from './helpers.js'

defineProps({
  siteName: { type: String, default: '' },
  blogUrl: { type: String, default: LIST_HREF },
})

const rssHref = RSS_HREF
const year = new Date().getFullYear()
</script>
