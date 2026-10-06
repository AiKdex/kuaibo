<template>
  <div class="sa-view">
    <div class="sa-head">
      <div>
        <h2>{{ t('store.admin') }}</h2>
        <p class="sa-sub">{{ t('store.adminProducts') }} · {{ t('store.adminOrders') }} · {{ t('store.adminCoupons') }}</p>
      </div>
      <div class="sa-tools">
        <button class="btn" :disabled="loading" @click="loadAll">
          <AikIcon name="refresh" :size="14" />{{ t('common.refresh') }}
        </button>
      </div>
    </div>

    <div class="sa-tabs">
      <button v-for="tb in tabs" :key="tb.k" :class="{ on: tab === tb.k }" @click="switchTab(tb.k)">
        {{ t(tb.label) }}
      </button>
    </div>

    <div v-if="err" class="sa-err">{{ err }}</div>

    <!-- 商品 -->
    <section v-if="tab === 'products'">
      <div class="bar">
        <button class="btn primary" @click="openProduct()">{{ t('store.newProduct') }}</button>
        <span class="cnt">{{ products.length }}</span>
      </div>
      <div class="card form" v-if="form">
        <div class="grid2">
          <label class="f"><span>{{ t('store.title') }}*</span>
            <input v-model="form.title" :placeholder="t('store.title')" /></label>
          <label class="f"><span>{{ t('store.slug') }}</span>
            <input v-model="form.slug" placeholder="my-product" /></label>
          <label class="f"><span>{{ t('store.kind') }}</span>
            <select v-model="form.kind">
              <option value="digital">{{ t('store.kindDigital') }}</option>
              <option value="physical">{{ t('store.kindPhysical') }}</option>
            </select></label>
          <label class="f"><span>{{ t('store.priceCents') }}*</span>
            <input v-model.number="form.price_cents" type="number" min="0" /></label>
          <label class="f"><span>{{ t('store.sku') }}</span>
            <input v-model="form.sku" /></label>
          <label class="f"><span>{{ t('store.stock') }} ({{ t('store.unlimited') }}=-1)</span>
            <input v-model.number="form.stock_raw" type="number" :min="-1" /></label>
          <label class="f"><span>{{ t('store.coverUrl') }}</span>
            <div class="coverRow">
              <img v-if="form.cover" :src="coverPreview(form.cover)" class="coverPrev" alt="" />
              <input v-model="form.cover" :placeholder="t('store.coverPlaceholder')" />
              <label class="btn up">
                <input type="file" accept="image/*" hidden @change="onPickCover" />
                {{ t('store.coverUpload') }}
              </label>
            </div>
            <em class="tip-inline">{{ t('store.coverTip') }}</em>
          </label>
          <label class="f"><span>{{ t('store.sort') }}</span>
            <input v-model.number="form.sort" type="number" /></label>
        </div>
        <label class="f"><span>{{ t('store.summary') }}</span>
          <input v-model="form.summary" /></label>
        <label class="f" v-if="form.kind === 'digital'"><span>{{ t('store.fileId') }} <em class="tip-inline">{{ t('store.fileIdTip') }}</em></span>
          <input v-model="form.file_id" placeholder="f_xxxxxxxx" /></label>
        <label class="f" v-if="form.kind === 'digital'"><span>{{ t('store.maxDownloads') }} <em class="tip-inline">{{ t('store.maxDownloadsTip') }}</em></span>
          <input v-model.number="form.max_downloads" type="number" min="0" /></label>
        <label class="f"><span>{{ t('store.body') }}</span>
          <textarea v-model="form.body" rows="4"></textarea></label>
        <label class="f"><span>{{ t('store.orderStatus') }}</span>
          <select v-model="form.status">
            <option value="published">{{ t('store.published') }}</option>
            <option value="draft">{{ t('store.draft') }}</option>
          </select></label>
        <div class="acts">
          <button class="btn primary" :disabled="saving" @click="saveProduct">{{ t('common.save') }}</button>
          <button class="btn" @click="form = null">{{ t('common.cancel') }}</button>
        </div>
      </div>
      <div class="table">
        <div class="tr th">
          <span>{{ t('store.title') }}</span><span>{{ t('store.kind') }}</span>
          <span>{{ t('store.price') }}</span><span>{{ t('store.stock') }}</span>
          <span>{{ t('store.orderStatus') }}</span><span class="ops"></span>
        </div>
        <div class="tr" v-for="p in products" :key="p.id">
          <span class="ell" :title="p.title">
            <img v-if="p.cover" :src="coverPreview(p.cover)" class="rowThumb" alt="" />{{ p.title }}
          </span>
          <span>{{ p.kind === 'digital' ? t('store.kindDigital') : t('store.kindPhysical') }}</span>
          <span class="money">{{ money(p.price_cents, p.currency) }}</span>
          <span>{{ p.stock === null ? t('store.unlimited') : p.stock }}</span>
          <span>{{ p.status === 'published' ? t('store.published') : t('store.draft') }}</span>
          <span class="ops">
            <button class="mini" @click="openProduct(p)">{{ t('common.edit') }}</button>
            <button class="mini danger" @click="delProduct(p)">{{ t('common.delete') }}</button>
          </span>
        </div>
        <div v-if="!products.length" class="empty">{{ t('common.empty') }}</div>
      </div>
    </section>

    <!-- 订单 -->
    <section v-else-if="tab === 'orders'">
      <div class="bar">
        <select v-model="ofilter" @change="loadOrders" class="fsel">
          <option value="">{{ t('common.all') }}</option>
          <option v-for="s in ORDER_STATUS" :key="s" :value="s">{{ statusText(s) }}</option>
        </select>
        <a class="btn" :href="EXPORT_URL" target="_blank" rel="noopener">{{ t('store.export') }}</a>
        <span class="cnt">{{ orders.length }}</span>
      </div>
      <div class="table">
        <div class="tr th">
          <span>{{ t('store.orderNo') }}</span><span>{{ t('store.orderStatus') }}</span>
          <span>{{ t('store.total') }}</span><span>{{ t('store.contactName') }}</span>
          <span>{{ t('store.createdAt') }}</span><span class="ops"></span>
        </div>
        <div class="tr" v-for="o in orders" :key="o.order_no">
          <span class="ell">{{ o.order_no }}</span>
          <span><em class="st" :class="statusClass(o.status)">{{ statusText(o.status) }}</em></span>
          <span class="money">{{ money(o.total_cents, o.currency) }}</span>
          <span class="ell">{{ o.contact_name || o.contact_email || '-' }}</span>
          <span>{{ fmt(o.created_at) }}</span>
          <span class="ops">
            <button class="mini" @click="expand(o)">{{ t('store.detail') }}</button>
            <button v-if="o.status === 'paid'" class="mini" @click="fulfill(o)">{{ t('store.fulfill') }}</button>
            <button v-if="o.status === 'paid' || o.status === 'fulfilled'" class="mini danger" @click="refund(o)">{{ t('store.refund') }}</button>
          </span>
        </div>
        <div v-if="!orders.length" class="empty">{{ t('common.empty') }}</div>
      </div>
      <div class="card" v-if="detail">
        <div class="bar"><b>#{{ detail.order_no }}</b><button class="btn" @click="detail = null">×</button></div>
        <div v-for="it in detail.items" :key="it.id" class="kv">
          <span>{{ it.title }} × {{ it.qty }}</span><span class="money">{{ money(it.subtotal_cents, detail.currency) }}</span>
        </div>
        <div class="kv"><span>{{ t('store.subtotal') }}</span><span class="money">{{ money(detail.subtotal_cents, detail.currency) }}</span></div>
        <div class="kv"><span>{{ t('store.discount') }}</span><span class="money">-{{ money(detail.discount_cents, detail.currency) }}</span></div>
        <div class="kv"><span>{{ t('store.total') }}</span><span class="money"><b>{{ money(detail.total_cents, detail.currency) }}</b></span></div>
        <div class="kv"><span>{{ t('store.contactEmail') }}</span><span>{{ detail.contact_email || '-' }}</span></div>
        <div class="kv"><span>{{ t('store.address') }}</span><span>{{ detail.address || '-' }}</span></div>
        <div class="kv"><span>{{ t('store.gateway') }}</span><span>{{ gwText(detail.gateway) }}</span></div>
        <div class="kv" v-if="refunds.length"><span>{{ t('store.refundLedger') }}</span><span></span></div>
        <div class="rf" v-for="r in refunds" :key="r.id">
          <span class="mono">{{ r.refund_no || r.id }}</span>
          <span class="money">-{{ money(r.amount_cents, detail.currency) }}</span>
          <span class="why">{{ r.reason || '-' }}</span>
        </div>
        <div class="bar" v-if="detail.status === 'paid' || detail.status === 'fulfilled'">
          <input v-model="refundAmt" type="number" min="0" step="0.01" :placeholder="t('store.refundFull')" class="rfamt" />
          <input v-model="refundReason" :placeholder="t('store.refundReason')" />
          <button class="btn" :disabled="busy" @click="doRefund(detail)">{{ t('store.refund') }}</button>
        </div>
        <p class="tip">{{ t('store.refundTip') }}</p>
      </div>
    </section>

    <!-- 退货申请 -->
    <section v-else-if="tab === 'returns'">
      <div class="bar">
        <select v-model="rfilter" @change="loadReturns" class="fsel">
          <option value="">{{ t('common.all') }}</option>
          <option value="pending">{{ t('store.returnPending') }}</option>
          <option value="refunded">{{ t('store.returnRefunded') }}</option>
          <option value="rejected">{{ t('store.returnRejected') }}</option>
        </select>
        <span class="cnt">{{ returns.length }}</span>
      </div>
      <div class="table">
        <div class="tr th">
          <span>{{ t('store.orderNo') }}</span><span>{{ t('store.returnReason') }}</span>
          <span>{{ t('store.total') }}</span><span>{{ t('store.orderStatus') }}</span><span class="ops"></span>
        </div>
        <div class="tr" v-for="rt in returns" :key="rt.id">
          <span class="ell">{{ rt.order_no }}</span>
          <span class="ell" :title="rt.reason">{{ rt.reason }}</span>
          <span class="money">{{ rt.amount_cents > 0 ? money(rt.amount_cents) : t('store.refundFull') }}</span>
          <span><em class="st" :class="rtStatusClass(rt.status)">{{ rtStatusText(rt.status) }}</em></span>
          <span class="ops">
            <template v-if="rt.status === 'pending'">
              <button class="mini" @click="decide(rt, true)">{{ t('common.confirm') }}</button>
              <button class="mini danger" @click="decide(rt, false)">{{ t('store.returnReject') }}</button>
            </template>
            <span v-else class="ell">{{ rt.admin_note || '-' }}</span>
          </span>
        </div>
        <div v-if="!returns.length" class="empty">{{ t('common.empty') }}</div>
      </div>
    </section>

    <!-- 优惠券 -->
    <section v-else-if="tab === 'coupons'">
      <div class="bar">
        <button class="btn primary" @click="openCoupon()">{{ t('store.couponCreate') }}</button>
        <span class="cnt">{{ coupons.length }}</span>
      </div>
      <div class="card form" v-if="cform">
        <div class="grid2">
          <label class="f"><span>{{ t('store.couponCode') }}*</span>
            <input v-model="cform.code" placeholder="WELCOME10" /></label>
          <label class="f"><span>{{ t('store.couponKind') }}</span>
            <select v-model="cform.kind">
              <option value="percent">{{ t('store.couponPercent') }}</option>
              <option value="fixed">{{ t('store.couponFixed') }}</option>
              <option value="free_shipping">{{ t('store.couponFreeShip') }}</option>
            </select></label>
          <label class="f"><span>{{ t('store.couponValue') }}</span>
            <input v-model.number="cform.value" type="number" step="0.01" /></label>
          <label class="f"><span>{{ t('store.couponMin') }}</span>
            <input v-model.number="cform.min_subtotal_cents" type="number" min="0" /></label>
          <label class="f"><span>{{ t('store.couponMax') }}</span>
            <input v-model.number="cform.max_discount_cents" type="number" min="0" /></label>
          <label class="f"><span>{{ t('store.couponLimit') }}</span>
            <input v-model.number="cform.usage_limit" type="number" min="0" /></label>
        </div>
        <label class="f"><span>{{ t('store.couponStart') }} / {{ t('store.couponEnd') }}</span>
          <input v-model="cform.starts_at" placeholder="2026-01-01" /><input v-model="cform.ends_at" placeholder="2026-12-31" /></label>
        <div class="acts">
          <button class="btn primary" :disabled="saving" @click="saveCoupon">{{ t('common.save') }}</button>
          <button class="btn" @click="cform = null">{{ t('common.cancel') }}</button>
        </div>
      </div>
      <div class="table">
        <div class="tr th">
          <span>{{ t('store.couponCode') }}</span><span>{{ t('store.couponKind') }}</span>
          <span>{{ t('store.couponValue') }}</span><span>{{ t('store.couponUsed') }}</span>
          <span>{{ t('store.couponLimit') }}</span><span class="ops"></span>
        </div>
        <div class="tr" v-for="c in coupons" :key="c.id">
          <span class="ell"><code>{{ c.code }}</code></span>
          <span>{{ c.kind === 'percent' ? t('store.couponPercent') : c.kind === 'fixed' ? t('store.couponFixed') : t('store.couponFreeShip') }}</span>
          <span>{{ c.kind === 'percent' ? c.value + '%' : money(Math.round((c.value || 0) * 100)) }}</span>
          <span>{{ c.used_count || 0 }}</span>
          <span>{{ c.usage_limit || '∞' }}</span>
          <span class="ops">
            <button class="mini" @click="openCoupon(c)">{{ t('common.edit') }}</button>
            <button class="mini danger" @click="delCoupon(c)">{{ t('common.delete') }}</button>
          </span>
        </div>
        <div v-if="!coupons.length" class="empty">{{ t('common.empty') }}</div>
      </div>
    </section>

    <!-- 支付配置 -->
    <section v-else>
      <div class="card">
        <div class="bar"><b>{{ t('store.config') }}</b><span class="cnt">{{ cfg.currency }}</span></div>
        <div class="kv" v-for="g in cfg.gateways" :key="g.name">
          <span>{{ gwText(g.name) }}</span>
          <span class="st" :class="g.configured ? 'ok' : 'no'">
            {{ g.configured ? t('store.gatewayConfigured') : t('store.gatewayUnconfig') }}
          </span>
        </div>
        <p class="tip">{{ t('store.payTip') }}</p>
      </div>
    </section>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import AikIcon from '@/components/AikIcon.vue'
