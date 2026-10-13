<template>
  <!-- post_bottom 挂载点示例：文章评论（评论核 API 展示层；核心页不硬编码）。
       读公开、写需登录。第三方可替换为更完善的评论/第三方服务。 -->
  <section v-if="ctx.token" class="cm-box">
    <h4 class="cm-title">评论 <span v-if="list.length" class="cm-n">{{ list.length }}</span></h4>
    <div v-if="loading" class="cm-hint">加载中…</div>
    <div v-else-if="!list.length" class="cm-hint">还没有评论</div>
    <div v-else class="cm-list">
      <div v-for="c in list" :key="c.id" class="cm-item" :class="{ reply: c.parent_id }">
        <div class="cm-head">
          <span class="cm-author">{{ c.author }}</span>
          <span class="cm-time">{{ fmt(c.created_at) }}</span>
        </div>
        <p class="cm-body">{{ c.body }}</p>
      </div>
    </div>
    <div v-if="authed" class="cm-write">
      <textarea v-model="draft" class="cm-input" rows="2" maxlength="2000" placeholder="写下你的评论…（回车换行，Ctrl+Enter 发送）" @keydown.ctrl.enter.prevent="send"></textarea>
      <div class="cm-actions">
        <button class="btn btn-sm cm-send" :disabled="sending || !draft.trim()" @click="send">发表评论</button>
      </div>
    </div>
    <div v-else class="cm-login-hint">登录后可发表评论</div>
  </section>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { publicComments, createComment, isAuthed } from '@/api'

const props = defineProps({ ctx: { type: Object, default: () => ({}) } })
const list = ref([])
const loading = ref(true)
const draft = ref('')
const sending = ref(false)
const authed = ref(false)

function fmt(ms) {
  if (!ms) return ''
  const d = new Date(ms)
  const p = (n) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`
}

async function load() {
  if (!props.ctx.token) return
  loading.value = true
  try {
    const d = await publicComments(props.ctx.token, props.ctx.path || '')
    list.value = d.items || []
  } catch (_) {
    list.value = []
  } finally {
    loading.value = false
  }
}

async function send() {
  const body = draft.value.trim()
  if (!body || sending.value) return
  sending.value = true
  try {
    await createComment(props.ctx.token, props.ctx.path || '', body)
    draft.value = ''
    await load()
  } catch (e) {
    authed.value = false
  } finally {
    sending.value = false
  }
}

onMounted(() => {
  authed.value = isAuthed()
  load()
})
</script>

<style scoped>
.cm-box {
  width: min(100%, 860px);
  margin: 0 auto;
  padding: 18px 24px 8px;
  font-size: 13px;
}

.cm-title {
  margin: 0 0 10px;
  font-size: 15px;
  font-weight: 600;
  color: var(--text, #1f2329);
}

.cm-n {
  font-size: 12px;
  font-weight: 400;
  color: var(--text-3, #8a919f);
}

.cm-hint {
  color: var(--text-3, #8a919f);
  padding: 6px 0;
}

.cm-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
  margin-bottom: 12px;
}

.cm-item {
  border: 1px solid var(--border, #e3e6eb);
  border-radius: 8px;
  padding: 10px 12px;
  background: var(--surface, #fff);
}

.cm-item.reply {
  margin-left: 22px;
}

.cm-head {
  display: flex;
  align-items: baseline;
  gap: 10px;
  margin-bottom: 5px;
}

.cm-author {
  font-weight: 600;
  color: var(--text-2, #4b5563);
}

.cm-time {
  font-size: 11px;
  color: var(--text-3, #8a919f);
}

.cm-body {
  margin: 0;
  color: var(--text, #1f2329);
  line-height: 1.6;
  white-space: pre-wrap;
  word-break: break-word;
}

.cm-write {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.cm-input {
  width: 100%;
  box-sizing: border-box;
  border: 1px solid var(--border, #e3e6eb);
  border-radius: 8px;
  padding: 8px 10px;
  font-size: 13px;
  font-family: inherit;
  resize: vertical;
  background: var(--surface, #fff);
  color: var(--text, #1f2329);
}

.cm-input:focus {
  outline: none;
  border-color: var(--primary, #4c7df0);
}

.cm-actions {
  display: flex;
  justify-content: flex-end;
}

.cm-login-hint {
  color: var(--text-3, #8a919f);
  padding: 4px 0 10px;
}
</style>
