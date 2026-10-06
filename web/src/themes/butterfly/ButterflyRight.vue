<template>
  <div class="bf-right" :class="{ 'is-on': visible }">
    <!-- 目录（仅文章页提供 toc 时出现） -->
    <button v-if="toc && toc.length" class="bf-right-btn" type="button" :title="$t('目录')" @click="tocOpen = !tocOpen">
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round"><path d="M4 6.5h10M4 12h16M4 17.5h7" /></svg>
    </button>

    <div v-if="toc && toc.length && tocOpen" class="bf-toc">
      <button
        v-for="(h, i) in toc"
        :key="i"
        class="bf-toc-i"
        :class="{ sub: h.level === 3 }"
        type="button"
        @click="$emit('scroll-to', i)"
      >{{ h.text }}</button>
    </div>

    <!-- 夜间模式 -->
    <button class="bf-right-btn" type="button" :title="dark ? $t('日间模式') : $t('夜间模式')" @click="$emit('toggle-dark')">
      <svg v-if="dark" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="4.4" /><path d="M12 2.4v2.4M12 19.2v2.4M4.8 4.8l1.7 1.7M17.5 17.5l1.7 1.7M2.4 12h2.4M19.2 12h2.4M4.8 19.2l1.7-1.7M17.5 6.5l1.7-1.7" /></svg>
      <svg v-else viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z" /></svg>
    </button>

    <!-- 回到顶部 + 滚动进度环 -->
    <button class="bf-right-btn bf-right-top" type="button" :title="$t('回到顶部')" @click="toTop">
      <svg class="bf-ring" viewBox="0 0 40 40" aria-hidden="true">
        <circle class="bf-ring-bg" cx="20" cy="20" r="16" />
        <circle
          class="bf-ring-fg"
          cx="20" cy="20" r="16"
          :stroke-dasharray="C"
          :stroke-dashoffset="C * (1 - percent / 100)"
        />
      </svg>
      <svg class="bf-ring-ic" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 19V5" /><path d="M5.5 11.5 12 5l6.5 6.5" /></svg>
    </button>
  </div>
</template>

<script setup>
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { scrollToTop } from './helpers.js'

defineProps({
  dark: { type: Boolean, default: false },
  toc: { type: Array, default: null },
})
defineEmits(['toggle-dark', 'scroll-to'])

const C = 2 * Math.PI * 16
const visible = ref(false)
const percent = ref(0)
const tocOpen = ref(false)

function onScroll() {
  const y = window.scrollY || 0
  const h = Math.max(1, document.documentElement.scrollHeight - window.innerHeight)
  percent.value = Math.min(100, Math.max(0, Math.round((y / h) * 100)))
  visible.value = y > 200
}

function toTop() {
  tocOpen.value = false
  scrollToTop()
}

onMounted(() => {
  window.addEventListener('scroll', onScroll, { passive: true })
  window.addEventListener('resize', onScroll, { passive: true })
  onScroll()
})
onBeforeUnmount(() => {
  window.removeEventListener('scroll', onScroll)
  window.removeEventListener('resize', onScroll)
})
</script>
