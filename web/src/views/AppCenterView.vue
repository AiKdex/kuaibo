<template>
  <div class="ac">
    <div class="ac-head">
      <div class="ac-head-l">
        <h2>{{  t('应用中心')  }}</h2>
        <p class="ac-sub">
          {{  t('官方应用目录：为主题与插件提供统一安装来源。目录由官方维护，不随本实例站点变化。')  }}
        </p>
      </div>
      <div class="ac-head-r">
        <button class="btn btn-sm" :disabled="busy" @click="load">{{  t('刷新目录')  }}</button>
        <!-- 产品约定：自部署实例的应用中心恒指向官方站点（B33） -->
        <a
          class="btn btn-sm btn-primary"
          :href="siteUrl"
          target="_blank"
          rel="noopener"
          :title="t('在新标签打开官方应用中心网站')"
        >
          <AikIcon name="globe" :size="14" />
          <span>{{  t('打开官网')  }}</span>
        </a>
      </div>
    </div>

    <div class="ac-meta">
      <span class="ac-chip">{{  t('目录来源')  }} <code>{{  indexUrl  }}</code></span>
      <span class="ac-chip">{{  t('本壳')  }} <code>{{  shell  }}</code></span>
      <span class="ac-chip">{{  t('版本档位')  }} <strong>{{  edition.toUpperCase()  }}</strong></span>
      <span v-if="err" class="ac-err">{{  err  }}</span>
    </div>

    <!-- 远程索引签名验签：默认强制；自托管/社区索引可显式信任跳过（逐包 sha256 仍校验） -->
    <div class="ac-verify">
      <label class="ac-switch">
        <input type="checkbox" v-model="skipVerify" @change="setIndexVerify(skipVerify)" :disabled="verifyBusy" />
        <span>{{  t('信任此目录源（跳过签名验签）')  }}</span>
      </label>
      <span class="ac-dim">{{  t('仅当索引来源可信时开启；关闭后每个安装包仍会校验 sha256 完整性')  }}</span>
    </div>

    <!-- 许可证：付费应用安装门禁（community 免费 / pro 解锁 tier=paid） -->
    <div class="ac-license">
      <span class="ac-lic-badge" :class="{ pro: edition === 'pro' }">{{  edition === 'pro' ? 'PRO' : 'COMMUNITY'  }}</span>
      <span v-if="license?.expires_at" class="ac-dim">{{  t('到期')  }} {{  String(license.expires_at).slice(0, 10)  }}</span>
      <span v-if="license?.reason" class="ac-err">{{  license.reason  }}</span>
      <span class="ac-dim">{{  t('输入签发的 license key 解锁付费应用安装；留空保持社区版')  }}</span>
      <input v-model="licenseKey" class="ac-input" placeholder="license key…" />
      <button class="btn btn-sm" :disabled="licBusy || !licenseKey" @click="activateLicense">{{  t('激活')  }}</button>
      <button v-if="license?.key_set" class="btn btn-sm" :disabled="licBusy" @click="deactivateLicense">{{  t('清除')  }}</button>
      <span v-if="licMsg" class="ac-dim">{{  licMsg  }}</span>
    </div>

    <div class="ac-tabs">
      <button class="ac-tab" :class="{ on: tab === 'plugins' }" @click="tab = 'plugins'">
        {{  t('插件')  }}<span v-if="plugins.length">({{  plugins.length  }})</span>
      </button>
      <button class="ac-tab" :class="{ on: tab === 'themes' }" @click="tab = 'themes'">
        {{  t('主题')  }}<span v-if="themes.length">({{  themes.length  }})</span>
      </button>
      <button class="ac-tab" :class="{ on: tab === 'ai' }" @click="tab = 'ai'">
        {{  t('AI 供给')  }}<span v-if="aiSupplies.length">({{  aiSupplies.length  }})</span>
      </button>
      <button class="ac-tab" :class="{ on: tab === 'stats' }" @click="switchStats">
        {{  t('装机统计')  }}
      </button>
    </div>

    <!-- B47 装机统计（官方侧看板；仅管理员可见数据） -->
    <div v-if="tab === 'stats'" class="ac-stats">
      <p class="ac-stats-note">{{  t(statsNote)  }}</p>
      <div v-if="statsErr" class="ac-err">{{  statsErr  }}</div>
      <div v-else-if="stats" class="ac-stats-body">
        <div class="ac-kpis">
          <div class="ac-kpi">
            <span class="ac-kpi-n">{{  stats.instances  }}</span>
            <span class="ac-kpi-l">{{  t('去重装机数')  }}</span>
          </div>
          <div class="ac-kpi">
            <span class="ac-kpi-n">{{  stats.active_24h  }}</span>
            <span class="ac-kpi-l">{{  t('24 小时活跃')  }}</span>
          </div>
          <div class="ac-kpi">
            <span class="ac-kpi-n">{{  stats.active_7d  }}</span>
            <span class="ac-kpi-l">{{  t('7 天活跃')  }}</span>
          </div>
          <div class="ac-kpi">
            <span class="ac-kpi-n">{{  stats.hits  }}</span>
            <span class="ac-kpi-l">{{  t('累计回源次数')  }}</span>
          </div>
        </div>
        <div class="ac-stats-cols">
          <div class="ac-stats-box">
            <h4>{{  t('版本分布')  }}</h4>
            <div v-if="!stats.versions.length" class="ac-dim">{{  t('暂无数据')  }}</div>
            <div v-for="v in stats.versions" :key="v.version" class="ac-bar-row">
              <span class="ac-bar-lb">{{  v.version  }}</span>
              <span class="ac-bar"><i :style="{ width: pct(v.count, stats.instances) }"></i></span>
              <span class="ac-bar-n">{{  v.count  }}</span>
            </div>
          </div>
          <div class="ac-stats-box">
            <h4>{{  t('壳分布')  }}</h4>
            <div v-if="!stats.shells.length" class="ac-dim">{{  t('暂无数据')  }}</div>
            <div v-for="s in stats.shells" :key="s.shell" class="ac-bar-row">
              <span class="ac-bar-lb">{{  s.shell  }}</span>
              <span class="ac-bar"><i :style="{ width: pct(s.count, stats.instances) }"></i></span>
              <span class="ac-bar-n">{{  s.count  }}</span>
            </div>
          </div>
        </div>
      </div>
    </div>

    <div v-if="tab === 'plugins'" class="ac-grid">
      <div v-for="p in plugins" :key="p.id" class="ac-item">
        <div class="ac-item-t">
          <span class="ac-name">{{  p.name  }}</span>
          <span v-if="!p.applicable" class="ac-badge ac-badge-muted">{{  t('本壳不可用')  }}</span>
          <span v-if="p.tier === 'paid'" class="ac-badge ac-badge-paid">{{  p.price ? t('付费 · ') + p.price : t('付费')  }}</span>
          <span v-if="p.installed" class="ac-badge ac-badge-ok">{{  t('已安装')  }}</span>
        </div>
        <p class="ac-desc">{{  p.description  }}</p>
        <div class="ac-item-f">
          <span class="ac-dim">v{{  p.version  }} · {{  p.author || '—'  }}</span>
          <a v-if="p.tier === 'paid' && p.purchase_url && !p.installed" class="btn btn-sm" :href="p.purchase_url" target="_blank" rel="noopener">{{  t('去购买')  }}</a>
          <button v-if="!p.installed && p.applicable" class="btn btn-sm" :disabled="busy" @click="install(p)">
            {{  p.tier === 'paid' ? t('购买安装') : t('安装')  }}
          </button>
          <span v-else-if="!p.applicable" class="ac-dim">{{  t('不可用')  }}</span>
          <span v-else class="ac-dim">{{  t('已安装')  }}</span>
        </div>
      </div>
      <div v-if="!plugins.length" class="ac-empty">{{  busy ? t('加载中…') : (err ? t('目录加载失败') : t('暂无插件'))  }}</div>
    </div>

    <div v-if="tab === 'themes'" class="ac-grid">
      <div v-for="t in themes" :key="t.id" class="ac-item">
        <img v-if="t.cover" :src="t.cover" class="ac-cover" alt="" loading="lazy" />
        <div class="ac-item-t">
          <span class="ac-name">{{  t.name  }}</span>
          <span v-if="!t.applicable" class="ac-badge ac-badge-muted">{{  t('本壳不可用')  }}</span>
          <span v-if="t.tier === 'paid'" class="ac-badge ac-badge-paid">{{  t.price ? t('付费 · ') + t.price : t('付费')  }}</span>
          <span v-if="t.installed" class="ac-badge ac-badge-ok">{{  t('已安装')  }}</span>
        </div>
        <p class="ac-desc">{{  t.description  }}</p>
        <div class="ac-item-f">
          <span class="ac-dim">v{{  t.version  }} · {{  t.author || '—'  }}</span>
          <a v-if="t.tier === 'paid' && t.purchase_url && !t.installed" class="btn btn-sm" :href="t.purchase_url" target="_blank" rel="noopener">{{  t('去购买')  }}</a>
          <button v-if="!t.installed && t.applicable" class="btn btn-sm" :disabled="busy" @click="install(t)">
            {{  t.tier === 'paid' ? t('购买安装') : t('安装')  }}
          </button>
          <span v-else-if="!t.applicable" class="ac-dim">{{  t('不可用')  }}</span>
          <span v-else class="ac-dim">{{  t('已安装')  }}</span>
        </div>
      </div>
      <div v-if="!themes.length" class="ac-empty">{{  busy ? t('加载中…') : (err ? t('目录加载失败') : t('暂无主题'))  }}</div>
    </div>

    <!-- AI 供给（B38）：平台供模型套餐，开通即登记平台网关 provider + 站点池充值 -->
    <div v-if="tab === 'ai'" class="ac-grid">
      <div v-for="s in aiSupplies" :key="s.id" class="ac-item">
        <div class="ac-item-t">
          <span class="ac-name">{{  s.name  }}</span>
          <span v-if="!s.applicable" class="ac-badge ac-badge-muted">{{  t('本壳不可用')  }}</span>
          <span v-if="s.price" class="ac-badge ac-badge-paid">{{  s.price  }}</span>
          <span v-if="s.installed" class="ac-badge ac-badge-ok">{{  t('已开通')  }}</span>
        </div>
        <p class="ac-desc">{{  s.description  }}</p>
        <div class="ac-item-f">
          <span class="ac-dim">v{{  s.version  }}</span>
          <a v-if="s.purchase_url && !s.installed" class="btn btn-sm" :href="s.purchase_url" target="_blank" rel="noopener">{{  t('去购买')  }}</a>
          <button v-if="!s.installed && s.applicable" class="btn btn-sm" :disabled="busy" @click="install(s)">
            {{  s.purchase_url ? t('购买开通') : t('开通')  }}
          </button>
          <span v-else-if="!s.applicable" class="ac-dim">{{  t('不可用')  }}</span>
          <span v-else class="ac-dim">{{  t('已开通')  }}</span>
        </div>
      </div>
      <div v-if="!aiSupplies.length" class="ac-empty">{{  busy ? t('加载中…') : (err ? t('目录加载失败') : t('暂无 AI 供给套餐'))  }}</div>
    </div>
  </div>
