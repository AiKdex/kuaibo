<template>
  <div v-if="allowVisitorSwitch" ref="root" class="thsw" :class="[`thsw--${mode}`]">
    <button
      type="button"
      class="thsw-btn"
      :title="$t('切换主题（当前：{title}）', { title: current ? current.title : $t('默认') })"
      :aria-expanded="open"
      @click="open = !open"
    >
      <span class="thsw-ico" aria-hidden="true">◑</span>
      <span class="thsw-cur">{{  current ? current.title : $t('主题')  }}</span>
      <span class="thsw-caret" :class="{ up: open }">▾</span>
    </button>

    <div v-if="open" class="thsw-menu">
      <div class="thsw-menu-head">{{  $t('选择主题')  }}</div>
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
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { listThemes, setActiveTheme, activeThemeId, readOverride, allowVisitorSwitch } from '@/themes'
import { isAuthed, requestJSON } from '@/api'
import { t } from '@/i18n'

const props = defineProps({
  mode: { type: String, default: 'inline' }, // inline | dock
})

const root = ref(null)
const open = ref(false)
const saving = ref(false)
const themes = computed(() => listThemes())
const activeId = computed(() => activeThemeId.value)
const overridden = computed(() => !!readOverride() && open.value)
const isAdmin = computed(() => isAuthed())
const current = computed(() => themes.value.find(t => t.id === activeId.value) || null)

// 点击切换（click-to-toggle）：关闭改由「点击外部 / ESC」触发，
// 不再用 mouseleave——否则鼠标从按钮移到下拉菜单会先离开容器盒子（菜单是 absolute，不占容器高度）
// 导致菜单瞬间闭合、选不到主题。
function onDocClick(e) {
  if (open.value && root.value && !root.value.contains(e.target)) {
    open.value = false
  }
}
function onKey(e) {
  if (e.key === 'Escape') open.value = false
}
onMounted(() => {
  document.addEventListener('click', onDocClick)
  document.addEventListener('keydown', onKey)
})
onBeforeUnmount(() => {
  document.removeEventListener('click', onDocClick)
  document.removeEventListener('keydown', onKey)
})

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
  top: calc(100% + 4px);
  right: 0;
  z-index: 9999;
  min-width: 236px;
  max-height: 64vh;
  overflow: auto;
  padding: 6px;
  border-radius: 14px;
  border: 1px solid rgba(15, 23, 42, 0.08);
  background: #fff;
  color: #1f2937;
  box-shadow: 0 12px 36px rgba(15, 23, 42, 0.16);
  animation: thsw-pop 0.14s ease;
}

/* 透明桥接：连接按钮与菜单，消除间隙造成的「鼠标移出即闭合」死区 */
.thsw-menu::before {
  content: '';
  position: absolute;
  top: -8px;
  left: 0;
  right: 0;
  height: 8px;
}

@keyframes thsw-pop {
  from { opacity: 0; transform: translateY(-4px); }
  to   { opacity: 1; transform: translateY(0); }
}

.thsw-menu-head {
  padding: 4px 10px 6px;
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.04em;
  color: #94a3b8;
  text-transform: uppercase;
}

.thsw-item {
  position: relative;
  display: flex;
  flex-direction: column;
  width: 100%;
  padding: 8px 10px;
  border: 0;
  border-radius: 9px;
  background: transparent;
  text-align: left;
  cursor: pointer;
  color: inherit;
  font-size: 13px;
  line-height: 1.35;
  transition: background 0.12s ease, transform 0.12s ease;
}

.thsw-item:hover {
  background: #f1f5f9;
}

.thsw-item:active {
  transform: scale(0.99);
}

.thsw-item.on {
  background: rgba(13, 122, 106, 0.1);
  color: #0d7a6a;
}

.thsw-item.on::after {
  content: '✓';
  position: absolute;
  top: 9px;
  right: 10px;
  font-size: 12px;
  color: #0d7a6a;
}

.thsw-item-t {
  display: block;
  font-weight: 600;
}

.thsw-item-d {
  display: block;
  margin-top: 2px;
  font-size: 11px;
  font-weight: 400;
  font-style: normal;
  color: #64748b;
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
