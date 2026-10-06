<template>
  <div class="jd-root">
    <div class="jd-bg-ambient"></div><div class="jd-bg-noise"></div>
    <header class="jd-topbar"><div class="jd-topbar-in">
      <a class="jd-logo" :href="homeHref"><span class="jd-logo-dot">●</span><span>{{ ctx.siteName || 'AiKlog' }}</span></a>
      <nav class="jd-nav"><a :href="homeHref" class="jd-nav-a">{{ $t('← 返回') }}</a><ThemeSwitch /></nav>
    </div></header>
    <section class="jd-hero"><h1 class="jd-hero-h1"><span class="jd-hero-line">{{ $t('标签：') }}{{ tag }}</span></h1></section>
    <div class="jd-content"><div class="jd-wrapper"><main class="jd-main">
      <p class="jd-status">{{ $t('标签筛选功能即将上线') }}</p>
    </main></div></div>
    <footer class="jd-footer"><div class="jd-footer-brand"><span class="jd-logo-dot">●</span> {{ ctx.siteName || 'AiKlog' }}</div><p class="jd-footer-copy">© {{ year }} · Powered by AiKlog</p></footer>
  </div>
    <AskWidget />
</template>
<script setup>
import AskWidget from '../AskWidget.vue'
import { computed, inject } from 'vue'
import ThemeSwitch from '@/components/ThemeSwitch.vue'
import './style.css'
const raw = inject('themeContext')
const ctx = computed(() => raw?.value || raw || { posts: [] })
const page = computed(() => ctx.value.page || {})
const tag = computed(() => page.value.tag || '')
const year = new Date().getFullYear()
const homeHref = '/app#/blog?view=public'
</script>