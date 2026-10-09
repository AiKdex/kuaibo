<template>
  <div class="cx-root">
    <!-- Top Bar -->
    <header class="cx-top"><div class="cx-top-in">
      <div class="cx-logo"><div class="cx-logo-dot">☀️</div><span>{{ ctx.siteName || '晨曦笔记' }}</span></div>
      <nav class="cx-nav">
        <a :href="homeHref" class="cx-nav-a cx-on">{{ $t('首页') }}</a>
        <a :href="archiveHref" class="cx-nav-a">{{ $t('归档') }}</a>
        <a :href="rssHref" class="cx-nav-a">RSS</a>
        <ThemeSwitch />
      </nav>
      <div class="cx-nav-right"><div class="cx-search-trigger" @click="showSearch=true">{{ $t('🔍 搜索文章...') }}</div></div>
    </div></header>

    <!-- Search Modal -->
    <div v-if="showSearch" class="cx-search-overlay" @click.self="showSearch=false">
      <div class="cx-search-modal">
        <input ref="searchRef" v-model="searchQ" class="cx-search-input" :placeholder="$t('搜索文章标题、摘要...')" @keyup.escape="showSearch=false" />
        <div v-if="searchResults.length" class="cx-search-results">
          <a v-for="p in searchResults" :key="p.token" class="cx-search-item" :href="postHref(p)">
            <span class="cx-search-item-title">{{ postTitle(p) }}</span>
            <span class="cx-search-item-cat">{{ postCat(p)||$t("未分类") }}</span>
          </a>
        </div>
        <p v-else-if="searchQ.trim()" class="cx-search-empty">{{ $t('未找到匹配文章') }}</p>
      </div>
    </div>

    <!-- Hero -->
    <section class="cx-hero"><div class="cx-hero-card"><div class="cx-hero-inner">
      <div class="cx-hero-text">
        <h1 class="cx-hero-h1">{{ $t('记录技术') }}<br>{{ $t('与') }}<em>{{ $t('生活的点滴') }}</em></h1>
        <p class="cx-hero-p">{{ ctx.siteDesc || $t("关注互联网及软件 IT 技术的个人博客，记录成长的每一步。") }}</p>
        <div class="cx-hero-tags">
          <span v-for="c in cats.slice(0,4)" :key="c.name" class="cx-hero-tag" :class="catColorClass(c.name)">{{ catEmoji(c.name) }} {{ c.name }}</span>
        </div>
      </div>
      <div class="cx-hero-stats-card">
        <div class="cx-hero-stat-title">{{ $t('📊 博客概览') }}</div>
        <div class="cx-hero-stat-grid">
          <div class="cx-hero-stat"><div class="cx-hero-stat-icon">📝</div><span class="cx-hero-stat-num">{{ ctx.posts?.length||0 }}</span><span class="cx-hero-stat-label">{{ $t('文章') }}</span></div>
          <div class="cx-hero-stat"><div class="cx-hero-stat-icon">📂</div><span class="cx-hero-stat-num">{{ cats.length }}</span><span class="cx-hero-stat-label">{{ $t('分类') }}</span></div>
          <div class="cx-hero-stat"><div class="cx-hero-stat-icon">🕐</div><span class="cx-hero-stat-num">{{ latestDate }}</span><span class="cx-hero-stat-label">{{ $t('最近更新') }}</span></div>
          <div class="cx-hero-stat"><div class="cx-hero-stat-icon">✨</div><span class="cx-hero-stat-num">{{ $t('持续') }}</span><span class="cx-hero-stat-label">{{ $t('更新中') }}</span></div>
        </div>
      </div>
    </div></div></section>

    <!-- Category Chips -->
    <div class="cx-cats-bar">
      <button type="button" :class="['cx-chip',{'cx-on':!filterCat}]" @click="filterCat=''"><span class="cx-chip-e">✨</span> {{ $t('全部') }}</button>
      <button type="button" :class="['cx-chip',{'cx-on':filterCat===c.name}]" v-for="c in cats" :key="c.name" @click="filterCat=c.name"><span class="cx-chip-e">{{ catEmoji(c.name) }}</span> {{ c.name }}</button>
    </div>

    <!-- Status -->
    <div v-if="loading" class="cx-status"><p>{{ $t('⏳ 加载中...') }}</p></div>
    <div v-else-if="error" class="cx-status"><p>❌ {{ error }}</p></div>
    <div v-else-if="!ctx.posts?.length" class="cx-status"><p>{{ $t('📭 还没有公开文章') }}</p></div>

    <!-- Main + Sidebar -->
    <div v-else class="cx-main-wrap">
      <main>
        <div class="cx-post-list">
          <!-- Featured (first) -->
          <a v-if="featured" class="cx-post-card cx-featured" :href="postHref(featured)">
            <PostCover :post="featured" round>
              <div class="cx-post-thumb" :style="thumbStyle(featured)"><div class="cx-thumb-inner">{{ catEmoji(postCat(featured)) }}</div></div>
            </PostCover>
            <div class="cx-post-body">
              <div class="cx-post-cats"><span class="cx-post-cat" :class="catColorClass(postCat(featured))">{{ postCat(featured)||$t("未分类") }}</span></div>
              <h2>{{ postTitle(featured) }}</h2>
              <p>{{ featured.preview||$t("暂无摘要") }}</p>
              <div class="cx-post-meta"><span>📅 {{ postDate(featured) }}</span></div>
            </div>
          </a>
          <!-- Rest -->
          <a v-for="p in restPosts" :key="p.token" class="cx-post-card" :href="postHref(p)">
            <PostCover :post="p" round>
              <div class="cx-post-thumb" :style="thumbStyle(p)"><div class="cx-thumb-inner">{{ catEmoji(postCat(p)) }}</div></div>
            </PostCover>
            <div class="cx-post-body">
              <div class="cx-post-cats"><span class="cx-post-cat" :class="catColorClass(postCat(p))">{{ postCat(p)||$t("未分类") }}</span></div>
              <h2>{{ postTitle(p) }}</h2>
              <p>{{ p.preview||$t("暂无摘要") }}</p>
              <div class="cx-post-meta"><span>📅 {{ postDate(p) }}</span></div>
            </div>
          </a>
        </div>
      </main>

      <aside class="cx-side">
        <!-- About -->
        <div class="cx-sb cx-sb-about">
          <div class="cx-sb-avatar">{{ avatarLetter }}</div>
          <h3>{{ ctx.siteName||'晨曦笔记' }}</h3>
          <p>{{ ctx.siteDesc||$t("记录技术与生活的点滴。") }}</p>
          <div class="cx-sb-about-stats">
            <div class="cx-sb-about-stat"><strong>{{ ctx.posts?.length||0 }}</strong><span>{{ $t('文章') }}</span></div>
            <div class="cx-sb-about-stat"><strong>{{ cats.length }}</strong><span>{{ $t('分类') }}</span></div>
          </div>
        </div>
        <!-- Hot -->
        <div class="cx-sb"><div class="cx-sb-title">{{ $t('🔥 最新文章') }}</div>
          <ul class="cx-hot-list">
            <li v-for="(p,i) in latest.slice(0,5)" :key="p.token" class="cx-hot-item">
              <span class="cx-hot-rank">{{ i+1 }}</span>
              <div class="cx-hot-text"><div class="cx-hot-name">{{ postTitle(p) }}</div><div class="cx-hot-meta">{{ postCat(p)||$t("未分类") }} · {{ postDate(p,'short') }}</div></div>
            </li>
          </ul>
        </div>
        <!-- Tags -->
        <div v-if="ctx.tags&&ctx.tags.length" class="cx-sb"><div class="cx-sb-title">{{ $t('🏷️ 标签云') }}</div>
          <div class="cx-tag-cloud"><span v-for="t in ctx.tags" :key="t.name" class="cx-tag-item">{{ t.name }}</span></div>
        </div>
        <!-- Archives -->
        <div v-if="archives.length" class="cx-sb"><div class="cx-sb-title">{{ $t('📅 归档') }}</div>
          <ul class="cx-hot-list"><li v-for="a in archives" :key="a.label" class="cx-hot-item"><div class="cx-hot-text"><div class="cx-hot-name">{{ a.label }}</div></div><span class="cx-hot-meta">{{ a.count }}</span></li></ul>
        </div>
        <!-- Links -->
        <div class="cx-sb"><div class="cx-sb-title">{{ $t('🔗 链接') }}</div>
          <div class="cx-sb-links"><a :href="rssHref">{{ $t('📡 RSS 订阅') }}</a><a :href="homeHref">{{ $t('🏠 首页') }}</a></div>
        </div>
      </aside>
    </div>

    <!-- Footer -->
    <footer class="cx-footer">
      <div class="cx-footer-brand">☀️ {{ ctx.siteName||'晨曦笔记' }}</div>
      <div class="cx-footer-links"><a :href="rssHref">RSS</a><a :href="homeHref">{{ $t('首页') }}</a></div>
      <p class="cx-footer-copy">© {{ year }} {{ ctx.siteName||'晨曦笔记' }} · Powered by AiKlog</p>
    </footer>
  </div>
    <AskWidget />
