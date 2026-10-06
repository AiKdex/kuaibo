<template>
  <div class="app-shell" :class="{ 'ai-collapsed': !aiOpen }">
    <!-- 左侧：深色侧栏（视觉真值结构，可折叠） -->
    <aside class="sidebar" :class="{ collapsed: sidebarCollapsed }">
      <div class="brand">
        <div class="brand-mark"><span>K</span><i class="brand-dot"></i></div>
        <div v-if="!sidebarCollapsed" class="brand-text">
          <h1>{{  $t('爱库录')  }}</h1>
          <p>{{  $t('AiKlog · AI 知识库博客')  }}</p>
        </div>
      </div>

      <nav class="nav">
        <button class="nav-pin" :title="sidebarCollapsed ? $t('nav.pin.expand') : $t('nav.pin.collapse')" @click="sidebarCollapsed = !sidebarCollapsed">
          <AikIcon :name="sidebarCollapsed ? 'menu' : 'chevronLeft'" :size="15" />
        </button>

        <!-- 内容（文件为中心：写作/文件库/草稿/回收站） -->
        <div class="nav-group">
          <div class="nav-group-head" @click="toggleGroup('content')">
            <AikIcon name="chevronDown" :size="11" class="arw" :class="{ closed: !groups.content }" />
            {{  t('nav.group.content')  }}
          </div>
          <div v-show="groups.content" class="nav-group-body">
            <RouterLink to="/write" class="nav-item" active-class="active" :title="t('nav.writeTip')">
              <AikIcon name="edit" :size="15" /><span v-if="!sidebarCollapsed">{{  t('nav.write')  }}</span>
            </RouterLink>
            <div
              class="tree-root"
              :class="{ active: isFilesRoute && !files.currentParent, 'drop-over': rootOver }"
              @click="openRoot"
              @dragover="onRootOver($event)"
              @dragleave="rootOver = false"
              @drop="onRootDrop($event)"
            >
              <AikIcon name="database" :size="15" />
              <span v-if="!sidebarCollapsed" class="tree-root-name">{{  t('nav.files')  }}</span>
            </div>
            <div v-if="!sidebarCollapsed" class="dir-tree">
              <DirTreeNode
                v-for="n in treeRoots"
                :key="n.id"
                :node="n"
                :depth="1"
                :children="treeChildren"
                :expanded="expandedSet"
                :current="files.currentParent"
                @toggle="toggleTree"
                @open="openDir"
                @move="onTreeMove"
              />
            </div>
            <RouterLink to="/drafts" class="nav-item-sub" active-class="active" :title="t('nav.draftsTip')">
              <AikIcon name="fileText" :size="12" /><span v-if="!sidebarCollapsed">{{  t('nav.storage.drafts')  }}</span>
            </RouterLink>
            <RouterLink to="/trash" class="nav-item-sub" active-class="active" :title="t('nav.trashTip')">
              <AikIcon name="trash" :size="12" /><span v-if="!sidebarCollapsed">{{  t('nav.storage.trash')  }}</span>
            </RouterLink>
            <RouterLink to="/inbox" class="nav-item" active-class="active" :title="t('nav.inboxTip')">
              <AikIcon name="bell" :size="15" /><span v-if="!sidebarCollapsed">{{  t('nav.kb.inbox')  }}</span>
            </RouterLink>
            <RouterLink to="/mentions" class="nav-item" active-class="active" :title="t('nav.mentionsTip')">
              <AikIcon name="user" :size="15" /><span v-if="!sidebarCollapsed">{{  t('nav.mentions')  }}</span>
            </RouterLink>
          </div>
        </div>

        <!-- 博客（发布/展示为中心） -->
        <div class="nav-group">
          <div class="nav-group-head" @click="toggleGroup('blog')">
            <AikIcon name="chevronDown" :size="11" class="arw" :class="{ closed: !groups.blog }" />
            {{  t('nav.group.blog')  }}
          </div>
          <div v-show="groups.blog" class="nav-group-body">
            <RouterLink to="/manage/blog" class="nav-item" active-class="active" :title="t('nav.blogManageTip')">
              <AikIcon name="book" :size="15" /><span v-if="!sidebarCollapsed">{{  t('nav.blogManage')  }}</span>
            </RouterLink>
            <a class="nav-item" href="/blog" target="_blank" rel="noopener" :title="t('nav.staticBlogTip')">
              <AikIcon name="globe" :size="15" /><span v-if="!sidebarCollapsed">{{  t('nav.staticBlog')  }}</span>
            </a>
            <RouterLink to="/shares" class="nav-item" active-class="active" :title="t('nav.sharesTip')">
              <AikIcon name="share" :size="15" /><span v-if="!sidebarCollapsed">{{  t('nav.publish.shares')  }}</span>
            </RouterLink>
            <RouterLink to="/analytics" class="nav-item" active-class="active" :title="t('nav.readStatsTip')">
              <AikIcon name="eye" :size="15" /><span v-if="!sidebarCollapsed">{{  t('nav.readStats')  }}</span>
            </RouterLink>
          </div>
        </div>

        <!-- 知识 · AI -->
        <div class="nav-group">
          <div class="nav-group-head" @click="toggleGroup('kb')">
            <AikIcon name="chevronDown" :size="11" class="arw" :class="{ closed: !groups.kb }" />
            {{  t('nav.group.kbAi')  }}
          </div>
          <div v-show="groups.kb" class="nav-group-body">
            <RouterLink to="/kb" class="nav-item" active-class="active" :title="t('nav.kbOverTip')">
              <AikIcon name="layers" :size="15" /><span v-if="!sidebarCollapsed">{{  t('nav.kbOver')  }}</span>
            </RouterLink>
            <RouterLink to="/tags" class="nav-item" active-class="active" :title="t('nav.kb.tagsTip')">
              <AikIcon name="tag" :size="15" /><span v-if="!sidebarCollapsed">{{  t('nav.kb.tags')  }}</span>
            </RouterLink>
            <RouterLink to="/digest" class="nav-item" active-class="active" :title="t('nav.digestTip')">
              <AikIcon name="clock" :size="15" /><span v-if="!sidebarCollapsed">{{  t('nav.digest')  }}</span>
            </RouterLink>
            <RouterLink to="/prompts" class="nav-item" active-class="active" :title="t('nav.promptsTip')">
              <AikIcon name="sparkles" :size="15" /><span v-if="!sidebarCollapsed">{{  t('nav.prompts')  }}</span>
            </RouterLink>
            <RouterLink to="/manage/webdav" class="nav-item" active-class="active" :title="t('nav.extMountTip')">
              <AikIcon name="link" :size="15" /><span v-if="!sidebarCollapsed">{{  t('nav.extMount')  }}</span>
            </RouterLink>
            <div v-if="!sidebarCollapsed" class="tag-tree">
              <div class="tag-tree-head" @click="toggleTagTree">
                <AikIcon name="chevronRight" :size="11" class="tag-tree-arw" :class="{ open: tagTreeOpen }" />
                <AikIcon name="tag" :size="13" />
                {{  t('nav.kb.tagTree')  }}
                <span v-if="tagItems.length" class="tag-tree-count">{{  tagItems.length  }}</span>
              </div>
              <div v-show="tagTreeOpen" class="tag-tree-body">
                <div v-if="tagLoading" class="tag-tree-hint">{{  t('nav.kb.tagTreeLoading')  }}</div>
                <div v-else-if="!tagItems.length" class="tag-tree-hint">{{  t('nav.tagTreeEmptyShort')  }}</div>
                <template v-else>
                  <div
                    v-for="t in tagTreeRows"
                    :key="t.id"
                    class="tag-tree-item"
                    :class="{ active: isTagActive(t.id) }"
                    :style="{ paddingLeft: 8 + t.depth * 14 + 'px' }"
                    :title="$t('{path}（{count} 个文件）', { path: t.path, count: t.count })"
                    @click="goTag(t.id)"
                  >
                    <AikIcon name="tag" :size="11" /><span class="tag-tree-name">{{  t.name  }}</span>
                    <span v-if="t.count" class="tag-tree-num">{{  t.count  }}</span>
                  </div>
                </template>
              </div>
            </div>
          </div>
        </div>

        <!-- 系统 -->
        <div class="nav-group">
          <div class="nav-group-head" @click="toggleGroup('sys')">
            <AikIcon name="chevronDown" :size="11" class="arw" :class="{ closed: !groups.sys }" />
            {{  t('nav.group.system')  }}
          </div>
          <div v-show="groups.sys" class="nav-group-body">
            <RouterLink to="/apps" class="nav-item" active-class="active" :title="t('nav.appsTip')">
              <AikIcon name="grid" :size="15" /><span v-if="!sidebarCollapsed">{{  t('nav.apps')  }}</span>
            </RouterLink>
            <RouterLink to="/manage/sites" class="nav-item" active-class="active" :title="t('nav.sitesTip')">
              <AikIcon name="home" :size="15" /><span v-if="!sidebarCollapsed">{{  t('nav.sites')  }}</span>
            </RouterLink>
            <RouterLink to="/manage/im-bind" class="nav-item" active-class="active" :title="t('nav.imBindTip')">
              <AikIcon name="link" :size="15" /><span v-if="!sidebarCollapsed">{{  t('nav.imBind')  }}</span>
            </RouterLink>
            <RouterLink to="/manage/skills" class="nav-item" active-class="active" :title="t('nav.skillsTip')">
              <AikIcon name="sparkles" :size="15" /><span v-if="!sidebarCollapsed">{{  t('nav.skills')  }}</span>
            </RouterLink>
            <RouterLink v-if="caps.org" to="/org" class="nav-item" active-class="active" :title="t('nav.orgTreeTip')">
              <AikIcon name="grid" :size="15" /><span v-if="!sidebarCollapsed">{{  t('nav.orgTree')  }}</span>
            </RouterLink>
            <RouterLink v-if="caps.family" to="/family" class="nav-item" active-class="active" :title="t('nav.familyTip')">
              <AikIcon name="heart" :size="15" /><span v-if="!sidebarCollapsed">{{  t('nav.family')  }}</span>
            </RouterLink>
            <RouterLink v-if="caps.csInbox" to="/cs" class="nav-item" active-class="active" :title="t('cs.title')">
              <AikIcon name="chat" :size="15" /><span v-if="!sidebarCollapsed">{{  t('cs.title')  }}</span>
            </RouterLink>
            <RouterLink v-if="caps.store" to="/manage/store" class="nav-item" active-class="active" :title="t('store.admin')">
              <AikIcon name="cart" :size="15" /><span v-if="!sidebarCollapsed">{{  t('store.admin')  }}</span>
            </RouterLink>
            <RouterLink to="/knowledge-graph" class="nav-item" active-class="active" :title="t('kg.title')">
              <AikIcon name="grid" :size="15" /><span v-if="!sidebarCollapsed">{{  t('kg.title')  }}</span>
            </RouterLink>
            <RouterLink to="/manage/users" class="nav-item" active-class="active" :title="t('nav.usersTip')">
              <AikIcon name="user" :size="15" /><span v-if="!sidebarCollapsed">{{  t('nav.system.users')  }}</span>
            </RouterLink>
            <RouterLink to="/settings" class="nav-item" :class="{ active: route.path === '/settings' && !route.query.tab }" :title="t('nav.settingsTip')">
              <AikIcon name="settings" :size="15" /><span v-if="!sidebarCollapsed">{{  t('nav.system.settings')  }}</span>
            </RouterLink>
          </div>
        </div>
      </nav>

      <div class="sidebar-foot">
        <div class="kb-info">
          <div class="kb-avatar">{{ $t('本') }}</div>
          <div v-if="!sidebarCollapsed" class="kb-text">
            <span class="kb-name">{{  t('nav.foot.selfhost')  }}</span>
            <span class="kb-sub">{{  t('nav.foot.phase1')  }}</span>
          </div>
          <!-- 消息通知入口：侧栏底部个人区（铃铛 + 未读角标；脉冲仅新增时一次） -->
          <button class="kb-bell" :class="{ 'has-unread': unread > 0, open: notifOpen }" :title="t('nav.foot.notif')" @click.stop="notifOpen = !notifOpen">
            <AikIcon name="bell" :size="16" />
            <span v-if="unread > 0" class="kb-badge">{{  unread > 99 ? '99+' : unread  }}</span>
          </button>
        </div>
      </div>

      <!-- 通知抽屉：从侧栏底部个人区上方弹出 -->
      <transition name="notif-fade">
        <div v-if="notifOpen" class="notif-panel" @click.stop>
          <div class="notif-head">
            {{  t('nav.foot.notif')  }}
            <button class="notif-close" :title="t('nav.foot.close')" @click="notifOpen = false"><AikIcon name="close" :size="14" /></button>
          </div>
          <div class="notif-body">
            <template v-if="notifItems.length">
              <div class="notif-item"
                v-for="n in notifItems"
                :key="n.id"
                :class="{ unread: !n.read_at }"
                @click="goNotif(n)"
              >
                <AikIcon :name="notifIcon(n.type)" :size="14" class="notif-ic" />
                <div class="notif-main">
                  <div class="notif-title">
                    <span>{{  n.title  }}</span>
                    <span v-if="!n.read_at" class="notif-dot"></span>
                  </div>
                  <div v-if="n.message" class="notif-msg">{{  n.message  }}</div>
                  <div class="notif-time">{{  fmtNotifTime(n.created_at)  }}</div>
                </div>
              </div>
              <div class="notif-foot">
                <button class="notif-allread" :disabled="notifBusy" @click="readAll">
                  {{  notifBusy ? $t('处理中…') : $t('全部标为已读')  }}
                </button>
              </div>
            </template>
            <div v-else class="notif-empty">
              <AikIcon name="bell" :size="22" />
              <p>{{ $t('暂无通知') }}</p>
              <p class="notif-tip">{{ $t('导入导出终态 / 系统事件会在这里提醒你') }}</p>
            </div>
          </div>
        </div>
      </transition>
    </aside>

    <!-- 主区 -->
    <div class="main-body">
      <!-- 顶栏：面包屑 + 搜索同一行（阅读页同样保留，侧栏全局化） -->
      <header class="topbar">
        <div class="crumb" v-if="isFilesView">
          <template v-for="(c, i) in files.breadcrumbs" :key="c.id || 'root'">
            <span v-if="i > 0" class="crumb-sep">/</span>
            <button
              class="crumb-btn"
              :class="{ current: i === files.breadcrumbs.length - 1 }"
              @click="openDir(c.id)"
            >{{  c.name  }}</button>
          </template>
        </div>
        <div v-else class="crumb"><span class="crumb-btn current">{{  pageTitle  }}</span></div>

        <div class="search">
          <AikIcon name="search" :size="14" />
          <input
            v-model="quickQuery"
            :placeholder="$t('搜索文件、内容…（Enter 跳转）')"
            @keydown.enter="goSearch"
          />
          <span class="kbd">Enter</span>
        </div>

        <div class="tb-actions">
          <a class="btn btn-ghost site-link" href="/#/blog?view=public" target="_blank" rel="noopener" :title="$t('nav.visitSiteTip')">
            <AikIcon name="globe" :size="15" /><span class="site-link-text">{{ $t('nav.visitSite') }}</span>
          </a>
          <div class="theme-switcher">
            <button
              v-for="t in THEMES"
              :key="t.id"
              class="theme-dot"
              :class="{ active: theme.current === t.id }"
              :title="t.name"
              @click="theme.set(t.id)"
            >
              <span :class="'dot dot-' + t.id"></span>
            </button>
          </div>
          <LangSwitch mode="topbar" />
          <button class="btn btn-ghost icon-btn ai-toggle" :class="{ on: aiOpen }" :title="$t('AI 助手（点击隐藏/展开）')" @click="aiOpen = !aiOpen">
            <span class="ai-text-btn">AI</span>
          </button>
          <div class="user-avatar" :title="$t('管理员')" @click.stop="userMenuOpen = !userMenuOpen">{{ $t('管') }}</div>
          <div v-if="userMenuOpen" class="user-menu" @click.stop>
            <button class="user-menu-item" @click="openPasswordModal">
              <AikIcon name="lock" :size="13" /><span>{{ $t('修改密码') }}</span>
            </button>
            <button class="user-menu-item danger" @click="doLogout">
              <AikIcon name="logout" :size="13" /><span>{{ $t('退出登录') }}</span>
            </button>
          </div>
        </div>
      </header>

      <main class="content">
        <router-view />
      </main>

      <!-- 底部状态栏（KodExplorer 式：项目数 / 选中数，低调不抖动） -->
      <footer v-if="isFilesView" class="status-bar">
        <span class="st-item">{{  files.items.length  }} {{ $t('个项目') }}</span>
        <span v-if="files.selected.length" class="st-item st-strong">
          {{ $t('已选') }} {{  files.selected.length  }} {{ $t('项') }}{{  selectedSizeLabel  }}
        </span>
        <span v-else class="st-item st-hint">{{ $t('单击选中 · 双击打开 · 右键更多操作') }}</span>
      </footer>
    </div>

    <!-- 右侧：AI 助手面板（全局可用，可收起；左缘可自由拖宽，持久化） -->
    <aside v-if="aiOpen" class="ai-panel">
      <div class="ai-resize" :title="$t('拖拽调整宽度')" @mousedown.prevent="startAiResize($event)"></div>
      <div class="ai-head">
        <div class="ai-badge"><span class="ai-text-btn">AI</span></div>
        <div class="ai-title">
          <h3>{{ $t('AI 助手') }}</h3>
          <p>{{ $t('基于你的知识库回答') }}</p>
        </div>
        <button class="ai-close" :title="$t('隐藏 AI 助手')" @click="aiOpen = false"><AikIcon name="close" :size="14" /></button>
      </div>
      <div class="ai-convbar">
        <select v-model="currentConvId" class="ai-conv-select" :disabled="aiConvLoading" @change="switchConv">
          <option value="" disabled>{{ $t('选择对话话题…') }}</option>
          <option v-for="c in aiConvs" :key="c.id" :value="c.id">{{  c.title  }}</option>
        </select>
        <button class="ai-conv-btn" :title="$t('新建对话')" @click="newConv"><AikIcon name="plus" :size="14" /></button>
        <button class="ai-conv-btn" :title="$t('删除当前对话')" :disabled="!currentConvId" @click="delConv"><AikIcon name="trash" :size="14" /></button>
      </div>
      <div class="ai-body" ref="aiBodyRef">
        <template v-if="aiMsgs.length === 0">
          <div class="ai-placeholder">
            <AikIcon name="sparkles" :size="28" />
            <p>{{ $t('对话 Agent 已接入') }}</p>
            <p class="muted-3">{{ $t('问它找文件、查内容，工具自动检索你的知识库') }}</p>
          </div>
          <div class="ai-sug">
            <button v-for="s in AI_SUGGESTIONS" :key="s" class="ai-sug-item" @click="askSug(s)">{{  s  }}</button>
          </div>
        </template>
        <div v-for="(m, i) in aiMsgs" :key="i" class="ai-msg" :class="m.role">
          <div class="ai-msg-bubble">{{  m.content  }}</div>
          <div v-if="m.tools && m.tools.length" class="ai-tool-card">
            <div v-for="(t, ti) in m.tools" :key="ti" class="ai-tool">
              <div class="ai-tool-head">
                <AikIcon name="search" :size="12" />
                {{  toolSummary(t)  }}
              </div>
              <div v-if="toolExpanded(t)" class="ai-tool-expanded">{{  toolExpanded(t)  }}</div>
              <div class="ai-tool-files">
                <button
                  v-for="(f, fi) in toolFiles(t)"
                  :key="fi"
                  class="ai-tool-file"
                  @click="openAiFile(f)"
                >
                  <AikIcon :name="f.kind === 'dir' ? 'folder' : 'fileText'" :size="13" />
                  <span class="tf-name">{{  f.name  }}</span>
                  <span class="tf-meta">{{  f.kind === 'dir' ? $t('文件夹') : fmtSize(f.size)  }}</span>
                </button>
                <div v-if="toolPreview(t)" class="ai-tool-preview">{{  toolPreview(t)  }}</div>
                <div v-if="!toolFiles(t).length && !toolPreview(t)" class="ai-tool-empty">{{ $t('未找到匹配的文件') }}</div>
              </div>
            </div>
          </div>
        </div>
        <div v-if="aiBusy" class="ai-msg assistant">
          <div class="ai-msg-bubble thinking"><span class="spinner"></span>{{ $t('思考中…') }}</div>
        </div>
      </div>
      <div class="ai-input">
        <input
          v-model="aiInput"
          :placeholder="$t('问 AiKMap：找文件、查内容…')"
          :disabled="aiBusy"
          @keydown.enter="sendAi"
        />
        <button class="btn btn-sm ai-send" :disabled="aiBusy || !aiInput.trim()" @click="sendAi">{{ $t('发送') }}</button>
      </div>
    </aside>

    <!-- 修改密码弹窗（安全加固） -->
    <div v-if="pwdModal" class="modal-mask" @click.self="pwdModal = false">
      <div class="modal-box pwd-box">
        <h3 class="pwd-title">{{ $t('修改密码') }}</h3>
        <div class="pwd-field">
          <label>{{ $t('当前密码') }}</label>
          <input v-model="pwdOld" type="password" autocomplete="current-password" @keyup.enter="doChangePwd" />
        </div>
        <div class="pwd-field">
          <label>{{ $t('新密码（至少 8 位）') }}</label>
          <input v-model="pwdNew" type="password" autocomplete="new-password" @keyup.enter="doChangePwd" />
        </div>
        <div class="pwd-field">
          <label>{{ $t('确认新密码') }}</label>
          <input v-model="pwdNew2" type="password" autocomplete="new-password" @keyup.enter="doChangePwd" />
        </div>
        <p v-if="pwdErr" class="pwd-err">{{  pwdErr  }}</p>
        <div class="pwd-actions">
          <button class="btn" @click="pwdModal = false">{{ $t('取消') }}</button>
          <button class="btn btn-primary" :disabled="pwdBusy" @click="doChangePwd">
            {{  pwdBusy ? $t('提交中…') : $t('确认修改')  }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { t as i18t } from '@/i18n'
import { ref, reactive, computed, watch, onMounted, onUnmounted, nextTick } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AikIcon from './AikIcon.vue'
import LangSwitch from './LangSwitch.vue'
import DirTreeNode from './DirTreeNode.vue'
import { useThemeStore, THEMES } from '@/stores/theme'
import { useFilesStore } from '@/stores/files'
import { useToastStore } from '@/stores/toast'
import { t } from '@/i18n'
import { aiChat, aiConversations, aiCreateConversation, aiDeleteConversation, aiConversationMessages, authMe, setAuthFlag } from '@/api'
import * as api from '@/api'

const route = useRoute()
const router = useRouter()
const theme = useThemeStore()
const files = useFilesStore()
const toastStore = useToastStore()

const isFilesView = computed(() => route.path.startsWith('/files'))

// —— 用户区（安全加固：修改密码 / 退出登录） ——
const userMenuOpen = ref(false)
const pwdModal = ref(false)
const pwdOld = ref('')
const pwdNew = ref('')
const pwdNew2 = ref('')
const pwdErr = ref('')
const pwdBusy = ref(false)
function openPasswordModal() {
  userMenuOpen.value = false
  pwdOld.value = ''
  pwdNew.value = ''
  pwdNew2.value = ''
  pwdErr.value = ''
  pwdModal.value = true
}
async function doChangePwd() {
  if (pwdBusy.value) return
  if (pwdNew.value.length < 8) {
    pwdErr.value = t('新密码至少 8 位')
    return
  }
  if (pwdNew.value !== pwdNew2.value) {
    pwdErr.value = i18t('两次输入的新密码不一致')
    return
  }
  pwdBusy.value = true
  pwdErr.value = ''
  try {
    await api.authPassword(pwdOld.value, pwdNew.value)
    pwdModal.value = false
    toastStore.push(t('密码已修改'))
  } catch (e) {
    pwdErr.value = e.message || t('修改失败')
  } finally {
    pwdBusy.value = false
  }
}
async function doLogout() {
  userMenuOpen.value = false
  try {
    await api.authLogout()
  } catch (_) {
    /* 忽略网络错误，仍本地登出 */
  }
  api.clearToken()
  api.clearAuthFlag()
  router.replace('/desk')
}
// 点击页面其他区域关闭用户菜单/通知面板
function onDocClick() {
  userMenuOpen.value = false
  if (notifOpen.value) notifOpen.value = false
}
onMounted(() => {
  document.addEventListener('click', onDocClick)
  // Cookie 会话恢复：刷新后 isAuthed() 仅看 localStorage/sessionStorage，
  // HttpOnly cookie 仍在时用 /auth/me 探活并写回登录标记，否则会把后台误判为游客
  // （点「博客」会落到公开主题，看起来像跳出管理壳）。
  if (!api.isAuthed()) {
    authMe().then(() => setAuthFlag()).catch(() => {})
  }
})

// —— 侧栏标签树（组织·聚合层）：标签作为检索/过滤维度，点击直达文件列表按标签筛选 ——
const tagTreeOpen = ref(false)
const tagItems = ref([])
const tagLoading = ref(false)
let tagLoaded = false
async function toggleTagTree() {
  tagTreeOpen.value = !tagTreeOpen.value
  if (tagTreeOpen.value && !tagLoaded) {
    tagLoading.value = true
    try {
      tagItems.value = (await api.listTags()) || []
      tagLoaded = true
    } catch (e) {
      tagItems.value = []
    } finally {
      tagLoading.value = false
    }
  }
}
const tagTreeRows = computed(() => {
  const byParent = {}
  tagItems.value.forEach((t) => {
    const k = t.parent_id || ''
    ;(byParent[k] = byParent[k] || []).push(t)
  })
  const rows = []
  const walk = (parentKey, depth) => {
    const kids = byParent[parentKey] || []
    kids.sort((a, b) => (a.name || '').localeCompare(b.name || '', 'zh'))
    kids.forEach((t) => {
      rows.push({ ...t, depth })
      walk(t.id, depth + 1)
    })
  }
  walk('', 0)
  return rows
})
function goTag(id) {
  if (route.query.tag === id) return
  router.push({ path: '/files', query: { tag: id } })
}
function isTagActive(id) {
  return route.query.tag === id
}
const pageTitle = computed(() => {
  if (route.path.startsWith('/search')) return i18t('智能检索')
  if (route.path.startsWith('/settings')) return i18t('设置')
  if (route.path.startsWith('/read/')) return i18t('阅读')
  return i18t('我的空间')
})
const selectedSizeLabel = computed(() => {
  if (!files.selected.length) return ''
  let size = 0
  for (const id of files.selected) {
    const f = files.items.find((x) => x.id === id)
    if (f && f.kind === 'file') size += f.size
  }
  if (!size) return ''
  if (size < 1024) return `（${size} B）`
  if (size < 1024 * 1024) return `（${(size / 1024).toFixed(1)} KB）`
  return `（${(size / 1024 / 1024).toFixed(1)} MB）`
})

// ---- 侧栏存储目录树（资源管理器式） ----
const treeChildren = computed(() => {
  const map = {}
  for (const d of files.tree) {
    const pid = d.parent_id || ''
    ;(map[pid] = map[pid] || []).push(d)
  }
  for (const k of Object.keys(map)) {
    map[k].sort((a, b) => a.name.localeCompare(b.name, 'zh'))
  }
  return map
})
const treeRoots = computed(() => treeChildren.value[''] || [])
const expandedSet = ref(new Set())

function toggleTree(id) {
  const s = new Set(expandedSet.value)
  if (s.has(id)) s.delete(id)
  else s.add(id)
  expandedSet.value = s
}
function openDir(id) {
  router.push('/files')
  files.openDir(id)
}
function openRoot() {
  router.push('/files')
  files.openDir('')
}
// 目录树拖放：文件/文件夹行拖到树节点 → 移动
const rootOver = ref(false)
function onRootOver(e) {
  if (e.dataTransfer && Array.from(e.dataTransfer.types).includes('text/plain')) {
    e.preventDefault()
    e.dataTransfer.dropEffect = 'move'
    rootOver.value = true
  }
}
function onRootDrop(e) {
  e.preventDefault()
  rootOver.value = false
  const id = e.dataTransfer ? e.dataTransfer.getData('text/plain') : ''
  if (!id) return
  onTreeMove({ id, target: '' })
}
async function onTreeMove({ id, target }) {
  try {
    await api.moveFile(id, target)
    await files.refresh()
    await files.loadTree()
    toastStore.push(t('已移动到目标文件夹'))
  } catch (e) {
    toastStore.push(t('移动失败：') + e.message)
  }
}

// 目录树自动关联：无论从列表/面包屑/树节点进入目录，展开其祖先链
watch(
  () => files.currentParent,
  (id) => {
    if (!id) return
    const byId = {}
    for (const d of files.tree) byId[d.id] = d
    const s = new Set(expandedSet.value)
    let cur = byId[id]
    while (cur && cur.parent_id) {
      s.add(cur.parent_id)
      cur = byId[cur.parent_id]
    }
    expandedSet.value = s
  }
)

const sidebarCollapsed = ref(false)
const aiOpen = ref(true)

// 消息通知：采集/导入导出等终态 → 铃铛未读红点（新增脉冲）+ 面板列表（点击跳转/标记已读）
const notifOpen = ref(false)
const unread = ref(0)
const notifItems = ref([])
const notifBusy = ref(false)
async function refreshUnread() {
  try {
    const d = await api.notificationsUnread()
    unread.value = d.count || 0
  } catch (_) {
    /* 轮询失败静默 */
  }
}
async function loadNotifs() {
  try {
    const d = await api.listNotifications(50, false)
    notifItems.value = d.items || []
    unread.value = d.unread_count || 0
  } catch (_) {
    notifItems.value = []
  }
}
function notifIcon(type) {
  if (type === 'collect') return 'book'
  if (type === 'impex') return 'upload'
  if (type === 'index') return 'layers'
  if (type === 'subscription') return 'star'
  if (type === 'mention') return 'user'
  if (type === 'comment') return 'chat'
  return 'bell'
}
function goNotif(n) {
  if (!n.read_at) {
    api.notificationsRead(n.id).catch(() => {})
    n.read_at = Date.now()
    if (unread.value > 0) unread.value -= 1
  }
  notifOpen.value = false
  if (n.link) {
    const p = n.link.replace(/^\/?#\/?/, '')
    router.push('/' + p)
  }
}
async function readAll() {
  if (notifBusy.value) return
  notifBusy.value = true
  try {
    await api.notificationsReadAll()
    notifItems.value.forEach((n) => { n.read_at = n.read_at || Date.now() })
    unread.value = 0
  } catch (_) { /* ignore */ } finally {
    notifBusy.value = false
  }
}
function fmtNotifTime(ts) {
  const d = new Date(ts * 1000)
  const now = new Date()
  const diff = Math.floor((now - d) / 60000)
  if (diff < 1) return t('刚刚')
  if (diff < 60) return diff + t(' 分钟前')
  if (diff < 1440) return Math.floor(diff / 60) + t(' 小时前')
  return `${d.getMonth() + 1}-${d.getDate()} ${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
}
watch(notifOpen, (open) => {
  if (open) loadNotifs()
})
// F2 修复：未读数轮询定时器必须持有句柄并在卸载时清理 ——
// 此前 setInterval 从不 clearInterval，登出后仍每 30s 打接口，401 反复触发 handle401 重定向。
let _unreadTimer = null
// 可安装能力启用态（应用中心装了才亮）：family/org 默认按已启用渲染，探测到未安装即隐藏，
// 接口失败保持现状（宁可多显示入口，不因探测失败藏功能）。
const caps = reactive({ family: true, org: true, csInbox: true, store: true })
async function loadCaps() {
  try {
    const d = await api.publicCapabilities()
    caps.family = d.family !== false
    caps.org = d.org !== false
    caps.csInbox = d['cs.inbox'] !== false
    caps.store = d.store !== false
  } catch (_) { /* 探测失败保持默认 */ }
}
onMounted(() => {
  loadCaps()
  refreshUnread()
  // 30s 轮询未读数（轻量接口，仅查 count）
  if (!_unreadTimer) _unreadTimer = setInterval(refreshUnread, 30000)
})
onUnmounted(() => {
  if (_unreadTimer) {
    clearInterval(_unreadTimer)
    _unreadTimer = null
  }
})

// AI 面板宽度：可拖拽自由调宽（无上限），持久化
const AI_MIN_W = 260
const aiWidth = ref(loadAiWidth())
function loadAiWidth() {
  try {
    const v = parseInt(localStorage.getItem('aikmap.aiwidth') || '280', 10)
    return Number.isFinite(v) && v >= AI_MIN_W ? v : 280
  } catch (_) {
    return 280
  }
}
function startAiResize(e) {
  const startX = e.clientX
  const startW = aiWidth.value
  const onMove = (ev) => {
    // 面板在右：往左拖 → 变宽；无上限自由拖
    aiWidth.value = Math.max(AI_MIN_W, startW - (ev.clientX - startX))
    document.documentElement.style.setProperty('--ai-w', aiWidth.value + 'px')
  }
  const onUp = () => {
    localStorage.setItem('aikmap.aiwidth', String(aiWidth.value))
    document.body.style.cursor = ''
    document.body.style.userSelect = ''
    document.removeEventListener('mousemove', onMove)
    document.removeEventListener('mouseup', onUp)
  }
  document.body.style.cursor = 'ew-resize'
  document.body.style.userSelect = 'none'
  document.addEventListener('mousemove', onMove)
  document.addEventListener('mouseup', onUp)
}
onMounted(() => {
  document.documentElement.style.setProperty('--ai-w', aiWidth.value + 'px')
})

// 进入阅读页时自动收起 AI 面板（保证正文宽度），可手动再开
watch(
  () => route.path,
  (p) => {
    if (p.startsWith('/read/')) aiOpen.value = false
  },
  { immediate: true }
)
const quickQuery = ref('')

// ---- AI 对话（Agent + search_files 工具链）----
const aiInput = ref('')
const aiBusy = ref(false)
const aiMsgs = ref([])
const aiBodyRef = ref(null)
const AI_SUGGESTIONS = [
  t('找一下 upload 相关的文件'),
  t('最近上传了哪些文件？'),
  t('有哪些图片文件？')
]

function pushMsg(m) {
  aiMsgs.value.push(m)
  nextTick(scrollAi)
}
function scrollAi() {
  if (aiBodyRef.value) aiBodyRef.value.scrollTop = aiBodyRef.value.scrollHeight
}
// 当前浏览上下文（阅读页文件 / 文件页目录），让 Agent 知道"你在看什么"
function aiCtx() {
  const ctx = {}
  if (route.name === 'read' && route.params.id) ctx.file_id = route.params.id
  if (route.name === 'files' && route.query.parent) ctx.dir_id = route.query.parent
  return ctx
}
async function sendAi() {
  const text = aiInput.value.trim()
  if (!text || aiBusy.value) return
  aiInput.value = ''
  pushMsg({ role: 'user', content: text })
  aiBusy.value = true
  try {
    // 后端为权威：只发当前消息 + 会话 id（历史由后端从库加载）
    const res = await aiChat([{ role: 'user', content: text }], aiCtx(), currentConvId.value)
    pushMsg({ role: 'assistant', content: res.reply, tools: res.tools || [] })
    if (res.conversation_id && res.conversation_id !== currentConvId.value) {
      currentConvId.value = res.conversation_id
      loadConvs()
    } else {
      refreshConvTitle()
    }
  } catch (e) {
    pushMsg({ role: 'assistant', content: t('出错了：') + (e.message || e) })
  } finally {
    aiBusy.value = false
  }
}
function askSug(s) {
  aiInput.value = s
  sendAi()
}

// ---- 多话题会话管理 ----
const aiConvs = ref([])
const currentConvId = ref('')
const aiConvLoading = ref(false)

onMounted(loadConvs)

async function loadConvs() {
  try {
    const d = await aiConversations()
    aiConvs.value = d.conversations || []
    if (!currentConvId.value && aiConvs.value.length) {
      currentConvId.value = aiConvs.value[0].id
      switchConv()
    }
  } catch (_) {
    /* 会话列表加载失败静默 */
  }
}

async function newConv() {
  aiConvLoading.value = true
  try {
    const d = await aiCreateConversation()
    currentConvId.value = d.id
    aiMsgs.value = []
    await loadConvs()
  } catch (_) {
    /* ignore */
  } finally {
    aiConvLoading.value = false
  }
}

async function delConv() {
  if (!currentConvId.value) return
  const id = currentConvId.value
  try {
    await aiDeleteConversation(id)
    aiConvs.value = aiConvs.value.filter((c) => c.id !== id)
    aiMsgs.value = []
    currentConvId.value = ''
    if (aiConvs.value.length) {
      currentConvId.value = aiConvs.value[0].id
      switchConv()
    }
  } catch (_) {
    /* ignore */
  }
}

async function switchConv() {
  if (!currentConvId.value) {
    aiMsgs.value = []
    return
  }
  aiConvLoading.value = true
  try {
    const d = await aiConversationMessages(currentConvId.value)
    aiMsgs.value = (d.messages || []).map((m) => ({ role: m.role, content: m.content }))
    scrollAiConv()
  } catch (_) {
    aiMsgs.value = []
  } finally {
    aiConvLoading.value = false
  }
}

function refreshConvTitle() {
  const c = aiConvs.value.find((x) => x.id === currentConvId.value)
  if (c && c.title === '新对话') loadConvs()
}
function scrollAiConv() {
  nextTick(() => {
    const el = aiBodyRef.value
    if (el) el.scrollTop = el.scrollHeight
  })
}
function toolFiles(t) {
  try {
    const d = JSON.parse(t.result)
    return Array.isArray(d.files) ? d.files : []
  } catch (_) {
    return []
  }
}
// read_file 等工具的内容预览（非文件列表型结果）
function toolPreview(t) {
  try {
    const d = JSON.parse(t.result)
    if (t.name === 'read_file') {
      if (d.readable === false) return d.note || i18t('无法读取该文件内容')
      if (d.ai_summary) return (d.summary || '') + (d.tags && d.tags.length ? t(' ｜ 标签：') + d.tags.join('、') : '')
      const c = d.content || ''
      return (d.truncated ? c + t('…（内容较长已截断）') : c).slice(0, 140)
    }
    return ''
  } catch (_) {
    return ''
  }
}
// search_files 结果扩展词（语义检索提示）
function toolExpanded(t) {
  try {
    const d = JSON.parse(t.result)
    if (t.name === 'search_files' && Array.isArray(d.expanded) && d.expanded.length) {
      return t('语义扩展：') + d.expanded.slice(0, 8).join('、')
    }
    return ''
  } catch (_) {
    return ''
  }
}
function toolSummary(t) {
  try {
    const d = JSON.parse(t.result)
    if (t.name === 'read_file') return i18t('已读取：') + (d.name || '')
    return `检索到 ${d.count ?? 0} 项` + (d.semantic ? t('（语义检索）') : '')
  } catch (_) {
    return t('已执行 ') + t.name
  }
}
function openAiFile(f) {
  if (f.kind === 'dir') {
    router.push({ path: '/files', query: { parent: f.id } })
  } else {
    router.push('/read/' + f.id)
  }
}
function fmtSize(n) {
  if (!n) return '—'
  if (n < 1024) return n + ' B'
  if (n < 1048576) return (n / 1024).toFixed(1) + ' KB'
  if (n < 1073741824) return (n / 1048576).toFixed(1) + ' MB'
  return (n / 1073741824).toFixed(1) + ' GB'
}

const groups = reactive({
  content: true,
  blog: false,
  kb: false,
  sys: false,
})

function toggleGroup(k) {
  // 手风琴：开一个收其他
  for (const key of Object.keys(groups)) {
    if (key !== k) groups[key] = false
  }
  groups[k] = !groups[k]
}

// 按当前路由自动展开对应分组（系统 / 知识·AI / 博客 / 内容）
watch(() => route.path, (p) => {
  if (p === '/settings' || p.startsWith('/manage/') || p === '/org') groups.sys = true
  else if (p === '/digest') groups.kb = true
  else if (p === '/prompts') groups.kb = true
  else if (p === '/analytics') groups.blog = true
  else if (p === '/inbox') groups.content = true
  else if (p === '/mentions') groups.content = true
}, { immediate: true })

const isReading = computed(() => route.meta?.reading === true)

function goSearch() {
  if (!quickQuery.value.trim()) return
  router.push({ path: '/search', query: { q: quickQuery.value.trim() } })
}
</script>

<style scoped>
.app-shell {
  display: flex;
  height: 100%;
  min-height: 100vh;
  background: var(--bg);
}

/* ---- 侧栏（浅色：可读性优先） ---- */
.sidebar {
  width: var(--sidebar-w);
  min-width: var(--sidebar-w);
  background: var(--sidebar);
  color: var(--sidebar-text);
  border-right: 1px solid var(--border);
  display: flex;
  flex-direction: column;
  flex-shrink: 0;
  transition: width 0.2s ease, min-width 0.2s ease;
  overflow: hidden;
  z-index: var(--z-sidebar);
}

.sidebar.collapsed {
  width: 60px;
  min-width: 60px;
}

.brand {
  padding: 18px 18px 14px;
  display: flex;
  align-items: center;
  gap: 11px;
  flex-shrink: 0;
}

.brand-mark {
  position: relative;
  width: 34px;
  height: 34px;
  border-radius: 9px;
  background: var(--brand-grad);
  display: flex;
  align-items: center;
  justify-content: center;
  font-weight: 600;
  font-size: 14px;
  color: #fff;
  letter-spacing: -0.5px;
  flex-shrink: 0;
}

/* 品牌节点蓝点（UI 规范：K + 节点蓝点，代表知识节点） */
.brand-dot {
  position: absolute;
  right: 4px;
  bottom: 4px;
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: #6db5ff;
  box-shadow: 0 0 6px rgba(109, 181, 255, 0.9);
}

.brand-text h1 {
  font-size: 16px;
  font-weight: 600;
  color: var(--sidebar-text);
  letter-spacing: 0.2px;
  white-space: nowrap;
}

.brand-text p {
  font-size: 10.5px;
  color: var(--sidebar-text-2);
  margin-top: 1px;
  letter-spacing: 0.3px;
  white-space: nowrap;
}

.nav {
  flex: 1;
  overflow-y: auto;
  overflow-x: hidden;
  padding: 2px 8px;
  display: flex;
  flex-direction: column;
  gap: 1px;
}

.nav-pin {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 7px;
  padding: 8px;
  border-radius: 6px;
  color: var(--sidebar-text);
  font-size: 10.5px;
  cursor: pointer;
  margin-bottom: 6px;
  transition: background 0.14s ease;
  width: 100%;
}

.nav-pin:hover {
  background: var(--sidebar-hover);
  color: var(--sidebar-text);
}

.nav-group + .nav-group {
  margin-top: 10px;
}

.nav-group-head {
  display: flex;
  align-items: center;
  gap: 7px;
  padding: 9px 10px 6px;
  font-size: 11px;
  color: var(--sidebar-text-2);
  opacity: 0.72;
  text-transform: uppercase;
  letter-spacing: 1.2px;
  cursor: pointer;
  user-select: none;
  font-weight: 700;
  transition: 0.14s;
  border-radius: 6px;
  white-space: nowrap;
}

.nav-group-head:hover {
  color: var(--sidebar-text);
  background: var(--sidebar-hover);
}

.nav-group-head .arw {
  width: 11px;
  height: 11px;
  transition: 0.18s;
  flex-shrink: 0;
}

.nav-group-head .arw.closed {
  transform: rotate(-90deg);
}

.nav-group-body {
  display: flex;
  flex-direction: column;
  gap: 1px;
  padding-left: 4px;
}

.nav-item {
  display: flex;
  align-items: center;
  gap: 9px;
  padding: 7px 10px 7px 16px;
  border-radius: 6px;
  color: var(--sidebar-text);
  font-size: 13px;
  cursor: pointer;
  transition: background 0.14s ease, color 0.14s ease;
  white-space: nowrap;
  text-decoration: none;
}

.nav-item:hover {
  background: var(--sidebar-hover);
  color: var(--sidebar-text);
}

.nav-item.active {
  background: var(--sidebar-active);
  color: var(--primary);
  font-weight: 600;
}

.nav-item-btn {
  width: 100%;
  border: none;
  background: transparent;
  font: inherit;
  text-align: left;
}

.nav-item-disabled {
  opacity: 0.45;
  cursor: not-allowed;
}

.nav-item-disabled:hover {
  background: transparent;
  color: var(--sidebar-text);
}

/* 次级导航项：从属功能（回收站/草稿箱等），与主菜单拉开层次 */
.nav-item-sub {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 5px 10px 5px 30px;
  border-radius: 6px;
  color: var(--sidebar-text-2);
  font-size: 12px;
  cursor: pointer;
  transition: background 0.14s ease, color 0.14s ease;
  white-space: nowrap;
  text-decoration: none;
}

.nav-item-sub:hover {
  background: var(--sidebar-hover);
  color: var(--sidebar-text);
}

.nav-item-sub.active {
  background: var(--sidebar-active);
  color: var(--primary);
  font-weight: 600;
}

.sidebar-foot {
  padding: 12px 14px;
  border-top: 1px solid var(--border);
  flex-shrink: 0;
}

.kb-info {
  display: flex;
  align-items: center;
  gap: 9px;
}

/* 消息通知铃铛（侧栏底部个人区） */
.kb-bell {
  position: relative;
  margin-left: auto;
  width: 30px;
  height: 30px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: none;
  border-radius: 8px;
  background: transparent;
  color: var(--sidebar-text-2);
  cursor: pointer;
  transition: background 0.15s ease, color 0.15s ease;
}

.kb-bell:hover {
  background: var(--sidebar-hover);
  color: var(--sidebar-text);
}

.kb-bell.open {
  background: var(--sidebar-active);
  color: var(--primary);
}

.kb-badge {
  position: absolute;
  top: -3px;
  right: -3px;
  min-width: 16px;
  height: 16px;
  padding: 0 4px;
  border-radius: 8px;
  background: #e05e2a;
  color: #fff;
  font-size: 10px;
  font-weight: 600;
  line-height: 16px;
  text-align: center;
  box-shadow: 0 0 0 2px var(--sidebar);
  /* 新增消息时脉冲一次（prefers-reduced-motion 下自动禁用） */
  animation: notif-pulse 0.5s ease;
}

@keyframes notif-pulse {
  0% { transform: scale(0.5); }
  60% { transform: scale(1.25); }
  100% { transform: scale(1); }
}

/* 通知抽屉：从个人区上方弹出 */
.notif-panel {
  position: fixed;
  left: 14px;
  bottom: 64px;
  width: 340px;
  max-width: calc(100vw - 28px);
  background: var(--surface-1, #fff);
  border: 1px solid var(--border);
  border-radius: 12px;
  box-shadow: 0 12px 40px rgba(0, 0, 0, 0.22);
  z-index: var(--z-topbar);
  overflow: hidden;
}

.notif-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 14px;
  border-bottom: 1px solid var(--border);
  font-size: 13px;
  font-weight: 600;
  color: var(--text);
}

.notif-close {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  border: none;
  border-radius: 6px;
  background: transparent;
  color: var(--text-3);
  cursor: pointer;
}

.notif-close:hover {
  background: var(--surface-2);
  color: var(--text);
}

.notif-body {
  max-height: 340px;
  overflow-y: auto;
}

.notif-item {
  display: flex;
  gap: 9px;
  padding: 10px 14px;
  cursor: pointer;
  border-bottom: 1px solid var(--border);
  transition: background 0.12s ease;
}

.notif-item:hover {
  background: var(--surface-2);
}

.notif-item.unread {
  background: var(--primary-soft);
}

.notif-ic {
  margin-top: 2px;
  color: var(--text-3);
  flex-shrink: 0;
}

.notif-item.unread .notif-ic {
  color: var(--primary);
}

.notif-main {
  flex: 1;
  min-width: 0;
}

.notif-title {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  font-weight: 600;
  color: var(--text);
}

.notif-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--primary);
  flex-shrink: 0;
}

.notif-msg {
  font-size: 12px;
  color: var(--text-2);
  line-height: 1.5;
  margin-top: 2px;
  word-break: break-word;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.notif-time {
  font-size: 11px;
  color: var(--text-3);
  margin-top: 3px;
}

.notif-foot {
  padding: 8px 12px;
  text-align: center;
}

.notif-allread {
  padding: 5px 12px;
  border-radius: var(--radius-sm);
  border: 1px solid var(--border);
  background: var(--surface-2);
  color: var(--text-2);
  font-size: 12px;
  cursor: pointer;
  transition: all 0.14s ease;
}

.notif-allread:hover:not(:disabled) {
  color: var(--primary);
  border-color: var(--primary);
}

.notif-allread:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.notif-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
  padding: 30px 20px;
  color: var(--text-3);
  font-size: 13px;
  text-align: center;
}

.notif-empty p {
  margin: 0;
}

.notif-tip {
  font-size: 12px;
  line-height: 1.6;
  color: var(--text-3);
}

.notif-fade-enter-active,
.notif-fade-leave-active {
  transition: opacity 0.15s ease, transform 0.15s ease;
}

.notif-fade-enter-from,
.notif-fade-leave-to {
  opacity: 0;
  transform: translateY(6px);
}

.kb-avatar {
  width: 28px;
  height: 28px;
  border-radius: 8px;
  background: var(--sidebar-active);
  color: var(--primary);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 12px;
  font-weight: 600;
  flex-shrink: 0;
}

.kb-text {
  display: flex;
  flex-direction: column;
  line-height: 1.3;
  white-space: nowrap;
}

.kb-name {
  font-size: 12px;
  color: var(--sidebar-text);
}

.kb-sub {
  font-size: 10px;
  color: var(--sidebar-text-2);
}

/* ---- 主区 ---- */
.main-body {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-width: 0;
  position: relative;
}

.topbar {
  height: var(--topbar-h);
  min-height: var(--topbar-h);
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 0 16px;
  background: var(--surface);
  border-bottom: 1px solid var(--border);
  z-index: var(--z-topbar);
}

.crumb {
  display: flex;
  align-items: center;
  gap: 2px;
  font-size: 14px;
  font-weight: 600;
  color: var(--text);
  white-space: nowrap;
  min-width: 0;
  overflow: hidden;
}

.crumb-btn {
  padding: 3px 7px;
  border-radius: 6px;
  color: var(--text-2);
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
  white-space: nowrap;
  transition: background 0.14s ease, color 0.14s ease;
}

.crumb-btn:hover {
  background: var(--surface-2);
  color: var(--text);
}

.crumb-btn.current {
  color: var(--text);
  font-weight: 600;
}

.crumb-sep {
  color: var(--text-3);
  font-size: 12px;
  padding: 0 1px;
}

/* 侧栏存储目录树 */
.tree-root {
  display: flex;
  align-items: center;
  gap: 9px;
  padding: 7px 10px 7px 16px;
  border-radius: 6px;
  color: var(--sidebar-text);
  font-size: 13px;
  cursor: pointer;
  transition: background 0.14s ease, color 0.14s ease;
  white-space: nowrap;
}

.tree-root:hover {
  background: var(--sidebar-hover);
  color: var(--sidebar-text);
}

.tree-root.active {
  background: var(--sidebar-active);
  color: var(--primary);
  font-weight: 600;
}

.tree-root.drop-over {
  background: var(--primary-soft);
  outline: 2px dashed var(--primary);
  outline-offset: -2px;
  color: var(--primary);
}

.tree-root-name {
  font-weight: 600;
}

.dir-tree {
  margin-bottom: 4px;
}

.tree-trash {
  margin-top: 2px;
}

.search {
  flex: 1;
  max-width: 420px;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 10px;
  border-radius: var(--radius-sm);
  background: var(--surface-2);
  border: 1px solid var(--border);
  color: var(--text-3);
}

.search input {
  flex: 1;
  border: none;
  background: none;
  outline: none;
  color: var(--text);
  font-size: var(--fs-base);
}

.search input::placeholder {
  color: var(--text-3);
}

.kbd {
  font-size: 10px;
  color: var(--text-3);
  border: 1px solid var(--border);
  border-radius: 4px;
  padding: 1px 5px;
  background: var(--surface);
  white-space: nowrap;
}

.tb-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-left: auto;
}

.theme-switcher {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 4px 6px;
  border-radius: var(--radius-sm);
  background: var(--surface-2);
  border: 1px solid var(--border);
}

.theme-dot {
  padding: 3px;
  border-radius: 50%;
  display: flex;
}

.theme-dot .dot {
  width: 13px;
  height: 13px;
  border-radius: 50%;
  border: 2px solid transparent;
  transition: transform 0.15s ease;
}

.theme-dot.active .dot {
  transform: scale(1.15);
  box-shadow: 0 0 0 2px var(--primary);
}

.dot-indigo { background: #4f46e5; }
.dot-aurora { background: #0e8fd8; }
.dot-sunset { background: #e05e2a; }
.dot-dark { background: #1e222e; border-color: #4a5164 !important; }

.btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 32px;
  padding: 0 12px;
  border-radius: var(--radius-sm);
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
  transition: background 0.15s ease, color 0.15s ease, border-color 0.15s ease;
}

.btn-ghost {
  background: var(--surface-2);
  border: 1px solid var(--border);
  color: var(--text-2);
}

.btn-ghost:hover {
  color: var(--text);
  border-color: var(--text-3);
}

.icon-btn {
  width: 32px;
  height: 32px;
  padding: 0;
  justify-content: center;
}

.user-avatar {
  width: 30px;
  height: 30px;
  border-radius: 50%;
  background: var(--primary);
  color: #fff;
  font-size: 13px;
  font-weight: 600;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  position: relative;
}
.user-avatar:hover {
  opacity: 0.9;
}
.user-menu {
  position: absolute;
  top: 48px;
  right: 14px;
  z-index: 300;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  box-shadow: 0 10px 30px rgba(22, 24, 43, 0.12);
  min-width: 140px;
  padding: 6px;
}
.user-menu-item {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  padding: 8px 10px;
  border: none;
  border-radius: 6px;
  background: transparent;
  color: var(--text);
  font-size: 13px;
  cursor: pointer;
  text-align: left;
}
.user-menu-item:hover {
  background: var(--primary-soft);
}
.user-menu-item.danger {
  color: var(--danger);
}
.user-menu-item.danger:hover {
  background: var(--danger-soft);
}
.modal-mask {
  position: fixed;
  inset: 0;
  z-index: 400;
  background: rgba(22, 24, 43, 0.45);
  display: flex;
  align-items: center;
  justify-content: center;
}
.modal-box {
  background: var(--surface);
  border-radius: var(--radius);
  padding: 22px 24px;
  width: 340px;
  max-width: calc(100vw - 40px);
  box-shadow: 0 20px 50px rgba(22, 24, 43, 0.2);
}
.pwd-title {
  margin: 0 0 16px;
  font-size: 16px;
  color: var(--text);
}
.pwd-field {
  margin-bottom: 12px;
}
.pwd-field label {
  display: block;
  font-size: 12.5px;
  color: var(--text-2);
  margin-bottom: 5px;
}
.pwd-field input {
  width: 100%;
  box-sizing: border-box;
  padding: 8px 10px;
  border: 1px solid var(--border);
  border-radius: 8px;
  font-size: 14px;
  color: var(--text);
  background: var(--surface-2);
  outline: none;
}
.pwd-field input:focus {
  border-color: var(--primary);
}
.pwd-err {
  color: var(--danger);
  font-size: 12.5px;
  margin: 4px 0 8px;
}
.pwd-actions {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  margin-top: 16px;
}

.content {
  flex: 1;
  overflow-y: auto;
  min-height: 0;
  padding-bottom: 30px;
}

/* 底部状态栏：常驻主区底部，极简低调 */
.status-bar {
  position: absolute;
  left: 0;
  right: 0;
  bottom: 0;
  height: 29px;
  display: flex;
  align-items: center;
  gap: 18px;
  padding: 0 16px;
  background: var(--surface);
  border-top: 1px solid var(--border);
  font-size: 11.5px;
  color: var(--text-3);
  z-index: var(--z-topbar);
  white-space: nowrap;
  overflow: hidden;
}

.st-strong {
  color: var(--primary);
  font-weight: 600;
}

.st-hint {
  opacity: 0.75;
}

/* ---- 右侧 AI 面板 ---- */
.ai-panel {
  width: var(--ai-w, 280px);
  min-width: var(--ai-w, 280px);
  background: var(--surface);
  border-left: 1px solid var(--border);
  display: flex;
  flex-direction: column;
  position: relative;
  z-index: var(--z-topbar);
}

/* 左缘拖宽手柄（自由调整宽度，无上限） */
.ai-resize {
  position: absolute;
  left: -4px;
  top: 0;
  bottom: 0;
  width: 8px;
  cursor: ew-resize;
  z-index: 6;
}

.ai-resize::after {
  content: '';
  position: absolute;
  top: 0;
  bottom: 0;
  left: 3px;
  width: 2px;
  background: transparent;
  transition: background 0.12s ease;
}

.ai-resize:hover::after {
  background: var(--primary);
}

.ai-head {
  padding: 13px 16px;
  border-bottom: 1px solid var(--border);
  display: flex;
  align-items: center;
  gap: 10px;
}

.ai-head .ai-title { flex: 1; min-width: 0; }

.ai-close {
  width: 24px;
  height: 24px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  background: none;
  border: none;
  border-radius: 6px;
  color: var(--text-3);
  cursor: pointer;
  flex-shrink: 0;
}
.ai-close:hover { background: var(--bg-3); color: var(--text-1); }

/* "AI" 文字徽章（工具栏开关 + 面板徽标通用） */
.ai-text-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 20px;
  height: 16px;
  padding: 0 4px;
  border-radius: 4px;
  background: var(--primary);
  color: #fff;
  font-size: 10.5px;
  font-weight: 700;
  letter-spacing: 0.5px;
  line-height: 1;
}
.ai-toggle.on .ai-text-btn { box-shadow: 0 0 0 2px var(--primary-soft); }
.ai-toggle:not(.on) .ai-text-btn { background: var(--text-3); }

/* 顶栏「访问站点」入口（新窗口打开站点首页） */
.site-link {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  height: 28px;
  padding: 0 8px;
  border-radius: 8px;
  color: var(--text-2);
  text-decoration: none;
  font-size: 12px;
  white-space: nowrap;
  flex: none;
}
.site-link:hover { background: var(--bg-3); color: var(--text-1); }
@media (max-width: 900px) {
  .site-link-text { display: none; }
  .site-link { padding: 0 6px; }
}

.ai-badge {
  width: 28px;
  height: 28px;
  border-radius: 8px;
  background: var(--primary-soft);
  color: var(--primary);
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.ai-convbar {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 8px 12px;
  border-bottom: 1px solid var(--border);
}

.ai-conv-select {
  flex: 1;
  min-width: 0;
  height: 28px;
  padding: 0 8px;
  border-radius: var(--radius-sm);
  border: 1px solid var(--border);
  background: var(--surface-2);
  color: var(--text);
  font-size: var(--fs-micro);
  outline: none;
  cursor: pointer;
}

.ai-conv-btn {
  width: 28px;
  height: 28px;
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: var(--radius-sm);
  border: 1px solid var(--border);
  background: var(--surface-2);
  color: var(--text-2);
  cursor: pointer;
  transition: all 0.15s ease;
}

.ai-conv-btn:hover:not(:disabled) {
  color: var(--primary);
  border-color: var(--primary);
}

.ai-conv-btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.ai-title h3 {
  font-size: 13.5px;
  font-weight: 600;
  color: var(--text);
}

.ai-title p {
  font-size: 11px;
  color: var(--text-3);
}

.ai-body {
  flex: 1;
  overflow-y: auto;
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.ai-placeholder {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  padding: 28px 12px;
  color: var(--text-3);
  text-align: center;
  border: 1px dashed var(--border);
  border-radius: var(--radius);
}

.ai-placeholder p {
  font-size: 12.5px;
}

.ai-sug {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.ai-sug-item {
  padding: 9px 12px;
  border-radius: var(--radius-sm);
  background: var(--surface-2);
  border: 1px solid var(--border);
  font-size: 12.5px;
  color: var(--text-2);
  cursor: pointer;
  transition: background 0.14s ease;
}

.ai-sug-item:hover {
  background: var(--primary-soft);
  color: var(--primary);
}

/* AI 对话消息 */
.ai-msg {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.ai-msg.user {
  align-items: flex-end;
}

.ai-msg.assistant {
  align-items: flex-start;
}

.ai-msg-bubble {
  max-width: 92%;
  padding: 9px 12px;
  border-radius: var(--radius);
  font-size: 13px;
  line-height: 1.65;
  white-space: pre-wrap;
  word-break: break-word;
}

.ai-msg.user .ai-msg-bubble {
  background: var(--primary);
  color: #fff;
  border-bottom-right-radius: 4px;
}

.ai-msg.assistant .ai-msg-bubble {
  background: var(--surface-2);
  border: 1px solid var(--border);
  color: var(--text);
  border-bottom-left-radius: 4px;
}

.ai-msg-bubble.thinking {
  display: flex;
  align-items: center;
  gap: 8px;
  color: var(--text-3);
}

.ai-tool-card {
  max-width: 92%;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  overflow: hidden;
  background: var(--surface);
  align-self: flex-start;
}

.ai-tool-head {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 7px 10px;
  font-size: 11.5px;
  color: var(--text-2);
  background: var(--surface-2);
  border-bottom: 1px solid var(--border);
}

.ai-tool-files {
  padding: 5px;
}

.ai-tool-file {
  display: flex;
  align-items: center;
  gap: 7px;
  width: 100%;
  padding: 7px 8px;
  border-radius: var(--radius-sm);
  font-size: 12.5px;
  color: var(--text);
  text-align: left;
  cursor: pointer;
  transition: background 0.12s ease;
}

.ai-tool-file:hover {
  background: var(--primary-soft);
  color: var(--primary);
}

.ai-tool-file .tf-name {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ai-tool-file .tf-meta {
  font-size: 11px;
  color: var(--text-3);
  flex-shrink: 0;
}

.ai-tool-empty {
  padding: 8px;
  font-size: 12px;
  color: var(--text-3);
  text-align: center;
}

.ai-tool-preview {
  padding: 8px 10px;
  font-size: 12px;
  line-height: 1.6;
  color: var(--text-2);
  border-top: 1px dashed var(--border);
  white-space: pre-wrap;
  word-break: break-word;
  max-height: 96px;
  overflow: hidden;
}

.ai-tool-expanded {
  padding: 6px 10px;
  font-size: 11px;
  color: var(--primary);
  background: var(--primary-soft);
  border-radius: var(--radius-sm);
  margin: 6px 8px 0;
}

.ai-input {
  padding: 12px 16px;
  border-top: 1px solid var(--border);
  display: flex;
  gap: 8px;
}

.ai-input input {
  flex: 1;
  min-width: 0;
  height: 34px;
  padding: 0 12px;
  border-radius: var(--radius-sm);
  border: 1px solid var(--border);
  background: var(--surface-2);
  color: var(--text);
  font-size: 12.5px;
  outline: none;
  transition: border-color 0.14s ease;
}

.ai-input input:focus {
  border-color: var(--primary);
}

.ai-input .ai-send {
  flex-shrink: 0;
  align-self: center;
}

/* ---- 断点 ---- */
@media (max-width: 1359px) {
  .ai-panel {
    position: fixed;
    right: 0;
    top: 0;
    bottom: 0;
    box-shadow: var(--shadow-lg);
  }
}

@media (max-width: 767px) {
  .sidebar {
    position: fixed;
    left: 0;
    top: 0;
    bottom: 0;
    box-shadow: var(--shadow-lg);
  }
  .sidebar.collapsed {
    transform: translateX(-100%);
    width: var(--sidebar-w);
    min-width: var(--sidebar-w);
  }
  .search {
    max-width: none;
  }
  .theme-switcher {
    display: none;
  }
}
/* —— 侧栏标签树 —— */
.tag-tree {
  margin: 2px 8px 6px;
  border: 1px solid var(--border);
  border-radius: 8px;
  background: var(--surface-2);
  overflow: hidden;
}
.tag-tree-head {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 7px 10px;
  cursor: pointer;
  color: var(--sidebar-text);
  font-size: 12.5px;
  user-select: none;
}
.tag-tree-head:hover {
  background: var(--sidebar-hover);
}
.tag-tree-arw {
  transition: transform 0.15s;
}
.tag-tree-arw.open {
  transform: rotate(90deg);
}
.tag-tree-count {
  margin-left: auto;
  font-size: 11px;
  color: var(--sidebar-text-2);
}
.tag-tree-body {
  max-height: 260px;
  overflow-y: auto;
  padding: 2px 0 6px;
}
.tag-tree-hint {
  padding: 6px 10px 8px;
  font-size: 11.5px;
  color: var(--sidebar-text-2);
}
.tag-tree-item {
  display: flex;
  align-items: center;
  gap: 5px;
  padding: 4px 10px;
  cursor: pointer;
  font-size: 12.5px;
  color: var(--sidebar-text-2);
  white-space: nowrap;
  overflow: hidden;
}
.tag-tree-item:hover {
  background: var(--sidebar-hover);
  color: var(--sidebar-text);
}
.tag-tree-item.active {
  background: var(--sidebar-active);
  color: var(--primary);
}
.tag-tree-name {
  overflow: hidden;
  text-overflow: ellipsis;
}
.tag-tree-num {
  margin-left: auto;
  font-size: 10.5px;
  color: var(--sidebar-text-2);
}
</style>
