<template>
  <div class="jd-root">
    <div class="jd-bg-ambient"></div><div class="jd-bg-noise"></div>
    <header class="jd-topbar"><div class="jd-topbar-in">
      <a class="jd-logo" :href="homeHref"><span class="jd-logo-dot">●</span><span>{{ ctx.siteName || 'AiKlog' }}</span></a>
      <nav class="jd-nav"><a :href="homeHref" class="jd-nav-a">{{ $t('← 返回') }}</a><ThemeSwitch /></nav>
    </div></header>
    <section class="jd-hero"><h1 class="jd-hero-h1"><span class="jd-hero-line">{{ $t('归档') }}</span></h1><p class="jd-hero-p">{{ $t('共') }} {{ ctx.posts?.length || 0 }} {{ $t('篇文章') }}</p></section>
    <div class="jd-content"><div class="jd-wrapper"><main class="jd-main">
      <div v-for="group in archives" :key="group.ym" style="margin-bottom:40px">
        <h2 style="font-size:22px;font-weight:700;color:#fff;margin-bottom:16px;display:flex;align-items:center;gap:10px">
          <span style="width:8px;height:8px;border-radius:50%;background:#2dd4a8;flex-shrink:0"></span>
          {{ group.label }}
          <span style="font-size:12px;color:#64748b;background:rgba(255,255,255,.04);padding:2px 8px;border-radius:100px">{{ group.posts.length }}</span>
        </h2>
        <a v-for="p in group.posts" :key="p.token" class="jd-card" :href="postHref(p)" style="margin-bottom:12px">
          <div class="jd-card-img" style="background:linear-gradient(135deg,#1a1a2a,#0d0d33)"><span class="jd-card-emoji">📄</span></div>
          <div class="jd-card-body">
            <h2 class="jd-card-h2">{{ postTitle(p) }}</h2>
            <p class="jd-card-desc">{{ p.preview || $t("暂无摘要") }}</p>
            <div class="jd-card-foot"><span>📅 {{ postDate(p) }}</span></div>
          </div>
        </a>
      </div>
      <p v-if="!archives.length" class="jd-status">{{ $t('暂无文章') }}</p>
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
const year = new Date().getFullYear()
const homeHref = '/app#/blog?view=public'
function postTitle(p) { const n = p.file?.name || p.path || '' ; return String(n).replace(/\.(md|markdown)$/i,'').split('/').pop() || t('common.unnamed') }
function postDate(p) { const v = p.file?.updated_at ; if(!v) return '' ; const ms = typeof v==='number'?(v>1e12?v:v*1000):Date.parse(v) ; if(!Number.isFinite(ms)) return '' ; const d=new Date(ms) ; const z=n=>String(n).padStart(2,'0') ; return `${d.getFullYear()}-${z(d.getMonth()+1)}-${z(d.getDate())}` }
function postHref(p) { return ctx.value.postUrl ? ctx.value.postUrl(p) : '#' }
const archives = computed(() => {
  const map = {} ; const posts = ctx.value.posts || []
  for (const p of posts) { const v = p.file?.updated_at ; if(!v) continue ; const ms = typeof v==='number'?(v>1e12?v:v*1000):Date.parse(v) ; if(!Number.isFinite(ms)) continue ; const d = new Date(ms) ; const ym = `${d.getFullYear()}-${String(d.getMonth()+1).padStart(2,'0')}` ; if(!map[ym]) map[ym] = { ym, label: `${d.getFullYear()} 年 ${d.getMonth()+1} 月`, posts: [] } ; map[ym].posts.push(p) }
  return Object.values(map).sort((a,b) => b.ym.localeCompare(a.ym))
})
</script>