<template>
  <!-- AI 对话窗：锚定博客内容容器内（sticky 跟随容器右缘 + 滚动贴底可见，不跑出容器）。
       站长关闭 blog.ai_ask_open（themeContext.aiAskOpen 显式 false）时整体卸载。 -->
  <template v-if="askOpen">
    <div class="hj-ask">
      <!-- 悬浮按钮 -->
      <button
        class="hj-ask-fab"
        :class="{ on: open }"
        type="button"
        :title="open ? $t('收起 AI 助手') : $t('AI 问答')"
        :aria-label="open ? $t('收起 AI 助手') : $t('AI 问答')"
        @click="open = !open"
      >
        <svg v-if="!open" viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 11.5a8.38 8.38 0 0 1-.9 3.8 8.5 8.5 0 0 1-7.6 4.7 8.38 8.38 0 0 1-3.8-.9L3 21l1.9-5.7a8.38 8.38 0 0 1-.9-3.8 8.5 8.5 0 0 1 4.7-7.6 8.38 8.38 0 0 1 3.8-.9h.5a8.48 8.48 0 0 1 8 8v.5z"/></svg>
        <svg v-else viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M18 6L6 18M6 6l12 12"/></svg>
      </button>

      <!-- 对话面板 -->
      <div v-if="open" class="hj-ask-panel" role="dialog" :aria-label="$t('AI 问答')">
        <div class="hj-ask-head">
          <span class="hj-ask-title">{{ $t('AI 助手') }}</span>
          <span class="hj-ask-sub">{{ $t('基于本站文章回答') }}</span>
        </div>

        <div ref="listEl" class="hj-ask-body">
          <div class="hj-ask-msg hj-ask-ai">
            <span class="hj-ask-badge">AI</span>
            <div class="hj-ask-bubble">
              <p>{{ $t("你好，我是这个博客的 AI 助手，可以围绕本站文章回答你的问题。") }}</p>
            </div>
          </div>

          <div
            v-for="(m, i) in qa"
            :key="i"
            class="hj-ask-msg"
            :class="m.role === 'user' ? 'hj-ask-user' : 'hj-ask-ai'"
          >
            <span v-if="m.role !== 'user'" class="hj-ask-badge">AI</span>
            <div class="hj-ask-bubble">
              <p v-if="m.text" class="hj-ask-text">{{ m.text }}</p>
              <p v-else class="hj-ask-thinking">{{ $t('正在思考…') }}</p>
            </div>
          </div>
        </div>

        <form class="hj-ask-form" @submit.prevent="submit">
          <input
            v-model="q"
            class="hj-ask-input"
            type="text"
            maxlength="800"
            :placeholder="$t('输入问题，回车提问…')"
            :disabled="loading"
          />
          <button class="hj-ask-send" type="submit" :disabled="!q.trim() || loading">
            {{ loading ? $t("思考中") : $t("发送") }}
          </button>
        </form>
      </div>
    </div>
  </template>
</template>

<script setup>
import { t } from '@/i18n'
import { ref, computed, nextTick, inject } from 'vue'

const ctx = inject('themeContext')

// 站长级 AI 问答开关：themeContext.aiAskOpen（宿主自 /public/site 的 site.ai_ask_open 透传，
// 缺省视为开）。兼容 ref（列表页）与 reactive（文章页）两种 provide 形态。
const askOpen = computed(() => {
  const src = ctx && ctx.value !== undefined ? ctx.value : ctx
  return (src || {}).aiAskOpen !== false
})

const open = ref(false)
const q = ref('')
const loading = ref(false)
const qa = ref([])
const listEl = ref(null)

async function submit() {
  const question = q.value.trim()
  if (!question || loading.value) return
  loading.value = true
  qa.value.push({ role: 'user', text: question })
  q.value = ''
  const pending = { role: 'ai', text: '' }
  qa.value.push(pending)
  scrollToBottom()

  try {
    const raw = ctx && ctx.value !== undefined ? ctx.value : (ctx || {})
    const path = raw.page?.post?.path || ''
    const res = await fetch('/api/v1/public/blog/ask', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ question, path }),
    })
    if (res.status === 429) {
      const d = await res.json().catch(() => null)
      pending.text = (d && d.error && d.error.message) || t('提问太频繁了，请稍等几分钟再试。')
      return
    }
    if (!res.ok) {
      const d = await res.json().catch(() => null)
      pending.text = (d && d.error && d.error.message) || t('AI 暂时没有响应，请稍后重试。')
      return
    }
    const data = await res.json()
    if (data && data.degraded) {
      pending.text = data.reply || t('AI 服务当前不可用，请稍后再来。')
      return
    }
    pending.text = data?.reply || t('抱歉，我没有理解这个问题，换个说法试试？')
  } catch (e) {
    pending.text = t('网络似乎出了点问题，请稍后重试。')
  } finally {
    loading.value = false
    scrollToBottom()
  }
}

function scrollToBottom() {
  nextTick(() => {
    if (listEl.value) listEl.value.scrollTop = listEl.value.scrollHeight
  })
}
</script>
