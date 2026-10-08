<template>
  <div class="lsw" :class="[`lsw--${mode}`]">
    <button
      type="button"
      class="lsw-btn"
      :title="$t('界面语言（当前：{cur}）', { cur: currentLabel })"
      :aria-label="$t('界面语言')"
      :aria-expanded="open ? 'true' : 'false'"
      @click="open = !open"
    >
      <span class="lsw-ico" aria-hidden="true">文</span>
      <span v-if="mode === 'inline'" class="lsw-cur">{{  currentLabel  }}</span>
    </button>

    <div v-if="open" class="lsw-menu">
      <button
        v-for="l in LOCALES"
        :key="l.value"
        type="button"
        class="lsw-item"
        :class="{ on: l.value === locale }"
        @click="pick(l.value)"
      >
        <span class="lsw-item-t">{{  $t(l.labelKey)  }}</span>
        <span class="lsw-item-code">{{  l.value === 'zh-CN' ? 'ZH' : 'EN'  }}</span>
      </button>
    </div>
  </div>
</template>

<script setup>
/**
 * LangSwitch —— 界面语言切换器（框架级，全站生效）。
 *
 * 两种用法（与 ThemeSwitch 同一套范式）：
 *  - mode="topbar"：控制台顶栏（AppShell）内联使用。
 *  - mode="dock"：公开博客页右上角悬浮版 —— 公开页（/blog、/p/:token）**脱离应用外壳**，
 *    不渲染 AppShell，因此顶栏那颗按钮覆盖不到访客；公开页必须自己挂一颗，
 *    否则「切换对访客无效」。
 *
 * 生效范围：locale 是 Vue ref，切换后所有 $t(...) 重新求值，无需刷新。
 * ⚠️ 但 t() 在 <script setup> 顶层被 `const x = t(...)` 捕获时只求值一次、不会跟随切换，
 *    那种写法必须改成 computed（已修 InboxView.inboxDir）。新增文案时留意。
 *
 * 未翻译的键回退中文原文（渐进翻译），故中文用户无副作用。
 */
import { ref, computed } from 'vue'
import { t, locale, setLocale, LOCALES } from '@/i18n'

const props = defineProps({
  mode: { type: String, default: 'topbar' }, // topbar | inline | dock
})

const open = ref(false)

const currentLabel = computed(() =>
  locale.value === 'en-US' ? t('settings.langEn') : t('settings.langZh')
)

function pick(v) {
  setLocale(v)
  open.value = false
}
</script>

<style scoped>
.lsw {
  position: relative;
  display: inline-block;
}
.lsw-btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 5px 9px;
  border: 1px solid var(--border, #e3e6eb);
  border-radius: 8px;
  background: var(--surface, #fff);
  color: var(--text, #1f2328);
  font-size: 13px;
  line-height: 1.2;
  cursor: pointer;
}
.lsw-btn:hover {
  background: var(--primary-soft, #eef2ff);
}
.lsw-ico {
  font-size: 13px;
  font-weight: 600;
  opacity: 0.75;
}
.lsw-menu {
  position: absolute;
  top: calc(100% + 6px);
  right: 0;
  z-index: 300;
  min-width: 132px;
  padding: 6px;
  background: var(--surface, #fff);
  border: 1px solid var(--border, #e3e6eb);
  border-radius: var(--radius-sm, 8px);
  box-shadow: 0 10px 30px rgba(22, 24, 43, 0.12);
}
.lsw-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  width: 100%;
  padding: 7px 10px;
  border: none;
  border-radius: 6px;
  background: transparent;
  color: var(--text, #1f2328);
  font-size: 13px;
  text-align: left;
  cursor: pointer;
}
.lsw-item:hover {
  background: var(--primary-soft, #eef2ff);
}
.lsw-item.on {
  font-weight: 600;
  color: var(--primary, #2f6feb);
}
.lsw-item-code {
  font-size: 11px;
  opacity: 0.6;
}

/* 公开页悬浮版：与 ThemeSwitch dock 同位，主题自带页头时不重复出现（宿主按需渲染） */
.lsw--dock {
  position: fixed;
  top: 10px;
  right: 12px;
  z-index: 9998;
}
.lsw--dock .lsw-btn {
  background: rgba(255, 255, 255, 0.92);
  border-color: rgba(0, 0, 0, 0.12);
  color: #1f2937;
  box-shadow: 0 2px 10px rgba(0, 0, 0, 0.12);
  backdrop-filter: blur(6px);
  opacity: 1;
}
</style>