import { t } from '@/i18n'
import { useToastStore } from '@/stores/toast'
import {
  adminStoreProducts, adminStoreProductCreate, adminStoreProductUpdate, adminStoreProductDelete,
  adminStoreOrders, adminStoreOrderDetail, adminStoreOrderFulfill, adminStoreOrderRefund, adminStoreOrderRefunds,
  adminStoreCoupons, adminStoreCouponCreate, adminStoreCouponUpdate, adminStoreCouponDelete,
  adminStoreReturns, adminStoreReturnDecide,
  adminStoreConfig, storeCoverUrl, uploadFiles
} from '@/api'

const toast = useToastStore()
const EXPORT_URL = '/api/v1/admin/store/orders/export'
const ORDER_STATUS = ['pending', 'paid', 'fulfilled', 'refunded', 'cancelled', 'expired']
const tabs = [
  { k: 'products', label: 'store.adminProducts' },
  { k: 'orders', label: 'store.adminOrders' },
  { k: 'returns', label: 'store.adminReturns' },
  { k: 'coupons', label: 'store.adminCoupons' },
  { k: 'config', label: 'store.config' }
]
const tab = ref('products')
const loading = ref(false)
const saving = ref(false)
const err = ref('')
const products = ref([])
const orders = ref([])
const coupons = ref([])
const cfg = reactive({ gateways: [], currency: 'CNY' })
const ofilter = ref('')
const form = ref(null)
const cform = ref(null)
const detail = ref(null)
const refunds = ref([])
const refundAmt = ref('')
const refundReason = ref('')
const busy = ref(false)
const returns = ref([])
const rfilter = ref('')

