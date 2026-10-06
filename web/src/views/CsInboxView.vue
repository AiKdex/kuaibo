<template>
  <div class="cs-root">
    <header class="cs-head">
      <h2 class="cs-title">{{  t('cs.title')  }}</h2>
      <div class="cs-filters">
        <select v-model="status" class="cs-sel" @change="loadInbox">
          <option value="all">{{  t('cs.allStatus')  }}</option>
          <option value="open">{{  t('cs.status.open')  }}</option>
          <option value="pending">{{  t('cs.status.pending')  }}</option>
          <option value="resolved">{{  t('cs.status.resolved')  }}</option>
          <option value="closed">{{  t('cs.status.closed')  }}</option>
        </select>
        <button class="cs-btn" :disabled="loading" @click="loadInbox">{{  loading ? t('cs.loading') : t('cs.refresh')  }}</button>
      </div>
    </header>

    <p v-if="err" class="cs-err">{{  err  }}</p>

    <div class="cs-body">
      <!-- 左：会话列表 -->
      <aside class="cs-list">
        <div v-if="!convs.length && !loading" class="cs-empty">{{  t('cs.empty')  }}</div>
        <button
          v-for="c in convs"
          :key="c.id"
          class="cs-item"
          :class="{ active: c.id === current?.id }"
          @click="openConv(c)"
        >
          <div class="cs-item-top">
            <span class="cs-item-name">{{  displayName(c.contact_id)  }}</span>
            <span class="cs-item-time">{{  fmtTime(c.last_message_at)  }}</span>
          </div>
          <div class="cs-item-sub">
            <span class="cs-chan">{{  c.channel  }}</span>
            <span class="cs-subj">{{  c.subject || t('cs.noSubject')  }}</span>
          </div>
          <span v-if="c.unread_count" class="cs-badge">{{  c.unread_count  }}</span>
        </button>
      </aside>

      <!-- 右：会话详情 -->
      <section v-if="current" class="cs-conv">
        <div class="cs-conv-head">
          <div>
            <div class="cs-conv-name">{{  displayName(current.contact_id)  }}</div>
            <div class="cs-conv-meta">{{  current.channel  }} · {{  statusText(current.status)  }}</div>
          </div>
          <div class="cs-conv-actions">
            <select :value="current.status" class="cs-sel" @change="setStatus($event.target.value)">
              <option value="open">{{  t('cs.status.open')  }}</option>
              <option value="pending">{{  t('cs.status.pending')  }}</option>
              <option value="resolved">{{  t('cs.status.resolved')  }}</option>
              <option value="closed">{{  t('cs.status.closed')  }}</option>
            </select>
          </div>
        </div>

        <div ref="scrollBox" class="cs-msgs">
          <div v-if="!msgs.length" class="cs-empty">{{  t('cs.noMessages')  }}</div>
          <div
            v-for="m in msgs"
            :key="m.id"
            class="cs-msg"
            :class="m.direction === 'in' ? 'in' : 'out'"
          >
            <div class="cs-msg-meta">
              <span>{{  m.author_type  }}</span>
              <span>{{  fmtTime(m.created_at)  }}</span>
            </div>
            <div class="cs-msg-body">{{  m.text  }}</div>
          </div>
        </div>

        <div class="cs-reply">
          <div class="cs-reply-tools">
            <button
              class="cs-btn"
              :disabled="!current || aiDrafting"
              :title="t('cs.aiDraftTip')"
              @click="aiDraft"
            >
              {{  aiDrafting ? t('cs.aiDrafting') : t('cs.aiDraft')  }}
            </button>
            <span v-if="aiDraftErr" class="cs-ai-err">{{ aiDraftErr }}</span>
          </div>
          <textarea
            v-model="draft"
            class="cs-textarea"
            :placeholder="t('cs.replyPlaceholder')"
            rows="3"
          ></textarea>
          <button class="cs-send" :disabled="!draft.trim() || sending" @click="send">
            {{  sending ? t('cs.sending') : t('cs.send')  }}
          </button>
        </div>
      </section>
      <section v-else class="cs-conv cs-conv-empty">
        <div class="cs-empty">{{  t('cs.pickConversation')  }}</div>
      </section>
    </div>
  </div>
