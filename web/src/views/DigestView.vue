<template>
  <div class="dg-view">
    <div class="dg-head">
      <div>
        <h2>{{  $t('每日知识日报')  }}</h2>
        <p class="dg-sub">
          {{  $t('汇总当天入库、评论、冲突与知识库规模，推送给站长。可在此预览或立即推送一次； 定时推送由服务端配置（')  }}<code>digest.enabled</code> / <code>digest.hour</code>{{  $t('，默认每天 8 点）。')  }}
        </p>
      </div>
      <div class="dg-tools">
        <button class="btn" :disabled="loading" @click="loadPreview"><AikIcon name="refresh" :size="14" />{{  $t('刷新预览')  }}</button>
        <button class="btn btn-primary" :disabled="sending" @click="doSend">
          <AikIcon name="bell" :size="14" />{{  sending ? $t('推送中…') : $t('立即推送')  }}
        </button>
      </div>
    </div>

    <div v-if="loading && !report" class="dg-empty">{{  $t('加载中…')  }}</div>

    <template v-else-if="report">
      <div class="dg-kpis">
        <div class="dg-kpi">
          <span class="dg-kpi-n">{{  report.inbox_today || 0  }}</span>
          <span class="dg-kpi-l">{{  $t('今日入库')  }}</span>
        </div>
        <div class="dg-kpi" :class="{ warn: report.conflicts > 0 }">
          <span class="dg-kpi-n">{{  report.conflicts || 0  }}</span>
          <span class="dg-kpi-l">{{  $t('未处理冲突')  }}</span>
        </div>
        <div class="dg-kpi">
          <span class="dg-kpi-n">{{  report.comments || 0  }}</span>
          <span class="dg-kpi-l">{{  $t('今日新增评论')  }}</span>
        </div>
        <div class="dg-kpi">
          <span class="dg-kpi-n">{{  report.total_files || 0  }}</span>
          <span class="dg-kpi-l">{{  $t('知识库文件总数')  }}</span>
        </div>
      </div>

      <div class="dg-main">
        <div class="dg-card">
          <div class="dg-card-head">
            <b>{{  $t('今日入库')  }}</b>
            <span class="dg-hint">{{  (report.inbox_items || []).length  }} {{  $t('条（最多列前 5）')  }}</span>
          </div>
          <div v-if="!(report.inbox_items || []).length" class="dg-empty-sm">{{  $t('今天还没有新内容入库')  }}</div>
          <ul v-else class="dg-list">
            <li v-for="(n, i) in report.inbox_items" :key="i">
              <AikIcon name="fileText" :size="13" /><span :title="n">{{  n  }}</span>
            </li>
          </ul>
        </div>

        <div class="dg-card">
          <div class="dg-card-head"><b>{{  $t('关注提示')  }}</b></div>
          <div class="dg-tips">
            <div class="dg-tip" :class="{ warn: report.conflicts > 0 }">
              <AikIcon name="alert" :size="14" />
              <span v-if="report.conflicts > 0">{{  $t('有')  }} {{  report.conflicts  }} {{  $t('条冲突通知待处理（导入时同源内容冲突）')  }}</span>
              <span v-else>{{  $t('没有待处理的冲突通知')  }}</span>
            </div>
            <div class="dg-tip" :class="{ warn: !!report.inactive_dir }">
              <AikIcon name="folder" :size="14" />
              <span v-if="report.inactive_dir">{{  $t('活跃度最低目录：')  }}{{  report.inactive_dir  }} {{  $t('—— 可考虑补充内容')  }}</span>
              <span v-else>{{  $t('目录活跃度分布正常')  }}</span>
            </div>
            <div class="dg-tip">
              <AikIcon name="bell" :size="14" />
              <span>{{  $t('推送目标：当前登录账号（')  }}{{  meName  }}{{  $t('）的通知中心')  }}</span>
            </div>
          </div>
        </div>
      </div>

      <div class="dg-card">
        <div class="dg-card-head">
          <b>{{  $t('日报原文')  }}</b>
          <span class="dg-hint">{{  $t('日期')  }} {{  report.date  }}</span>
          <button class="btn btn-sm dg-copy" @click="copyText"><AikIcon name="copy" :size="13" />{{  $t('复制')  }}</button>
        </div>
        <pre class="dg-pre">{{  text  }}</pre>
      </div>
    </template>

    <div v-else class="dg-empty">
      <AikIcon name="bell" :size="28" />
      <p>{{ $t('无法生成日报') }}</p>
      <button class="btn" @click="loadPreview">{{ $t('重试') }}</button>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import AikIcon from '@/components/AikIcon.vue'
