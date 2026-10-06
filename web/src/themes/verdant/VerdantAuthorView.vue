<template>
  <div class="vd-root">
    <VerdantHead :site-name="siteName" :blog-url="blogUrl" :slogan="$t('AUTHOR · 作者')" />

    <div v-if="loading" class="vd-wrap"><div class="vd-state">{{ $t('正在加载文章…') }}</div></div>
    <div v-else-if="error" class="vd-wrap"><div class="vd-state vd-state-err">{{ error }}</div></div>

    <template v-else>
      <section class="vd-sub-hero">
        <div class="vd-wrap">
          <div class="vd-sect">
            <h2 class="vd-sect-t">{{ author || $t("作者") }}</h2>
            <span class="vd-rule"></span>
            <span class="vd-sect-note">{{ list.length }} {{ $t('篇') }}</span>
          </div>
          <!-- 作者字段属系统侧待透出项（规范 §10-10）：拿不到就如实说明，不伪造 -->
          <p v-if="!hasAuthorField" class="vd-sub-d">{{ $t("当前列表数据未包含作者字段，以下为全部公开文章；系统补齐后本页会自动按作者过滤。") }}
          </p>
          <p v-else class="vd-sub-d">{{ $t('按作者聚合的公开文章。') }}</p>
          <div class="vd-filters">
            <a class="vd-btn vd-btn-g vd-btn-sm" :href="blogUrl">{{ $t('‹ 返回全部文章') }}</a>
          </div>
        </div>
      </section>

      <section class="vd-body">
        <div class="vd-wrap">
          <div v-if="list.length" class="vd-grid">
            <VerdantCard v-for="(p, i) in list" :key="p.token || p.path" :post="p" :index="i" />
          </div>
          <div v-else class="vd-state">{{ $t('该作者下暂无文章') }}</div>
        </div>
      </section>
    </template>

    <VerdantFoot :site-name="siteName" :blog-url="blogUrl" :posts="posts" />
  </div>
    <AskWidget />
</template>

<script setup>
import AskWidget from '../AskWidget.vue'
import { computed } from 'vue'
import VerdantHead from './VerdantHead.vue'
import VerdantCard from './VerdantCard.vue'
import VerdantFoot from './VerdantFoot.vue'
import './style.css'
import { LIST_HREF, readPageParam, useBlog } from './helpers.js'

const ctx = useBlog()
const posts = computed(() => ctx.value.posts || [])
const loading = computed(() => !!ctx.value.loading)
const error = computed(() => ctx.value.error || '')
const siteName = computed(() => ctx.value.siteName || '')
const blogUrl = computed(() => ctx.value.blogUrl || LIST_HREF)

const author = computed(() =>
  readPageParam(ctx.value, ['author'], [/\/author\/([^/?#]+)/, /[?&]author=([^&#]+)/]),
)

/* file.author 为系统侧待透出字段：存在才过滤，缺失则如实降级（不编造匹配） */
const hasAuthorField = computed(() => posts.value.some((p) => p?.file?.author?.name || p?.file?.author))

const list = computed(() => {
  if (!hasAuthorField.value) return posts.value
  return posts.value.filter((p) => {
    const a = p?.file?.author
    const name = typeof a === 'string' ? a : a?.name || a?.username || ''
    return name && name === author.value
  })
})
</script>
