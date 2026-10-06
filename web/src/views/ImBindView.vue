<template>
  <div class="ib-view">
    <div class="ib-head">
      <div>
        <h2>{{  $t('IM 绑定')  }}</h2>
        <p class="ib-sub">
          {{  $t('把 Telegram / 企业微信账号绑到站内身份：之后在 IM 里发文、查询都记在该账号名下， 日报与通知也能推到你的会话。绑定走「网页生成码 → IM 发送 /bind 码」，不用在聊天里输密码。')  }}
        </p>
      </div>
      <button class="btn" :disabled="loading" @click="load"><AikIcon name="refresh" :size="14" />{{  $t('刷新')  }}</button>
    </div>

    <section class="ib-card">
      <div class="ib-card-head">
        <h3>{{  $t('生成绑定码')  }}</h3>
        <span class="ib-tip">{{  $t('在 IM 里发送')  }} <code>{{  $t('/bind 绑定码')  }}</code> {{  $t('完成绑定')  }}</span>
      </div>
      <div class="ib-gen">
        <label class="ib-field">
          <span>{{  $t('平台')  }}</span>
          <select v-model="platform">
            <option value="">{{  $t('任意平台')  }}</option>
            <option value="telegram">Telegram</option>
            <option value="wecom">{{  $t('企业微信')  }}</option>
          </select>
        </label>
        <button class="btn btn-primary" :disabled="generating" @click="genCode">
          <AikIcon name="link" :size="14" />{{  generating ? $t('生成中…') : $t('生成绑定码')  }}
        </button>
      </div>

      <div v-if="code" class="ib-code-box">
        <div class="ib-code">{{  code.code  }}</div>
        <div class="ib-code-meta">
          <span>{{  $t('在 IM 里发送：')  }}<code>/bind {{  code.code  }}</code></span>
          <span :class="{ exp: remain <= 0 }">
{{ remain > 0 ? $t('剩余 ') + fmtRemain(remain) : $t('已过期，请重新生成') }}
          </span>
        </div>
        <button class="btn btn-sm" @click="copyCmd"><AikIcon name="copy" :size="13" />{{  $t('复制指令')  }}</button>
      </div>

      <p v-if="platformState" class="ib-warn"><AikIcon name="alert" :size="13" />{{  platformState  }}</p>
    </section>

    <section class="ib-card">
      <div class="ib-card-head">
        <h3>{{  $t('已绑定')  }} <span class="ib-n">{{  items.length  }}</span></h3>
        <span class="ib-tip">{{  $t('一个 IM 账号同时只对应一个站内账号')  }}</span>
      </div>

      <div v-if="loading" class="ib-empty">{{  $t('加载中…')  }}</div>
      <div v-else-if="!items.length" class="ib-empty">
        <AikIcon name="link" :size="26" />
        <p>{{  $t('还没有绑定任何 IM 账号')  }}</p>
        <p class="ib-hint">{{  $t('先在上面生成绑定码，再到 Telegram / 企业微信里发给机器人。')  }}</p>
      </div>

      <div v-for="b in items" :key="b.id" class="ib-row">
        <span class="ib-ico"><AikIcon name="chat" :size="15" /></span>
        <div class="ib-main">
          <div class="ib-title">
            <b>{{  platName(b.platform)  }}</b>
            <span class="ib-id">{{  b.platform_name || b.platform_user_id  }}</span>
          </div>
          <div class="ib-meta">
            <span>{{  $t('绑定于')  }} {{  fmtTime(b.bound_at)  }}</span>
            <span v-if="b.last_active_at">{{  $t('· 最近活跃')  }} {{  fmtTime(b.last_active_at)  }}</span>
            <span v-if="b.chat_id">{{  $t('· 推送目标')  }} {{  b.chat_id  }}</span>
          </div>
        </div>
        <button class="btn btn-sm" @click="unbind(b)"><AikIcon name="trash" :size="13" />{{  $t('解绑')  }}</button>
      </div>
    </section>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import AikIcon from '@/components/AikIcon.vue'
import { listImBindings, createImBindCode, deleteImBinding } from '@/api'
import { t } from '@/i18n'

const items = ref([])
const platforms = ref({})
const loading = ref(false)
const generating = ref(false)
const platform = ref('')
const code = ref(null)
const remain = ref(0)
let timer = null

const platName = (p) => (p === 'telegram' ? 'Telegram' : p === 'wecom' ? t('企业微信') : p || t('未知平台'))

const platformState = computed(() => {
  if (!platform.value) return ''
  const st = platforms.value[platform.value]
  if (st && st.enabled === false) {
    return `${platName(platform.value)} 尚未在站点侧配置，现在绑定也收不到消息。请先到「设置 → 消息」填写 Bot / 应用配置。`
  }
  return ''
})

async function load() {
  loading.value = true
  try {
    const res = await listImBindings()
    items.value = res.items || []
    platforms.value = res.platforms || {}
  } catch (e) {
    items.value = []
  } finally {
    loading.value = false
  }
}

