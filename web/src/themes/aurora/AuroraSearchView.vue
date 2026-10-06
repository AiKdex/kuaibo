<template>
  <div class="au-root">
    <AuroraHead />

    <main class="au-main">
      <section class="au-hero au-hero-sm">
        <p class="au-kicker">{{ $t('检索') }}</p>
        <div class="au-hero-name">{{ $t('站内检索') }}</div>
        <p class="au-hero-desc">{{ $t("在当前已加载的公开文章里匹配标题与摘要。") }}</p>
      </section>

      <div class="au-sbox">
        <input
          v-model="kw"
          class="au-sinput"
          type="search"
          :placeholder="$t('输入关键词，回车不跳转')"
          :aria-label="$t('站内检索关键词')"
        />
        <button v-if="kw" class="au-sclear" type="button" @click="clear">{{ $t('清空') }}</button>
      </div>

      <p class="au-snote">
        {{ $t('共') }} {{ posts.length }} {{ $t('篇公开文章参与检索') }}{{ kw ? $t("，匹配 {list} 篇", { list: list.length }) : '' }}。
      </p>

      <AuroraGrid v-if="list.length" :posts="list" />

      <div v-else class="au-state">
        <div class="au-state-t">{{ kw ? $t("没有匹配的文章") : $t("输入关键词开始检索") }}</div>
        <div class="au-state-d">{{ $t("当前为客户端匹配，只覆盖已加载的公开文章。") }}</div>
      </div>
    </main>

    <AuroraFoot />
  </div>
    <AskWidget />
</template>

<script setup>
import AskWidget from '../AskWidget.vue'
/**
 * AuroraSearchView —— 搜索页（entries.search）。
 * 客户端匹配标题 / 摘要（规范 §4.8）；关键词优先取 ctx.page.q，缺失时回退解析 hash（只读）。
 * 输入框只改本地 ref，不写 location.hash，也不触发路由跳转。
 */
import { computed, ref, watch } from 'vue'
import AuroraHead from './AuroraHead.vue'
import AuroraFoot from './AuroraFoot.vue'
import AuroraGrid from './AuroraGrid.vue'
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
