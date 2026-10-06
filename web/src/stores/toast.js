// 全局轻提示 store：视口顶部居中，滑动出现/消失
import { defineStore } from 'pinia'

let seed = 0

export const useToastStore = defineStore('toast', {
  state: () => ({
    items: [] // [{ id, text }]
  }),
  actions: {
    push(text, duration = 2600) {
      const id = ++seed
      this.items.push({ id, text })
      setTimeout(() => {
        const i = this.items.findIndex((t) => t.id === id)
        if (i >= 0) this.items.splice(i, 1)
      }, duration)
      return id
    },
    success(text) {
      return this.push(text)
    },
    error(text) {
      return this.push(text)
    }
  }
})
