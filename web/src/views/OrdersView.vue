<template>
  <section class="orders">
    <h1>{{ t('store.orders') }}</h1>

    <div class="form" v-if="!order">
      <p class="tip">{{ t('store.orderLookup') }}</p>
      <input v-model="email" :placeholder="t('store.contactEmail')" />
      <input v-model="no" :placeholder="t('store.orderNo')" />
      <button class="btn primary" :disabled="searching" @click="lookup">{{ t('store.lookup') }}</button>
    </div>

    <div v-if="searching" class="hint">{{ t('common.loading') }}</div>

    <div v-if="searched && !order" class="hint error">{{ t('store.notFound') }}</div>

    <div v-if="order" class="detail">
      <div class="head">
        <div>
          <div class="ono">#{{ order.order_no }}</div>
          <span class="st" :class="statusClass(order.status)">{{ statusText(order.status) }}</span>
        </div>
        <button class="btn ghost" @click="reset">{{ t('store.lookup') }}</button>
      </div>

      <div class="items">
        <div class="it" v-for="it in order.items" :key="it.id">
          <span class="t">{{ it.title }} × {{ it.qty }}
            <em v-if="it.kind === 'digital' && quotaText(it)" class="q">{{ quotaText(it) }}</em>
          </span>
          <span class="s">{{ money(it.subtotal_cents, order.currency) }}</span>
          <a v-if="it.kind === 'digital' && paid && !exhausted(it)" class="dl" :href="dlUrl(it)">{{ t('store.download') }}</a>
          <span v-else-if="it.kind === 'digital' && exhausted(it)" class="dl off">{{ t('store.downloadUsed') }}</span>
          <span v-else-if="it.kind === 'digital'" class="dl off">{{ t('store.notPaid') }}</span>
        </div>
      </div>
      <p v-if="hasDigital" class="dhint">{{ t('store.deliverHint') }}</p>

      <div class="amounts">
        <div><span>{{ t('store.subtotal') }}</span><span>{{ money(order.subtotal_cents, order.currency) }}</span></div>
        <div v-if="order.discount_cents"><span>{{ t('store.discount') }}</span><span>-{{ money(order.discount_cents, order.currency) }}</span></div>
        <div class="tot"><span>{{ t('store.total') }}</span><strong>{{ money(order.total_cents, order.currency) }}</strong></div>
      </div>

      <div class="meta">
        <div><b>{{ t('store.contactName') }}</b> {{ order.contact_name || '-' }}</div>
        <div><b>{{ t('store.contactEmail') }}</b> {{ order.contact_email || '-' }}</div>
        <div><b>{{ t('store.address') }}</b> {{ addrText(order.address) }}</div>
        <div v-if="order.remark"><b>{{ t('store.remark') }}</b> {{ order.remark }}</div>
        <div><b>{{ t('store.gateway') }}</b> {{ gwText(order.gateway) }}</div>
        <div><b>{{ t('store.createdAt') }}</b> {{ fmt(order.created_at) }}</div>
        <div v-if="order.paid_at"><b>{{ t('store.paidAt') }}</b> {{ fmt(order.paid_at) }}</div>
      </div>

      <RouterLink to="/store" class="btn">{{ t('store.continueShop') }}</RouterLink>

      <!-- 买家侧退货申请：仅 paid/fulfilled 可发起 -->
      <div v-if="returnable" class="ret">
        <h3>{{ t('store.returnTitle') }}</h3>
        <p class="tip">{{ t('store.returnTip') }}</p>
        <textarea v-model="retReason" rows="2" :placeholder="t('store.returnReason')"></textarea>
        <button class="btn" :disabled="submitting" @click="submitReturn">{{ t('store.returnApply') }}</button>
      </div>
    </div>
  </section>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { storeOrderLookup, storeOrderDownloadUrl, storeReturnCreate } from '@/api'
import { t } from '@/i18n'
import { useToastStore } from '@/stores/toast'

const route = useRoute()
const toast = useToastStore()
const email = ref('')
const no = ref('')
const order = ref(null)
const searching = ref(false)
const searched = ref(false)
const retReason = ref('')
const submitting = ref(false)

const paid = computed(() => order.value?.status === 'paid' || order.value?.status === 'fulfilled')
const returnable = computed(() => order.value?.status === 'paid' || order.value?.status === 'fulfilled')
const hasDigital = computed(() => (order.value?.items || []).some((i) => i.kind === 'digital'))
function dlUrl(it) {
  return storeOrderDownloadUrl(order.value.order_no, it.id, email.value, it.download_token || '')
}
// 次数上限用尽（max_downloads>0 且已达上限）
function exhausted(it) {
  return it.max_downloads > 0 && (it.download_count || 0) >= it.max_downloads
}
// 剩余次数文案，如「下载 2/5」
function quotaText(it) {
  if (!it.max_downloads) return ''
  return t('store.downloadQuota', { used: it.download_count || 0, max: it.max_downloads })
}