</template>

<script setup>
/**
 * AppCenterView —— 独立应用中心（B33）。
 *
 * 为什么独立成页：应用中心是「官方目录」，不是本实例的设置项。
 * 产品约定 = 自部署版本装好后，用户点应用中心看到的**恒是官方站点**，
 * 而不是某次部署的快照（否则每个自部署站点都成了各自的应用市场，目录必然分叉）。
 *
 * 与设置页的分工：
 *   本页 = 在线目录（装什么）
 *   设置 → 已装应用 = 本地管理（装了的启停/卸载/配置）
 *
 * 目录数据仍经后端 /admin/apps/market 拉取（走 SSRF-safe 客户端 + 官方索引），
 * site_url 由后端下发（officialMarketSiteURL），前端不硬编码官网地址。
 */
import { onMounted, ref } from 'vue'
import { t } from '@/i18n'
import * as api from '@/api'
import { useToastStore } from '@/stores/toast'
import AikIcon from '@/components/AikIcon.vue'

const toast = useToastStore()

const tab = ref('plugins')
const plugins = ref([])
const themes = ref([])
const aiSupplies = ref([])
const indexUrl = ref('')
const siteUrl = ref('https://aikmap.cn/market')
const shell = ref('aiklog')
const edition = ref('community')
const err = ref('')
const busy = ref(false)

