<template>
  <div class="cx-root">
    <header class="cx-top"><div class="cx-top-in">
      <div class="cx-logo"><div class="cx-logo-dot">☀️</div><span>{{ ctx.siteName||'晨曦笔记' }}</span></div>
      <nav class="cx-nav"><a :href="homeHref" class="cx-nav-a">{{ $t('首页') }}</a><ThemeSwitch /></nav>
    </div></header>
    <section class="cx-hero"><div class="cx-hero-card"><div class="cx-hero-inner"><div class="cx-hero-text">
      <h1 class="cx-hero-h1">{{ $t('作者：') }}<em>{{ author }}</em></h1><p class="cx-hero-p">{{ list.length }} {{ $t('篇文章') }}</p>
    </div></div></div></section>
    <div class="cx-main-wrap"><main>
      <div v-if="list.length" class="cx-post-list">
        <a v-for="p in list" :key="p.token" class="cx-post-card" :href="postHref(p)">
          <div class="cx-post-thumb" style="background:linear-gradient(135deg,#ede9fe,#ddd6fe)"><div class="cx-thumb-inner">👤</div></div>
          <div class="cx-post-body">
            <h2>{{ postTitle(p) }}</h2><p>{{ p.preview||$t("暂无摘要") }}</p>
            <div class="cx-post-meta"><span>📅 {{ postDate(p) }}</span></div>
          </div>
        </a>
      </div>
      <p v-else class="cx-status">{{ $t('该作者暂无文章') }}</p>
    </main></div>
    <footer class="cx-footer"><div class="cx-footer-brand">☀️ {{ ctx.siteName||'晨曦笔记' }}</div><p class="cx-footer-copy">© {{ year }} · Powered by AiKlog</p></footer>
  </div>
    <AskWidget />
</template>
<script setup>
import { t } from '@/i18n'
import AskWidget from '../AskWidget.vue'
import { computed, inject } from 'vue'
import ThemeSwitch from '@/components/ThemeSwitch.vue'
import './style.css'
const raw=inject('themeContext');const ctx=computed(()=>raw?.value||raw||{posts:[]})
const page=computed(()=>ctx.value.page||{});const author=computed(()=>page.value.author||'')
const year=new Date().getFullYear();const homeHref='/app#/blog?view=public'
const list=computed(()=>(ctx.value.posts||[]).filter(p=>{const a=p.file?.author?.name||'';return a===author.value}))
function postTitle(p){const n=p.file?.name||p.path||'';return String(n).replace(/\.(md|markdown)$/i,'').split('/').pop()|| t('common.unnamed')}
function postDate(p){const v=p.file?.updated_at;if(!v)return'';const ms=typeof v==='number'?(v>1e12?v:v*1000):Date.parse(v);if(!Number.isFinite(ms))return'';const d=new Date(ms);const z=n=>String(n).padStart(2,'0');return`${d.getFullYear()}-${z(d.getMonth()+1)}-${z(d.getDate())}`}
function postHref(p){return ctx.value.postUrl?ctx.value.postUrl(p):'#'}
</script>
