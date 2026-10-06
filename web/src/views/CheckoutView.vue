<template>
  <section class="checkout" v-if="!done">
    <h1>{{ t('store.checkoutTitle') }}</h1>
    <div v-if="loading" class="hint">{{ t('common.loading') }}</div>
    <template v-else>
      <div v-if="!lines.length" class="hint empty">
        <p>{{ t('store.cartEmpty') }}</p>
        <RouterLink to="/store" class="btn primary">{{ t('store.goShop') }}</RouterLink>
      </div>
      <div v-else class="wrap">
        <div class="left">
          <h3>{{ t('store.items') }}</h3>
          <div class="line" v-for="l in lines" :key="l.product.id">
            <span class="t">{{ l.product.title }} × {{ l.qty }}</span>
            <span class="s">{{ lineSub(l) }}</span>
          </div>
          <div class="sub">
            <span>{{ t('store.subtotal') }}</span><strong>{{ subtotalText }}</strong>
          </div>

          <div class="field">
            <label>{{ t('store.couponCode') }}</label>
            <div class="row">
              <input v-model="coupon" :placeholder="t('store.couponCode')" />
              <span v-if="couponMsg" class="msg">{{ couponMsg }}</span>
            </div>
          </div>

          <div class="field">
            <label>{{ t('store.contactName') }} *</label>
            <input v-model="form.name" :placeholder="t('store.contactName')" />
          </div>
          <div class="field">
            <label>{{ t('store.contactEmail') }} *</label>
            <input v-model="form.email" :placeholder="t('store.contactEmail')" />
          </div>
          <div class="field">
            <label>{{ t('store.address') }}</label>
            <textarea v-model="form.address" :placeholder="t('store.address')" rows="2"></textarea>
          </div>
          <div class="field">
            <label>{{ t('store.remark') }}</label>
            <textarea v-model="form.remark" :placeholder="t('store.remark')" rows="2"></textarea>
          </div>
        </div>

        <div class="right">
          <h3>{{ t('store.gateway') }}</h3>
          <label v-for="g in gateways" :key="g.name" class="gw" :class="{ on: form.gateway === g.name }">
            <input type="radio" :value="g.name" v-model="form.gateway" />
            <span class="gl">{{ gwLabel(g) }}</span>
            <span class="gd" :class="g.configured ? 'ok' : 'no'">
              {{ g.configured ? t('store.gatewayConfigured') : t('store.gatewayUnconfig') }}
            </span>
          </label>
          <div class="total">
            <span>{{ t('store.total') }}</span>
            <strong>{{ subtotalText }}</strong>
          </div>
          <button class="btn primary pay" :disabled="submitting" @click="submit">
            {{ submitting ? t('store.paying') : t('store.pay') }}
          </button>
          <p class="tip">{{ t('store.payTip') }}</p>
        </div>
      </div>
    </template>
  </section>

  <!-- 支付结果态 -->
  <section class="result" v-else>
    <h1>{{ t('store.paySuccess') }}</h1>
    <p class="ono">#{{ orderNo }}</p>
    <div v-if="payment && payment.pay_url" class="paybox">
      <a class="btn primary" :href="payment.pay_url" target="_blank" rel="noopener">{{ t('store.redirectPay') }}</a>
    </div>
    <div v-if="payment && payment.code_url" class="paybox">
      <p>{{ t('store.scanToPay') }}</p>
      <code class="code">{{ payment.code_url }}</code>
    </div>
    <RouterLink :to="lookupLink" class="btn">{{ t('store.orders') }}</RouterLink>
  </section>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { storeCartGet, storeGateways, storeCheckout, storeMockPay } from '@/api'
import { t } from '@/i18n'
import { useToastStore } from '@/stores/toast'

const route = useRoute()
const router = useRouter()
const toast = useToastStore()
const lines = ref([])
const gateways = ref([])
const loading = ref(true)
const submitting = ref(false)
const coupon = ref(route.query.coupon || '')
const couponMsg = ref('')
const form = ref({ name: '', email: '', address: '', remark: '', gateway: 'mock' })
const done = ref(false)
const orderNo = ref('')
const payment = ref(null)

const subtotal = computed(() =>
  lines.value.reduce((s, l) => s + (l.product?.price_cents || 0) * (l.qty || 0), 0)
)
const currency = computed(() => lines.value[0]?.product?.currency || 'CNY')
const subtotalText = computed(() => {
  const v = (subtotal.value / 100).toFixed(2)
  return (currency.value === 'CNY' ? '¥' : currency.value + ' ') + v
})
function lineSub(l) {
  const v = ((l.product?.price_cents || 0) * (l.qty || 0) / 100).toFixed(2)
  return (currency.value === 'CNY' ? '¥' : currency.value + ' ') + v
}
function gwLabel(g) {
  return { mock: t('store.gatewayMock'), wechat: t('store.gatewayWechat'), alipay: t('store.gatewayAlipay'), stripe: t('store.gatewayStripe') }[g.name] || g.label || g.name
}
const lookupLink = computed(() => ({
  path: '/store/orders',
  query: { order_no: orderNo.value, email: form.value.email }
}))

