<template>
  <div class="sk-view">
    <div class="sk-head">
      <div>
        <h2>{{  $t('技能包')  }}</h2>
        <p class="sk-sub">
          {{  $t('可授权的能力资产（tool / skill / workflow / extension）。管理员上架后在此目录可见； 免费包可自助领取，付费包由管理员发放授权。 与应用中心（主题 / 插件 / 站点授权）是两条线：那条管站点能力，这条管能力资产。')  }}
        </p>
      </div>
      <button class="btn" :disabled="loading" @click="load"><AikIcon name="refresh" :size="14" />{{  $t('刷新')  }}</button>
    </div>

    <div class="sk-tabs">
      <button class="sk-tab" :class="{ on: kind === '' }" @click="setKind('')">{{  $t('全部')  }}</button>
      <button
        v-for="k in kindOptions"
        :key="k.v"
        class="sk-tab"
        :class="{ on: kind === k.v }"
        @click="setKind(k.v)"
      >{{  k.t  }}</button>
      <span class="sk-count">{{  items.length  }} {{  $t('个')  }}</span>
    </div>

    <div class="sk-card">
      <div v-if="loading" class="sk-empty">{{  $t('加载中…')  }}</div>
      <div v-else-if="!items.length" class="sk-empty">
        <AikIcon name="sparkles" :size="26" />
        <p>{{  $t('目录里还没有技能包')  }}</p>
        <p class="sk-hint">{{  $t('管理员可通过接口上架（POST /api/v1/admin/skills，status=published 后可见）。')  }}</p>
      </div>

      <div v-for="p in items" :key="p.id" class="sk-row">
        <span class="sk-ico"><AikIcon name="sparkles" :size="15" /></span>
        <div class="sk-main">
          <div class="sk-title">
            <b>{{  p.title || p.name  }}</b>
            <span class="sk-kind">{{  kindLabel(p.kind)  }}</span>
            <span class="sk-ver">v{{  p.version  }}</span>
            <span v-if="p.price_cents > 0" class="sk-price">￥{{  (p.price_cents / 100).toFixed(2)  }}</span>
            <span v-else class="sk-free">{{  $t('免费')  }}</span>
          </div>
          <div class="sk-desc">{{  p.description || $t('（暂无说明）')  }}</div>
        </div>
        <div class="sk-ops">
          <span v-if="p.granted" class="sk-owned"><AikIcon name="check" :size="13" />{{  $t('已拥有')  }}</span>
          <button
            v-else-if="p.price_cents === 0"
            class="btn btn-primary btn-sm"
            :disabled="claiming === p.id"
            @click="claim(p)"
          >{{  claiming === p.id ? $t('领取中…') : $t('领取')  }}</button>
          <span v-else class="sk-paid">{{  $t('需授权')  }}</span>
        </div>
      </div>
    </div>

    <div class="sk-card">
      <div class="sk-card-head">
        <h3>{{  $t('我的授权')  }} <span class="sk-n">{{  grants.length  }}</span></h3>
        <span class="sk-hint-inline">{{  $t('仅显示生效中的授权（已吊销 / 已过期不列出）')  }}</span>
      </div>
      <div v-if="!grants.length" class="sk-empty">{{  $t('暂无生效中的授权')  }}</div>
      <div v-for="g in grants" :key="g.id" class="sk-grant">
        <span class="sk-gico"><AikIcon name="check" :size="13" /></span>
        <span class="sk-gid">{{  nameOf(g.package_id)  }}</span>
        <span class="sk-gsrc">{{  sourceLabel(g.source)  }}</span>
        <span class="sk-gtime">{{  g.expires_at ? $t('到期 ') + fmtTime(g.expires_at) : $t('永久有效')  }}</span>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import AikIcon from '@/components/AikIcon.vue'
import { listSkills, listSkillGrants, claimSkill } from '@/api'
import { t } from '@/i18n'

const items = ref([])
const grants = ref([])
const kind = ref('')
const loading = ref(false)
const claiming = ref('')

const kindOptions = [
  { v: 'tool', t: t('工具') },
  { v: 'skill', t: t('技能') },
  { v: 'workflow', t: t('工作流') },
  { v: 'extension', t: t('扩展') }
]

const kindLabel = (k) => kindOptions.find((o) => o.v === k)?.t || k
const sourceLabel = (s) =>
  ({ free: t('免费领取'), purchase: t('购买'), trial: t('试用'), manual: t('管理员发放') })[s] || s

function nameOf(id) {
  const p = items.value.find((x) => x.id === id)
  return p ? p.title || p.name : id
}

