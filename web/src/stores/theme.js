// 主题 store：4 套主题切换（仅覆盖 CSS 变量，组件零改动）
import { defineStore } from 'pinia'

export const THEMES = [
  { id: 'indigo', name: '靛蓝' },
  { id: 'aurora', name: '极光蓝' },
  { id: 'sunset', name: '落日橙' },
  { id: 'dark', name: '暗色' }
]

const KEY = 'aikmap.theme'

export const useThemeStore = defineStore('theme', {
  state: () => ({
    current: localStorage.getItem(KEY) || 'indigo'
  }),
  actions: {
    apply() {
      document.documentElement.setAttribute('data-theme', this.current)
    },
    set(id) {
      if (THEMES.some((t) => t.id === id)) {
        this.current = id
        localStorage.setItem(KEY, id)
        this.apply()
      }
    },
    toggle() {
      const idx = THEMES.findIndex((t) => t.id === this.current)
      this.set(THEMES[(idx + 1) % THEMES.length].id)
    }
  }
})
