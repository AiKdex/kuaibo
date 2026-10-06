<template>
  <div class="au-root">
    <AuroraHead />

    <main class="au-main">
      <section class="au-hero au-hero-sm">
        <p class="au-kicker">{{ $t('作者') }}</p>
        <div class="au-hero-name">{{ author || $t("未识别到作者参数") }}</div>
        <p class="au-hero-desc">{{ $t('共') }} {{ list.length }} {{ $t('篇公开文章。') }}</p>
      </section>

      <AuroraGrid v-if="list.length" :posts="list" />

      <div v-else class="au-state">
        <div class="au-state-t">
          {{ author ? $t("该作者暂无公开文章") : $t("未识别到作者参数") }}
        </div>
        <div class="au-state-d">{{ $t("作者字段由系统侧透出后自动生效；当前仅对已带作者信息的文章聚合。") }}</div>
      </div>
    </main>

    <AuroraFoot />
  </div>
    <AskWidget />
</template>

<script setup>
import AskWidget from '../AskWidget.vue'
/**
 * AuroraAuthorView —— 作者页（entries.author）。
 * file.author 属系统侧待透出字段（规范 §10-10）：全部走可选链降级，
 * 取不到作者信息时如实说明，不伪造数据。
 */
import { computed } from 'vue'
import AuroraHead from './AuroraHead.vue'
import AuroraFoot from './AuroraFoot.vue'
import AuroraGrid from './AuroraGrid.vue'
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
