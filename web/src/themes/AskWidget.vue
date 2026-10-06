<template>
  <!-- 共享 AI 浮窗：锚定视口右下角（fixed），一次编写服务全部主题。
       样式用主题令牌（--th-accent / --th-card / --th-ink / --th-line / --th-paper）+ 安全兜底色，
       主题未定义令牌时自动回落中性配色，因此每个主题既统一又自动套用自身强调色。
       站长关闭 blog.ai_ask_open（themeContext.aiAskOpen 显式 false）时整体卸载。 -->
  <template v-if="askOpen">
    <div class="aw-ask">
      <button
        class="aw-ask-fab"
        :class="{ on: open }"
        type="button"
        :title="open ? $t('收起 AI 助手') : $t('AI 问答')"
        :aria-label="open ? $t('收起 AI 助手') : $t('AI 问答')"
        @click="open = !open"
      >
        <svg v-if="!open" viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 11.5a8.38 8.38 0 0 1-.9 3.8 8.5 8.5 0 0 1-7.6 4.7 8.38 8.38 0 0 1-3.8-.9L3 21l1.9-5.7a8.38 8.38 0 0 1-.9-3.8 8.5 8.5 0 0 1 4.7-7.6 8.38 8.38 0 0 1 3.8-.9h.5a8.48 8.48 0 0 1 8 8v.5z"/></svg>
        <svg v-else viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M18 6L6 18M6 6l12 12"/></svg>
      </button>

      <div v-if="open" class="aw-ask-panel" role="dialog" :aria-label="$t('AI 问答')">
        <div class="aw-ask-head">
          <span class="aw-ask-badge aw-ask-badge-sm">AI</span>
          <span class="aw-ask-title">{{ $t('AI 助手') }}</span>
          <span class="aw-ask-sub">{{ $t('基于本站文章回答') }}</span>
          <button class="aw-ask-close" type="button" :aria-label="$t('收起')" @click="open = false">×</button>
        </div>

        <div ref="listEl" class="aw-ask-body">
          <div class="aw-ask-msg aw-ask-ai">
            <span class="aw-ask-badge">AI</span>
            <div class="aw-ask-bubble">
              <p>{{ $t("你好，我是这个博客的 AI 助手，可以围绕本站文章回答你的问题。") }}</p>
            </div>
          </div>

          <div
            v-for="(m, i) in qa"
            :key="i"
            class="aw-ask-msg"
            :class="m.role === 'user' ? 'aw-ask-user' : 'aw-ask-ai'"
          >
            <span v-if="m.role !== 'user'" class="aw-ask-badge">AI</span>
            <div class="aw-ask-bubble">
              <p v-if="m.text" class="aw-ask-text">{{ m.text }}</p>
              <p v-else class="aw-ask-thinking">{{ $t('正在思考…') }}</p>
            </div>
          </div>
        </div>

        <form class="aw-ask-form" @submit.prevent="submit">
          <input
            v-model="q"
            class="aw-ask-input"
            type="text"
            maxlength="800"
            :placeholder="$t('输入问题，回车提问…')"
            :disabled="loading"
          />
          <button class="aw-ask-send" type="submit" :disabled="!q.trim() || loading">
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
/* 共享 AI 浮窗 —— 锚定视口右下角（fixed），独立于页面布局流。
   全部颜色走主题令牌，未定义时回落中性好看的安全值，因此每个主题自动套用自身强调色。 */
.aw-ask { position: fixed; right: 24px; bottom: 24px; z-index: 9999; font-family: var(--th-font-b, var(--th-font, system-ui, sans-serif)); }
.aw-ask-fab {
  width: 54px; height: 54px; border-radius: 50%; border: 0;
  background: linear-gradient(135deg, var(--th-accent, #2b6e6e), var(--th-ink, #1f2a44));
  color: #fff; box-shadow: 0 6px 20px rgba(20, 30, 50, 0.30); cursor: pointer;
  display: flex; align-items: center; justify-content: center; transition: 0.2s;
}
.aw-ask-fab:hover { transform: translateY(-2px); box-shadow: 0 10px 26px rgba(20, 30, 50, 0.38); }
.aw-ask-fab.on { background: linear-gradient(135deg, var(--th-ink, #1f2a44), var(--th-accent, #2b6e6e)); }
.aw-ask-panel {
  position: fixed; right: 24px; bottom: 90px; width: 360px; max-width: calc(100vw - 32px);
  background: var(--th-card, #fff); border: 1px solid var(--th-line, #e3e8ef); border-radius: 12px;
  box-shadow: 0 16px 48px rgba(20, 30, 50, 0.22); display: flex; flex-direction: column; overflow: hidden;
}
.aw-ask-head {
  display: flex; align-items: center; gap: 10px; padding: 13px 16px;
  background: linear-gradient(135deg, var(--th-accent, #2b6e6e), var(--th-ink, #1f2a44)); color: #fff;
}
.aw-ask-badge {
  flex-shrink: 0; width: 26px; height: 26px; border-radius: 50%;
  background: rgba(255, 255, 255, 0.22); color: #fff;
  font-size: 11px; font-weight: 700; display: flex; align-items: center; justify-content: center;
}
.aw-ask-badge-sm { width: 22px; height: 22px; font-size: 10px; }
.aw-ask-title { font-weight: 700; font-size: 15px; letter-spacing: 1px; }
.aw-ask-sub { font-size: 11px; opacity: 0.78; }
.aw-ask-close { margin-left: auto; background: none; border: 0; color: rgba(255,255,255,0.82); font-size: 20px; line-height: 1; cursor: pointer; }
.aw-ask-close:hover { color: #fff; }
.aw-ask-body { max-height: 360px; overflow-y: auto; padding: 14px; display: flex; flex-direction: column; gap: 12px; background: var(--th-paper, #f6f8fb); }
.aw-ask-msg { display: flex; gap: 8px; align-items: flex-start; }
.aw-ask-msg.aw-ask-user { flex-direction: row-reverse; }
.aw-ask-bubble {
  max-width: 78%; padding: 9px 13px; border-radius: 12px; font-size: 13px; line-height: 1.6;
  background: var(--th-card, #fff); border: 1px solid var(--th-line, #e3e8ef); color: var(--th-ink, #222);
}
.aw-ask-user .aw-ask-bubble { background: var(--th-accent, #2b6e6e); color: #fff; border-color: var(--th-accent, #2b6e6e); }
.aw-ask-text { margin: 0; }
.aw-ask-thinking { margin: 0; color: var(--th-ink-3, #8a94a6); font-style: italic; }
.aw-ask-form { display: flex; border-top: 1px solid var(--th-line, #e3e8ef); }
.aw-ask-input {
  flex: 1; min-width: 0; border: 0; outline: none; padding: 12px 14px; font-size: 13px;
  color: var(--th-ink, #222); background: var(--th-card, #fff); font-family: inherit;
}
.aw-ask-send {
  border: 0; background: linear-gradient(120deg, var(--th-accent, #2b6e6e), var(--th-ink, #1f2a44)); color: #fff;
  font-size: 13px; font-weight: 600; padding: 0 18px; cursor: pointer;
}
.aw-ask-send:disabled { filter: grayscale(0.4); opacity: 0.6; cursor: default; }
</style>