</template>
<script setup>
import { t } from '@/i18n'
import AskWidget from '../AskWidget.vue'
import { ref, computed, inject, watch, nextTick } from 'vue'
import ThemeSwitch from '@/components/ThemeSwitch.vue'
import PostCover from '@/components/PostCover.vue'
import './style.css'
const raw=inject('themeContext')
const ctx=computed(()=>raw?.value||raw||{posts:[],tags:[],loading:true})
const loading=computed(()=>ctx.value.loading)
const error=computed(()=>ctx.value.error||'')
const year=new Date().getFullYear()
const homeHref='/app#/blog?view=public'
const rssHref='/api/v1/blog/feed.xml'
const archiveHref='/app#/blog?view=public'
function postTitle(p){const n=p.file?.name||p.path||'';return String(n).replace(/\.(md|markdown)$/i,'').split('/').pop()|| t('common.unnamed')}
function postCat(p){const path=p.path||'';return path.includes('/')?path.split('/')[0]:''}
function postDate(p,fmt){const v=p.file?.updated_at;if(!v)return'';const ms=typeof v==='number'?(v>1e12?v:v*1000):Date.parse(v);if(!Number.isFinite(ms))return'';const d=new Date(ms);const z=n=>String(n).padStart(2,'0');if(fmt==='short')return`${z(d.getMonth()+1)}-${z(d.getDate())}`;return`${d.getFullYear()}-${z(d.getMonth()+1)}-${z(d.getDate())}`}
function postHref(p){return ctx.value.postUrl?ctx.value.postUrl(p):'#'}
const cats=computed(()=>{const map={};for(const p of(ctx.value.posts||[])){const c=postCat(p);map[c]=(map[c]||0)+1}return Object.entries(map).map(([name,count])=>({name,count})).sort((a,b)=>b.count-a.count)})
const filterCat=ref('')
const avatarLetter=computed(()=>(ctx.value.siteName||'晨')[0])
const latestDate=computed(()=>{const l=latest.value;return l.length?postDate(l[0],'short'):'--'})
const featured=computed(()=>ctx.value.posts?.[0]||null)
const restPosts=computed(()=>{const posts=filterCat.value?(ctx.value.posts||[]).filter(p=>postCat(p)===filterCat.value):(ctx.value.posts||[]);return posts.slice(filterCat.value?0:1)})
function dateMs(p){const v=p.file?.updated_at;if(!v)return 0;const ms=typeof v==='number'?(v>1e12?v:v*1000):Date.parse(v);return Number.isFinite(ms)?ms:0}
const latest=computed(()=>[...(ctx.value.posts||[])].sort((a,b)=>dateMs(b)-dateMs(a)))
const archives=computed(()=>{const map={};for(const p of(ctx.value.posts||[])){const d=postDate(p);if(!d)continue;const ym=d.slice(0,7);map[ym]=(map[ym]||0)+1}return Object.entries(map).map(([label,count])=>({label:label.replace('-',' 年 ')+' 月',count,raw:label})).sort((a,b)=>b.raw.localeCompare(a.raw))})
const catEmojiMap={};const emojis=['💻','🌐','🔧','📦','💬','📢','🤖','🎨','📱','🔐','🚀','💡','📊','☁️']
function catEmoji(name){if(!catEmojiMap[name]){const idx=cats.value.findIndex(c=>c.name===name);catEmojiMap[name]=emojis[(idx>=0?idx:0)%emojis.length]}return catEmojiMap[name]}
const colorClasses=['cx-c-pink','cx-c-purple','cx-c-orange','cx-c-green','cx-c-sky']
function catColorClass(name){const idx=cats.value.findIndex(c=>c.name===name);return colorClasses[(idx>=0?idx:0)%colorClasses.length]}
const grads=['linear-gradient(135deg,#fce7f3,#e0e7ff)','linear-gradient(135deg,#e0e7ff,#c7d2fe)','linear-gradient(135deg,#d1fae5,#a7f3d0)','linear-gradient(135deg,#fef3c7,#fde68a)','linear-gradient(135deg,#e0f2fe,#bae6fd)','linear-gradient(135deg,#ede9fe,#ddd6fe)','linear-gradient(135deg,#fce7f3,#fbcfe8)']
function thumbStyle(p){let h=0;const s=postCat(p);for(let i=0;i<s.length;i++)h=((h<<5)-h+s.charCodeAt(i))|0;return{background:grads[Math.abs(h)%grads.length]}}
const showSearch=ref(false);const searchQ=ref('');const searchRef=ref(null)
const searchResults=computed(()=>{const q=searchQ.value.trim().toLowerCase();if(!q)return[];return(ctx.value.posts||[]).filter(p=>postTitle(p).toLowerCase().includes(q)||(p.preview||'').toLowerCase().includes(q)).slice(0,10)})
watch(showSearch,v=>{if(v)nextTick(()=>searchRef.value?.focus())})
</script>