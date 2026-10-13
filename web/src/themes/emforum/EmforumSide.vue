<template>
  <!-- 侧栏：站点信息 / AI 问答 / 最新更新 / 标签（各页共用，取自同一 themeContext） -->
  <aside class="ef-side">
    <section class="ef-panel">
      <div class="ef-panel-head"><h3>站点信息</h3></div>
      <div class="ef-site-card">
        <div class="ef-site-avatar">{{ (siteName || 'A')[0] }}</div>
        <div class="ef-site-name">{{ siteName || '爱库录' }}</div>
        <div class="ef-site-desc">{{ siteDesc || '目录即站点，文件即文章' }}</div>
        <div class="ef-site-btns">
          <a class="primary" :href="blogUrl">全部文章</a>
          <a :href="rssHref">RSS</a>
        </div>
      </div>
    </section>

    <section class="ef-panel">
      <div class="ef-panel-head"><h3>AI 问答</h3></div>
      <div class="ef-ai">
        <div class="ef-ai-box">
          <input
            v-model="aiQuery"
            placeholder="问点什么…"
            :disabled="aiLoading"
            @keyup.enter="ask"
          />
          <button type="button" :disabled="!aiQuery.trim() || aiLoading" @click="ask">
            {{ aiLoading ? '…' : '问' }}
          </button>
        </div>
        <p v-if="aiAnswer" class="ef-ai-ans">{{ aiAnswer }}</p>
      </div>
    </section>

    <section class="ef-panel">
      <div class="ef-panel-head"><h3>最新更新</h3></div>
      <ul class="ef-latest">
        <li v-for="(p, i) in latest" :key="p.token || i">
          <span class="ef-latest-no">{{ i + 1 }}</span>
          <a class="ef-latest-t" :href="postHref(ctx, p)">{{ postTitle(p) }}</a>
          <span class="ef-latest-v">{{ postDate(p, 'short') }}</span>
        </li>
      </ul>
    </section>

    <!-- 标签云：ctx.tags 当前恒为空 → 整块隐藏；有数据时显示 t.name 且不输出死链（规范 §4.6） -->
    <section v-if="tags && tags.length" class="ef-panel">
      <div class="ef-panel-head"><h3>标签</h3></div>
      <div class="ef-cloud">
        <span v-for="t in tags" :key="t.name" class="ef-cloud-tag" :class="{ big: t.count >= 3 }">
          {{ t.name }}<i>{{ t.count }}</i>
        </span>
      </div>
    </section>
  </aside>
</template>

<script setup>
import { computed, ref } from 'vue'
import { postDate, postHref, postTitle, LIST_HREF, RSS_HREF, askBlogAI, useBlog } from './helpers.js'

const ctx = useBlog()
const posts = computed(() => ctx.value.posts || [])
const tags = computed(() => ctx.value.tags || [])
const siteName = computed(() => ctx.value.siteName || '')
const siteDesc = computed(() => ctx.value.siteDesc || '')
const blogUrl = computed(() => ctx.value.blogUrl || LIST_HREF)

/* 侧栏「最新更新」：按 updated_at 倒序取前 5（派生视图，不影响主列表的系统排序，规范 §2.4）。
   契约无浏览量字段，因此不做任何「热度」伪派生，标题与视觉也不暗示热度（规范 §2.2）。 */
const latest = computed(() =>
  [...posts.value]
    .sort((a, b) => (b.file?.updated_at || 0) - (a.file?.updated_at || 0))
    .slice(0, 5),
)

const aiQuery = ref('')
const aiAnswer = ref('')
const aiLoading = ref(false)
const rssHref = RSS_HREF

async function ask() {
  if (!aiQuery.value.trim() || aiLoading.value) return
  aiAnswer.value = '思考中…'
  aiAnswer.value = await askBlogAI(aiQuery.value, { onLoading: (v) => (aiLoading.value = v) })
}
</script>