function money(cents, cur) {
  const v = ((cents || 0) / 100).toFixed(2)
  return (cur === 'CNY' ? '¥' : (cur || '') + ' ') + v
}
function statusText(s) {
  return {
    pending: t('store.pending'), paid: t('store.paid'), fulfilled: t('store.fulfilled'),
    refunded: t('store.refunded'), expired: t('store.expired'), cancelled: t('store.cancelled')
  }[s] || s
}
function statusClass(s) {
  return { pending: 'pen', paid: 'ok', fulfilled: 'ok', refunded: 'rf', expired: 'no', cancelled: 'no' }[s] || ''
}
function gwText(g) {
  return { mock: t('store.gatewayMock'), wechat: t('store.gatewayWechat'), alipay: t('store.gatewayAlipay'), stripe: t('store.gatewayStripe') }[g] || g
}
function fmt(ms) {
  if (!ms) return '-'
  const d = new Date(ms)
  return isNaN(d) ? '-' : d.toLocaleString()
}

async function loadProducts() { products.value = await adminStoreProducts() }
async function loadOrders() { orders.value = await adminStoreOrders(ofilter.value) }
async function loadReturns() { returns.value = await adminStoreReturns(rfilter.value) }
function rtStatusText(s) {
  return { pending: t('store.returnPending'), refunded: t('store.returnRefunded'), rejected: t('store.returnRejected') }[s] || s
}
function rtStatusClass(s) {
  return { pending: 'pen', refunded: 'ok', rejected: 'no' }[s] || ''
}
async function decide(rt, approve) {
  const note = approve ? '' : (prompt(t('store.returnRejectReason') || '') || '')
  if (!approve && note === null) return
  if (!confirm(`${approve ? t('store.returnApprove') : t('store.returnReject')} #${rt.order_no}?`)) return
  try {
    await adminStoreReturnDecide(rt.id, approve, note)
    toast.success(t('common.save') + ' ✓')
    await loadReturns()
  } catch (e) {
    // 批准但网关退款失败 → 后端保持 pending，这里把原因如实透出
    toast.error(e.message || 'failed')
  }
}
async function loadCoupons() { coupons.value = await adminStoreCoupons() }
async function loadCfg() {
  const d = await adminStoreConfig()
  cfg.gateways = d.gateways || []
  cfg.currency = d.currency || 'CNY'
}
async function loadAll() {
  loading.value = true
  err.value = ''
  try {
    if (tab.value === 'products') await loadProducts()
    else if (tab.value === 'orders') await loadOrders()
    else if (tab.value === 'returns') await loadReturns()
    else if (tab.value === 'coupons') await loadCoupons()
    else await loadCfg()
  } catch (e) {
    err.value = e.message || 'load failed'
  } finally {
    loading.value = false
  }
}
function switchTab(k) { tab.value = k; loadAll() }

