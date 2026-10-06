<template>
  <div class="cx-root">
    <header class="cx-top"><div class="cx-top-in">
      <div class="cx-logo"><div class="cx-logo-dot">☀️</div><span>{{ ctx.siteName||'晨曦笔记' }}</span></div>
      <nav class="cx-nav"><a :href="homeHref" class="cx-nav-a">{{ $t('首页') }}</a><ThemeSwitch /></nav>
    </div></header>
    <section class="cx-hero"><div class="cx-hero-card"><div class="cx-hero-inner"><div class="cx-hero-text">
      <h1 class="cx-hero-h1"><em>{{ $t('归档') }}</em></h1><p class="cx-hero-p">{{ $t('共') }} {{ ctx.posts?.length||0 }} {{ $t('篇文章') }}</p>
    </div></div></div></section>
    <div class="cx-main-wrap"><main>
      <div v-for="group in archives" :key="group.ym" style="margin-bottom:36px">
        <div style="display:flex;align-items:center;gap:10px;margin-bottom:16px">
          <span style="width:8px;height:8px;border-radius:50%;background:#f472b6;flex-shrink:0"></span>
          <h2 style="font-size:20px;font-weight:700;color:#1e1b4b">{{ group.label }}</h2>
          <span style="font-size:11px;color:#94a3b8;background:#fff1f2;padding:2px 8px;border-radius:100px">{{ group.posts.length }}</span>
        </div>
        <div class="cx-post-list">
          <a v-for="p in group.posts" :key="p.token" class="cx-post-card" :href="postHref(p)">
            <div class="cx-post-thumb" style="background:linear-gradient(135deg,#e0f2fe,#bae6fd)"><div class="cx-thumb-inner">📄</div></div>
            <div class="cx-post-body">
              <h2>{{ postTitle(p) }}</h2><p>{{ p.preview||$t("暂无摘要") }}</p>
              <div class="cx-post-meta"><span>📅 {{ postDate(p) }}</span></div>
            </div>
          </a>
        </div>
      </div>
      <p v-if="!archives.length" class="cx-status">{{ $t('暂无文章') }}</p>
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
const year=new Date().getFullYear();const homeHref='/app#/blog?view=public'
function postTitle(p){const n=p.file?.name||p.path||'';return String(n).replace(/\.(md|markdown)$/i,'').split('/').pop()|| t('common.unnamed')}
function postDate(p){const v=p.file?.updated_at;if(!v)return'';const ms=typeof v==='number'?(v>1e12?v:v*1000):Date.parse(v);if(!Number.isFinite(ms))return'';const d=new Date(ms);const z=n=>String(n).padStart(2,'0');return`${d.getFullYear()}-${z(d.getMonth()+1)}-${z(d.getDate())}`}
function postHref(p){return ctx.value.postUrl?ctx.value.postUrl(p):'#'}
const archives=computed(()=>{const map={};for(const p of(ctx.value.posts||[])){const v=p.file?.updated_at;if(!v)continue;const ms=typeof v==='number'?(v>1e12?v:v*1000):Date.parse(v);if(!Number.isFinite(ms))continue;const d=new Date(ms);const ym=`${d.getFullYear()}-${String(d.getMonth()+1).padStart(2,'0')}`;if(!map[ym])map[ym]={ym,label:`${d.getFullYear()} 年 ${d.getMonth()+1} 月`,posts:[]};map[ym].posts.push(p)}return Object.values(map).sort((a,b)=>b.ym.localeCompare(a.ym))})
</script>
