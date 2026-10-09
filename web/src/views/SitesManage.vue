<template>
  <div class="sm-view">
    <div class="sm-head">
      <div>
        <h2>{{  $t('多站点')  }}</h2>
        <p class="sm-sub">{{  $t('一套系统运营多个站点（矩阵站群）：自定义域名 / 子域名 / 子目录三种接入，内容与主题按站点隔离。')  }}</p>
      </div>
      <div class="sm-tools">
        <button class="btn" :disabled="loading" @click="loadAll"><AikIcon name="refresh" :size="14" />{{  $t('刷新')  }}</button>
        <button class="btn btn-primary" @click="openCreate"><AikIcon name="plus" :size="14" />{{  $t('新建站点')  }}</button>
      </div>
    </div>

    <!-- 站点数上限（cap）汇总 -->
    <div class="sm-cap">
      <div class="sm-cap-item">
        <span class="sm-cap-n">{{  cap.base  }}</span><span class="sm-cap-l">{{  $t('基础免费')  }}</span>
      </div>
      <div class="sm-cap-item">
        <span class="sm-cap-n">+{{  cap.permanent  }}</span><span class="sm-cap-l">{{  $t('永久授权')  }}</span>
      </div>
      <div class="sm-cap-item">
        <span class="sm-cap-n">+{{  cap.subscription  }}</span><span class="sm-cap-l">{{  $t('订阅生效中')  }}</span>
      </div>
      <div class="sm-cap-item sm-cap-total" :class="{ full: cap.used >= cap.total }">
        <span class="sm-cap-n">{{  cap.used  }} / {{  cap.total  }}</span><span class="sm-cap-l">{{  $t('已用 / 上限')  }}</span>
      </div>
      <button class="btn btn-sm sm-cap-btn" @click="openLicense"><AikIcon name="lock" :size="13" />{{  $t('录入授权')  }}</button>
    </div>

    <div class="sm-main">
      <!-- 左：站点列表 -->
      <div class="sm-list">
        <div v-if="loading" class="sm-empty">{{  $t('加载中…')  }}</div>
        <div v-else-if="!sites.length" class="sm-empty">
          <AikIcon name="grid" :size="28" />
          <p>{{  $t('还没有站点')  }}</p>
        </div>
        <div
          v-for="s in sites"
          :key="s.id"
          class="sm-card"
          :class="{ sel: cur && cur.id === s.id }"
          @click="selectSite(s)"
        >
          <div class="sm-card-top">
            <b class="sm-card-slug">{{  s.slug  }}</b>
            <span v-if="s.id === 'default'" class="sm-badge st-default">{{  $t('主站')  }}</span>
            <span class="sm-badge" :class="'st-' + s.status">{{  statusLabel(s.status)  }}</span>
          </div>
          <div class="sm-card-route"><AikIcon name="link" :size="12" />{{  routeLabel(s)  }}</div>
          <div class="sm-card-ops">
            <button class="btn btn-sm" @click.stop="selectSite(s)"><AikIcon name="settings" :size="13" />{{  $t('设置')  }}</button>
            <button
              class="btn btn-sm btn-danger-ghost"
              :disabled="s.id === 'default'"
              :title="s.id === 'default' ? $t('主站不可删除') : $t('删除站点（保留数据）')"
              @click.stop="confirmDel = s"
            ><AikIcon name="trash" :size="13" />{{  $t('删除')  }}</button>
          </div>
        </div>
      </div>

      <!-- 右：站点设置 -->
      <div class="sm-detail">
        <template v-if="cur">
          <div class="sm-detail-head">
            <div class="sm-detail-title">
              <AikIcon name="home" :size="15" />
              <b>{{  cur.slug  }}</b>
              <span class="sm-detail-id">{{  cur.id  }}</span>
            </div>
            <span class="sm-detail-route">{{  routeLabel(cur)  }}</span>
          </div>

          <div v-if="!settings" class="sm-empty">{{  $t('加载中…')  }}</div>
          <div v-else class="sm-form">
            <div class="sm-row">
              <label>{{  $t('站点标题')  }}</label>
              <input v-model="settings.title" class="input" :placeholder="$t('显示在页头 / SEO 的站点名')" />
            </div>
            <div class="sm-row">
              <label>{{  $t('副标题 / 一句话描述')  }}</label>
              <input v-model="settings.subtitle" class="input" :placeholder="$t('选填')" />
            </div>
            <div class="sm-row sm-row-2">
              <div>
                <label>{{  $t('默认主题')  }}</label>
                <select v-model="settings.default_theme" class="input">
                  <option v-for="t in themeOptions" :key="t.id" :value="t.id">{{  t.label  }}</option>
                </select>
              </div>
              <div>
                <label>{{  $t('语言')  }}</label>
                <input v-model="settings.locale" class="input" placeholder="zh-CN" />
              </div>
            </div>
            <div class="sm-row sm-switch">
              <label class="sm-switch-label">
                <input type="checkbox" v-model="settings.allow_visitor_theme_switch" />
                <span>{{  $t('允许访客切换主题')  }}</span>
              </label>
              <p class="sm-hint">{{  $t('关闭后，该站强制使用上面的默认主题，访客无法自行切换。')  }}</p>
            </div>
            <div class="sm-row">
              <label>{{  $t('SEO 标题')  }}</label>
              <input v-model="settings.seo_title" class="input" :placeholder="$t('留空则用站点标题')" />
            </div>
            <div class="sm-row">
              <label>{{  $t('SEO 描述')  }}</label>
              <textarea v-model="settings.seo_description" class="input sm-area" rows="2" :placeholder="$t('搜索引擎摘要')"></textarea>
            </div>
            <div class="sm-row">
              <label>{{  $t('SEO 关键词')  }}</label>
              <input v-model="settings.seo_keywords" class="input" :placeholder="$t('逗号分隔')" />
            </div>
            <div class="sm-actions">
              <button class="btn" :disabled="savingSettings" @click="selectSite(cur)">{{  $t('还原')  }}</button>
              <button class="btn btn-primary" :disabled="savingSettings" @click="saveSettings">
                {{  savingSettings ? $t('保存中…') : $t('保存设置')  }}
              </button>
            </div>
          </div>
        </template>
        <div v-else class="sm-empty">
          <AikIcon name="grid" :size="28" />
          <p>{{ $t('选择左侧站点进行设置') }}</p>
        </div>
      </div>
    </div>

    <!-- 授权列表 -->
    <div class="sm-lic">
      <div class="sm-lic-head">
        <b>{{ $t('站点数授权') }}</b>
        <span class="sm-hint">{{ $t('每张授权含可用站点数；订阅到期后额度自动回落。') }}</span>
      </div>
      <div v-if="!licenses.length" class="sm-empty-sm">{{ $t('暂无授权（基础免费档 1 个站点）') }}</div>
      <div v-else class="sm-lic-rows">
        <div v-for="l in licenses" :key="l.id" class="sm-lic-row">
          <span class="sm-lic-id" :title="l.id">{{  l.id  }}</span>
          <span class="sm-badge" :class="l.license_type === 'permanent' ? 'st-perm' : 'st-sub'">
            {{  l.license_type === 'permanent' ? $t('永久') : $t('订阅')  }}
          </span>
          <span class="sm-lic-seats">+{{  l.seats  }} {{ $t('站') }}</span>
          <span class="sm-lic-ed">{{  l.edition || '—'  }}</span>
          <span class="sm-lic-exp">{{  l.license_type === 'permanent' ? $t('永久有效') : $t('至 ') + fmtTime(l.expires_at)  }}</span>
          <span class="sm-badge" :class="'st-' + l.status">{{  l.status === 'active' ? $t('生效') : l.status  }}</span>
        </div>
      </div>
    </div>

    <!-- 新建站点 -->
    <div v-if="createOpen" class="modal-mask" @click.self="createOpen = false">
      <div class="modal-box sm-modal">
        <h3 class="sm-modal-title">{{ $t('新建站点') }}</h3>
        <div class="sm-row">
          <label>{{ $t('站标识 slug') }} <span class="req">*</span></label>
          <input v-model="createForm.slug" class="input" :placeholder="$t('如 docs / shop（唯一，不含空格）')" />
        </div>
        <div class="sm-row sm-row-2">
          <div>
            <label>{{ $t('自定义域名') }}</label>
            <input v-model="createForm.domain" class="input" :placeholder="$t('如 blog.example.com')" />
          </div>
          <div>
            <label>{{ $t('子域名前缀') }}</label>
            <input v-model="createForm.subdomain" class="input" :placeholder="$t('如 shop')" />
          </div>
        </div>
        <div class="sm-row">
          <label>{{ $t('子目录前缀') }}</label>
          <input v-model="createForm.path_prefix" class="input" :placeholder="$t('如 /docs（需运维层 rewrite）')" />
        </div>
        <div class="sm-row sm-row-2">
          <div>
            <label>{{ $t('站点标题') }}</label>
            <input v-model="createForm.title" class="input" :placeholder="$t('选填')" />
          </div>
          <div>
            <label>{{ $t('默认主题') }}</label>
            <select v-model="createForm.default_theme" class="input">
              <option v-for="t in themeOptions" :key="t.id" :value="t.id">{{  t.label  }}</option>
            </select>
          </div>
        </div>
        <label class="sm-switch-label sm-create-switch">
          <input type="checkbox" v-model="createForm.allow_visitor_theme_switch" />
          <span>{{ $t('允许访客切换主题') }}</span>
        </label>
        <p class="sm-hint">{{ $t('接入方式三选一即可：自定义域名 / 子域名 / 子目录。三者都不填时该站仅通过后台预览。') }}</p>
        <div class="sm-modal-actions">
          <button class="btn" @click="createOpen = false">{{ $t('取消') }}</button>
          <button class="btn btn-primary" :disabled="creating" @click="doCreate">
            {{  creating ? $t('创建中…') : $t('创建站点')  }}
          </button>
        </div>
      </div>
    </div>

    <!-- 录入授权 -->
    <div v-if="licenseOpen" class="modal-mask" @click.self="licenseOpen = false">
      <div class="modal-box sm-modal">
        <h3 class="sm-modal-title">{{ $t('录入站点数授权') }}</h3>
        <p class="sm-hint">{{ $t('粘贴在应用中心购买的授权 key（含') }} <code>feature:multisite</code> {{ $t('与站点数），验签通过后即时提升上限。') }}</p>
        <textarea v-model="licenseKey" class="input sm-area sm-lic-input" rows="4" placeholder="payload.signature"></textarea>
        <div class="sm-modal-actions">
          <button class="btn" @click="licenseOpen = false">{{ $t('取消') }}</button>
          <button class="btn btn-primary" :disabled="granting" @click="doGrantLicense">
            {{  granting ? $t('校验中…') : $t('录入授权')  }}
          </button>
        </div>
      </div>
    </div>

    <ConfirmDialog
      v-if="confirmDel"
      :title="$t('删除站点')"
      :message="$t('删除站点「{v0}」？站点将停用并从列表移除，已有内容数据保留。', { v0: confirmDel.slug })"
      confirm-text="删除"
      danger
      @confirm="doDelete(confirmDel)"
      @cancel="confirmDel = null"
    />
  </div>
