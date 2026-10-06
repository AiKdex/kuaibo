<template>
  <aside class="rai-card">
    <h4 class="rai-title">{{ $t('读者问答') }}</h4>
    <p class="rai-tip">{{ $t("基于本站公开文章回答，不知道会直说。") }}</p>
    <form class="rai-form" @submit.prevent="ask">
      <input
        v-model="q"
        class="rai-input"
        type="text"
        maxlength="400"
        :placeholder="$t('例如：目录即站点是什么意思？')"
        :disabled="busy"
      />
      <button class="rai-btn" type="submit" :disabled="busy || !q.trim()">{{ busy ? $t("思考中…") : $t("提问") }}</button>
    </form>
    <div v-if="answer" class="rai-answer">{{ answer }}</div>
    <div v-if="error" class="rai-error">{{ error }}</div>
  </aside>
</template>

<script setup>
import { t } from '@/i18n'
import { ref } from 'vue'
import { publicBlogAsk } from '@/api'

const props = defineProps({
  ctx: { type: Object, default: () => ({}) },
})

const q = ref('')
const busy = ref(false)
const answer = ref('')
const error = ref('')

async function ask() {
  const question = q.value.trim()
  if (!question || busy.value) return
  busy.value = true
  error.value = ''
  answer.value = ''
  try {
    const d = await publicBlogAsk(question, props.ctx?.path || '')
    answer.value = d.reply || t('（空回答）')
  } catch (e) {
    error.value = e.message || t('提问失败')
  } finally {
    busy.value = false
  }
}
</script>

<style scoped>
.rai-card {
  border: 1px solid var(--border, #e3e6eb);
  border-radius: 10px;
  background: var(--surface, #fff);
  padding: 14px 16px;
  font-size: 13px;
}
.rai-title { margin: 0 0 6px; font-size: 14px; font-weight: 600; }
.rai-tip { margin: 0 0 10px; color: var(--text-3, #8a919f); font-size: 12px; }
.rai-form { display: flex; gap: 8px; }
.rai-input {
  flex: 1;
  min-width: 0;
  padding: 8px 10px;
  border: 1px solid var(--border, #e3e6eb);
  border-radius: 8px;
  font: inherit;
}
.rai-btn {
  padding: 8px 14px;
  border-radius: 8px;
  border: none;
  background: #0d7a6a;
  color: #fff;
  cursor: pointer;
  font: inherit;
}
.rai-btn:disabled { opacity: 0.55; cursor: not-allowed; }
.rai-answer {
  margin-top: 12px;
  padding: 10px 12px;
  background: var(--bg-2, #f7faf9);
  border-radius: 8px;
  line-height: 1.6;
  white-space: pre-wrap;
}
.rai-error { margin-top: 8px; color: #b91c1c; font-size: 12px; }
</style>
