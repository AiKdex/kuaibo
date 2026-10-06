<template>
  <section class="store-home">
    <div class="hero">
      <h1>{{ t('store.catalog') }}</h1>
      <div class="search">
        <input
          v-model="q"
          :placeholder="t('store.search')"
          @keyup.enter="load"
        />
        <button class="btn" @click="load">{{ t('common.search') || '搜索' }}</button>
      </div>
    </div>

    <div v-if="loading" class="hint">{{ t('common.loading') || '加载中…' }}</div>
    <div v-else-if="!products.length" class="hint empty">
      <p>{{ t('store.cartEmpty') }}</p>
      <RouterLink to="/store" class="btn">{{ t('store.goShop') }}</RouterLink>
    </div>

    <div v-else class="grid">
      <article
        v-for="p in products"
        :key="p.id"
        class="card"
        @click="goDetail(p.slug)"
      >
        <div class="cover" :style="coverStyle(p)">
          <span v-if="!p.cover" class="ph">{{ t('store.products') }}</span>
        </div>
        <div class="body">
          <h3 :title="p.title">{{ p.title }}</h3>
          <p class="sum">{{ p.summary || '' }}</p>
          <div class="row">
            <span class="price">{{ priceText(p) }}</span>
            <span class="stock" :class="stockClass(p)">{{ stockText(p) }}</span>
          </div>
          <div class="actions">
            <button class="btn primary" @click.stop="addCart(p)">{{ t('store.addCart') }}</button>
            <button class="btn ghost" @click.stop="goDetail(p.slug)">{{ t('store.detail') }}</button>
          </div>
        </div>
      </article>
    </div>
  </section>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { storeProducts, storeCartAdd, storeCoverUrl } from '@/api'
import { t } from '@/i18n'
import { useToastStore } from '@/stores/toast'
import { refreshCart } from '@/stores/storeCart'

const router = useRouter()
const toast = useToastStore()
const q = ref('')
const products = ref([])
const loading = ref(true)

function priceText(p) {
  const v = (p.price_cents || 0) / 100
  return (p.currency === 'CNY' ? '¥' : (p.currency || '') + ' ') + v.toFixed(2)
}
function stockText(p) {
  if (p.stock === null || p.stock === undefined) return t('store.unlimited')
  return (p.stock > 0 ? t('store.inStock') + ' ' : '') + p.stock
}
function stockClass(p) {
  if (p.stock === null || p.stock === undefined) return 'ok'
  return p.stock > 0 ? 'ok' : 'out'
}
function coverStyle(p) {
  const u = storeCoverUrl(p.cover)
  return u ? { backgroundImage: `url(${u})` } : {}
}
function goDetail(slug) {
  router.push(`/store/product/${slug}`)
}
async function addCart(p) {
  try {
    await storeCartAdd(p.id, 1)
    await refreshCart()
    toast.success(t('store.addCart') + ' ✓')
  } catch (e) {
    toast.error(e.message || t('store.couponInvalid'))
  }
}
async function load() {
  loading.value = true
  try {
    products.value = await storeProducts(q.value.trim())
  } catch (e) {
    toast.error(e.message || 'load failed')
  } finally {
    loading.value = false
  }
}
onMounted(load)
</script>

<style scoped>
.hero { margin-bottom: 18px; }
.hero h1 { font-size: 22px; margin: 0 0 12px; }
.search { display: flex; gap: 8px; }
.search input {
  flex: 1; padding: 9px 12px; border: 1px solid var(--border, #e6e8eb);
  border-radius: 8px; background: var(--surface, #fff); color: var(--text, #1f2329);
}
.btn {
  padding: 9px 14px; border: 1px solid var(--border, #e6e8eb); border-radius: 8px;
  background: var(--surface, #fff); color: var(--text, #1f2329); cursor: pointer; font-size: 14px;
}
.btn.primary { background: var(--accent, #2f6bff); color: #fff; border-color: transparent; }
.btn.ghost { background: transparent; }
.hint { padding: 40px; text-align: center; color: var(--text-2, #5b6168); }
.empty { display: flex; flex-direction: column; align-items: center; gap: 14px; }
.grid {
  display: grid; grid-template-columns: repeat(auto-fill, minmax(220px, 1fr)); gap: 16px;
}
.card {
  background: var(--surface, #fff); border: 1px solid var(--border, #e6e8eb); border-radius: 12px;
  overflow: hidden; cursor: pointer; transition: box-shadow .15s, transform .15s;
}
.card:hover { box-shadow: 0 6px 20px rgba(0,0,0,.08); transform: translateY(-2px); }
.cover {
  height: 130px; background: #eef1f5 center/cover no-repeat; display: flex; align-items: center; justify-content: center;
}
.cover .ph { color: #aab; font-size: 13px; }
.body { padding: 12px 14px 14px; }
.body h3 { margin: 0 0 6px; font-size: 15px; line-height: 1.3;
  display: -webkit-box; -webkit-line-clamp: 1; -webkit-box-orient: vertical; overflow: hidden; }
.sum { margin: 0 0 10px; font-size: 12px; color: var(--text-2, #5b6168); min-height: 16px;
  display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; }
.row { display: flex; justify-content: space-between; align-items: baseline; margin-bottom: 10px; }
.price { font-weight: 700; color: #e8453c; }
.stock { font-size: 12px; }
.stock.ok { color: #2e9e5b; }
.stock.out { color: #e8453c; }
.actions { display: flex; gap: 8px; }
.actions .btn { flex: 1; padding: 7px 0; font-size: 13px; }
</style>
