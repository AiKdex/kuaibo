<template>
  <!-- 侧栏区：全宽三栏横排，避免挤压网格密度 -->
  <aside class="bt-side">

    <!-- 1. 分类目录（path 首段派生） -->
    <div class="bt-box">
      <div class="bt-box-h"><i class="bt-sq c1"></i>分类目录</div>
      <ul class="bt-catlist">
        <li v-for="c in cats" :key="c.name">
          <button type="button" class="bt-catrow" @click="$emit('pick-cat', c.name)">
            <span class="bt-catname">{{ c.name }}</span>
            <span class="bt-catcnt">{{ c.count }}</span>
          </button>
        </li>
        <li v-if="!cats.length" class="bt-muted">暂无分类</li>
      </ul>
    </div>

    <!-- 2. 最近更新（系统顺序：置顶优先 + 时间倒序，不重排） -->
    <div class="bt-box">
      <div class="bt-box-h"><i class="bt-sq c2"></i>最近更新</div>
      <ol class="bt-recent">
        <li v-for="(p, i) in recent" :key="p.token || p.path">
          <span class="bt-recent-no">{{ seqNo(i) }}</span>
          <span>
            <a class="bt-recent-a" :href="postHref(ctx, p)">{{ postTitle(p) }}</a>
            <span class="bt-meta">{{ postDate(p, 'short') }} · {{ postSize(p) || catName(p) }}</span>
          </span>
        </li>
        <li v-if="!recent.length" class="bt-muted">暂无文章</li>
      </ol>
    </div>

    <!-- 3. AI 问答（规范 §7.1：读 reply，处理 429，空输入禁用） -->
    <div class="bt-box bt-box-ai">
      <div class="bt-box-h"><i class="bt-sq c3"></i>问问这个博客</div>
      <p class="bt-ai-hint">用自然语言提问，AI 会基于全部文章回答。</p>
      <form class="bt-ai-form" @submit.prevent="submitAsk">
        <input
          v-model="question"
          class="bt-ai-input"
          type="text"
          placeholder="例如：这个博客主要写什么？"
          :disabled="asking"
        >
        <button class="bt-btn bt-btn-ink" type="submit" :disabled="asking || !question.trim()">
          {{ asking ? '思考中…' : '提问' }}
        </button>
      </form>
      <p v-if="answer" class="bt-ai-answer">{{ answer }}</p>
    </div>

    <!-- 4. 标签（系统未注入时为恒空数组，自动隐藏；非链接 span，不产生 ?tag= 死链） -->
    <div v-if="tags.length" class="bt-box bt-box-tags">
      <div class="bt-box-h"><i class="bt-sq c4"></i>标签</div>
      <div class="bt-cloud">
        <span v-for="t in tags" :key="t.name" class="bt-cloud-i">{{ t.name }}</span>
      </div>
    </div>

  </aside>
</template>

<script setup>
import { computed, ref } from 'vue'
import {
  askBlogAI,
  catCounts,
  catName,
  postDate,
  postHref,
  postSize,
  postTitle,
  seqNo,
  useBlog,
} from './helpers.js'

defineEmits(['pick-cat'])

const ctx = useBlog()
const posts = computed(() => ctx.value.posts || [])
const tags = computed(() => ctx.value.tags || [])

const cats = computed(() => catCounts(posts.value))
const recent = computed(() => posts.value.slice(0, 4))

const question = ref('')
const answer = ref('')
const asking = ref(false)

async function submitAsk() {
  if (!question.value.trim() || asking.value) return
  answer.value = ''
  const reply = await askBlogAI(question.value, { onLoading: (v) => (asking.value = v) })
  answer.value = reply
}
</script>