// B47 装机统计：懒加载（点开页签才拉），失败如实显示原因而不是空白。
// 不在前端判管理员 —— 权限由后端 /admin/** 把守，前端硬判只会出现"按钮在、点开报错"的错觉。
const stats = ref(null)
const statsErr = ref('')
const statsNote = ref('统计口径：去重装机数按匿名 install_id 统计；内网与离线安装不回源，故此数为下界。')

function pct(n, total) {
  if (!total) return '0%'
  return Math.max(2, Math.round((n * 100) / total)) + '%'
}

async function switchStats() {
  tab.value = 'stats'
  if (stats.value || statsErr.value) return
  try {
    const d = await api.adminMarketBeacon()
    stats.value = d
    if (d && d.note) statsNote.value = d.note
  } catch (e) {
    statsErr.value = e && e.message ? e.message : String(e)
  }
}

const license = ref(null)
const licenseKey = ref('')
const licBusy = ref(false)
const licMsg = ref('')

// 远程索引签名验签开关：skipVerify=true 表示已信任来源、跳过验签（plugin_market.index_verify=false）。
const skipVerify = ref(false)
const verifyBusy = ref(false)
async function setIndexVerify(v) {
  verifyBusy.value = true
  try {
    await api.requestJSON('/admin/settings', {
      method: 'PUT',
      body: JSON.stringify({ key: 'plugin_market.index_verify', value: v ? 'false' : 'true' }),
    })
    await load()
    toast.success(v ? t('已信任此目录源，跳过索引签名验签') : t('已恢复索引签名验签'))
  } catch (e) {
    skipVerify.value = !v
    toast.error(t('设置失败：') + (e?.message || e))
  } finally {
    verifyBusy.value = false
  }
}

