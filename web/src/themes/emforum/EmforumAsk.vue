<template>
  <!-- AI 浮窗：锚定视口右下角（fixed），各页共用同一 themeContext。
       站长关闭 blog.ai_ask_open（themeContext.aiAskOpen 显式 false）时整体卸载。 -->
  <template v-if="askOpen">
    <div class="ef-ask">
      <!-- 悬浮按钮 -->
      <button
        class="ef-ask-fab"
        :class="{ on: open }"
        type="button"
        :title="open ? $t('收起 AI 助手') : $t('AI 问答')"
        :aria-label="open ? $t('收起 AI 助手') : $t('AI 问答')"
        @click="open = !open"
      >
        <svg v-if="!open" viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 11.5a8.38 8.38 0 0 1-.9 3.8 8.5 8.5 0 0 1-7.6 4.7 8.38 8.38 0 0 1-3.8-.9L3 21l1.9-5.7a8.38 8.38 0 0 1-.9-3.8 8.5 8.5 0 0 1 4.7-7.6 8.38 8.38 0 0 1 3.8-.9h.5a8.48 8.48 0 0 1 8 8v.5z"/></svg>
        <svg v-else viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M18 6L6 18M6 6l12 12"/></svg>
      </button>

      <!-- 对话面板 -->
      <div v-if="open" class="ef-ask-panel" role="dialog" :aria-label="$t('AI 问答')">
        <div class="ef-ask-head">
          <span class="ef-ask-badge ef-ask-badge-sm">AI</span>
          <span class="ef-ask-title">{{ $t('AI 助手') }}</span>
          <span class="ef-ask-sub">{{ $t('基于本站文章回答') }}</span>
          <button class="ef-ask-close" type="button" :aria-label="$t('收起')" @click="open = false">×</button>
        </div>

        <div ref="listEl" class="ef-ask-body">
          <div class="ef-ask-msg ef-ask-ai">
            <span class="ef-ask-badge">AI</span>
            <div class="ef-ask-bubble">
              <p>{{ $t("你好，我是这个博客的 AI 助手，可以围绕本站文章回答你的问题。") }}</p>
            </div>
          </div>

          <div
            v-for="(m, i) in qa"
            :key="i"
            class="ef-ask-msg"
            :class="m.role === 'user' ? 'ef-ask-user' : 'ef-ask-ai'"
          >
            <span v-if="m.role !== 'user'" class="ef-ask-badge">AI</span>
            <div class="ef-ask-bubble">
              <p v-if="m.text" class="ef-ask-text">{{ m.text }}</p>
              <p v-else class="ef-ask-thinking">{{ $t('正在思考…') }}</p>
            </div>
          </div>
        </div>

        <form class="ef-ask-form" @submit.prevent="submit">
          <input
            v-model="q"
            class="ef-ask-input"
            type="text"
            maxlength="800"
            :placeholder="$t('输入问题，回车提问…')"
            :disabled="loading"
          />
          <button class="ef-ask-send" type="submit" :disabled="!q.trim() || loading">
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

// 复用主题内 useBlog 注入的同一 themeContext（兼容 ref / reactive 两种 provide 形态）
function useCtx() {
  const raw = inject('themeContext', null)
  return computed(() => raw?.value || raw || {})
}
const ctx = useCtx()

// 站长级 AI 问答开关：themeContext.aiAskOpen（宿主自 /public/site 的 site.ai_ask_open 透传，缺省视为开）
const askOpen = computed(() => (ctx.value || {}).aiAskOpen !== false)

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
    const path = ctx.value?.page?.post?.path || ''
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

<style scoped>
/* AI 浮窗 —— 锚定视口右下角（fixed），复用主题令牌，独立于页面布局流 */
.ef-ask { position: fixed; right: 24px; bottom: 24px; z-index: 9999; font-family: var(--th-font-b); }
.ef-ask-fab {
  width: 54px; height: 54px; border-radius: 50%; border: 0;
  background: linear-gradient(135deg, var(--th-navy), var(--th-navy-2));
  color: #fff; box-shadow: 0 6px 20px rgba(26, 35, 50, 0.35); cursor: pointer;
  display: flex; align-items: center; justify-content: center; transition: 0.2s;
}
.ef-ask-fab:hover { transform: translateY(-2px); box-shadow: 0 10px 26px rgba(26, 35, 50, 0.45); }
.ef-ask-fab.on { background: linear-gradient(135deg, var(--th-accent-2), #8a6d1c); }
.ef-ask-panel {
  position: fixed; right: 24px; bottom: 90px; width: 360px; max-width: calc(100vw - 32px);
  background: var(--th-card); border: 1px solid var(--th-line); border-radius: 10px;
  box-shadow: 0 16px 48px rgba(26, 35, 50, 0.28); display: flex; flex-direction: column; overflow: hidden;
}
.ef-ask-head {
  display: flex; align-items: center; gap: 10px; padding: 13px 16px;
  background: linear-gradient(135deg, var(--th-navy), var(--th-navy-2)); color: #fff;
}
.ef-ask-badge {
  flex-shrink: 0; width: 26px; height: 26px; border-radius: 50%;
  background: linear-gradient(135deg, var(--th-accent), var(--th-navy)); color: #fff;
  font-size: 11px; font-weight: 700; display: flex; align-items: center; justify-content: center;
}
.ef-ask-badge-sm { width: 22px; height: 22px; font-size: 10px; }
.ef-ask-title { font-weight: 700; font-size: 15px; letter-spacing: 1px; }
.ef-ask-sub { font-size: 11px; color: #a9b8c9; }
.ef-ask-close { margin-left: auto; background: none; border: 0; color: #cfd8e3; font-size: 20px; line-height: 1; cursor: pointer; }
.ef-ask-close:hover { color: #fff; }
.ef-ask-body { max-height: 360px; overflow-y: auto; padding: 14px; display: flex; flex-direction: column; gap: 12px; background: var(--th-paper); }
.ef-ask-msg { display: flex; gap: 8px; align-items: flex-start; }
.ef-ask-msg.ef-ask-user { flex-direction: row-reverse; }
.ef-ask-bubble {
  max-width: 78%; padding: 9px 13px; border-radius: 12px; font-size: 13px; line-height: 1.6;
  background: var(--th-card); border: 1px solid var(--th-line); color: var(--th-ink);
}
.ef-ask-user .ef-ask-bubble { background: var(--th-accent); color: #fff; border-color: var(--th-accent); }
.ef-ask-text { margin: 0; }
.ef-ask-thinking { margin: 0; color: var(--th-ink-3); font-style: italic; }
.ef-ask-form { display: flex; border-top: 1px solid var(--th-line); }
.ef-ask-input {
  flex: 1; min-width: 0; border: 0; outline: none; padding: 12px 14px; font-size: 13px;
  color: var(--th-ink); background: var(--th-card); font-family: inherit;
}
.ef-ask-send {
  border: 0; background: linear-gradient(120deg, #8a6d1c, var(--th-accent-2)); color: #fff;
  font-size: 13px; font-weight: 600; padding: 0 18px; cursor: pointer;
}
.ef-ask-send:disabled { filter: grayscale(0.5); opacity: 0.6; cursor: default; }
</style>
