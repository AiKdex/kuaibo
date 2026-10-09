<template>
  <div v-if="allowVisitorSwitch" ref="root" class="thsw" :class="[`thsw--${mode}`]">
    <button
      type="button"
      class="thsw-btn"
      :title="$t('切换主题（当前：{title}）', { title: current ? current.title : $t('默认') })"
      :aria-expanded="open"
      @click="toggle"
    >
      <svg
        class="thsw-ico"
        viewBox="0 0 24 24"
        width="14"
        height="14"
        fill="none"
        stroke="currentColor"
        stroke-width="1.8"
        stroke-linecap="round"
        stroke-linejoin="round"
        aria-hidden="true"
      >
        <path d="M12 3a9 9 0 1 0 0 18c1.1 0 1.7-1 1.4-2-.3-.9.3-2 1.3-2H17a4 4 0 0 0 4-4c0-4.4-4-8-9-8Z" />
        <circle cx="7.5" cy="11" r="1.1" fill="currentColor" stroke="none" />
        <circle cx="11" cy="7.5" r="1.1" fill="currentColor" stroke="none" />
        <circle cx="15.5" cy="8.5" r="1.1" fill="currentColor" stroke="none" />
      </svg>
      <span class="thsw-cur">{{ current ? current.title : $t('主题') }}</span>
      <span class="thsw-caret" :class="{ up: open }">▾</span>
    </button>

    <transition name="thsw-pop">
      <Teleport to="body">
        <div
          v-if="open"
          ref="menu"
          class="thsw-menu"
          :style="menuStyle"
        >
          <div class="thsw-menu-head">
            <span class="thsw-menu-title">{{ $t('选择主题') }}</span>
            <span class="thsw-menu-count">{{ themes.length }} {{ $t('套') }}</span>
          </div>

          <div class="thsw-grid">
            <button
              v-for="t in themes"
              :key="t.id"
              type="button"
              class="thsw-item"
              :class="{ on: t.id === activeId }"
              :style="{ '--accent': accentOf(t.id) }"
              @click="pick(t.id)"
            >
              <span class="thsw-bar" aria-hidden="true"></span>
              <span class="thsw-item-body">
                <span class="thsw-item-top">
                  <span class="thsw-item-t">{{ t.title }}</span>
                  <span class="thsw-check" aria-hidden="true">✓</span>
                </span>
                <em class="thsw-item-d">{{ t.desc }}</em>
              </span>
            </button>
          </div>

          <div v-if="overridden" class="thsw-foot">
            <button type="button" class="thsw-link" @click="pick('')">{{ $t('恢复站点默认') }}</button>
            <button v-if="isAdmin" type="button" class="thsw-link primary" @click="saveAsSite">
              {{ saving ? $t('保存中…') : $t('设为站点主题') }}
            </button>
          </div>
        </div>
      </Teleport>
    </transition>
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
import { ref, computed, reactive, nextTick, onMounted, onBeforeUnmount } from 'vue'
import { listThemes, setActiveTheme, activeThemeId, readOverride, allowVisitorSwitch, ensureTheme, applyInstalledThemes, isThemeInstalled, themeCatalog } from '@/themes'
import { isAuthed, requestJSON, publicBlogThemes } from '@/api'
import { t } from '@/i18n'

const props = defineProps({
  mode: { type: String, default: 'inline' }, // inline | dock
})

const root = ref(null)
const menu = ref(null)
const open = ref(false)
const saving = ref(false)
// 菜单 Teleport 到 body 后用 fixed 定位，避免被主题页头的 overflow/CSS 作用域裁剪或改写
const menuPos = reactive({ top: 0, left: 0, width: 250 })
const menuStyle = computed(() => ({
  position: 'fixed',
  top: menuPos.top + 'px',
  left: menuPos.left + 'px',
  width: menuPos.width + 'px',
}))
function placeMenu() {
  const r = root.value?.getBoundingClientRect()
  if (!r) return
  const w = window.innerWidth <= 640 ? 220 : 250
  let left = r.right - w
  if (left < 8) left = 8
  menuPos.width = w
  menuPos.left = left
  menuPos.top = r.bottom + 6
}
// 主题列表 = 已注册（default/aiklog/已加载懒主题）+ 已安装但尚未加载的懒主题
// （后者用 catalog 元数据补齐展示，点击时 ensureTheme 拉起 chunk 再激活）。
const themes = computed(() => {
  const arr = listThemes()
  const seen = new Set(arr.map((x) => x.id))
  for (const c of themeCatalog()) {
    if (!seen.has(c.id) && isThemeInstalled(c.id)) {
      arr.push({ id: c.id, title: c.title, desc: c.desc, version: c.version, entry: null, lazy: true })
    }
  }
  return arr
})
const activeId = computed(() => activeThemeId.value)
const overridden = computed(() => !!readOverride() && open.value)
const isAdmin = computed(() => isAuthed())
const current = computed(() => themes.value.find(t => t.id === activeId.value) || null)