async function load() {
  busy.value = true
  err.value = ''
  try {
    const m = await api.appsMarketList()
    plugins.value = m.plugins || []
    themes.value = m.themes || []
    aiSupplies.value = m.ai_supplies || []
    indexUrl.value = m.index_url || ''
    if (m.site_url) siteUrl.value = m.site_url
    shell.value = m.shell || 'aiklog'
    edition.value = m.edition || 'community'
    skipVerify.value = m.index_verify === false
  } catch (e) {
    err.value = e?.message || String(e)
    plugins.value = []
    themes.value = []
  } finally {
    busy.value = false
  }
}

async function install(item) {
  busy.value = true
  try {
    const r = await api.appsMarketInstall({ id: item.id })
    if (r?.ok) {
      toast.success(r?.message || t('已安装：') + (item.name || item.id))
      await load()
    } else {
      toast.error(r?.error?.message || t('安装失败'))
    }
  } catch (e) {
    toast.error(e?.message || t('安装失败'))
  } finally {
    busy.value = false
  }
}

async function activateLicense() {
  licBusy.value = true
  licMsg.value = ''
  try {
    license.value = await api.requestJSON('/license', { method: 'POST', body: JSON.stringify({ key: licenseKey.value }) })
    licMsg.value = license.value?.edition === 'pro' ? t('已激活 PRO') : t('已保存，但未达到 PRO 条件')
    licenseKey.value = ''
    await load()
  } catch (e) {
    licMsg.value = t('激活失败：') + (e?.message || e)
  } finally {
    licBusy.value = false
  }
}

async function deactivateLicense() {
  licBusy.value = true
  licMsg.value = ''
  try {
    license.value = await api.requestJSON('/license', { method: 'DELETE' })
    licMsg.value = t('已清除许可证')
    await load()
  } catch (e) {
    licMsg.value = t('清除失败：') + (e?.message || e)
  } finally {
    licBusy.value = false
  }
}

onMounted(async () => {
  try {
    license.value = await api.requestJSON('/license')
  } catch (_) {
    license.value = null
  }
  await load()
})
</script>

<style scoped>
.ac {
  padding: 20px 22px 32px;
  max-width: 1080px;
}
.ac-head {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  flex-wrap: wrap;
  margin-bottom: 14px;
}
.ac-head-l { flex: 1; min-width: 260px; }
.ac-head h2 { margin: 0 0 4px; font-size: 19px; color: var(--text); }
.ac-sub { margin: 0; font-size: 12.5px; color: var(--text-3); line-height: 1.6; }
.ac-head-r { display: flex; align-items: center; gap: 8px; }

.ac-meta {
  display: flex; align-items: center; gap: 8px; flex-wrap: wrap;
  margin-bottom: 14px; font-size: 12px;
}
.ac-chip {
  background: var(--surface-2); border: 1px solid var(--border);
  border-radius: 999px; padding: 3px 10px; color: var(--text-2);
}
.ac-chip code { background: transparent; padding: 0; font-size: 11.5px; }
.ac-err { color: var(--danger); font-size: 12px; }

.ac-verify {
  display: flex; align-items: center; gap: 10px; flex-wrap: wrap;
  padding: 9px 12px; margin-bottom: 16px;
  background: var(--surface-2); border: 1px dashed var(--border); border-radius: 8px;
}
.ac-switch { display: inline-flex; align-items: center; gap: 7px; cursor: pointer; font-size: 13px; color: var(--text); }
.ac-switch input { width: 15px; height: 15px; accent-color: var(--primary); cursor: pointer; }