async function submit() {
  if (!form.value.email || !/^[^@\s]+@[^@\s]+\.[^@\s]+$/.test(form.value.email)) {
    toast.error(t('store.contactEmail') + ' *')
    return
  }
  if (!form.value.gateway) { toast.error(t('store.gateway')); return }
  submitting.value = true
  try {
    const res = await storeCheckout({
      gateway: form.value.gateway,
      coupon_code: coupon.value.trim(),
      contact_email: form.value.email,
      contact_name: form.value.name,
      address: form.value.address,
      remark: form.value.remark
    })
    orderNo.value = res.order
    payment.value = res.payment
    if (form.value.gateway === 'mock') {
      await storeMockPay(res.order)
      done.value = true
      toast.success(t('store.paySuccess'))
    } else {
      done.value = true
    }
  } catch (e) {
    toast.error(e.message || 'checkout failed')
  } finally {
    submitting.value = false
  }
}
onMounted(async () => {
  loading.value = true
  try {
    const [c, g] = await Promise.all([storeCartGet(), storeGateways()])
    lines.value = c.lines || []
    gateways.value = (g.gateways || []).filter((x) => x.configured)
    const cfg = gateways.value.find((x) => x.configured)
    if (cfg) form.value.gateway = cfg.name
  } catch (e) {
    toast.error(e.message || 'fail')
  } finally {
    loading.value = false
  }
})
</script>

<style scoped>
.checkout, .result { max-width: 860px; margin: 0 auto; }
h1 { font-size: 22px; margin: 0 0 16px; }
.hint { padding: 50px; text-align: center; color: var(--text-2, #5b6168); }
.empty { display: flex; flex-direction: column; align-items: center; gap: 14px; }
.wrap { display: flex; gap: 20px; flex-wrap: wrap; }
.left { flex: 1; min-width: 320px; }
.right { width: 280px; flex: none; background: var(--surface, #fff); border: 1px solid var(--border, #e6e8eb); border-radius: 10px; padding: 16px; align-self: flex-start; }
h3 { font-size: 15px; margin: 0 0 10px; }
.line { display: flex; justify-content: space-between; padding: 8px 0; border-bottom: 1px dashed var(--border, #e6e8eb); }
.line .s { color: #e8453c; }
.sub { display: flex; justify-content: space-between; margin-top: 10px; font-size: 15px; }
.sub strong { color: #e8453c; }
.field { margin-top: 12px; }
.field label { display: block; font-size: 13px; color: var(--text-2, #5b6168); margin-bottom: 4px; }
.field input, .field textarea, .coupon input {
  width: 100%; box-sizing: border-box; padding: 9px 10px; border: 1px solid var(--border, #e6e8eb);
  border-radius: 8px; background: var(--surface, #fff); color: var(--text, #1f2329); font-family: inherit;
}
.row { display: flex; gap: 8px; align-items: center; }
.msg { font-size: 12px; color: var(--text-2, #5b6168); }
.gw { display: flex; align-items: center; gap: 8px; padding: 10px; border: 1px solid var(--border, #e6e8eb); border-radius: 8px; margin-bottom: 8px; cursor: pointer; }
.gw.on { border-color: var(--accent, #2f6bff); background: color-mix(in srgb, var(--accent, #2f6bff) 8%, transparent); }
.gl { flex: 1; }
.gd { font-size: 11px; padding: 1px 6px; border-radius: 6px; }
.gd.ok { color: #2e9e5b; background: #e7f6ec; }
.gd.no { color: #e8453c; background: #fdecea; }
.total { display: flex; justify-content: space-between; align-items: baseline; margin: 12px 0; }
.total strong { font-size: 20px; color: #e8453c; }
.pay { width: 100%; }
.btn { padding: 9px 16px; border: 1px solid var(--border, #e6e8eb); border-radius: 8px; background: var(--surface, #fff); color: var(--text, #1f2329); cursor: pointer; text-decoration: none; display: inline-block; text-align: center; }
.btn.primary { background: var(--accent, #2f6bff); color: #fff; border-color: transparent; }
.btn:disabled { opacity: .6; cursor: default; }
.tip { font-size: 12px; color: var(--text-2, #5b6168); margin-top: 10px; line-height: 1.5; }
.result { text-align: center; padding-top: 20px; }
.result .ono { font-size: 18px; font-weight: 700; color: #2e9e5b; }
.paybox { margin: 18px 0; display: flex; flex-direction: column; align-items: center; gap: 8px; }
.code { background: #f3f4f6; padding: 8px 10px; border-radius: 6px; font-size: 12px; word-break: break-all; max-width: 100%; }
</style>
