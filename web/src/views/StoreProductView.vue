<template>
  <section class="detail" v-if="p">
    <button class="back" @click="router.back()">← {{ t('store.back') }}</button>
    <div class="top">
      <div class="cover" :style="coverStyle">
        <span v-if="!p.cover" class="ph">{{ t('store.products') }}</span>
      </div>
      <div class="info">
        <h1>{{ p.title }}</h1>
        <p class="sum">{{ p.summary || '' }}</p>
        <div class="meta">
          <span class="price">{{ priceText }}</span>
          <span class="stock" :class="stockClass">{{ stockText }}</span>
          <span class="kind">{{ p.kind === 'digital' ? t('store.kindDigital') : t('store.kindPhysical') }}</span>
          <span v-if="p.sku" class="sku">SKU: {{ p.sku }}</span>
        </div>
        <div class="buy">
          <div class="stepper">
            <button @click="qty = Math.max(1, qty - 1)">−</button>
            <input v-model.number="qty" type="number" min="1" />
            <button @click="qty++">+</button>
          </div>
          <button class="btn primary" @click="addCart">{{ t('store.addCart') }}</button>
          <button class="btn" @click="buyNow">{{ t('store.buyNow') }}</button>
        </div>
      </div>
    </div>
    <div class="desc">
      <h2>{{ t('store.detail') }}</h2>
      <div class="body">{{ p.body || p.summary || '' }}</div>
    </div>
  </section>
  <div v-else-if="loading" class="hint">{{ t('common.loading') || '加载中…' }}</div>
  <div v-else class="hint">{{ t('store.notFound') }}</div>
</template>

<script setup>
import { ref, computed, onMounted, watch } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { storeProductDetail, storeCartAdd, storeCartSet, storeCoverUrl } from '@/api'
import { t } from '@/i18n'
import { useToastStore } from '@/stores/toast'
import { refreshCart } from '@/stores/storeCart'

const router = useRouter()
const route = useRoute()
const toast = useToastStore()
const p = ref(null)
const loading = ref(true)
const qty = ref(1)

const priceText = computed(() => {
  const v = ((p.value?.price_cents || 0) / 100).toFixed(2)
  return (p.value?.currency === 'CNY' ? '¥' : (p.value?.currency || '') + ' ') + v
})
const stockText = computed(() => {
  const s = p.value?.stock
  if (s === null || s === undefined) return t('store.unlimited')
  return (s > 0 ? t('store.inStock') + ' ' : '') + s
})
const stockClass = computed(() => {
  const s = p.value?.stock
  if (s === null || s === undefined) return 'ok'
  return s > 0 ? 'ok' : 'out'
})
const coverStyle = computed(() => {
  const u = storeCoverUrl(p.value?.cover)
  return u ? { backgroundImage: `url(${u})` } : {}
})

async function load(slug) {
  loading.value = true
  p.value = null
  try {
    p.value = await storeProductDetail(slug)
  } catch (e) {
    toast.error(e.message || t('store.notFound'))
  } finally {
    loading.value = false
  }
}
async function addCart() {
  try {
    await storeCartAdd(p.value.id, qty.value)
    await refreshCart()
    toast.success(t('store.addCart') + ' ✓')
  } catch (e) {
    toast.error(e.message || 'fail')
  }
}
async function buyNow() {
  try {
    await storeCartSet(p.value.id, qty.value)
    await refreshCart()
    router.push('/store/checkout')
  } catch (e) {
    toast.error(e.message || 'fail')
  }
}
onMounted(() => load(route.params.slug))
watch(() => route.params.slug, (s) => s && load(s))
</script>

<style scoped>
.detail { max-width: 920px; margin: 0 auto; }
.back { background: none; border: none; color: var(--text-2, #5b6168); cursor: pointer; font-size: 14px; margin-bottom: 14px; }
.top { display: flex; gap: 24px; flex-wrap: wrap; }
.cover {
  width: 320px; height: 240px; border-radius: 12px; flex: none;
  background: #eef1f5 center/cover no-repeat; display: flex; align-items: center; justify-content: center;
}
.cover .ph { color: #aab; }
.info { flex: 1; min-width: 260px; }
.info h1 { margin: 0 0 8px; font-size: 22px; }
.sum { color: var(--text-2, #5b6168); margin: 0 0 14px; }
.meta { display: flex; flex-wrap: wrap; gap: 14px; align-items: center; margin-bottom: 18px; }
.price { font-size: 22px; font-weight: 700; color: #e8453c; }
.stock { font-size: 13px; }
.stock.ok { color: #2e9e5b; }
.stock.out { color: #e8453c; }
.kind, .sku { font-size: 12px; color: var(--text-2, #5b6168); border: 1px solid var(--border, #e6e8eb); padding: 2px 8px; border-radius: 6px; }
.buy { display: flex; gap: 10px; align-items: center; flex-wrap: wrap; }
.stepper { display: flex; align-items: center; border: 1px solid var(--border, #e6e8eb); border-radius: 8px; overflow: hidden; }
.stepper button { width: 32px; height: 36px; border: none; background: var(--surface, #fff); cursor: pointer; font-size: 16px; }
.stepper input { width: 48px; height: 36px; border: none; text-align: center; border-left: 1px solid var(--border, #e6e8eb); border-right: 1px solid var(--border, #e6e8eb); background: var(--surface, #fff); color: var(--text, #1f2329); }
.btn { padding: 9px 16px; border: 1px solid var(--border, #e6e8eb); border-radius: 8px; background: var(--surface, #fff); color: var(--text, #1f2329); cursor: pointer; }
.btn.primary { background: var(--accent, #2f6bff); color: #fff; border-color: transparent; }
.desc { margin-top: 28px; }
.desc h2 { font-size: 16px; margin: 0 0 10px; }
.body { white-space: pre-wrap; line-height: 1.7; color: var(--text, #1f2329); background: var(--surface, #fff); border: 1px solid var(--border, #e6e8eb); border-radius: 10px; padding: 16px; }
.hint { padding: 60px; text-align: center; color: var(--text-2, #5b6168); }
</style>
