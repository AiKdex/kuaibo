<template>
  <header class="zc-head">
    <div class="zc-head-in">
      <a class="zc-brand" :href="listHref">
        <span class="zc-brand-mark" aria-hidden="true"></span>
        <span class="zc-brand-txt">{{ siteName }}</span>
      </a>
      <nav class="zc-nav">
        <a class="zc-nav-link" :href="listHref">{{ $t('文章') }}</a>
        <a class="zc-nav-link" :href="rssHref" target="_blank" rel="noopener">RSS</a>
        <a class="zc-nav-link" href="https://aiklog.com" target="_blank" rel="noopener">{{ $t('官网') }}</a>
        <ThemeSwitch />
      </nav>
    </div>
  </header>
</template>

<script setup>
/**
 * ZirconHead —— 主题页头（毛玻璃吸顶导航），列表页与全部内页共用，
 * 保证换页不割裂。主题自带 <ThemeSwitch />，manifest 已声明 ui.themeSwitcher。
 * 站名用 <div>/<span>，不占 <h1>（h1 归文章页，规范 §5.3）。
 */
import { computed } from 'vue'
import ThemeSwitch from '@/components/ThemeSwitch.vue'
import { useBlog, LIST_HREF, RSS_HREF } from './helpers.js'

const ctx = useBlog()
const siteName = computed(() => ctx.value.siteName || '爱库录')
const listHref = LIST_HREF
const rssHref = RSS_HREF
</script>