.ac-license {
  display: flex; align-items: center; gap: 8px; flex-wrap: wrap;
  padding: 10px 12px; margin-bottom: 16px;
  background: var(--surface-2); border: 1px solid var(--border); border-radius: 8px;
}
.ac-lic-badge {
  font-size: 11px; font-weight: 700; letter-spacing: .5px;
  padding: 2px 8px; border-radius: 999px;
  background: var(--text-3); color: #fff;
}
.ac-lic-badge.pro { background: var(--primary); }
.ac-dim { font-size: 12px; color: var(--text-3); }
.ac-input {
  max-width: 260px; padding: 5px 10px;
  border: 1px solid var(--border); border-radius: 6px;
  background: var(--surface); color: var(--text); font-size: 12.5px;
}

.ac-tabs { display: flex; gap: 6px; margin-bottom: 12px; }

/* B47 装机统计 */
.ac-stats-note {
  margin: 0 0 12px;
  font-size: 12px;
  line-height: 1.6;
  color: var(--text-2, #5b6168);
  background: var(--surface-2, #f7f8fa);
  border: 1px solid var(--border, #e6e8eb);
  border-radius: var(--border-radius-md, 8px);
  padding: 8px 10px;
}
.ac-kpis { display: grid; grid-template-columns: repeat(auto-fit, minmax(130px, 1fr)); gap: 10px; margin-bottom: 14px; }
.ac-kpi {
  display: flex; flex-direction: column; gap: 2px;
  padding: 12px 14px;
  border: 1px solid var(--border, #e6e8eb);
  border-radius: var(--border-radius-lg, 12px);
  background: var(--surface, #fff);
}
.ac-kpi-n { font-size: 22px; font-weight: 500; line-height: 1.2; }
.ac-kpi-l { font-size: 12px; color: var(--text-2, #5b6168); }
.ac-stats-cols { display: grid; grid-template-columns: repeat(auto-fit, minmax(280px, 1fr)); gap: 12px; }
.ac-stats-box {
  padding: 12px 14px;
  border: 1px solid var(--border, #e6e8eb);
  border-radius: var(--border-radius-lg, 12px);
  background: var(--surface, #fff);
}
.ac-stats-box h4 { margin: 0 0 10px; font-size: 13px; font-weight: 500; }
.ac-bar-row { display: grid; grid-template-columns: 120px 1fr 40px; align-items: center; gap: 8px; padding: 3px 0; }
.ac-bar-lb { font-size: 12px; color: var(--text-2, #5b6168); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.ac-bar { height: 6px; border-radius: 3px; background: var(--surface-2, #eef0f3); overflow: hidden; }
.ac-bar i { display: block; height: 100%; border-radius: 3px; background: var(--accent, #2f6bff); }
.ac-bar-n { font-size: 12px; text-align: right; font-variant-numeric: tabular-nums; }
.ac-tab {
  padding: 6px 12px; border-radius: 8px; cursor: pointer;
  border: 1px solid var(--border); background: var(--surface);
  color: var(--text-2); font-size: 13px;
}
.ac-tab.on { background: var(--primary-soft); color: var(--primary); font-weight: 600; }

.ac-grid {
  display: grid; gap: 12px;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
}
.ac-item {
  display: flex; flex-direction: column; gap: 8px;
  padding: 14px; border: 1px solid var(--border);
  border-radius: 10px; background: var(--surface);
}
.ac-cover {
  width: 100%; height: 120px; object-fit: cover;
  border-radius: 8px; border: 1px solid var(--border);
}
.ac-item-t { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; }
.ac-name { font-weight: 600; font-size: 14px; color: var(--text); }
.ac-desc {
  margin: 0; font-size: 12.5px; line-height: 1.6; color: var(--text-2);
  flex: 1;
}
.ac-item-f {
  display: flex; align-items: center; gap: 8px; flex-wrap: wrap;
}
.ac-badge {
  font-size: 10.5px; font-weight: 600; padding: 2px 7px; border-radius: 999px;
}
.ac-badge-muted { background: var(--surface-2); color: var(--text-3); border: 1px solid var(--border); }
.ac-badge-paid { background: var(--primary-soft); color: var(--primary); }
.ac-badge-ok { background: var(--primary); color: #fff; }
.ac-empty { grid-column: 1 / -1; color: var(--text-3); font-size: 13px; padding: 20px 0; }
</style>
