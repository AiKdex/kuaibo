<template>
  <div class="csp" v-if="nodeType || hasFields">
    <!-- 内容类型徽标 -->
    <div v-if="nodeType" class="csp-badge" :class="'csp-badge--' + nodeType">
      <AikIcon :name="badgeIcon" :size="14" />
      <span>{{  badgeLabel  }}</span>
    </div>

    <!-- 外链卡片（node_type=link） -->
    <section v-if="nodeType === 'link' && linkUrl" class="csp-block">
      <h4 class="csp-title">{{  $t('外部链接')  }}</h4>
      <p v-if="fields.desc" class="csp-desc">{{  fields.desc  }}</p>
      <div class="csp-row">
        <a class="btn btn-primary" :href="linkUrl" target="_blank" rel="noopener noreferrer">{{  $t('访问原站 ↗')  }}</a>
        <button type="button" class="btn btn-ghost" :disabled="checking" @click="checkLink">
          {{  checking ? $t('体检中…') : (checkDone ? $t('重新体检') : $t('链接体检'))  }}
        </button>
        <span v-if="checkDone" class="csp-status" :class="checkOk ? 'ok' : 'bad'">
          <AikIcon :name="checkOk ? 'check' : 'alert'" :size="13" />
          {{  checkOk ? $t('可达') : ($t('不可达') + (checkErr ? '：' + checkErr : ''))  }}
        </span>
      </div>
      <p v-if="fields.site_name" class="csp-sub">{{  $t('来源：')  }}{{  fields.site_name  }}</p>
    </section>

    <!-- 资源下载区（node_type=resource） -->
    <section v-if="nodeType === 'resource'" class="csp-block">
      <h4 class="csp-title">{{  $t('资源下载')  }}</h4>
      <a v-if="downloadUrl" class="btn btn-primary" :href="downloadUrl" :download="fileName">{{  $t('下载资源')  }}</a>
      <a v-else-if="fields.download_url" class="btn btn-primary" :href="fields.download_url" target="_blank" rel="noopener noreferrer">{{  $t('前往下载 ↗')  }}</a>
      <p v-if="fields.version || fields.platform" class="csp-sub">
        <span v-if="fields.version">{{  $t('版本')  }} {{  fields.version  }}</span>
        <span v-if="fields.platform"> · {{  fields.platform  }}</span>
      </p>
    </section>

    <!-- 内容信息卡：所有类型共享的自定义字段（筛选/卡片数据源） -->
    <section v-if="hasFields" class="csp-block">
      <h4 class="csp-title">{{  $t('内容信息')  }}</h4>
      <dl class="csp-dl">
        <div v-for="f in fieldsList" :key="f.key" class="csp-dl-row">
          <dt>{{  f.label  }}</dt>
          <dd v-if="f.key === 'url' || f.key === 'download_url' || f.key === 'homepage'">
            <a :href="f.value" target="_blank" rel="noopener noreferrer">{{  f.value  }}</a>
          </dd>
          <dd v-else>{{  f.value  }}</dd>
        </div>
      </dl>
    </section>
  </div>
</template>

<script setup>
import { computed, ref } from 'vue'
import AikIcon from '@/components/AikIcon.vue'
import { parseContentState, fieldList } from '@/utils/contentState'
import { publicLinkCheck } from '@/api'
import { t } from '@/i18n'

const props = defineProps({
  file: { type: Object, default: () => ({}) },
  token: { type: String, default: '' },
  path: { type: String, default: '' },
  downloadUrl: { type: String, default: '' }
})

const cs = computed(() => parseContentState(props.file))
const nodeType = computed(() => cs.value.nodeType)
const fields = computed(() => cs.value.fields)
const hasFields = computed(() => Object.keys(fields.value).length > 0)
// 信息卡隐藏已在徽标/专区内展示的字段，避免重复
const fieldsList = computed(() =>
  fieldList(props.file, { hide: ['paid_preview', 'desc', 'access', 'url', 'download_url', 'homepage', 'site_name', 'version', 'platform', 'file_name'] })
)

// 内容类型 → 图标/标签（AikIcon 无 tool/doc，用 settings/file 替代）
const BADGES = {
  resource: { label: t('资源'), icon: 'download' },
  link: { label: t('外链'), icon: 'link' },
  doc: { label: t('文档'), icon: 'file' },
  tool: { label: t('工具'), icon: 'settings' },
  article: { label: t('文章'), icon: 'file' }
}
const badge = computed(() => BADGES[nodeType.value] || { label: nodeType.value, icon: 'tag' })
const badgeLabel = computed(() => badge.value.label)
const badgeIcon = computed(() => badge.value.icon)
const linkUrl = computed(() => fields.value.url || '')
const fileName = computed(() => fields.value.file_name || props.file?.name || '')

// 链接体检（服务端探测，见 publicLinkCheck）
const checking = ref(false)
const checkDone = ref(false)
const checkOk = ref(false)
const checkErr = ref('')
async function checkLink() {
  if (!linkUrl.value) return
  checking.value = true
  checkErr.value = ''
  try {
    const d = await publicLinkCheck(linkUrl.value)
    checkDone.value = true
    checkOk.value = !!(d && d.ok)
    checkErr.value = d && d.error ? d.error : ''
  } catch (e) {
    checkDone.value = true
    checkOk.value = false
    checkErr.value = e.message || t('体检失败')
  } finally {
    checking.value = false
  }
}
</script>

<style scoped>
.csp {
  display: flex;
  flex-direction: column;
  gap: 14px;
  margin-top: 8px;
}
.csp-badge {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  align-self: flex-start;
  padding: 4px 10px;
  border-radius: 999px;
  font-size: 12px;
  font-weight: 600;
  background: color-mix(in srgb, var(--primary) 12%, transparent);
  color: var(--primary);
}
.csp-block {
  border: 1px solid var(--border);
  border-radius: var(--radius, 12px);
  padding: 16px 18px;
  background: var(--surface);
}
.csp-title {
  margin: 0 0 10px;
  font-size: 14px;
  font-weight: 700;
  color: var(--text);
}
.csp-desc {
  margin: 0 0 12px;
  font-size: 13px;
  color: var(--text-2, #555);
  line-height: 1.6;
}
.csp-row {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.csp-sub {
  margin: 10px 0 0;
  font-size: 12px;
  color: var(--text-3, #888);
}
.csp-status {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  font-weight: 600;
}
.csp-status.ok { color: #16a34a; }
.csp-status.bad { color: #dc2626; }
.csp-dl {
  margin: 0;
  display: grid;
  grid-template-columns: max-content 1fr;
  gap: 8px 16px;
  font-size: 13px;
}
.csp-dl-row {
  display: contents;
}
.csp-dl dt {
  color: var(--text-3, #888);
  white-space: nowrap;
}
.csp-dl dd {
  margin: 0;
  color: var(--text);
  word-break: break-word;
}
.csp-dl dd a {
  color: var(--primary);
  text-decoration: none;
}
.csp-dl dd a:hover { text-decoration: underline; }
</style>