async function load() {
  loading.value = true
  try {
    const [res, gres] = await Promise.all([listSkills(kind.value), listSkillGrants()])
    items.value = res.items || []
    grants.value = gres.items || []
  } catch (e) {
    items.value = []
    grants.value = []
  } finally {
    loading.value = false
  }
}

function setKind(k) {
  kind.value = k
  load()
}

async function claim(p) {
  claiming.value = p.id
  try {
    await claimSkill(p.id)
    await load()
  } catch (e) {
    window.alert(t('领取失败：') + (e.message || e))
  } finally {
    claiming.value = ''
  }
}

function fmtTime(ms) {
  if (!ms) return '-'
  const d = new Date(ms)
  const p = (n) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`
}

onMounted(load)
</script>

<style scoped>
.sk-view { max-width: 1200px; margin: 0 auto; padding: 20px 24px 40px; }
.sk-head { display: flex; justify-content: space-between; align-items: flex-start; gap: 16px; margin-bottom: 16px; }
.sk-head h2 { margin: 0 0 6px; font-size: 18px; }
.sk-sub { margin: 0; color: var(--text-2, #666); font-size: 13px; line-height: 1.6; max-width: 660px; }
.sk-tabs { display: flex; align-items: center; gap: 8px; margin-bottom: 12px; flex-wrap: wrap; }
.sk-tab { padding: 4px 12px; border: 1px solid var(--border, #d1d5db); border-radius: 999px; background: var(--panel, #fff); color: inherit; font-size: 12px; cursor: pointer; }
.sk-tab.on { background: var(--accent, #2563eb); border-color: var(--accent, #2563eb); color: #fff; }
.sk-count { margin-left: auto; font-size: 12px; color: var(--text-2, #888); }
.sk-card { border: 1px solid var(--border, #e5e7eb); border-radius: 10px; padding: 12px 16px; margin-bottom: 16px; background: var(--panel, #fff); }
.sk-card-head { display: flex; align-items: baseline; gap: 10px; margin-bottom: 10px; }
.sk-card-head h3 { margin: 0; font-size: 14px; }
.sk-n { color: var(--text-2, #888); font-weight: 400; }
.sk-hint-inline { font-size: 12px; color: var(--text-3, #999); }
.sk-empty { padding: 24px 0; text-align: center; color: var(--text-2, #888); font-size: 13px; }
.sk-empty p { margin: 6px 0; }
.sk-hint { font-size: 12px; color: var(--text-3, #aaa); }
.sk-row { display: flex; align-items: flex-start; gap: 12px; padding: 11px 4px; border-top: 1px solid var(--border, #f0f0f0); }
.sk-row:first-of-type { border-top: none; }
.sk-ico { color: var(--text-2, #666); margin-top: 2px; }
.sk-main { flex: 1; min-width: 0; }
.sk-title { display: flex; align-items: baseline; gap: 8px; font-size: 13px; flex-wrap: wrap; }
.sk-kind { font-size: 11px; padding: 1px 6px; border-radius: 4px; background: var(--bg, #f3f4f6); color: var(--text-2, #666); }
.sk-ver { font-size: 11px; color: var(--text-3, #999); }
.sk-price { font-size: 12px; color: #b45309; }
.sk-free { font-size: 12px; color: #15803d; }
.sk-desc { font-size: 12px; color: var(--text-2, #777); margin-top: 4px; line-height: 1.5; }
.sk-ops { display: flex; align-items: center; gap: 8px; }
.sk-owned { font-size: 12px; color: #15803d; display: inline-flex; align-items: center; gap: 4px; }
.sk-paid { font-size: 12px; color: var(--text-3, #999); }
.sk-grant { display: flex; align-items: center; gap: 10px; padding: 8px 4px; border-top: 1px solid var(--border, #f0f0f0); font-size: 12px; }
.sk-grant:first-of-type { border-top: none; }
.sk-gico { color: #15803d; }
.sk-gid { font-weight: 600; }
.sk-gsrc { color: var(--text-2, #777); }
.sk-gtime { margin-left: auto; color: var(--text-3, #999); }
.btn { display: inline-flex; align-items: center; gap: 5px; padding: 6px 12px; border: 1px solid var(--border, #d1d5db); border-radius: 6px; background: var(--panel, #fff); color: inherit; font-size: 13px; cursor: pointer; }
.btn:disabled { opacity: 0.5; cursor: not-allowed; }
.btn-sm { padding: 4px 10px; font-size: 12px; }
.btn-primary { background: var(--accent, #2563eb); border-color: var(--accent, #2563eb); color: #fff; }
</style>
