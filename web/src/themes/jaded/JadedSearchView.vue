<template>
  <div class="jd-root">
    <div class="jd-bg-ambient"></div><div class="jd-bg-noise"></div>
    <header class="jd-topbar"><div class="jd-topbar-in">
      <a class="jd-logo" :href="homeHref"><span class="jd-logo-dot">●</span><span>{{ ctx.siteName || 'AiKlog' }}</span></a>
      <nav class="jd-nav"><a :href="homeHref" class="jd-nav-a">{{ $t('← 返回') }}</a><ThemeSwitch /></nav>
    </div></header>
    <section class="jd-hero"><h1 class="jd-hero-h1"><span class="jd-hero-line">{{ q ? $t("搜索「") + q + '」' : $t("搜索") }}</span></h1><p class="jd-hero-p">{{ list.length }} {{ $t('条结果') }}</p></section>
    <div class="jd-content"><div class="jd-wrapper"><main class="jd-main">
      <a v-for="p in list" :key="p.token" class="jd-card" :href="postHref(p)">
        <div class="jd-card-img" style="background:linear-gradient(135deg,#1a1a2a,#0d0d33)"><span class="jd-card-emoji">🔍</span></div>
        <div class="jd-card-body">
          <div class="jd-card-tags"><span class="jd-chip">{{ postCat(p) || $t("未分类") }}</span></div>
          <h2 class="jd-card-h2">{{ postTitle(p) }}</h2>
          <p class="jd-card-desc">{{ p.preview || $t("暂无摘要") }}</p>
          <div class="jd-card-foot"><span>📅 {{ postDate(p) }}</span></div>
        </div>
      </a>
      <p v-if="!list.length" class="jd-status">{{ q ? $t("未找到匹配文章") : $t("请输入搜索关键词") }}</p>
    </main></div></div>
    <footer class="jd-footer"><div class="jd-footer-brand"><span class="jd-logo-dot">●</span> {{ ctx.siteName || 'AiKlog' }}</div><p class="jd-footer-copy">© {{ year }} · Powered by AiKlog</p></footer>
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
const q = computed(() => page.value.q || '')
const year = new Date().getFullYear()
const homeHref = '/app#/blog?view=public'
function postTitle(p) { const n = p.file?.name || p.path || '' ; return String(n).replace(/\.(md|markdown)$/i,'').split('/').pop() || t('common.unnamed') }
function postCat(p) { const path = p.path || '' ; return path.includes('/') ? path.split('/')[0] : '' }
function postDate(p) { const v = p.file?.updated_at ; if(!v) return '' ; const ms = typeof v==='number'?(v>1e12?v:v*1000):Date.parse(v) ; if(!Number.isFinite(ms)) return '' ; const d=new Date(ms) ; const z=n=>String(n).padStart(2,'0') ; return `${d.getFullYear()}-${z(d.getMonth()+1)}-${z(d.getDate())}` }
function postHref(p) { return ctx.value.postUrl ? ctx.value.postUrl(p) : '#' }
const list = computed(() => {
  const keyword = q.value.trim().toLowerCase() ; if (!keyword) return []
  return (ctx.value.posts || []).filter(p => postTitle(p).toLowerCase().includes(keyword) || (p.preview||'').toLowerCase().includes(keyword) || postCat(p).toLowerCase().includes(keyword))
})
</script>