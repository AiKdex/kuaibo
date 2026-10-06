<template>
  <div class="vd-side">
    <!-- 分类聚合（path 首段派生，客户端过滤） -->
    <section v-if="cats.length" class="vd-sb">
      <h3 class="vd-sb-h">{{ $t('按分类浏览') }}</h3>
      <ul class="vd-cat-list">
        <li v-for="c in cats" :key="c.name">
          <button type="button" class="vd-cat-item" @click="$emit('pick-cat', c.name)">
            <span class="vd-cat-name">{{ c.name }}</span>
            <span class="vd-cat-num">{{ c.count }}</span>
          </button>
        </li>
      </ul>
    </section>

    <!-- 最新更新（按 updated_at 取前 5，如实在标题里说明是「最新」） -->
    <section v-if="latest.length" class="vd-sb">
      <h3 class="vd-sb-h">{{ $t('最新更新') }}</h3>
      <ul class="vd-latest">
        <li v-for="(p, i) in latest" :key="p.token || p.path" class="vd-latest-item">
          <span class="vd-latest-no">{{ seqNo(i) }}</span>
          <a class="vd-latest-t" :href="postHref(ctx, p)">{{ postTitle(p) }}</a>
          <span class="vd-latest-d">{{ postDate(p, 'short') }}</span>
        </li>
      </ul>
    </section>

    <!-- 标签云：系统层 tags 目前恒为空数组（规范 §4.6），空时整块隐藏，非链接输出 -->
    <section v-if="tags.length" class="vd-sb">
      <h3 class="vd-sb-h">{{ $t('标签') }}</h3>
      <div class="vd-cloud">
        <span v-for="t in tags" :key="t.name" class="vd-cloud-tag">{{ t.name }}</span>
      </div>
    </section>

    <section class="vd-side-cta">
      <h3 class="vd-side-cta-h">{{ $t('新文章发出时') }}<br>{{ $t('第一个告诉你') }}</h3>
      <p class="vd-side-cta-p">{{ $t("用 RSS 阅读器订阅，没有广告，也没有中间商。") }}</p>
      <a class="vd-btn vd-btn-inv" :href="rssHref">{{ $t('订阅 RSS') }}</a>
    </section>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { RSS_HREF, catCounts, postDate, postHref, postTitle, seqNo, useBlog } from './helpers.js'

defineEmits(['pick-cat'])

const ctx = useBlog()
const posts = computed(() => ctx.value.posts || [])
const tags = computed(() => ctx.value.tags || [])
const rssHref = RSS_HREF

const cats = computed(() => catCounts(posts.value))
/* 序号与列表一致：均为系统返回顺序的前 5 条（不重排） */
const latest = computed(() => posts.value.slice(0, 5))
</script>