import * as api from '@/api'
import { useToastStore } from '@/stores/toast'
import { t } from '@/i18n'

const toast = useToastStore()

const loading = ref(false)
const sending = ref(false)
const report = ref(null)
const text = ref('')
const meName = ref(t('当前账号'))

async function loadPreview() {
  loading.value = true
  try {
    const d = await api.digestPreview()
    report.value = d.report || null
    text.value = d.text || ''
  } catch (e) {
    report.value = null
    toast.push(t('生成日报失败：') + (e.message || e))
  } finally {
    loading.value = false
  }
}

async function doSend() {
  sending.value = true
  try {
    await api.digestRun()
    toast.push(t('日报已推送'))
  } catch (e) {
    toast.push(t('推送失败：') + (e.message || e))
  } finally {
    sending.value = false
  }
}

async function copyText() {
  try {
    await navigator.clipboard.writeText(text.value || '')
    toast.push(t('已复制日报原文'))
  } catch (_) {
    toast.push(t('复制失败，请手动选择文本'))
  }
}

async function loadMe() {
  try {
    const me = await api.authMe()
    const u = me.user || me
    meName.value = u.display_name || u.username || t('当前账号')
  } catch (_) {
    /* 忽略：仅用于文案展示 */
  }
}

onMounted(() => {
  loadPreview()
  loadMe()
})
</script>

<style scoped>
.dg-view { max-width: 1200px; margin: 0 auto; padding: 20px 24px 40px; }
.dg-head { display: flex; justify-content: space-between; align-items: flex-start; gap: 12px; margin-bottom: 14px; }
.dg-head h2 { font-size: 20px; margin: 0 0 4px; }
.dg-sub { color: var(--text-3, #8a919f); font-size: 13px; margin: 0; max-width: 780px; line-height: 1.6; }
.dg-sub code { font-size: 12px; padding: 1px 5px; border-radius: 4px; background: var(--bg-3, #f0f1f4); }
.dg-tools { display: inline-flex; gap: 8px; flex-shrink: 0; }

.dg-kpis { display: flex; gap: 10px; flex-wrap: wrap; margin-bottom: 14px; }
.dg-kpi { flex: 1 1 150px; display: flex; flex-direction: column; gap: 2px; padding: 12px 14px; border: 1px solid var(--border, #e3e6ee); border-radius: 10px; background: var(--surface-2, #f7f8fa); }
.dg-kpi-n { font-size: 20px; font-weight: 700; color: var(--text, #20242c); }
.dg-kpi-l { font-size: 11.5px; color: var(--text-3, #8a919f); }
.dg-kpi.warn .dg-kpi-n { color: #d68a2e; }

.dg-main { display: flex; gap: 14px; align-items: flex-start; margin-bottom: 14px; }
.dg-main > .dg-card { flex: 1; min-width: 0; margin-bottom: 0; }

.dg-card { border: 1px solid var(--border, #e3e6ee); border-radius: 10px; background: var(--bg-2, #fff); margin-bottom: 14px; }
.dg-card-head { display: flex; align-items: center; gap: 8px; padding: 12px 14px; border-bottom: 1px solid var(--border, #ececf1); }
.dg-card-head b { font-size: 14px; }
.dg-hint { font-size: 11.5px; color: var(--text-3, #8a919f); }
.dg-copy { margin-left: auto; }

.dg-list { list-style: none; margin: 0; padding: 8px 14px 12px; display: flex; flex-direction: column; gap: 6px; }
.dg-list li { display: flex; align-items: center; gap: 7px; font-size: 13px; color: var(--text-2, #4a5164); }
.dg-list li span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

.dg-tips { display: flex; flex-direction: column; gap: 8px; padding: 12px 14px; }
.dg-tip { display: flex; align-items: flex-start; gap: 8px; font-size: 12.5px; color: var(--text-2, #4a5164); line-height: 1.6; }
.dg-tip.warn { color: #b8860b; }

.dg-pre { margin: 0; padding: 14px; font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 12.5px; line-height: 1.75; color: var(--text, #20242c); white-space: pre-wrap; word-break: break-word; background: var(--surface-2, #f7f8fa); border-radius: 0 0 10px 10px; }

.dg-empty { display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 8px; padding: 60px 0; color: var(--text-3, #8a919f); font-size: 13px; border: 1px dashed var(--border, #e3e6ee); border-radius: 10px; }
.dg-empty p { margin: 0; }
.dg-empty-sm { padding: 18px 14px; color: var(--text-3, #8a919f); font-size: 12.5px; }
</style>