</template>

<script setup>
import { t as i18t } from '@/i18n'
import { ref, onMounted } from 'vue'
import AikIcon from '@/components/AikIcon.vue'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import { listThemes, themeCatalog } from '@/themes'
import { ensureThemeInstalled } from '@/utils/themeInstall.js'
import * as api from '@/api'
import { useToastStore } from '@/stores/toast'
import { t } from '@/i18n'

const toast = useToastStore()

const loading = ref(false)
const sites = ref([])
const licenses = ref([])
const cap = ref({ base: 1, permanent: 0, subscription: 0, total: 1, used: 0 })
const cur = ref(null)
const settings = ref(null)
const savingSettings = ref(false)
const themeOptions = ref([])

const createOpen = ref(false)
const createForm = ref(blankCreate())
const creating = ref(false)

const licenseOpen = ref(false)
const licenseKey = ref('')
const granting = ref(false)

const confirmDel = ref(null)

function blankCreate() {
  return {
    slug: '',
    domain: '',
    subdomain: '',
    path_prefix: '',
    title: '',
    default_theme: 'aiklog',
    allow_visitor_theme_switch: true
  }
}

async function loadAll() {
  loading.value = true
  try {
    const d = await api.adminSitesList()
    sites.value = d.sites || []
    licenses.value = d.licenses || []
    if (d.site_cap) cap.value = d.site_cap
    const prev = cur.value ? sites.value.find((s) => s.id === cur.value.id) : null
    if (prev) {
      cur.value = prev
    } else if (sites.value.length) {
      await selectSite(sites.value[0])
    } else {
      cur.value = null
      settings.value = null
    }
  } catch (e) {
    toast.push(t('加载站点失败：') + (e.message || e))
  } finally {
    loading.value = false
  }
}