async function genCode() {
  generating.value = true
  try {
    const res = await createImBindCode(platform.value)
    code.value = res
    startCountdown(res.expires_at)
  } catch (e) {
    window.alert(t('生成失败：') + (e.message || e))
  } finally {
    generating.value = false
  }
}

function startCountdown(expiresAt) {
  if (timer) clearInterval(timer)
  const tick = () => {
    remain.value = Math.max(0, Math.floor((expiresAt - Date.now()) / 1000))
    if (remain.value <= 0 && timer) {
      clearInterval(timer)
      timer = null
    }
  }
  tick()
  timer = setInterval(tick, 1000)
}

function fmtRemain(s) {
  const m = Math.floor(s / 60)
  return m > 0 ? `${m} 分 ${s % 60} 秒` : `${s} 秒`
}

async function unbind(b) {
  if (!window.confirm(`确认解除 ${platName(b.platform)}（${b.platform_name || b.platform_user_id}）的绑定？`)) return
  try {
    await deleteImBinding(b.id)
    await load()
  } catch (e) {
    window.alert(t('解绑失败：') + (e.message || e))
  }
}

function copyCmd() {
  if (!code.value) return
  const text = `/bind ${code.value.code}`
  if (navigator.clipboard?.writeText) {
    navigator.clipboard.writeText(text)
  }
}

function fmtTime(ms) {
  if (!ms) return '-'
  const d = new Date(ms)
  const p = (n) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`
}

onMounted(load)
onBeforeUnmount(() => {
  if (timer) clearInterval(timer)
})
</script>

<style scoped>
.ib-view { max-width: 1200px; margin: 0 auto; padding: 20px 24px 40px; }
.ib-head { display: flex; justify-content: space-between; align-items: flex-start; gap: 16px; margin-bottom: 18px; }
.ib-head h2 { margin: 0 0 6px; font-size: 18px; }
.ib-sub { margin: 0; color: var(--text-2, #666); font-size: 13px; line-height: 1.6; max-width: 640px; }
.ib-card { border: 1px solid var(--border, #e5e7eb); border-radius: 10px; padding: 14px 16px; margin-bottom: 16px; background: var(--panel, #fff); }
.ib-card-head { display: flex; align-items: baseline; gap: 10px; margin-bottom: 12px; }
.ib-card-head h3 { margin: 0; font-size: 14px; }
.ib-tip { color: var(--text-2, #888); font-size: 12px; }
.ib-n { color: var(--text-2, #888); font-weight: 400; }
.ib-gen { display: flex; align-items: flex-end; gap: 12px; flex-wrap: wrap; }
.ib-field { display: flex; flex-direction: column; gap: 4px; font-size: 12px; color: var(--text-2, #666); }
.ib-field select { padding: 6px 10px; border: 1px solid var(--border, #d1d5db); border-radius: 6px; background: var(--panel, #fff); color: inherit; font-size: 13px; }
.ib-code-box { margin-top: 14px; padding: 12px 14px; border: 1px dashed var(--border, #d1d5db); border-radius: 8px; display: flex; flex-direction: column; gap: 8px; align-items: flex-start; }
.ib-code { font-size: 28px; font-weight: 700; letter-spacing: 4px; font-family: ui-monospace, SFMono-Regular, Menlo, monospace; }
.ib-code-meta { display: flex; gap: 14px; font-size: 12px; color: var(--text-2, #666); flex-wrap: wrap; }
.ib-code-meta .exp { color: #b91c1c; }
.ib-code-meta code, .ib-tip code { background: var(--bg, #f3f4f6); padding: 1px 5px; border-radius: 4px; font-size: 12px; }
.ib-warn { margin: 12px 0 0; font-size: 12px; color: #b45309; display: flex; align-items: center; gap: 6px; }
.ib-empty { padding: 26px 0; text-align: center; color: var(--text-2, #888); font-size: 13px; }
.ib-empty p { margin: 6px 0; }
.ib-hint { font-size: 12px; color: var(--text-3, #aaa); }
.ib-row { display: flex; align-items: center; gap: 12px; padding: 10px 4px; border-top: 1px solid var(--border, #f0f0f0); }
.ib-row:first-of-type { border-top: none; }
.ib-ico { color: var(--text-2, #666); }
.ib-main { flex: 1; min-width: 0; }
.ib-title { display: flex; align-items: baseline; gap: 8px; font-size: 13px; }
.ib-id { color: var(--text-2, #888); font-size: 12px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.ib-meta { font-size: 12px; color: var(--text-3, #999); margin-top: 3px; display: flex; gap: 6px; flex-wrap: wrap; }
.btn { display: inline-flex; align-items: center; gap: 5px; padding: 6px 12px; border: 1px solid var(--border, #d1d5db); border-radius: 6px; background: var(--panel, #fff); color: inherit; font-size: 13px; cursor: pointer; }
.btn:disabled { opacity: 0.5; cursor: not-allowed; }
.btn-sm { padding: 4px 9px; font-size: 12px; }
.btn-primary { background: var(--accent, #2563eb); border-color: var(--accent, #2563eb); color: #fff; }
</style>