// 内置主题协调配色（低饱和高级色）；未注册 id 用哈希兜底，保证任何主题都有色。
const ACCENTS = {
  default: '#0d7a6a', aiklog: '#0d7a6a',
  aurora: '#6366f1', butterfly: '#ec4899', chenxi: '#f59e0b',
  jaded: '#10b981', minimal: '#64748b', paper: '#a16207',
  parchment: '#b45309', verdant: '#22c55e', zircon: '#0ea5e9',
  brutal: '#ef4444', emforum: '#8b5cf6', huajian: '#f43f5e',
  aiknav: '#0d9488', docs: '#2563eb', elevated: '#475569',
  qiuzhi: '#0891b2', zhicang: '#7c3aed', clawblog: '#f97316',
}
const FALLBACK = ['#0d7a6a', '#2563eb', '#7c3aed', '#db2777', '#ea580c', '#0891b2', '#65a30d', '#9333ea']
function accentOf(id) {
  if (ACCENTS[id]) return ACCENTS[id]
  let h = 0
  for (let i = 0; i < id.length; i++) h = (h * 31 + id.charCodeAt(i)) >>> 0
  return FALLBACK[h % FALLBACK.length]
}

// 点击切换（click-to-toggle）：关闭改由「点击外部 / ESC」触发，
// 不再用 mouseleave——否则鼠标从按钮移到下拉菜单会先离开容器盒子（菜单是 absolute，不占容器高度）
// 导致菜单瞬间闭合、选不到主题。
function onDocClick(e) {
  if (open.value && root.value && !root.value.contains(e.target) && !(menu.value && menu.value.contains(e.target))) {
    open.value = false
  }
}
function onKey(e) {
  if (e.key === 'Escape') open.value = false
}
function toggle() {
  open.value = !open.value
  if (open.value) nextTick(placeMenu)
}
onMounted(() => {
  document.addEventListener('click', onDocClick)
  document.addEventListener('keydown', onKey)
  window.addEventListener('resize', placeMenu)
  // 兜底拉一次已安装主题列表（BlogView/BlogPostView 通常已拉；本组件独立挂载场景补齐）
  publicBlogThemes().then((r) => applyInstalledThemes((r.items || []).map((x) => x.id))).catch(() => { /* 失败降级为全量可见 */ })
})
onBeforeUnmount(() => {
  document.removeEventListener('click', onDocClick)
  document.removeEventListener('keydown', onKey)
  window.removeEventListener('resize', placeMenu)
})

async function pick(id) {
  open.value = false
  if (!id) { setActiveTheme(''); return }
  // 懒加载主题：先拉起 chunk（index.js 副作用完成注册），成功后再激活；失败回落站点默认
  const ok = await ensureTheme(id)
  setActiveTheme(ok ? id : '')
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
  gap: 5px;
  padding: 5px 10px;
  border: 1px solid color-mix(in srgb, currentColor 35%, transparent);
  border-radius: 999px;
  background: color-mix(in srgb, currentColor 6%, transparent);
  color: inherit;
  font: inherit;
  font-size: 12px;
  cursor: pointer;
  opacity: 0.9;
  transition: opacity 0.15s ease, background 0.15s ease, border-color 0.15s ease, transform 0.1s ease;
  white-space: nowrap;
}

.thsw-btn:hover {
  opacity: 1;
  background: color-mix(in srgb, currentColor 12%, transparent);
}

.thsw-btn:active {
  transform: scale(0.97);
}

.thsw-ico {
  flex: 0 0 auto;
}

.thsw-caret {
  font-size: 9px;
  transition: transform 0.18s ease;
}

.thsw-caret.up {
  transform: rotate(180deg);
}

