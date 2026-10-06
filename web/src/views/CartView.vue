<template>
  <section class="cart">
    <h1>{{ t('store.cart') }}</h1>

    <div v-if="loading" class="hint">{{ t('common.loading') }}</div>
    <div v-else-if="!lines.length" class="hint empty">
      <p>{{ t('store.cartEmpty') }}</p>
      <RouterLink to="/store" class="btn primary">{{ t('store.goShop') }}</RouterLink>
    </div>

    <template v-else>
      <div class="lines">
        <div class="line" v-for="l in lines" :key="l.product.id">
          <div class="cover" :style="coverStyle(l.product)">
            <span v-if="!l.product.cover" class="ph">·</span>
          </div>
          <div class="info" @click="goDetail(l.product.slug)">
            <div class="t">{{ l.product.title }}</div>
            <div class="p">{{ priceText(l.product) }}</div>
          </div>
          <div class="stepper">
            <button @click="setQty(l.product.id, l.qty - 1)">−</button>
            <input :value="l.qty" type="number" min="1" @change="onQty(l.product.id, $event)" />
            <button @click="setQty(l.product.id, l.qty + 1)">+</button>
          </div>
          <div class="sub">{{ lineSub(l) }}</div>
          <button class="rm" @click="remove(l.product.id)" :title="t('store.remove')">×</button>
        </div>
      </div>

      <div class="summary">
        <div class="coupon">
          <input v-model="coupon" :placeholder="t('store.couponCode')" />
          <button class="btn" @click="applyCoupon">{{ t('store.applyCoupon') }}</button>
          <span v-if="couponMsg" class="cm">{{ couponMsg }}</span>
        </div>
        <div class="total">
          <span>{{ t('store.subtotal') }}</span>
          <strong>{{ subtotalText }}</strong>
        </div>
        <div class="acts">
          <button class="btn" @click="clear">{{ t('store.clearCart') }}</button>
          <button class="btn primary" @click="goCheckout">{{ t('store.checkout') }}</button>
        </div>
      </div>
    </template>
  </section>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { storeCartGet, storeCartSet, storeCartRemove, storeCartClear, storeCoverUrl } from '@/api'
import { t } from '@/i18n'
import { useToastStore } from '@/stores/toast'
import { refreshCart } from '@/stores/storeCart'

const router = useRouter()
const toast = useToastStore()
const lines = ref([])
const loading = ref(true)
const coupon = ref('')
const couponMsg = ref('')

const subtotalText = computed(() => {
  const c = currency.value
  const v = (subtotal.value / 100).toFixed(2)
  return (c === 'CNY' ? '¥' : (c || '') + ' ') + v
})
const subtotal = computed(() =>
  lines.value.reduce((s, l) => s + (l.product?.price_cents || 0) * (l.qty || 0), 0)
)
const currency = computed(() => lines.value[0]?.product?.currency || 'CNY')

function priceText(p) {
  const v = ((p.price_cents || 0) / 100).toFixed(2)
  return (p.currency === 'CNY' ? '¥' : (p.currency || '') + ' ') + v
}
function lineSub(l) {
  const v = ((l.product?.price_cents || 0) * (l.qty || 0) / 100).toFixed(2)
  return (l.product?.currency === 'CNY' ? '¥' : (l.product?.currency || '') + ' ') + v
}
function coverStyle(p) {
  const u = storeCoverUrl(p.cover)
  return u ? { backgroundImage: `url(${u})` } : {}
}
function goDetail(slug) { router.push(`/store/product/${slug}`) }
async function reload() {
  const c = await storeCartGet()
  lines.value = c.lines || []
  return c
}
async function setQty(id, q) {
  if (q < 1) return remove(id)
  await storeCartSet(id, q)
  await reload(); await refreshCart()
}
function onQty(id, e) {
  const v = parseInt(e.target.value, 10)
  setQty(id, isNaN(v) ? 1 : v)
}
async function remove(id) {
  await storeCartRemove(id)
  await reload(); await refreshCart()
}
async function clear() {
  await storeCartClear()
  lines.value = []
  await refreshCart()
  toast.success(t('store.clearCart'))
}
function applyCoupon() {
  couponMsg.value = coupon.value.trim() ? '' : t('store.couponCode')
}
function goCheckout() {
  router.push({ path: '/store/checkout', query: coupon.value.trim() ? { coupon: coupon.value.trim() } : {} })
}
onMounted(async () => {
  loading.value = true
  try { await reload() } catch (e) { toast.error(e.message || 'fail') } finally { loading.value = false }
})
</script>

<style scoped>
.cart { max-width: 860px; margin: 0 auto; }
h1 { font-size: 22px; margin: 0 0 16px; }
.hint { padding: 50px; text-align: center; color: var(--text-2, #5b6168); }
.empty { display: flex; flex-direction: column; align-items: center; gap: 14px; }
.lines { display: flex; flex-direction: column; gap: 10px; }
.line {
  display: flex; align-items: center; gap: 14px; padding: 12px;
  background: var(--surface, #fff); border: 1px solid var(--border, #e6e8eb); border-radius: 10px;
}
.cover { width: 56px; height: 56px; border-radius: 8px; flex: none; background: #eef1f5 center/cover no-repeat; display: flex; align-items: center; justify-content: center; color: #aab; }
.info { flex: 1; cursor: pointer; }
.info .t { font-weight: 600; }
.info .p { font-size: 13px; color: #e8453c; margin-top: 2px; }
.stepper { display: flex; align-items: center; border: 1px solid var(--border, #e6e8eb); border-radius: 8px; overflow: hidden; }
.stepper button { width: 28px; height: 30px; border: none; background: var(--surface, #fff); cursor: pointer; }
.stepper input { width: 42px; height: 30px; border: none; text-align: center; border-left: 1px solid var(--border, #e6e8eb); border-right: 1px solid var(--border, #e6e8eb); background: var(--surface, #fff); color: var(--text, #1f2329); }
.sub { width: 90px; text-align: right; font-weight: 600; }
.rm { width: 30px; height: 30px; border: none; background: none; color: var(--text-2, #5b6168); font-size: 20px; cursor: pointer; }
.summary { margin-top: 18px; background: var(--surface, #fff); border: 1px solid var(--border, #e6e8eb); border-radius: 10px; padding: 16px; }
.coupon { display: flex; gap: 8px; align-items: center; margin-bottom: 12px; flex-wrap: wrap; }
.coupon input { flex: 1; min-width: 160px; padding: 8px 10px; border: 1px solid var(--border, #e6e8eb); border-radius: 8px; background: var(--surface, #fff); color: var(--text, #1f2329); }
.cm { font-size: 12px; color: var(--text-2, #5b6168); }
.total { display: flex; justify-content: space-between; align-items: baseline; font-size: 15px; margin-bottom: 12px; }
.total strong { font-size: 20px; color: #e8453c; }
.acts { display: flex; gap: 10px; justify-content: flex-end; }
.btn { padding: 9px 16px; border: 1px solid var(--border, #e6e8eb); border-radius: 8px; background: var(--surface, #fff); color: var(--text, #1f2329); cursor: pointer; }
.btn.primary { background: var(--accent, #2f6bff); color: #fff; border-color: transparent; }
</style>
