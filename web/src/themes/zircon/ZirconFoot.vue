<template>
  <footer class="zc-foot">
    <div class="zc-foot-in">
      <div class="zc-foot-col">
        <div class="zc-foot-t">{{ siteName }}</div>
        <p class="zc-foot-p">{{ siteDesc }}</p>
      </div>
      <div class="zc-foot-col">
        <div class="zc-foot-t">{{ $t('订阅') }}</div>
        <a class="zc-foot-link" :href="rssHref">{{ $t('RSS 订阅') }}</a>
      </div>
      <div class="zc-foot-col">
        <div class="zc-foot-t">{{ $t('版本') }}</div>
        <a class="zc-foot-link" :href="listHref">{{ $t('交互版列表') }}</a>
        <a class="zc-foot-link" :href="ssrHref">{{ $t('静态页列表') }}</a>
      </div>
    </div>
    <div class="zc-foot-bar">
      <span class="zc-foot-note">{{ $t('青璃 Zircon 主题 · 青绿胶囊') }}</span>
      <span class="zc-foot-note">{{ $t('目录即站点，文件即文章') }}</span>
    </div>
  </footer>
</template>

<script setup>
/**
 * ZirconFoot —— 主题页脚（深色三列），与页头同构、全站共用。
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