function openProduct(p) {
  if (!p) {
    form.value = { id: '', title: '', slug: '', kind: 'digital', price_cents: 0, stock_raw: -1, sku: '', cover: '', summary: '', body: '', file_id: '', max_downloads: 0, status: 'published', sort: 0 }
  } else {
    const meta = readMeta(p.meta)
    form.value = {
      id: p.id, title: p.title || '', slug: p.slug || '', kind: p.kind || 'digital',
      price_cents: p.price_cents || 0, stock_raw: p.stock === null || p.stock === undefined ? -1 : p.stock,
      sku: p.sku || '', cover: p.cover || '', summary: p.summary || '', body: p.body || '',
      file_id: meta.file_id || '', max_downloads: Number(meta.max_downloads) || 0,
      status: p.status || 'published', sort: p.sort || 0
    }
  }
}
// 商品 meta 约定：{"file_id":"...","max_downloads":N}（数字商品）
function readMeta(meta) {
  if (!meta) return {}
  try { return JSON.parse(meta) || {} } catch (_) { return {} }
}
// 封面预览：外链直接用，站内 file id 走受限图床
function coverPreview(cover) {
  return storeCoverUrl(cover)
}
// 选图上传：复用文件库上传，落库存 file id（图床据此放行）
async function onPickCover(e) {
  const f = e.target.files && e.target.files[0]
  e.target.value = ''
  if (!f) return
  if (!/^image\//.test(f.type)) { toast.error(t('store.coverTip')); return }
  saving.value = true
  try {
    const res = await uploadFiles([{ file: f, path: 'store-cover/' + f.name }])
    const r = (res && res[0]) || {}
    if (r.err || !r.file || !r.file.id) throw new Error(r.err || t('store.coverUpload'))
    form.value.cover = r.file.id
    toast.success(t('store.coverUpload') + ' ✓')
  } catch (err) {
    toast.error(err.message || t('store.coverUpload'))
  } finally {
    saving.value = false
  }
}
async function saveProduct() {
  const f = form.value
  if (!f || !f.title.trim()) { toast.error(t('store.titleReq')); return }
  saving.value = true
  // meta 只在数字商品上承载交付信息（实体无交付物）
  const meta = f.kind === 'digital'
    ? JSON.stringify({
        ...(f.file_id ? { file_id: String(f.file_id).trim() } : {}),
        ...(Number(f.max_downloads) > 0 ? { max_downloads: Number(f.max_downloads) } : {})
      })
    : '{}'
  const payload = {
    title: f.title.trim(), slug: f.slug.trim(), kind: f.kind,
    price_cents: Number(f.price_cents) || 0,
    stock: f.stock_raw === -1 || f.stock_raw === null || f.stock_raw === '' ? null : Number(f.stock_raw),
    sku: f.sku, cover: f.cover, summary: f.summary, body: f.body, meta, status: f.status, sort: Number(f.sort) || 0
  }
  try {
    if (f.id) await adminStoreProductUpdate(f.id, payload)
    else await adminStoreProductCreate(payload)
    toast.success(t('common.save'))
    form.value = null
    await loadProducts()
  } catch (e) {
    toast.error(e.message || 'save failed')
  } finally {
    saving.value = false
  }
}
async function delProduct(p) {
  if (!confirm(`${t('common.delete')} ${p.title}?`)) return
  try {
    await adminStoreProductDelete(p.id)
    await loadProducts()
  } catch (e) { toast.error(e.message || 'delete failed') }
}
async function expand(o) {
  try {
    detail.value = await adminStoreOrderDetail(o.order_no)
    refunds.value = await adminStoreOrderRefunds(o.order_no)
    refundAmt.value = ''
    refundReason.value = ''
  } catch (e) { toast.error(e.message || 'fail') }
}
async function fulfill(o) {
  try { await adminStoreOrderFulfill(o.order_no); toast.success(t('store.fulfill')); await loadOrders() }
  catch (e) { toast.error(e.message || 'fail') }
}
async function refund(o) {
  // 退款必须走带金额/原因的入口，避免"一键全额"误操作；此处保留旧按钮为全额退款
  if (!confirm(`${t('store.refund')} #${o.order_no}?\n${t('store.refundTip')}`)) return
  await doRefund(o)
}

// doRefund 退款：amount 为空=全额；部分退款由后端校验累计不超实付。
// 网关退失败时后端会回绝且**不改订单状态**，这里只需把原因如实显示给运营。
async function doRefund(o) {
  busy.value = true
  try {
    const yuan = parseFloat(refundAmt.value)
    const cents = isNaN(yuan) || yuan <= 0 ? 0 : Math.round(yuan * 100)
    const res = await adminStoreOrderRefund(o.order_no, cents, refundReason.value.trim())
    toast.success(`${t('store.refund')} ✓ ${money(res.refunded_cents || 0, o.currency)}`)
    refundAmt.value = ''
    refundReason.value = ''
    if (detail.value && detail.value.order_no === o.order_no) {
      detail.value = await adminStoreOrderDetail(o.order_no)
      refunds.value = await adminStoreOrderRefunds(o.order_no)
    }
    await loadOrders()
  } catch (e) {
    toast.error(e.message || 'refund failed')
  } finally {
    busy.value = false
  }
}
function openCoupon(c) {
  if (!c) {
    cform.value = { id: '', code: '', kind: 'percent', value: 10, min_subtotal_cents: 0, max_discount_cents: 0, usage_limit: 0, starts_at: '', ends_at: '' }
  } else {
    cform.value = {
      id: c.id, code: c.code || '', kind: c.kind || 'percent', value: c.value || 0,
      min_subtotal_cents: c.min_subtotal_cents || 0, max_discount_cents: c.max_discount_cents || 0,
      usage_limit: c.usage_limit || 0, starts_at: c.starts_at || '', ends_at: c.ends_at || ''
    }
  }
}
async function saveCoupon() {
  const c = cform.value
  if (!c || !c.code.trim()) { toast.error(t('store.couponCodeReq')); return }
  saving.value = true
  const payload = {
    code: c.code.trim().toUpperCase(), kind: c.kind, value: Number(c.value) || 0,
    min_subtotal_cents: Number(c.min_subtotal_cents) || 0,
    max_discount_cents: Number(c.max_discount_cents) || 0,
    usage_limit: Number(c.usage_limit) || 0
  }
  try {
    if (c.id) await adminStoreCouponUpdate(c.id, payload)
    else await adminStoreCouponCreate(payload)
    toast.success(t('common.save'))
    cform.value = null
    await loadCoupons()
  } catch (e) {
    toast.error(e.message || 'save failed')
  } finally {
    saving.value = false
  }
}
async function delCoupon(c) {
  if (!confirm(`${t('common.delete')} ${c.code}?`)) return
  try { await adminStoreCouponDelete(c.id); await loadCoupons() }
  catch (e) { toast.error(e.message || 'delete failed') }
}
onMounted(loadAll)
</script>

<style scoped>
.sa-view { padding: 20px 24px; max-width: 1180px; margin: 0 auto; }
.sa-head { display: flex; justify-content: space-between; align-items: flex-start; margin-bottom: 14px; }
.sa-head h2 { margin: 0 0 4px; font-size: 20px; }
.sa-sub { margin: 0; font-size: 13px; color: var(--text-2, #5b6168); }
.sa-tabs { display: flex; gap: 6px; border-bottom: 1px solid var(--border, #e6e8eb); margin-bottom: 16px; }
.sa-tabs button { background: none; border: none; padding: 9px 14px; cursor: pointer; color: var(--text-2, #5b6168); font-size: 14px; border-bottom: 2px solid transparent; }
.sa-tabs button.on { color: var(--accent, #2f6bff); border-bottom-color: var(--accent, #2f6bff); font-weight: 600; }
.sa-err { background: #fdecea; color: #e8453c; padding: 10px 12px; border-radius: 8px; margin-bottom: 12px; font-size: 13px; }
.bar { display: flex; align-items: center; gap: 10px; margin-bottom: 12px; flex-wrap: wrap; }
.cnt { font-size: 12px; color: var(--text-2, #5b6168); }
.btn { padding: 8px 14px; border: 1px solid var(--border, #e6e8eb); border-radius: 8px; background: var(--surface, #fff); color: var(--text, #1f2329); cursor: pointer; font-size: 13px; display: inline-flex; align-items: center; gap: 5px; text-decoration: none; }
.btn.primary { background: var(--accent, #2f6bff); color: #fff; border-color: transparent; }
.btn:disabled { opacity: .6; cursor: default; }
.fsel, .f select, .f input, .f textarea {
  padding: 8px 10px; border: 1px solid var(--border, #e6e8eb); border-radius: 8px;
  background: var(--surface, #fff); color: var(--text, #1f2329); font-family: inherit;
}
.card { background: var(--surface, #fff); border: 1px solid var(--border, #e6e8eb); border-radius: 10px; padding: 14px; margin-bottom: 14px; }
.grid2 { display: grid; grid-template-columns: repeat(auto-fit, minmax(220px, 1fr)); gap: 10px; }
.f { display: block; margin-bottom: 10px; }
.f > span { display: block; font-size: 12px; color: var(--text-2, #5b6168); margin-bottom: 4px; }
.tip-inline { font-style: normal; font-weight: 400; opacity: .8; }
.coverRow { display: flex; align-items: center; gap: 8px; }
.coverRow input { flex: 1; }
.coverPrev { width: 44px; height: 44px; object-fit: cover; border-radius: 6px; border: 1px solid var(--border, #e6e8eb); flex: none; background: var(--surface-2, #f2f3f5); }
.btn.up { flex: none; cursor: pointer; }
.f input, .f textarea, .f select { width: 100%; box-sizing: border-box; }
.acts { display: flex; gap: 10px; margin-top: 6px; }
.table { background: var(--surface, #fff); border: 1px solid var(--border, #e6e8eb); border-radius: 10px; overflow: hidden; }
.tr { display: grid; grid-template-columns: 2.2fr .8fr 1fr .8fr .9fr 1.4fr; gap: 10px; padding: 10px 14px; border-bottom: 1px solid var(--border, #e6e8eb); font-size: 13px; align-items: center; }
.tr.th { background: var(--surface-2, #f7f8fa); font-weight: 600; color: var(--text-2, #5b6168); font-size: 12px; }
.ell { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.rowThumb { width: 26px; height: 26px; object-fit: cover; border-radius: 4px; border: 1px solid var(--border, #e6e8eb); margin-right: 6px; vertical-align: middle; background: var(--surface-2, #f2f3f5); }
.money { color: #e8453c; }
.ops { display: flex; gap: 6px; justify-content: flex-end; flex-wrap: wrap; }
.mini { padding: 4px 8px; font-size: 12px; border: 1px solid var(--border, #e6e8eb); border-radius: 6px; background: var(--surface, #fff); color: var(--text, #1f2329); cursor: pointer; }
.mini.danger { color: #e8453c; border-color: #f5c6c2; }
.empty { padding: 30px; text-align: center; color: var(--text-2, #5b6168); font-size: 13px; }
.st { font-style: normal; font-size: 12px; padding: 2px 8px; border-radius: 6px; }
.st.pen { color: #b8860b; background: #fdf3e0; }
.st.ok { color: #2e9e5b; background: #e7f6ec; }
.st.rf { color: #6a5acd; background: #eeeafc; }
.st.no { color: #e8453c; background: #fdecea; }
.kv { display: flex; justify-content: space-between; padding: 6px 0; font-size: 13px; border-bottom: 1px dashed var(--border, #e6e8eb); gap: 16px; }
.tip { font-size: 12px; color: var(--text-2, #5b6168); margin: 10px 0 0; }
code { background: var(--surface-2, #f2f3f5); padding: 2px 6px; border-radius: 4px; }
</style>