async function loadThemeOptions() {
  try {
    const d = await api.requestJSON('/public/site')
    const opts = Array.isArray(d?.theme_options) ? d.theme_options : null
    if (opts && opts.length) {
      const out = opts.map((o) => ({ id: o.id, label: o.title ? `${o.title}（${o.id}）` : o.id }))
      // 补齐 catalog 中未安装的懒主题（改版 2026-10-09：主题应用中心按需安装；保存/建站时自动装）
      const seen = new Set(out.map((o) => o.id))
      for (const c of themeCatalog()) {
        if (!seen.has(c.id)) out.push({ id: c.id, label: `${c.title}（${t('未安装 · 保存时自动安装')}）` })
      }
      themeOptions.value = out
      return
    }
  } catch (_) {
    /* 离线兜底 */
  }
  const base = listThemes().map((t) => ({ id: t.id, label: t.title ? `${t.title}（${t.id}）` : t.id }))
  const seen = new Set(base.map((o) => o.id))
  for (const c of themeCatalog()) {
    if (!seen.has(c.id)) base.push({ id: c.id, label: `${c.title}（${t('未安装 · 保存时自动安装')}）` })
  }
  themeOptions.value = base
}

async function selectSite(s) {
  cur.value = s
  settings.value = null
  try {
    const d = await api.adminSiteSettingsGet(s.id)
    settings.value = normalizeSettings(d.settings || {}, s)
  } catch (e) {
    toast.push(t('加载站点设置失败：') + (e.message || e))
  }
}

