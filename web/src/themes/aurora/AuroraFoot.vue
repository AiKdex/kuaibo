<template>
  <footer class="au-foot">
    <div class="au-foot-in">
      <div class="au-foot-col">
        <div class="au-foot-t">{{ siteName }}</div>
        <p class="au-foot-p">{{ siteDesc }}</p>
      </div>
      <div class="au-foot-col">
        <div class="au-foot-t">{{ $t('订阅') }}</div>
        <a class="au-foot-link" :href="rssHref">{{ $t('RSS 订阅') }}</a>
      </div>
      <div class="au-foot-col">
        <div class="au-foot-t">{{ $t('版本') }}</div>
        <a class="au-foot-link" :href="listHref">{{ $t('交互版列表') }}</a>
        <a class="au-foot-link" :href="ssrHref">{{ $t('静态页列表') }}</a>
      </div>
    </div>
    <div class="au-foot-bar">
      <span class="au-foot-note">{{ $t('极光 Aurora 主题 · Bento Grid') }}</span>
      <span class="au-foot-note">{{ $t('目录即站点，文件即文章') }}</span>
    </div>
  </footer>
</template>

<script setup>
/**
 * AuroraFoot —— 主题页脚，与页头同构、全站共用。
 * 三个链接都是相对路径或站内 SPA 路由常量（不写死锚点）。
 */
import { computed } from 'vue'
import { useBlog, LIST_HREF, SSR_LIST_HREF, RSS_HREF } from './helpers.js'

const ctx = useBlog()
const siteName = computed(() => ctx.value.siteName || '爱库录')
const siteDesc = computed(() => ctx.value.siteDesc || '目录即站点，文件即文章')
const listHref = LIST_HREF
const ssrHref = SSR_LIST_HREF
const rssHref = RSS_HREF
</script>
