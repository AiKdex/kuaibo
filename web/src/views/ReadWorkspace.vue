<template>
  <div class="read-workspace">
    <!-- 多标签栏 -->
    <TabBar
      :tabs="tabs"
      :active-id="activeId"
      :dirty-map="dirtyMap"
      @activate="onActivate"
      @close="onClose"
      @close-others="onCloseOthers"
      @close-all="onCloseAll"
    />

    <!-- 多实例阅读视图：v-show 保持各自 DOM/滚动/编辑/播放状态 -->
    <div v-for="t in tabs" :key="t.fileId" class="read-pane" :class="{ active: t.fileId === activeId }" v-show="t.fileId === activeId">
      <ReadView
        :file-id="t.fileId"
        :hl="(hlMap[t.fileId] && hlMap[t.fileId].chunk) || ''"
        :hl-q="(hlMap[t.fileId] && hlMap[t.fileId].q) || ''"
        @dirty-change="(d) => tabsStore.setDirty(t.fileId, d)"
      />
    </div>

    <!-- 无标签空态 -->
    <div v-if="tabs.length === 0" class="read-empty">
      <AikIcon name="book" :size="34" />
      <p>{{  $t('没有打开的文档')  }}</p>
      <span>{{  $t('在文件库、知识库或搜索中点击文档即可在此打开')  }}</span>
    </div>

    <!-- 关闭未保存 tab 的确认 -->
    <Transition name="fade">
      <div v-if="confirmClose" class="modal-mask" @click.self="confirmClose = null">
        <div class="modal modal-sm">
          <h3>{{  $t('有未保存的修改')  }}</h3>
          <p class="modal-desc">「{{  confirmClose.name  }}{{  $t('」有未保存的修改，关闭后将丢失这些改动。')  }}</p>
          <div class="modal-actions">
            <button class="btn" @click="confirmClose = null">{{  $t('取消')  }}</button>
            <button class="btn btn-danger-ghost" @click="doClose(confirmClose.fileId)">{{  $t('放弃修改并关闭')  }}</button>
          </div>
        </div>
      </div>
    </Transition>
  </div>
</template>

<script setup>
import { ref, computed, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import TabBar from '@/components/TabBar.vue'
import ReadView from '@/views/ReadView.vue'
import AikIcon from '@/components/AikIcon.vue'
import { useTabsStore } from '@/stores/tabs'
import { getFile } from '@/api'
import { t } from '@/i18n'

const route = useRoute()
const router = useRouter()
const tabsStore = useTabsStore()

const tabs = computed(() => tabsStore.tabs)
const activeId = computed(() => tabsStore.activeId)
const dirtyMap = computed(() => tabsStore.dirtyMap)

const confirmClose = ref(null) // { fileId, name }
// 溯源定位：fileId → { chunk, q }（来自搜索页 ?hl= & ?q=）
const hlMap = ref({})

// 路由 → 打开 tab（文件列表/知识库/搜索/分享 打开都走这里）
watch(
  () => route.params.id,
  async (id) => {
    if (!id) return
    if (route.query.hl) {
      hlMap.value = { ...hlMap.value, [id]: { chunk: String(route.query.hl), q: route.query.q ? String(route.query.q) : '' } }
    }
    const isNew = tabsStore.open(id)
    if (isNew) {
      try {
        const f = await getFile(id)
        const t = tabsStore.tabs.find(x => x.fileId === id)
        if (t) { t.name = f.name; t.kind = f.kind }
      } catch (_) { /* 名称待加载，不影响打开 */ }
    }
  },
  { immediate: true }
)

// 激活 tab → 同步路由（AI 上下文跟随激活 tab）
function onActivate(fileId) {
  tabsStore.activate(fileId)
  if (route.params.id !== fileId) {
    router.replace({ path: `/read/${fileId}` })
  }
}

// 关闭：dirty 先确认
function onClose(fileId) {
  if (tabsStore.isDirty(fileId)) {
    const t = tabsStore.tabs.find(x => x.fileId === fileId)
    confirmClose.value = { fileId, name: t?.name || t('文档') }
    return
  }
  doClose(fileId)
}
function doClose(fileId) {
  confirmClose.value = null
  tabsStore.close(fileId)
  const next = tabsStore.activeId
  if (next) router.replace({ path: `/read/${next}` })
  else if (route.path.startsWith('/read/')) router.push('/files')
}
function onCloseOthers(fileId) {
  for (const t of [...tabsStore.tabs]) {
    if (t.fileId !== fileId) onClose(t.fileId)
  }
}
function onCloseAll() {
  for (const t of [...tabsStore.tabs]) onClose(t.fileId)
}
</script>

<style scoped>
.read-workspace {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
  background: var(--bg);
}
.read-pane {
  flex: 1;
  min-height: 0;
  overflow: auto;
  background: var(--bg);
}
.read-pane :deep(.read-view) {
  min-height: 100%;
}
.read-empty {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 8px;
  color: var(--text-3);
}
.read-empty p {
  margin: 4px 0 0;
  font-size: var(--fs-medium);
  color: var(--text-2);
}
.read-empty span {
  font-size: var(--fs-small);
}

.modal-mask {
  position: fixed;
  inset: 0;
  background: rgba(15, 20, 35, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: var(--z-modal, 1000);
}
.modal {
  width: 420px;
  max-width: calc(100vw - 48px);
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  box-shadow: var(--shadow-lg);
  padding: 20px 22px;
}
.modal h3 {
  margin: 0 0 10px;
  font-size: 16px;
  color: var(--text);
}
.modal-desc {
  margin: 0 0 18px;
  font-size: 13px;
  color: var(--text-3);
  line-height: 1.7;
}
.modal-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
.btn-danger-ghost {
  color: var(--danger);
  border-color: var(--danger);
  background: transparent;
}
.fade-enter-active, .fade-leave-active { transition: opacity 0.15s ease; }
.fade-enter-from, .fade-leave-to { opacity: 0; }
</style>
