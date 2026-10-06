// 阅读偏好 store：字号/行高/字体/背景（owner_id 维度预留；本地兜底）
import { defineStore } from 'pinia'

const KEY = 'aikmap.reading'

const DEFAULTS = {
  size: 16,          // 14–20
  lineHeight: 1.8,   // 1.5–2.1
  font: 'sans',      // sans|serif
  bg: 'default',     // default|sepia|green|dark
  width: 'auto'      // 页面宽度：standard(68ch) | wide(92ch) | auto(100%)
}

function load() {
  try {
    return { ...DEFAULTS, ...JSON.parse(localStorage.getItem(KEY) || '{}') }
  } catch (_) {
    return { ...DEFAULTS }
  }
}

export const useReadingStore = defineStore('reading', {
  state: () => load(),
  actions: {
    save() {
      localStorage.setItem(KEY, JSON.stringify({
        size: this.size, lineHeight: this.lineHeight, font: this.font, bg: this.bg, width: this.width
      }))
    },
    setWidth(w) {
      this.width = w
      this.save()
    },
    setSize(v) {
      this.size = Math.min(20, Math.max(14, v))
      this.save()
    },
    setLineHeight(v) {
      this.lineHeight = Math.min(2.1, Math.max(1.5, v))
      this.save()
    },
    setFont(f) {
      this.font = f
      this.save()
    },
    setBg(b) {
      this.bg = b
      this.save()
    },
    // 供阅读视图计算 CSS 变量
    cssVars() {
      return {
        '--reading-size': this.size + 'px',
        '--reading-lineheight': this.lineHeight,
        '--reading-font-family': this.font === 'serif' ? 'var(--font-serif)' : 'var(--font)',
        '--reading-bg-active': this.bgVars().bg,
        '--reading-text-active': this.bgVars().text,
        '--reading-maxw': this.widthMax()
      }
    },
    // 页面宽度：standard→68ch / wide→92ch / auto→100%（自适应容器）
    widthMax() {
      if (this.width === 'standard') return '68ch'
      if (this.width === 'wide') return '92ch'
      return '100%'
    },
    bgVars() {
      const root = getComputedStyle(document.documentElement)
      const map = {
        default: [root.getPropertyValue('--reading-bg').trim(), root.getPropertyValue('--reading-text').trim()],
        sepia: [root.getPropertyValue('--reading-bg-sepia').trim(), root.getPropertyValue('--reading-text').trim()],
        green: [root.getPropertyValue('--reading-bg-green').trim(), root.getPropertyValue('--reading-text').trim()],
        dark: [root.getPropertyValue('--reading-bg-dark').trim(), root.getPropertyValue('--reading-text-dark').trim()]
      }
      const [bg, text] = map[this.bg] || map.default
      return { bg, text }
    }
  }
})
