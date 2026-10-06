<template>
  <div class="an-root">
    <header class="an-top"><div class="an-top-in">
      <div class="an-logo"><span class="an-logo-icon">◆</span><span>{{ ctx.siteName || '好站导航' }}</span></div>
      <nav class="an-nav"><a class="an-nav-btn" :href="homeHref">{{ $t('全部') }}</a><ThemeSwitch /></nav>
    </div></header>
    <section class="an-hero"><h1 class="an-hero-h1">{{ $t('标签：') }}{{ tag }}</h1></section>
    <div class="an-wrap"><main class="an-main">
      <p class="an-empty">{{ $t('标签筛选功能即将上线') }}</p>
      <button type="button" class="an-back-btn" @click="goBack">{{ $t('← 返回列表') }}</button>
    </main></div>
    <footer class="an-footer"><p>© {{ year }} {{ ctx.siteName || '好站导航' }} · Powered by AiKlog</p></footer>
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
function goBack() { window.location.href = homeHref }
</script>