</template>

<script setup>
import { onMounted, onUnmounted, ref } from 'vue'
import { t } from '@/i18n'
import { requestJSON } from '@/api'

const status = ref('all')
const convs = ref([])
const contacts = ref({})
const current = ref(null)
const msgs = ref([])
const draft = ref('')
const loading = ref(false)
const sending = ref(false)
const aiDrafting = ref(false)
const aiDraftErr = ref('')
const err = ref('')
const scrollBox = ref(null)
let pollTimer = null

// 客户端幂等键：同一条草稿重复点击只发一条（后端按此去重）
function clientMsgId() {
  return 'cm-' + Date.now().toString(36) + '-' + Math.random().toString(36).slice(2, 8)
}

async function loadInbox() {
  loading.value = true
  err.value = ''
  try {
    const r = await requestJSON(`/cs/inbox?status=${encodeURIComponent(status.value)}&limit=100`)
    convs.value = (r && r.items) || []
  } catch (e) {
    err.value = e?.message || String(e)
  } finally {
    loading.value = false
  }
}

async function loadContacts() {
  try {
    const r = await requestJSON('/cs/contacts?limit=200')
    const map = {}
    for (const c of (r && r.items) || []) map[c.id] = c.display_name || c.email || c.id.slice(0, 8)
    contacts.value = map
  } catch { /* 档案加载失败不阻塞收件箱 */ }
}

function displayName(id) { return contacts.value[id] || (id ? id.slice(0, 8) : '—') }
function statusText(s) { return t('cs.status.' + (s || 'open')) }
function fmtTime(ms) {
  if (!ms) return ''
  const d = new Date(ms)
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')} ${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
}

async function openConv(c) {
  current.value = c
  try {
    const r = await requestJSON(`/cs/conversations/${encodeURIComponent(c.id)}/messages`)
    msgs.value = (r && r.items) || []
    scrollToEnd()
  } catch (e) {
    err.value = e?.message || String(e)
  }
}

function scrollToEnd() {
  // 等 DOM 更新后再滚到底
  setTimeout(() => { if (scrollBox.value) scrollBox.value.scrollTop = scrollBox.value.scrollHeight }, 0)
}

async function send() {
  if (!current.value || !draft.value.trim()) return
  sending.value = true
  err.value = ''
  try {
    await requestJSON(`/cs/conversations/${encodeURIComponent(current.value.id)}/reply`, {
      method: 'POST',
      body: JSON.stringify({ text: draft.value, client_msg_id: clientMsgId() }),
    })
    draft.value = ''
    await openConv(current.value)
    loadInbox()
  } catch (e) {
    err.value = e?.message || String(e)
  } finally {
    sending.value = false
  }
}

// aiDraft：让模型依据知识库起草一版回复，填入输入框（不自动发送，由坐席确认）。
async function aiDraft() {
  if (!current.value || aiDrafting.value) return
  aiDrafting.value = true
  aiDraftErr.value = ''
  try {
    const r = await requestJSON(`/cs/conversations/${encodeURIComponent(current.value.id)}/draft`, {
      method: 'POST',
      body: '{}',
    })
    if (r?.draft) {
      // 已有草稿时不覆盖坐席已写的内容
      if (!draft.value.trim()) draft.value = r.draft
    } else {
      aiDraftErr.value = t('cs.aiDraftEmpty')
    }
  } catch (e) {
    aiDraftErr.value = e?.message || String(e)
  } finally {
    aiDrafting.value = false
  }
}

async function setStatus(s) {  if (!current.value) return
  try {
    await requestJSON(`/cs/conversations/${encodeURIComponent(current.value.id)}/status`, {
      method: 'POST',
      body: JSON.stringify({ status: s }),
    })
    current.value.status = s
    loadInbox()
  } catch (e) {
    err.value = e?.message || String(e)
  }
}

onMounted(() => {
  loadContacts()
  loadInbox()
  // 轮询拉新（访客侧断线重连亦同源）
  pollTimer = setInterval(() => {
    loadInbox()
    if (current.value) openConv(current.value)
  }, 15000)
})
onUnmounted(() => { if (pollTimer) clearInterval(pollTimer) })
</script>

