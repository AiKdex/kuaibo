<template>
  <div class="cx-root">
    <header class="cx-top"><div class="cx-top-in">
      <div class="cx-logo"><div class="cx-logo-dot">☀️</div><span>{{ ctx.siteName||'晨曦笔记' }}</span></div>
      <nav class="cx-nav"><a :href="homeHref" class="cx-nav-a">{{ $t('首页') }}</a><ThemeSwitch /></nav>
    </div></header>
    <section class="cx-hero"><div class="cx-hero-card"><div class="cx-hero-inner"><div class="cx-hero-text">
      <h1 class="cx-hero-h1">{{ $t('标签：') }}<em>{{ tag }}</em></h1>
    </div></div></div></section>
    <div class="cx-main-wrap"><main><p class="cx-status">{{ $t('标签筛选功能即将上线') }}</p></main></div>
    <footer class="cx-footer"><div class="cx-footer-brand">☀️ {{ ctx.siteName||'晨曦笔记' }}</div><p class="cx-footer-copy">© {{ year }} · Powered by AiKlog</p></footer>
  </div>
    <AskWidget />
</template>
<script setup>
import AskWidget from '../AskWidget.vue'
import { computed, inject } from 'vue'
import ThemeSwitch from '@/components/ThemeSwitch.vue'
import './style.css'
const raw=inject('themeContext');const ctx=computed(()=>raw?.value||raw||{posts:[]})
const page=computed(()=>ctx.value.page||{});const tag=computed(()=>page.value.tag||'')
const year=new Date().getFullYear();const homeHref='/app#/blog?view=public'
</script>
