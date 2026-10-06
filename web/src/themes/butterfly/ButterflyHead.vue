<template>
  <header class="bf-nav" :class="{ 'is-hide': hidden, 'is-scrolled': scrolled }">
    <div class="bf-nav-in">
      <!-- 站名用 span，不占 h1（规范 §5.3） -->
      <a class="bf-site" :href="blogUrl">
        <span class="bf-site-mark" aria-hidden="true">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round">
            <path d="M12 12c-2.2-3.4-5.2-5-8-4.6.4 3 2.4 5.6 5.4 6.6" />
            <path d="M12 12c2.2-3.4 5.2-5 8-4.6-.4 3-2.4 5.6-5.4 6.6" />
            <path d="M12 12c-.6 3.2-.2 6 1.2 8.4 1.6-2.2 2.2-5 1.6-8.4" />
            <path d="M12 12c.6 3.2.2 6-1.2 8.4" />
          </svg>
        </span>
        <span class="bf-site-txt">
          <span class="bf-site-name">{{ siteName || '爱库录' }}</span>
          <span class="bf-site-sub">{{ slogan }}</span>
        </span>
      </a>

      <nav class="bf-menu">
        <button class="bf-menu-i" :class="{ 'is-on': mode === 'list' && !catActive }" type="button" @click="$emit('go-list')">
          <svg class="bf-ic" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M3 11l9-8 9 8" /><path d="M5 9.5V20h14V9.5" /></svg>
          <span>{{ $t('首页') }}</span>
        </button>

        <button class="bf-menu-i" :class="{ 'is-on': mode === 'archive' }" type="button" @click="$emit('go-archive')">
          <svg class="bf-ic" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M3 4h18v4H3z" /><path d="M5 8v12h14V8" /><path d="M9.5 12.5h5" /></svg>
          <span>{{ $t('归档') }}</span>
        </button>

        <button class="bf-menu-i" :class="{ 'is-on': mode === 'search' }" type="button" @click="$emit('go-search')">
          <svg class="bf-ic" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><circle cx="11" cy="11" r="7.5" /><path d="M21 21l-4.4-4.4" /></svg>
          <span>{{ $t('搜索') }}</span>
        </button>

        <a class="bf-menu-i" :href="rssHref" target="_blank" rel="noopener">
          <svg class="bf-ic" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M4 11a9 9 0 0 1 9 9" /><path d="M4 4a16 16 0 0 1 16 16" /><circle cx="5.2" cy="18.8" r="1.4" fill="currentColor" stroke="none" /></svg>
          <span>RSS</span>
        </a>

        <ThemeSwitch />

        <button class="bf-menu-i bf-menu-dark" type="button" :title="dark ? $t('切换到日间模式') : $t('切换到夜间模式')" @click="$emit('toggle-dark')">
          <svg v-if="dark" class="bf-ic" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="4.6" /><path d="M12 1.8v2.6M12 19.6v2.6M4.4 4.4l1.9 1.9M17.7 17.7l1.9 1.9M1.8 12h2.6M19.6 12h2.6M4.4 19.6l1.9-1.9M17.7 6.3l1.9-1.9" /></svg>
          <svg v-else class="bf-ic" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z" /></svg>
        </button>
      </nav>
    </div>
  </header>
</template>

<script setup>
import { onBeforeUnmount, onMounted, ref } from 'vue'
import ThemeSwitch from '@/components/ThemeSwitch.vue'
import { LIST_HREF, RSS_HREF } from './helpers.js'

defineProps({
  siteName: { type: String, default: '' },
  slogan: { type: String, default: 'BUTTERFLY · 简单卡片，优雅阅读' },
  blogUrl: { type: String, default: LIST_HREF },
  mode: { type: String, default: 'list' },
  catActive: { type: Boolean, default: false },
  dark: { type: Boolean, default: false },
})

defineEmits(['go-list', 'go-archive', 'go-search', 'toggle-dark'])

const rssHref = RSS_HREF
const hidden = ref(false)
const scrolled = ref(false)
let lastY = 0

/** 顶栏「向下滚动收起、向上滚动回落」——Butterfly 的标志性交互 */
function onScroll() {
  const y = window.scrollY || 0
  scrolled.value = y > 8
  if (y <= 80) hidden.value = false
  else if (y - lastY > 6) hidden.value = true
  else if (lastY - y > 6) hidden.value = false
  lastY = y
}

onMounted(() => {
  lastY = window.scrollY || 0
  window.addEventListener('scroll', onScroll, { passive: true })
  onScroll()
})
onBeforeUnmount(() => window.removeEventListener('scroll', onScroll))
</script>