function normalizeSettings(ss, s) {
  return {
    site_id: ss.site_id || s.id,
    title: ss.title || '',
    subtitle: ss.subtitle || '',
    locale: ss.locale || 'zh-CN',
    default_theme: ss.default_theme || 'aiklog',
    allow_visitor_theme_switch: ss.allow_visitor_theme_switch !== false,
    seo_title: ss.seo_title || '',
    seo_description: ss.seo_description || '',
    seo_keywords: ss.seo_keywords || ''
  }
}

async function saveSettings() {
  if (!cur.value || !settings.value) return
  // 选中未安装主题 → 保存前自动走应用中心安装（内置源码主题免 zip，登记即生效）
  if (!(await ensureThemeInstalled(settings.value.default_theme))) {
    toast.push(t('主题自动安装失败，请到应用中心手动安装后再保存'))
    return
  }
  savingSettings.value = true
  try {
    await api.adminSiteSettingsUpdate(cur.value.id, { ...settings.value })
    toast.push(t('站点设置已保存'))
    await loadAll()
  } catch (e) {
    toast.push(t('保存失败：') + (e.message || e))
  } finally {
    savingSettings.value = false
  }
}

function openCreate() {
  if (cap.value.used >= cap.value.total) {
    toast.push(`站点数已达上限（${cap.value.used}/${cap.value.total}），请先录入站点数授权`)
    openLicense()
    return
  }
  createForm.value = blankCreate()
  createOpen.value = true
}

async function doCreate() {
  const f = createForm.value
  if (!f.slug.trim()) {
    toast.push(t('请填写站标识 slug'))
    return
  }
  creating.value = true
  try {
    // 建站带未安装主题 → 先自动安装（登记失败则中止，避免站点指向不可用主题）
    if (!(await ensureThemeInstalled(f.default_theme))) {
      toast.push(t('主题自动安装失败，请到应用中心手动安装后再创建'))
      creating.value = false
      return
    }
    await api.adminSiteCreate({
      slug: f.slug.trim(),
      domain: f.domain.trim(),
      subdomain: f.subdomain.trim(),
      path_prefix: f.path_prefix.trim(),
      title: f.title.trim(),
      default_theme: f.default_theme,
      allow_visitor_theme_switch: !!f.allow_visitor_theme_switch
    })
    createOpen.value = false
    toast.push(t('站点已创建'))
    await loadAll()
  } catch (e) {
    const msg = e.message || ''
    if (/SITE_LIMIT_REACHED|上限/.test(msg)) {
      toast.push(t('站点数已达上限，请先录入授权'))
      openLicense()
    } else {
      toast.push(t('创建失败：') + msg)
    }
  } finally {
    creating.value = false
  }
}