.thsw-menu {
  position: fixed; /* Teleport 到 body 后由内联 style 精确定位，避免被主题页头裁剪 */
  z-index: 9999;
  width: 250px;
  max-height: 64vh;
  overflow: auto;
  padding: 8px;
  border-radius: 18px;
  border: 1px solid rgba(15, 23, 42, 0.08);
  background: #f5f7fb;
  color: #1f2937;
  box-shadow: 0 18px 48px rgba(15, 23, 42, 0.18), 0 2px 6px rgba(15, 23, 42, 0.08);
  /* 中文在非整数像素/合成层下易发虚：整数渲染 + 灰度平滑 */
  -webkit-font-smoothing: antialiased;
  -moz-osx-font-smoothing: grayscale;
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

.thsw-menu-head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  padding: 2px 10px 8px;
  margin-bottom: 4px;
  border-bottom: 1px solid rgba(15, 23, 42, 0.06);
}

.thsw-menu-title {
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.08em;
  color: #94a3b8;
}

.thsw-menu-count {
  font-size: 10px;
  color: #cbd5e1;
}

.thsw-grid {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.thsw-item {
  position: relative;
  display: block;
  width: 100%;
  overflow: hidden;
  padding: 11px 12px 11px 15px;
  border: 1px solid rgba(15, 23, 42, 0.07);
  border-radius: 12px;
  background: #ffffff;
  text-align: left;
  cursor: pointer;
  color: inherit;
  font-size: 13px;
  line-height: 1.35;
  box-shadow: 0 1px 2px rgba(15, 23, 42, 0.04);
  transition: border-color 0.16s ease, box-shadow 0.16s ease, transform 0.16s ease, background 0.16s ease;
}

/* 左侧 accent 竖条：标识主题色，选中时点亮 */
.thsw-bar {
  position: absolute;
  left: 0;
  top: 0;
  bottom: 0;
  width: 3px;
  background: var(--accent);
  opacity: 0.32;
  transition: opacity 0.16s ease;
}

.thsw-item:hover {
  border-color: var(--accent);
  box-shadow: 0 6px 16px rgba(15, 23, 42, 0.1);
  transform: translateY(-1px);
}

.thsw-item.on {
  border-color: color-mix(in srgb, var(--accent) 45%, transparent);
  background: color-mix(in srgb, var(--accent) 7%, #ffffff);
}

.thsw-item.on .thsw-bar {
  opacity: 1;
}

.thsw-item-body {
  display: block;
}

.thsw-item-top {
  display: flex;
  align-items: center;
  gap: 8px;
}

.thsw-item-t {
  font-weight: 600;
  color: #111827;
}

.thsw-check {
  margin-left: auto;
  font-size: 12px;
  font-weight: 700;
  color: var(--accent);
  opacity: 0;
  transition: opacity 0.16s ease;
}

.thsw-item.on .thsw-check {
  opacity: 1;
}

.thsw-item-d {
  display: block;
  margin-top: 3px;
  font-size: 11px;
  font-style: normal;
  color: #6b7280;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.thsw-foot {
  display: flex;
  gap: 6px;
  padding: 8px 4px 2px;
  margin-top: 6px;
  border-top: 1px solid rgba(15, 23, 42, 0.06);
}

.thsw-link {
  flex: 1;
  padding: 6px 6px;
  border: 1px solid rgba(15, 23, 42, 0.12);
  border-radius: 9px;
  background: #fff;
  color: #4b5563;
  font-size: 11px;
  cursor: pointer;
  transition: border-color 0.15s ease, color 0.15s ease;
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

.thsw-link.primary:hover {
  background: #0b6557;
  border-color: #0b6557;
}

/* 弹层出现动画 */
.thsw-pop-enter-active,
.thsw-pop-leave-active {
  transition: opacity 0.15s ease, transform 0.15s ease;
}
.thsw-pop-enter-from,
.thsw-pop-leave-to {
  opacity: 0;
  transform: translateY(-4px);
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
  border-color: rgba(15, 23, 42, 0.12);
  color: #1f2937;
  box-shadow: 0 2px 10px rgba(15, 23, 42, 0.12);
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
  .thsw-menu {
    width: 220px;
  }
  .thsw--dock {
    top: 8px;
    right: 8px;
  }
}
</style>
