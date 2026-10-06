// 多标签页 store：阅读工作区（浏览器/邮箱式多文档）
// 每个 tab 对应一个已打开文件（fileId 即唯一 key），独立状态由 ReadView 多实例 v-show 保持
import { defineStore } from 'pinia'

export const useTabsStore = defineStore('tabs', {
  state: () => ({
    tabs: [],          // [{ fileId, name, kind }]
    activeId: '',      // 当前激活 tab 的 fileId
    dirtyMap: {}       // { [fileId]: true } 未保存编辑标记
  }),
  getters: {
    activeTab: (s) => s.tabs.find(t => t.fileId === s.activeId) || null
  },
  actions: {
    // 打开文件：已存在 → 激活；否则新建 tab（kind 供图标用）
    open(fileId, name, kind) {
      if (!fileId) return
      const exist = this.tabs.find(t => t.fileId === fileId)
      if (exist) {
        if (name && exist.name !== name) exist.name = name
        this.activeId = fileId
        return false
      }
      this.tabs.push({ fileId, name: name || '文档', kind: kind || '' })
      this.activeId = fileId
      return true
    },
    // 关闭 tab（已确认无未保存/已处理 dirty），激活相邻 tab
    close(fileId) {
      const i = this.tabs.findIndex(t => t.fileId === fileId)
      if (i < 0) return
      this.tabs.splice(i, 1)
      delete this.dirtyMap[fileId]
      if (this.activeId === fileId) {
        const next = this.tabs[i] || this.tabs[i - 1]
        this.activeId = next ? next.fileId : ''
      }
    },
    activate(fileId) {
      if (this.tabs.some(t => t.fileId === fileId)) this.activeId = fileId
    },
    setDirty(fileId, dirty) {
      if (dirty) this.dirtyMap[fileId] = true
      else delete this.dirtyMap[fileId]
    },
    isDirty(fileId) {
      return !!this.dirtyMap[fileId]
    },
    // 关闭所有 tab（可选：保留当前激活的）
    closeAll(exceptActive = false) {
      if (exceptActive && this.activeId) {
        this.tabs = this.tabs.filter(t => t.fileId === this.activeId)
        this.dirtyMap = {}
        return
      }
      this.tabs = []
      this.dirtyMap = {}
      this.activeId = ''
    }
  }
})