async function doDelete(s) {
  confirmDel.value = null
  if (!s) return
  try {
    await api.adminSiteDelete(s.id)
    if (cur.value && cur.value.id === s.id) {
      cur.value = null
      settings.value = null
    }
    toast.push(t('站点已删除'))
    await loadAll()
  } catch (e) {
    toast.push(t(i18t('删除失败：')) + (e.message || e))
  }
}

function openLicense() {
  licenseKey.value = ''
  licenseOpen.value = true
}

async function doGrantLicense() {
  const k = licenseKey.value.trim()
  if (!k) {
    toast.push(t('请粘贴授权 key'))
    return
  }
  granting.value = true
  try {
    const d = await api.adminSiteLicenseGrant(k)
    licenseOpen.value = false
    toast.push(`授权已录入，站点数上限 → ${d.cap_total}`)
    await loadAll()
  } catch (e) {
    toast.push(t('授权无效：') + (e.message || e))
  } finally {
    granting.value = false
  }
}

function statusLabel(s) {
  if (s === 'active') return i18t('启用')
  if (s === 'suspended') return i18t('已停用')
  if (s === 'deleted') return i18t('已删除')
  return s || '—'
}

function routeLabel(s) {
  const parts = []
  if (s.domain && s.domain !== '*') parts.push(i18t('域名') + s.domain)
  if (s.subdomain) parts.push(t('子域名 ') + s.subdomain + '.*')
  if (s.path_prefix) parts.push(t('子目录 ') + s.path_prefix)
  if (!parts.length) parts.push(s.id === 'default' ? i18t('主域名（默认站）') : i18t('未绑定接入方式'))
  return parts.join(' · ')
}

function fmtTime(ts) {
  if (!ts) return '—'
  const d = new Date(ts * 1000)
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}

onMounted(() => {
  loadThemeOptions()
  loadAll()
})
</script>

