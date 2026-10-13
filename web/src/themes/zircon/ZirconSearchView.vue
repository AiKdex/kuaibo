<template>
  <div class="zc-root">
    <ZirconHead />

    <main class="zc-main">
      <section class="zc-hero zc-hero-sm">
        <p class="zc-kicker">检索</p>
        <div class="zc-hero-name">站内检索</div>
        <p class="zc-hero-desc">在当前已加载的公开文章里匹配标题与摘要。</p>
      </section>

      <div class="zc-sbox">
        <input
          v-model="kw"
          class="zc-sinput"
          type="search"
          placeholder="输入关键词，回车不跳转"
          aria-label="站内检索关键词"
        />
        <button v-if="kw" class="zc-sclear" type="button" @click="clear">清空</button>
      </div>

      <p class="zc-snote">
        共 {{ posts.length }} 篇公开文章参与检索{{ kw ? `，匹配 ${list.length} 篇` : '' }}。
      </p>

      <ZirconList v-if="list.length" :posts="list" />

      <div v-else class="zc-state">
        <div class="zc-state-t">{{ kw ? '没有匹配的文章' : '输入关键词开始检索' }}</div>
        <div class="zc-state-d">当前为客户端匹配，只覆盖已加载的公开文章。</div>
      </div>
    </main>

    <ZirconFoot />
  </div>
</template>

<script setup>
/**
 * ZirconSearchView —— 搜索页（entries.search）。
 * 客户端匹配标题 / 摘要（规范 §4.8）；关键词优先取 ctx.page.q，缺失时回退解析 hash（只读）。
 * 输入框只改本地 ref，不写 location.hash，也不触发路由跳转。
 */
import { computed, ref, watch } from 'vue'
import ZirconHead from './ZirconHead.vue'
import ZirconFoot from './ZirconFoot.vue'
import ZirconList from './ZirconList.vue'
import './style.css'
import { useBlog, postTitle, postExcerpt, readPageParam } from './helpers.js'

const ctx = useBlog()
const posts = computed(() => ctx.value.posts || [])

const kw = ref(readPageParam(ctx.value, ['q'], [/[?&]q=([^&#]+)/]))

watch(
  () => ctx.value?.page?.q,
  (v) => {
    if (v) kw.value = String(v)
  }
)

const list = computed(() => {
  const k = kw.value.trim().toLowerCase()
  if (!k) return []
  return posts.value.filter((p) => {
    const hay = `${postTitle(p)} ${postExcerpt(p, 400)}`.toLowerCase()
    return hay.includes(k)
  })
})

function clear() {
  kw.value = ''
}
</script>
