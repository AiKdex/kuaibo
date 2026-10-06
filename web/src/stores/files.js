// 文件库 store：当前目录、文件列表、选中项、加载状态
import { defineStore } from 'pinia'
import * as api from '@/api'

export const useFilesStore = defineStore('files', {
  state: () => ({
    currentParent: '',        // 当前目录 id（'' = 根）
    currentDir: null,         // 当前目录元数据
    items: [],                // 当前目录列表
    tree: [],                 // 目录树（全量 dir）
    loading: false,
    error: '',
    selected: [],             // 选中的 file id 列表
    viewMode: localStorage.getItem('aikmap.viewmode') || 'grid', // grid|list
    breadcrumbs: [],          // [{id,name}]
    draggingOver: false       // 拖拽悬停标志（全局遮罩）
  }),
  getters: {
    selectedItems: (s) => s.items.filter((f) => s.selected.includes(f.id))
  },
  actions: {
    async loadTree() {
      try {
        this.tree = await api.getTree()
      } catch (e) {
        this.error = e.message
      }
    },
    async openDir(parentId, dirMeta = null) {
      this.currentParent = parentId
      this.currentDir = dirMeta
      this.selected = []
      this.loading = true
      this.error = ''
      try {
        this.items = await api.listFiles(parentId)
        await this.buildBreadcrumbs(parentId)
      } catch (e) {
        this.error = e.message
      } finally {
        this.loading = false
      }
    },
    async buildBreadcrumbs(parentId) {
      const crumb = [{ id: '', name: '我的空间' }]
      if (!parentId) {
        this.breadcrumbs = crumb
        return
      }
      // 从树中回溯（阶段 1 简化：树已全量加载）
      const byId = {}
      for (const d of this.tree) byId[d.id] = d
      let cur = byId[parentId]
      const chain = []
      while (cur) {
        chain.unshift(cur)
        cur = cur.parent_id ? byId[cur.parent_id] : null
      }
      this.breadcrumbs = [...crumb, ...chain.map((d) => ({ id: d.id, name: d.name }))]
    },
    async refresh() {
      return this.openDir(this.currentParent, this.currentDir)
    },
    toggleSelect(id) {
      const i = this.selected.indexOf(id)
      if (i >= 0) this.selected.splice(i, 1)
      else this.selected.push(id)
    },
    // 单击行 = 单选选中（KodExplorer 式，低调高亮）
    selectOne(id) {
      if (this.selected.length === 1 && this.selected[0] === id) return
      this.selected = [id]
    },
    clearSelect() {
      this.selected = []
    },
    setViewMode(mode) {
      this.viewMode = mode
      localStorage.setItem('aikmap.viewmode', mode)
    },
    async upload(files, onProgress) {
      const results = await api.uploadFiles(files, this.currentParent, onProgress)
      await Promise.all([this.refresh(), this.loadTree()])
      return results
    },
    async mkdir(name) {
      await api.mkdir(name, this.currentParent)
      await this.refresh()
      await this.loadTree()
    },
    async remove(ids) {
      for (const id of ids) await api.deleteFile(id)
      this.selected = []
      await this.refresh()
      await this.loadTree()
    }
  }
})
