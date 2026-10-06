<template>
  <div class="zc-root">
    <header class="zc-top"><div class="zc-top-in">
      <div class="zc-logo"><div class="zc-logo-icon">📚</div><span>{{ ctx.siteName || '知藏' }}</span></div>
      <nav class="zc-nav"><a class="zc-nav-a" :href="homeHref">{{ $t('发现') }}</a><ThemeSwitch /></nav>
    </div></header>
    <section class="zc-hero"><div class="zc-hero-text"><h1 class="zc-hero-h1">{{ $t('标签：') }}<em>{{ tag }}</em></h1></div></section>
    <div class="zc-main-wrap"><main class="zc-main">
      <p class="zc-status">{{ $t('标签筛选功能即将上线') }}</p>
    </main></div>
    <footer class="zc-footer"><div class="zc-footer-brand">📚 {{ ctx.siteName || '知藏' }}</div><p class="zc-footer-copy">© {{ year }} · Powered by AiKlog</p></footer>
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