function money(cents, cur) {
  const v = ((cents || 0) / 100).toFixed(2)
  return (cur === 'CNY' ? '¥' : (cur || '') + ' ') + v
}
function statusText(s) {
  return { pending: t('store.pending'), paid: t('store.paid'), fulfilled: t('store.fulfilled'), refunded: t('store.refunded'), expired: t('store.expired'), cancelled: t('store.cancelled') }[s] || s
}
function statusClass(s) {
  return { pending: 'pen', paid: 'ok', fulfilled: 'ok', refunded: 'rf', expired: 'no', cancelled: 'no' }[s] || ''
}
function gwText(g) {
  return { mock: t('store.gatewayMock'), wechat: t('store.gatewayWechat'), alipay: t('store.gatewayAlipay'), stripe: t('store.gatewayStripe') }[g] || g
}
function addrText(a) {
  if (!a || a === '{}') return '-'
  try { const o = JSON.parse(a); return Object.values(o).filter(Boolean).join(' ') || '-' } catch { return a }
}
function fmt(ms) {
  if (!ms) return '-'
  const d = new Date(ms)
  return isNaN(d) ? '-' : d.toLocaleString()
}
async function submitReturn() {
  if (retReason.value.trim().length < 2) { toast.error(t('store.returnReason')); return }
  submitting.value = true
  try {
    await storeReturnCreate(order.value.order_no, email.value, retReason.value.trim())
    toast.success(t('store.returnApplied'))
    retReason.value = ''
  } catch (e) {
    toast.error(e.message || 'failed')
  } finally {
    submitting.value = false
  }
}
function reset() { order.value = null; searched.value = false; no.value = ''; email.value = '' }
async function lookup() {
  if (!email.value || !no.value) { toast.error(t('store.orderLookup')); return }
  searching.value = true
  try {
    order.value = await storeOrderLookup(email.value.trim(), no.value.trim())
    searched.value = true
  } catch (e) {
    order.value = null
    searched.value = true
    toast.error(e.message || t('store.notFound'))
  } finally {
    searching.value = false
  }
}
onMounted(() => {
  if (route.query.order_no && route.query.email) {
    no.value = String(route.query.order_no)
    email.value = String(route.query.email)
    lookup()
  }
})
</script>

<style scoped>
.orders { max-width: 640px; margin: 0 auto; }
h1 { font-size: 22px; margin: 0 0 16px; }
.form { display: flex; flex-direction: column; gap: 10px; background: var(--surface, #fff); border: 1px solid var(--border, #e6e8eb); border-radius: 10px; padding: 18px; }
.tip { font-size: 13px; color: var(--text-2, #5b6168); margin: 0 0 4px; }
.form input { padding: 10px 12px; border: 1px solid var(--border, #e6e8eb); border-radius: 8px; background: var(--surface, #fff); color: var(--text, #1f2329); }
.btn { padding: 9px 16px; border: 1px solid var(--border, #e6e8eb); border-radius: 8px; background: var(--surface, #fff); color: var(--text, #1f2329); cursor: pointer; text-decoration: none; display: inline-block; text-align: center; }
.btn.primary { background: var(--accent, #2f6bff); color: #fff; border-color: transparent; }
.btn.ghost { background: transparent; }
.btn:disabled { opacity: .6; }
.hint { padding: 30px; text-align: center; color: var(--text-2, #5b6168); }
.hint.error { color: #e8453c; }
.detail { background: var(--surface, #fff); border: 1px solid var(--border, #e6e8eb); border-radius: 10px; padding: 18px; }
.head { display: flex; justify-content: space-between; align-items: center; margin-bottom: 14px; }
.ono { font-size: 18px; font-weight: 700; }
.st { font-size: 12px; padding: 2px 8px; border-radius: 6px; margin-left: 8px; }
.st.pen { color: #b8860b; background: #fdf3e0; }
.st.ok { color: #2e9e5b; background: #e7f6ec; }
.st.rf { color: #6a5acd; background: #eeeafc; }
.st.no { color: #e8453c; background: #fdecea; }
.items { border-top: 1px dashed var(--border, #e6e8eb); }
.it { display: flex; justify-content: space-between; align-items: center; gap: 10px; padding: 8px 0; border-bottom: 1px dashed var(--border, #e6e8eb); }
.it .s { color: #e8453c; }
.dl { font-size: 12px; color: var(--accent, #2f6bff); text-decoration: none; border: 1px solid var(--border, #e6e8eb); border-radius: 6px; padding: 3px 10px; }
.dl:hover { background: color-mix(in srgb, var(--accent, #2f6bff) 10%, transparent); }
.dl.off { color: var(--text-3, #8a9099); cursor: default; }
.dl.off:hover { background: none; }
.q { font-style: normal; font-size: 11px; color: var(--text-3, #8a9099); margin-left: 6px; }
.ret { margin-top: 18px; padding-top: 14px; border-top: 1px solid var(--border, #e6e8eb); }
.ret h3 { font-size: 14px; margin: 0 0 4px; }
.ret .tip { font-size: 12px; color: var(--text-2, #5b6168); margin: 0 0 8px; }
.ret textarea { width: 100%; box-sizing: border-box; padding: 9px 10px; border: 1px solid var(--border, #e6e8eb); border-radius: 8px; background: var(--surface, #fff); color: var(--text, #1f2329); font-family: inherit; margin-bottom: 8px; }
.dhint { font-size: 12px; color: var(--text-2, #5b6168); margin: 10px 0 0; }
.amounts { margin: 12px 0; }
.amounts > div { display: flex; justify-content: space-between; padding: 4px 0; font-size: 14px; }
.amounts .tot { border-top: 1px solid var(--border, #e6e8eb); margin-top: 6px; padding-top: 8px; }
.amounts .tot strong { color: #e8453c; font-size: 18px; }
.meta { display: grid; grid-template-columns: 1fr 1fr; gap: 8px 16px; font-size: 13px; color: var(--text-2, #5b6168); margin: 14px 0; }
.meta b { color: var(--text, #1f2329); }
</style>
