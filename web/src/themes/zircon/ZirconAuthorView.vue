<template>
  <div class="zc-root">
    <ZirconHead />

    <main class="zc-main">
      <section class="zc-hero zc-hero-sm">
        <p class="zc-kicker">作者</p>
        <div class="zc-hero-name">{{ author || '未识别到作者参数' }}</div>
        <p class="zc-hero-desc">共 {{ list.length }} 篇公开文章。</p>
      </section>

      <ZirconList v-if="list.length" :posts="list" />

      <div v-else class="zc-state">
        <div class="zc-state-t">
          {{ author ? '该作者暂无公开文章' : '未识别到作者参数' }}
        </div>
        <div class="zc-state-d">作者字段由系统侧透出后自动生效；当前仅对已带作者信息的文章聚合。</div>
      </div>
    </main>

    <ZirconFoot />
  </div>
</template>

<script setup>
/**
 * ZirconAuthorView —— 作者页（entries.author）。
 * file.author 属系统侧待透出字段（规范 §10-10）：全部走可选链降级，
 * 取不到作者信息时如实说明，不伪造数据。
 */
import { computed } from 'vue'
import ZirconHead from './ZirconHead.vue'
import ZirconFoot from './ZirconFoot.vue'
import ZirconList from './ZirconList.vue'
import './style.css'
import { useBlog, postAuthor, readPageParam } from './helpers.js'

const ctx = useBlog()
const author = computed(() =>
  readPageParam(ctx.value, ['author'], [/author\/([^/?#]+)/, /[?&]author=([^&#]+)/])
)
const list = computed(() =>
  (ctx.value.posts || []).filter((p) => author.value && postAuthor(p) === author.value)
)
</script>
