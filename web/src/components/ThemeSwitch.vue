<template>
  <div v-if="allowVisitorSwitch" class="thsw" :class="[`thsw--${mode}`]" @mouseleave="open = false">
    <button
      type="button"
      class="thsw-btn"
      :title="$t('切换主题（当前：{title}）', { title: current ? current.title : $t('默认') })"
      @click="open = !open"
    >
      <span class="thsw-ico" aria-hidden="true">◑</span>
      <span class="thsw-cur">{{  current ? current.title : $t('主题')  }}</span>
      <span class="thsw-caret" :class="{ up: open }">▾</span>
    </button>

    <div v-if="open" class="thsw-menu">
      <button
        v-for="t in themes"
        :key="t.id"
        type="button"
        class="thsw-item"
        :class="{ on: t.id === activeId }"
        @click="pick(t.id)"
      >
        <span class="thsw-item-t">{{  t.title  }}</span>
        <em class="thsw-item-d">{{  t.desc  }}</em>
      </button>
      <div v-if="overridden" class="thsw-foot">
        <button type="button" class="thsw-link" @click="pick('')">{{  $t('恢复站点默认')  }}</button>
        <button v-if="isAdmin" type="button" class="thsw-link primary" @click="saveAsSite">
          {{  saving ? $t('保存中…') : $t('设为站点主题')  }}
        </button>
      </div>
    </div>
  </div>
</template>

<script setup>
/**
 * ThemeSwitch —— 公开博客页主题切换器。
 *
 * 两种用法：
 *  - mode="inline"（默认）：主题在自己的页头 nav 里渲染，紧邻 RSS 链接。
 *    主题需在 manifest 声明 ui: { themeSwitcher: true }，宿主便不再渲染悬浮版。
 *  - mode="dock"：宿主（BlogView）在公开页右上角渲染的悬浮兜底版，
 *    用于未自带切换器的第三方主题，保证任何主题都能在公开页换肤。
 *
 * 选择结果写入 localStorage（aiklog.theme.override），优先级高于站点设置 blog.theme；
 * 站长登录时可一键「设为站点主题」（PUT /admin/settings）变成全站默认。
 */
import { ref, computed } from 'vue'
import { listThemes, setActiveTheme, activeThemeId, readOverride, allowVisitorSwitch } from '@/themes'
import { isAuthed, requestJSON } from '@/api'
import { t } from '@/i18n'

const props = defineProps({
  mode: { type: String, default: 'inline' }, // inline | dock
})

const open = ref(false)
const saving = ref(false)
const themes = computed(() => listThemes())
const activeId = computed(() => activeThemeId.value)
const overridden = computed(() => !!readOverride() && open.value)
const isAdmin = computed(() => isAuthed())
const current = computed(() => themes.value.find(t => t.id === activeId.value) || null)

function pick(id) {
  setActiveTheme(id)
  open.value = false
}

async function saveAsSite() {
  if (!isAdmin.value || saving.value) return
  saving.value = true
  try {
    await requestJSON('/admin/settings', {
      method: 'PUT',
      body: JSON.stringify({ key: 'blog.theme', value: activeId.value || 'aiklog' }),
    })
    open.value = false
  } catch (e) {
    console.warn(t('[ThemeSwitch] 保存站点主题失败：'), e?.message || e)
  } finally {
    saving.value = false
  }
}
</script>

<style scoped>
.thsw {
  position: relative;
  display: inline-block;
  font: inherit;
  font-size: 12px;
  line-height: 1;
  color: inherit;
}

.thsw-btn {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 4px 8px;
  border: 1px solid currentColor;
  border-radius: 999px;
  background: transparent;
  color: inherit;
  font: inherit;
  font-size: 12px;
  cursor: pointer;
  opacity: 0.85;
  transition: opacity 0.15s ease;
  white-space: nowrap;
}

.thsw-btn:hover {
  opacity: 1;
}

.thsw-ico {
  font-size: 11px;
}

.thsw-caret {
  font-size: 9px;
  transition: transform 0.15s ease;
}

.thsw-caret.up {
  transform: rotate(180deg);
}

.thsw-menu {
  position: absolute;
  top: calc(100% + 6px);
  right: 0;
  z-index: 9999;
  min-width: 208px;
  max-height: 60vh;
  overflow: auto;
  padding: 4px;
  border-radius: 10px;
  border: 1px solid rgba(0, 0, 0, 0.1);
  background: #fff;
  color: #1f2937;
  box-shadow: 0 8px 28px rgba(0, 0, 0, 0.14);
}

.thsw-item {
  display: block;
  width: 100%;
  padding: 7px 9px;
  border: 0;
  border-radius: 7px;
  background: transparent;
  text-align: left;
  cursor: pointer;
  color: inherit;
  font-size: 12px;
  line-height: 1.4;
}

.thsw-item:hover {
  background: rgba(0, 0, 0, 0.05);
}

.thsw-item.on {
  background: rgba(13, 122, 106, 0.1);
  color: #0d7a6a;
  font-weight: 600;
}

.thsw-item-t {
  display: block;
}

.thsw-item-d {
  display: block;
  margin-top: 2px;
  font-size: 11px;
  font-style: normal;
  opacity: 0.6;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.thsw-foot {
  display: flex;
  gap: 6px;
  padding: 6px 4px 2px;
  margin-top: 2px;
  border-top: 1px solid rgba(0, 0, 0, 0.08);
}

.thsw-link {
  flex: 1;
  padding: 5px 6px;
  border: 1px solid rgba(0, 0, 0, 0.12);
  border-radius: 6px;
  background: #fff;
  color: #4b5563;
  font-size: 11px;
  cursor: pointer;
}

.thsw-link:hover {
  border-color: #0d7a6a;
  color: #0d7a6a;
}

.thsw-link.primary {
  border-color: #0d7a6a;
  background: #0d7a6a;
  color: #fff;
}

/* 悬浮兜底版：固定右上角，浅色胶囊，深浅主题上都可读 */
.thsw--dock {
  position: fixed;
  top: 10px;
  right: 12px;
  z-index: 9998;
  color: #1f2937;
}

.thsw--dock .thsw-btn {
  background: rgba(255, 255, 255, 0.92);
  border-color: rgba(0, 0, 0, 0.12);
  color: #1f2937;
  box-shadow: 0 2px 10px rgba(0, 0, 0, 0.12);
  backdrop-filter: blur(6px);
  opacity: 1;
}

.thsw--dock .thsw-btn:hover {
  border-color: #0d7a6a;
  color: #0d7a6a;
}

@media (max-width: 640px) {
  .thsw-cur,
  .thsw-item-d {
    display: none;
  }
  .thsw--dock {
    top: 8px;
    right: 8px;
  }
}
</style>