<style scoped>
.sm-view { max-width: 1200px; margin: 0 auto; padding: 20px 24px 40px; }
.sm-head { display: flex; justify-content: space-between; align-items: flex-start; gap: 12px; margin-bottom: 14px; }
.sm-head h2 { font-size: 20px; margin: 0 0 4px; }
.sm-sub { color: var(--text-3, #8a919f); font-size: 13px; margin: 0; max-width: 720px; line-height: 1.6; }
.sm-tools { display: inline-flex; gap: 8px; flex-shrink: 0; }

.sm-cap { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; padding: 12px 14px; border: 1px solid var(--border, #e3e6ee); border-radius: 10px; background: var(--surface-2, #f7f8fa); margin-bottom: 16px; }
.sm-cap-item { display: flex; flex-direction: column; min-width: 74px; }
.sm-cap-n { font-size: 16px; font-weight: 700; color: var(--text, #20242c); }
.sm-cap-l { font-size: 11.5px; color: var(--text-3, #8a919f); }
.sm-cap-total .sm-cap-n { color: var(--primary, #2f6bff); }
.sm-cap-total.full .sm-cap-n { color: #e03e3e; }
.sm-cap-btn { margin-left: auto; }

.sm-main { display: flex; gap: 16px; align-items: flex-start; }
.sm-list { flex: 0 0 320px; display: flex; flex-direction: column; gap: 8px; max-height: calc(100vh - 300px); overflow-y: auto; }
.sm-card { border: 1px solid var(--border, #e3e6ee); border-radius: 10px; background: var(--bg-2, #fff); padding: 11px 12px; cursor: pointer; transition: border-color .14s, box-shadow .14s; }
.sm-card:hover { border-color: var(--primary, #2f6bff); }
.sm-card.sel { border-color: var(--primary, #2f6bff); box-shadow: 0 0 0 3px var(--primary-soft, rgba(47,107,255,.12)); }
.sm-card-top { display: flex; align-items: center; gap: 6px; }
.sm-card-slug { font-size: 14px; color: var(--text, #20242c); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.sm-badge { font-size: 11px; padding: 1px 7px; border-radius: 8px; background: var(--bg-3, #f0f1f4); color: var(--text-3, #8a919f); flex-shrink: 0; }
.sm-badge.st-active { background: rgba(46,164,79,.14); color: #2ea44f; }
.sm-badge.st-suspended { background: rgba(224,62,62,.12); color: #e03e3e; }
.sm-badge.st-default { background: var(--primary-soft, #eaf1ff); color: var(--primary, #2f6bff); }
.sm-badge.st-perm { background: rgba(46,164,79,.14); color: #2ea44f; }
.sm-badge.st-sub { background: rgba(214,158,46,.16); color: #b8860b; }
.sm-card-route { display: flex; align-items: center; gap: 4px; font-size: 12px; color: var(--text-3, #8a919f); margin: 6px 0 8px; }
.sm-card-ops { display: inline-flex; gap: 6px; }

.sm-detail { flex: 1; min-width: 0; border: 1px solid var(--border, #e3e6ee); border-radius: 10px; background: var(--bg-2, #fff); min-height: 420px; }
.sm-detail-head { display: flex; align-items: center; justify-content: space-between; gap: 10px; padding: 13px 16px; border-bottom: 1px solid var(--border, #ececf1); flex-wrap: wrap; }
.sm-detail-title { display: flex; align-items: center; gap: 8px; font-size: 15px; min-width: 0; }
.sm-detail-id { font-size: 11px; color: var(--text-3, #8a919f); }
.sm-detail-route { font-size: 12px; color: var(--text-3, #8a919f); }

.sm-form { padding: 14px 16px; display: flex; flex-direction: column; gap: 12px; }
.sm-row { display: flex; flex-direction: column; gap: 5px; }
.sm-row-2 { flex-direction: row; gap: 12px; }
.sm-row-2 > div { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 5px; }
.sm-row label { font-size: 12.5px; color: var(--text-2, #4a5164); }
.req { color: #e03e3e; }
.sm-hint { font-size: 11.5px; color: var(--text-3, #8a919f); margin: 2px 0 0; line-height: 1.6; }
.sm-switch-label { display: inline-flex; align-items: center; gap: 7px; font-size: 13px; color: var(--text, #20242c); cursor: pointer; }

.input { width: 100%; box-sizing: border-box; height: 34px; padding: 0 12px; border-radius: var(--radius-sm, 8px); border: 1px solid var(--border, #e3e6ee); background: var(--surface-2, #f7f8fa); color: var(--text, #20242c); font-size: 13px; outline: none; transition: border-color .15s, box-shadow .15s, background .15s; }
.input::placeholder { color: var(--text-3, #a0a6b2); }
.input:focus { border-color: var(--primary, #2f6bff); background: var(--bg-1, #fff); box-shadow: 0 0 0 3px rgba(47,107,255,.12); }
select.input { height: 34px; }
.sm-area { height: auto; padding: 8px 12px; resize: vertical; font-family: inherit; line-height: 1.6; }

.sm-actions { display: flex; justify-content: flex-end; gap: 10px; margin-top: 4px; }

.sm-lic { margin-top: 18px; border: 1px solid var(--border, #e3e6ee); border-radius: 10px; background: var(--bg-2, #fff); padding: 14px 16px; }
.sm-lic-head { display: flex; align-items: baseline; gap: 8px; margin-bottom: 10px; }
.sm-lic-head b { font-size: 14px; }
.sm-lic-rows { display: flex; flex-direction: column; gap: 6px; }
.sm-lic-row { display: flex; align-items: center; gap: 10px; padding: 7px 9px; border-radius: 7px; background: var(--surface-2, #f7f8fa); font-size: 12.5px; }
.sm-lic-id { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--text, #20242c); }
.sm-lic-seats { font-weight: 600; color: var(--primary, #2f6bff); }
.sm-lic-ed { color: var(--text-3, #8a919f); }
.sm-lic-exp { color: var(--text-3, #8a919f); }
.sm-lic-input { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 12px; }

.sm-empty { flex: 1; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 8px; color: var(--text-3, #8a919f); padding: 40px 0; font-size: 13px; }
.sm-empty p { margin: 0; }
.sm-empty-sm { color: var(--text-3, #8a919f); font-size: 12.5px; padding: 6px 0; }

.modal-mask { position: fixed; inset: 0; z-index: 400; background: rgba(22,24,43,.45); display: flex; align-items: center; justify-content: center; }
.modal-box { background: var(--surface, #fff); border-radius: var(--radius, 12px); padding: 22px 24px; box-shadow: 0 20px 50px rgba(22,24,43,.2); }
.sm-modal { width: 520px; max-width: calc(100vw - 40px); display: flex; flex-direction: column; gap: 12px; }
.sm-modal-title { margin: 0; font-size: 16px; color: var(--text, #20242c); }
.sm-create-switch { margin-top: 2px; }
.sm-modal-actions { display: flex; justify-content: flex-end; gap: 10px; margin-top: 6px; }
</style>
