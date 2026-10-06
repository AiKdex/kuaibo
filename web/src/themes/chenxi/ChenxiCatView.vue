<template>
  <div class="cx-root">
    <header class="cx-top"><div class="cx-top-in">
      <div class="cx-logo"><div class="cx-logo-dot">☀️</div><span>{{ ctx.siteName||'晨曦笔记' }}</span></div>
      <nav class="cx-nav"><a :href="homeHref" class="cx-nav-a">{{ $t('首页') }}</a><ThemeSwitch /></nav>
    </div></header>
    <section class="cx-hero"><div class="cx-hero-card"><div class="cx-hero-inner"><div class="cx-hero-text">
      <h1 class="cx-hero-h1">{{ $t('分类：') }}<em>{{ cat }}</em></h1><p class="cx-hero-p">{{ list.length }} {{ $t('篇文章') }}</p>
    </div></div></div></section>
    <div class="cx-main-wrap"><main>
      <div v-if="list.length" class="cx-post-list">
        <a v-for="p in list" :key="p.token" class="cx-post-card" :href="postHref(p)">
          <div class="cx-post-thumb" :style="thumbStyle(p)"><div class="cx-thumb-inner">📁</div></div>
          <div class="cx-post-body">
            <div class="cx-post-cats"><span class="cx-post-cat cx-c-pink">{{ cat }}</span></div>
            <h2>{{ postTitle(p) }}</h2><p>{{ p.preview||$t("暂无摘要") }}</p>
            <div class="cx-post-meta"><span>📅 {{ postDate(p) }}</span></div>
          </div>
        </a>
      </div>
      <p v-else class="cx-status">{{ $t('该分类下暂无文章') }}</p>
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
const page=computed(()=>ctx.value.page||{});const cat=computed(()=>page.value.cat||'')
const year=new Date().getFullYear();const homeHref='/app#/blog?view=public'
const list=computed(()=>(ctx.value.posts||[]).filter(p=>{const c=(p.path||'').includes('/')?p.path.split('/')[0]:'';return c===cat.value}))
function postTitle(p){const n=p.file?.name||p.path||'';return String(n).replace(/\.(md|markdown)$/i,'').split('/').pop()|| t('common.unnamed')}
function postDate(p){const v=p.file?.updated_at;if(!v)return'';const ms=typeof v==='number'?(v>1e12?v:v*1000):Date.parse(v);if(!Number.isFinite(ms))return'';const d=new Date(ms);const z=n=>String(n).padStart(2,'0');return`${d.getFullYear()}-${z(d.getMonth()+1)}-${z(d.getDate())}`}
function postHref(p){return ctx.value.postUrl?ctx.value.postUrl(p):'#'}
const grads=['linear-gradient(135deg,#fce7f3,#e0e7ff)','linear-gradient(135deg,#e0e7ff,#c7d2fe)','linear-gradient(135deg,#d1fae5,#a7f3d0)','linear-gradient(135deg,#fef3c7,#fde68a)','linear-gradient(135deg,#e0f2fe,#bae6fd)']
function thumbStyle(p){let h=0;const s=cat.value;for(let i=0;i<s.length;i++)h=((h<<5)-h+s.charCodeAt(i))|0;return{background:grads[Math.abs(h)%grads.length]}}
</script>
