<template>
  <footer class="bt-foot">
    <div class="bt-wrap bt-foot-grid">
      <div>
        <span class="bt-foot-mark">{{ $t('粗') }}</span>
        <div class="bt-foot-name">{{ siteName || '爱库录' }}</div>
        <p class="bt-foot-desc">{{ $t("硬边框、硬阴影、不用渐变。这里只有文章本身。") }}</p>
      </div>
      <div>
        <div class="bt-foot-h">{{ $t('站点') }}</div>
        <ul>
          <li><a :href="blogUrl">{{ $t('全部文章') }}</a></li>
          <li><a :href="rssHref">{{ $t('RSS 订阅') }}</a></li>
          <li><a :href="homeHref">{{ $t('回到首页') }}</a></li>
        </ul>
      </div>
      <div>
        <div class="bt-foot-h">{{ $t('关于') }}</div>
        <ul>
          <li><span class="bt-muted">{{ $t('目录即分类，文件名即标题') }}</span></li>
          <li><span class="bt-muted">{{ $t('正文以 Markdown 撰写') }}</span></li>
        </ul>
      </div>
    </div>
    <div class="bt-wrap bt-foot-bottom">
      <span>{{ $t('Brutal 主题 · 新粗野风') }}</span>
      <span class="bt-foot-dots"><i></i><i></i><i></i><i></i></span>
      <span>Powered by AiKlog</span>
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
const homeHref = '/'
</script>
