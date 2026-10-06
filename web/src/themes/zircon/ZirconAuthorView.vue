<template>
  <div class="zc-root">
    <ZirconHead />

    <main class="zc-main">
      <section class="zc-hero zc-hero-sm">
        <p class="zc-kicker">{{ $t('作者') }}</p>
        <div class="zc-hero-name">{{ author || $t("未识别到作者参数") }}</div>
        <p class="zc-hero-desc">{{ $t('共') }} {{ list.length }} {{ $t('篇公开文章。') }}</p>
      </section>

      <ZirconList v-if="list.length" :posts="list" />

      <div v-else class="zc-state">
        <div class="zc-state-t">
          {{ author ? $t("该作者暂无公开文章") : $t("未识别到作者参数") }}
        </div>
        <div class="zc-state-d">{{ $t("作者字段由系统侧透出后自动生效；当前仅对已带作者信息的文章聚合。") }}</div>
      </div>
    </main>

    <ZirconFoot />
  </div>
    <AskWidget />
</template>

<script setup>
import AskWidget from '../AskWidget.vue'
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