<style scoped>
.cs-root { display: flex; flex-direction: column; gap: 12px; height: 100%; }
.cs-head { display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap; }
.cs-title { font-size: 18px; font-weight: 600; margin: 0; }
.cs-filters { display: flex; gap: 8px; }
.cs-sel, .cs-btn, .cs-send { border: 1px solid var(--border, #e3e6eb); background: var(--surface, #fff); color: var(--ink, #1f2328); border-radius: 8px; padding: 6px 10px; font-size: 13px; cursor: pointer; }
.cs-send { background: var(--accent, #2f6feb); color: #fff; border-color: transparent; }
.cs-reply-tools { display: flex; align-items: center; gap: 8px; margin-bottom: 6px; }
.cs-ai-err { color: var(--danger, #d1242f); font-size: 12px; }
.cs-send:disabled, .cs-btn:disabled { opacity: .6; cursor: not-allowed; }
.cs-err { color: #c0392b; font-size: 13px; margin: 0; }
.cs-body { display: grid; grid-template-columns: 300px 1fr; gap: 12px; min-height: 0; flex: 1; }
.cs-list { border: 1px solid var(--border, #e3e6eb); border-radius: 10px; overflow: auto; background: var(--surface, #fff); }
.cs-item { display: block; width: 100%; text-align: left; border: 0; border-bottom: 1px solid var(--border, #eef1f4); background: transparent; padding: 10px 12px; cursor: pointer; position: relative; }
.cs-item:hover { background: var(--hover, #f6f8fa); }
.cs-item.active { background: var(--accent-soft, #eef4ff); }
.cs-item-top { display: flex; justify-content: space-between; gap: 8px; }
.cs-item-name { font-weight: 600; font-size: 13px; }
.cs-item-time { font-size: 11px; color: var(--ink-3, #8b949e); }
.cs-item-sub { display: flex; gap: 6px; align-items: center; margin-top: 4px; }
.cs-chan { font-size: 10px; border: 1px solid var(--border, #e3e6eb); border-radius: 4px; padding: 0 4px; color: var(--ink-3, #8b949e); }
.cs-subj { font-size: 12px; color: var(--ink-2, #57606a); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.cs-badge { position: absolute; right: 10px; bottom: 8px; min-width: 16px; height: 16px; line-height: 16px; text-align: center; border-radius: 8px; background: #d1242f; color: #fff; font-size: 10px; padding: 0 4px; }
.cs-conv { border: 1px solid var(--border, #e3e6eb); border-radius: 10px; background: var(--surface, #fff); display: flex; flex-direction: column; min-height: 0; }
.cs-conv-empty { align-items: center; justify-content: center; }
.cs-conv-head { display: flex; justify-content: space-between; align-items: center; padding: 10px 12px; border-bottom: 1px solid var(--border, #eef1f4); }
.cs-conv-name { font-weight: 600; font-size: 14px; }
.cs-conv-meta { font-size: 11px; color: var(--ink-3, #8b949e); }
.cs-msgs { flex: 1; overflow: auto; padding: 12px; display: flex; flex-direction: column; gap: 10px; min-height: 0; }
.cs-msg { max-width: 78%; padding: 8px 10px; border-radius: 10px; font-size: 13px; }
.cs-msg.in { align-self: flex-start; background: var(--hover, #f2f4f7); }
.cs-msg.out { align-self: flex-end; background: var(--accent-soft, #eef4ff); }
.cs-msg-meta { display: flex; gap: 8px; font-size: 10px; color: var(--ink-3, #8b949e); margin-bottom: 3px; }
.cs-msg-body { white-space: pre-wrap; word-break: break-word; }
.cs-reply { display: flex; gap: 8px; padding: 10px 12px; border-top: 1px solid var(--border, #eef1f4); }
.cs-textarea { flex: 1; resize: vertical; border: 1px solid var(--border, #e3e6eb); border-radius: 8px; padding: 8px; font-size: 13px; font-family: inherit; background: var(--surface, #fff); color: var(--ink, #1f2328); }
.cs-empty { padding: 24px; text-align: center; color: var(--ink-3, #8b949e); font-size: 13px; }
</style>
