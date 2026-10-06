<template>
  <div class="an-root">
    <header class="an-top"><div class="an-top-in">
      <div class="an-logo"><span class="an-logo-icon">◆</span><span>{{ ctx.siteName || '好站导航' }}</span></div>
      <nav class="an-nav"><a class="an-nav-btn an-on" :href="homeHref">{{ $t('全部') }}</a><ThemeSwitch /></nav>
    </div></header>
    <section class="an-hero"><h1 class="an-hero-h1">{{ $t('分类：') }}{{ cat }}</h1><p class="an-hero-p">{{ list.length }} {{ $t('篇文章') }}</p></section>
    <div class="an-wrap"><main class="an-main">
      <div v-if="list.length" class="an-grid">
        <a v-for="p in list" :key="p.token" class="an-card" :href="postHref(p)">
          <div class="an-card-icon"><span>📁</span></div>
          <div class="an-card-info">
            <h3 class="an-card-h3">{{ postTitle(p) }}</h3>
            <p class="an-card-desc">{{ p.preview || $t("暂无摘要") }}</p>
            <div class="an-card-meta"><span class="an-tag an-tag-blue">{{ cat }}</span><span class="an-tag an-tag-green">{{ postDate(p) }}</span></div>
          </div><span class="an-arrow">→</span>
        </a>
      </div>
      <p v-else class="an-empty">{{ $t('该分类下暂无文章') }}</p>
      <button type="button" class="an-back-btn" @click="goBack">{{ $t('← 返回列表') }}</button>
    </main></div>
    <footer class="an-footer"><p>© {{ year }} {{ ctx.siteName || '好站导航' }} · Powered by AiKlog</p></footer>
  </div>
    <AskWidget />
</template>
<script setup>
import { t } from '@/i18n'
import AskWidget from '../AskWidget.vue'
import { computed, inject } from 'vue'
import ThemeSwitch from '@/components/ThemeSwitch.vue'
import './style.css'
const raw = inject('themeContext')
const ctx = computed(() => raw?.value || raw || { posts: [] })
const page = computed(() => ctx.value.page || {})
const cat = computed(() => page.value.cat || '')
const year = new Date().getFullYear()
const homeHref = '/app#/blog?view=public'
const list = computed(() => (ctx.value.posts || []).filter(p => { const c = (p.path||'').includes('/') ? p.path.split('/')[0] : '' ; return c === cat.value }))
function postTitle(p) { const n = p.file?.name || p.path || '' ; return String(n).replace(/\.(md|markdown)$/i,'').split('/').pop() || t('common.unnamed') }
function postDate(p) { const v = p.file?.updated_at ; if(!v) return '' ; const ms = typeof v==='number'?(v>1e12?v:v*1000):Date.parse(v) ; if(!Number.isFinite(ms)) return '' ; const d=new Date(ms) ; const z=n=>String(n).padStart(2,'0') ; return `${d.getFullYear()}-${z(d.getMonth()+1)}-${z(d.getDate())}` }
function postHref(p) { return ctx.value.postUrl ? ctx.value.postUrl(p) : '#' }
function goBack() { window.location.href = homeHref }
</script>