<template>
  <div
    class="files-view"
    @click.self="files.clearSelect()"
  >
    <!-- 拖拽遮罩 -->
    <Transition name="fade">
      <div v-if="dragging" class="drop-overlay">
        <div class="drop-hint">
          <AikIcon name="upload" :size="30" />
          <span>{{  $t('松开上传到当前位置')  }}</span>
        </div>
      </div>
    </Transition>

    <!-- 顶部工具行（面包屑已在顶栏，这里只放操作） -->
    <div class="toolbar">
      <div class="toolbar-actions">
        <label class="btn btn-primary">
          <AikIcon name="upload" :size="15" />
          <span>{{  $t('上传')  }}</span>
          <input type="file" multiple hidden @change="onPickFiles" />
        </label>
        <button class="btn" @click="openMkdir">
          <AikIcon name="plus" :size="15" />
          <span>{{  $t('新建文件夹')  }}</span>
        </button>
        <button class="btn" @click="openCreateDoc">
          <AikIcon name="fileText" :size="15" />
          <span>{{  $t('新建文档')  }}</span>
        </button>
        <button class="icon-btn" :title="$t('刷新')" @click="files.refresh()">
          <AikIcon name="refresh" :size="15" />
        </button>
        <button v-if="hasClipboard()" class="btn" @click="pasteClipboard" :title="$t('粘贴剪贴板中的文件')">
          <AikIcon name="clipboard" :size="14" />
          <span>{{  $t('粘贴')  }}</span>
        </button>
        <button class="btn" @click="exportAll" :disabled="exporting" :title="$t('一键导出全站 MD（含 博客/ 目录，ZIP）')">
          <AikIcon name="download" :size="14" />
          <span>{{  exporting ? $t('导出中…') : $t('导出全站')  }}</span>
        </button>

        <!-- 选取后操作（选中时出现，KodExplorer 式） -->
        <template v-if="files.selected.length">
          <span class="toolbar-sep"></span>
          <button class="btn" @click="toolbarCopy" :title="$t('复制选中项')">
            <AikIcon name="copy" :size="14" /><span>{{  $t('复制')  }}</span>
          </button>
          <button class="btn" @click="toolbarCut" :title="$t('剪切选中项')">
            <AikIcon name="cut" :size="14" /><span>{{  $t('剪切')  }}</span>
          </button>
          <button class="btn" @click="toolbarRename" :disabled="files.selected.length !== 1" :title="$t('重命名（仅单选）')">
            <AikIcon name="edit" :size="14" /><span>{{  $t('重命名')  }}</span>
          </button>
          <button class="btn btn-danger-ghost" @click="toolbarDelete" :title="$t('删除选中项')">
            <AikIcon name="trash" :size="14" /><span>{{  $t('删除')  }}</span>
          </button>
          <button class="btn" @click="openOrganize" :title="$t('AI 整理：建议目录/标签，检测相似重复')">
            <AikIcon name="sparkles" :size="14" /><span>{{  $t('AI 整理')  }}</span>
          </button>
        </template>

        <div class="view-switch">
          <button
            class="icon-btn"
            :class="{ active: files.viewMode === 'grid' }"
            :title="$t('网格视图')"
            @click="files.setViewMode('grid')"
          ><AikIcon name="grid" :size="16" /></button>
          <button
            class="icon-btn"
            :class="{ active: files.viewMode === 'list' }"
            :title="$t('列表视图')"
            @click="files.setViewMode('list')"
          ><AikIcon name="list" :size="16" /></button>
        </div>
        <!-- 标签筛选：跨目录按标签列文件（知识库/博客共用标签体系） -->
        <select v-if="allTags.length" v-model="tagFilter" class="tag-filter" @change="applyTagFilter">
          <option value="">{{ $t('按标签筛选') }}</option>
          <option v-for="t in allTags" :key="t.id" :value="t.id">{{  t.path  }}（{{  t.count  }}）</option>
        </select>
      </div>
    </div>

    <!-- 错误提示 -->
    <div v-if="files.error" class="error-banner">
      <AikIcon name="info" :size="14" />
      <span>{{  files.error  }}</span>
      <button class="btn-link" @click="files.refresh()">{{ $t('重试') }}</button>
    </div>

    <!-- 上传进度 -->
    <div v-if="uploadState" class="upload-progress">
      <AikIcon name="upload" :size="14" />
      <div class="up-bar">
        <div class="up-fill" :style="{ width: uploadState.percent + '%' }"></div>
      </div>
      <span class="up-text">{{  uploadState.percent  }}%</span>
    </div>

    <!-- 内容 -->
    <div v-if="files.loading" class="loading-state">
      <div class="spinner"></div>
      <span>{{ $t('加载中…') }}</span>
    </div>

    <!-- 空态 -->
    <div v-else-if="!files.items.length" class="empty-state" @click="pickFiles">
      <div class="empty-icon"><AikIcon name="upload" :size="28" /></div>
      <p class="empty-title">{{ $t('这个位置还没有内容') }}</p>
      <p class="empty-desc">{{ $t('点击或拖拽文件到此处上传，资料将保存在你的自有存储中') }}</p>
      <button class="btn btn-primary" @click.stop="pickFiles">
        <AikIcon name="upload" :size="15" />
        <span>{{ $t('上传文件') }}</span>
      </button>
      <input ref="emptyInput" type="file" multiple hidden @change="onPickFiles" />
    </div>

    <!-- 网格视图 -->
    <div v-else-if="files.viewMode === 'grid'" class="grid" @click.self="files.clearSelect()">
      <div
        v-for="f in visibleItems"
        :key="f.id"
        class="grid-item"
        :class="{ selected: files.selected.includes(f.id), 'drop-target': dropTargetId === f.id }"
        :data-kind="f.kind"
        :data-fid="f.id"
        @click="onItemClick(f, $event)"
        @dblclick="onItemDblClick(f)"
        @mousedown="onRowMouseDown($event, f)"
        @contextmenu="onRowContextMenu(f, $event)"
      >
        <div class="thumb">
          <img
            v-if="thumbUrl(f)"
            :src="thumbUrl(f)"
            loading="lazy"
            :alt="f.name"
            @error="onThumbError(f)"
          />
          <div v-else class="thumb-icon" :class="'kind-' + fileKind(f)">
            <AikIcon :name="kindIcon(f)" :size="30" />
          </div>
          <span v-if="thumbUrl(f) && fileKind(f) === 'video'" class="thumb-play" :title="$t('视频缩略图（点开可播放）')"></span>
        </div>
        <div class="item-name" :title="f.name">{{  f.name  }}
          <span v-if="f.kind === 'dir' && sharedDirIds.has(f.id)" class="dir-share-badge" :title="$t('此文件夹已整体分享到公开博客（强授权）')">{{ $t('已分享') }}</span>
        </div>
        <div class="item-meta">
          <span>{{  f.kind === 'dir' ? $t('文件夹') : formatSize(f.size)  }}</span>
        </div>
        <div v-if="isVideo(f)" class="grid-transcode" @click.stop>
          <button
            v-if="tcStatus(f) === '' || tcStatus(f) === 'failed' || tcStatus(f) === 'canceled'"
            class="mini-btn"
            @click="triggerTranscodeFor(f)"
          >{{  tcStatus(f) === 'failed' ? $t('重试转码') : $t('转码')  }}</button>
          <template v-else-if="tcStatus(f) === 'queued' || tcStatus(f) === 'running'">
            <span class="tc-progress">{{  tcStatus(f) === 'queued' ? $t('排队中') : $t('转码中')  }} {{  tcProgress(f)  }}</span>
            <span class="tc-bar"><i :style="{ width: tcProgress(f) }"></i></span>
            <button class="mini-btn ghost" @click="cancelTranscodeFor(f)">{{ $t('取消') }}</button>
          </template>
          <span v-else-if="tcStatus(f) === 'done'" class="tc-done">{{ $t('✓ 已转码') }}</span>
        </div>
        <div class="grid-ops" @click.stop>
          <button
            class="check"
            :class="{ on: files.selected.includes(f.id) }"
            @click="onCheckClick(f, $event)"
          >
            <AikIcon v-if="files.selected.includes(f.id)" name="check" :size="12" />
          </button>
          <button class="icon-btn grid-more" :data-mid="f.id" :title="$t('更多操作')" @click="toggleMenu(f.id)">
            <AikIcon name="more" :size="14" />
          </button>
          <FileMenu
            v-if="menuFor === f.id"
            :f="f"
            :pos="menuPos"
            @close="closeMenu"
            @open="menuOpen(f)"
            @download="menuDownload(f)"
            @copy="menuCopy(f)"
            @cut="menuCut(f)"
            @rename="menuRename(f)"
            @todraft="menuToDraft(f)"
            @tags="menuTags(f)"
            @share="menuShare(f)"
            @ocr="menuOcr(f)"
            @delete="menuDelete(f)"
          />
        </div>
      </div>
    </div>

    <!-- 列表视图 -->
    <div v-else class="list" :style="colStyle()" @click.self="files.clearSelect()">
      <div class="list-head">
        <span class="col-check">
          <button
            class="check select-all"
            :class="{ on: allSelected, half: someSelected && !allSelected }"
            :title="allSelected ? t('取消全选') : t('全选本页')"
            @click.stop="toggleSelectAll"
          >
            <AikIcon v-if="allSelected" name="check" :size="12" />
          </button>
        </span>
        <span class="col-name" :class="{ resizing: resizingCol === 'name' }">
          <button class="th-btn" :class="{ active: sortKey === 'name' }" @click="toggleSort('name')">
            {{ $t('名称') }}
            <AikIcon v-if="sortKey === 'name'" :name="sortDir === 1 ? 'arrowUp' : 'arrowDown'" :size="11" />
          </button>
          <span class="col-resize" @mousedown.prevent="startResize($event, 'name')"></span>
        </span>
        <span class="col-type" :class="{ resizing: resizingCol === 'type' }">
          <button class="th-btn" :class="{ active: sortKey === 'type' }" @click="toggleSort('type')">
            {{ $t('类型') }}
            <AikIcon v-if="sortKey === 'type'" :name="sortDir === 1 ? 'arrowUp' : 'arrowDown'" :size="11" />
          </button>
          <span class="col-resize" @mousedown.prevent="startResize($event, 'type')"></span>
        </span>
        <span class="col-size" :class="{ resizing: resizingCol === 'size' }">
          <button class="th-btn" :class="{ active: sortKey === 'size' }" @click="toggleSort('size')">
            {{ $t('大小') }}
            <AikIcon v-if="sortKey === 'size'" :name="sortDir === 1 ? 'arrowUp' : 'arrowDown'" :size="11" />
          </button>
          <span class="col-resize" @mousedown.prevent="startResize($event, 'size')"></span>
        </span>
        <span class="col-time" :class="{ resizing: resizingCol === 'time' }">
          <button class="th-btn" :class="{ active: sortKey === 'time' }" @click="toggleSort('time')">
            {{ $t('修改时间') }}
            <AikIcon v-if="sortKey === 'time'" :name="sortDir === 1 ? 'arrowUp' : 'arrowDown'" :size="11" />
          </button>
          <span class="col-resize" @mousedown.prevent="startResize($event, 'time')"></span>
        </span>
        <span class="col-action"></span>
      </div>
      <!-- 列宽拖拽边界线（跟随鼠标，明确方向） -->
      <div v-if="resizingCol" class="resize-line" :style="{ left: resizeLineX + 'px' }"></div>
      <div
        v-for="f in visibleItems"
        :key="f.id"
        class="list-row"
        :class="{ selected: files.selected.includes(f.id), 'drop-target': dropTargetId === f.id }"
        :data-kind="f.kind"
        :data-fid="f.id"
        @click="onItemClick(f, $event)"
        @dblclick="onItemDblClick(f)"
        @mousedown="onRowMouseDown($event, f)"
        @contextmenu="onRowContextMenu(f, $event)"
      >
        <span class="col-check">
          <button class="check" :class="{ on: files.selected.includes(f.id) }" @click.stop="onCheckClick(f, $event)">
            <AikIcon v-if="files.selected.includes(f.id)" name="check" :size="12" />
          </button>
        </span>
        <span class="col-name">
          <AikIcon :name="kindIcon(f)" :size="16" />
          <span class="row-name" :title="f.name">{{  f.name  }}</span>
          <span v-if="mediaMeta(f)" class="row-media">{{  mediaMeta(f)  }}</span>
          <span v-if="f.kind === 'dir' && sharedDirIds.has(f.id)" class="dir-share-badge" :title="$t('此文件夹已整体分享到公开博客（强授权）')">{{ $t('已分享') }}</span>
          <span v-for="t in visibleTags(f)" :key="t.id" class="row-tag" :title="`点击按「${t.path}」筛选`" @click.stop="filterByTag(t)">{{  t.path  }}</span>
          <span v-if="(f.tags || []).length > 2" class="row-tag more" :class="{ open: moreOpen === f.id }"
            @click.stop="toggleMore(f.id)">
            +{{  (f.tags || []).length - MAX_INLINE_TAGS  }}
            <span class="tag-pop" @click.stop>
              <button v-for="t in moreTags(f)" :key="t.id" class="tag-pop-item" @click.stop="filterByTag(t)">
                {{  t.path  }}
              </button>
            </span>
          </span>
        </span>
        <span class="col-type">{{  fileTypeLabel(f)  }}</span>
        <span class="col-size">{{  f.kind === 'dir' ? '—' : formatSize(f.size)  }}</span>
        <span class="col-time">{{  formatTime(f.updated_at)  }}</span>
        <span class="col-action">
          <div class="row-menu" @click.stop>
            <button class="icon-btn" :data-mid="f.id" :title="$t('更多操作')" @click="toggleMenu(f.id)">
              <AikIcon name="more" :size="16" />
            </button>
            <FileMenu
              v-if="menuFor === f.id"
              :f="f"
              :pos="menuPos"
              @close="closeMenu"
              @open="menuOpen(f)"
              @download="menuDownload(f)"
              @copy="menuCopy(f)"
              @cut="menuCut(f)"
              @rename="menuRename(f)"
              @todraft="menuToDraft(f)"
              @tags="menuTags(f)"
              @share="menuShare(f)"
              @ocr="menuOcr(f)"
              @delete="menuDelete(f)"
            />
          </div>
        </span>
      </div>
    </div>

    <!-- 标签筛选分页条 -->
    <div v-if="tagFilter && sortedItems.length > tagPageSize" class="tag-pager">
      <span class="tag-pager-info">{{ $t('共') }} {{  sortedItems.length  }} {{ $t('个文件') }}</span>
      <select v-model="tagPageSize" class="tag-pager-size" @change="tagPage = 1">
        <option :value="20">{{ $t('20 / 页') }}</option>
        <option :value="50">{{ $t('50 / 页') }}</option>
        <option :value="100">{{ $t('100 / 页') }}</option>
      </select>
      <button class="pager-btn" :disabled="tagPage <= 1" @click="tagGo(tagPage - 1)">{{ $t('上一页') }}</button>
      <span class="tag-pager-cur">{{  tagPage  }} / {{  tagTotalPages()  }}</span>
      <button class="pager-btn" :disabled="tagPage >= tagTotalPages()" @click="tagGo(tagPage + 1)">{{ $t('下一页') }}</button>
    </div>

    <!-- 标签 / 分类弹窗 -->
    <TagDialog v-if="tagFile" :file="tagFile" @close="tagFile = null" @saved="onTagsSaved" />

    <!-- AI 整理弹窗（P0-3：建议目录/标签 + 相似冲突检测） -->
    <OrganizeDialog
      v-if="showOrganize"
      :file-ids="organizeIds"
      @close="showOrganize = false"
      @applied="onOrganizeApplied"
      @open="openOrganizeTarget"
    />

    <!-- 新建文件夹弹窗 -->
    <Transition name="fade">
      <div v-if="showMkdir" class="modal-mask" @click.self="showMkdir = false">        <div class="modal modal-sm">
          <h3>{{ $t('新建文件夹') }}</h3>
          <input
            ref="mkdirInput"
            v-model="newDirName"
            class="modal-input"
            :placeholder="$t('文件夹名称')"
            @keydown.enter="confirmMkdir"
          />
          <div class="modal-actions">
            <button class="btn" @click="showMkdir = false">{{ $t('取消') }}</button>
            <button class="btn btn-primary" @click="confirmMkdir">{{ $t('创建') }}</button>
          </div>
        </div>
      </div>
    </Transition>

    <!-- 新建文档弹窗 -->
    <Transition name="fade">
      <div v-if="showDoc" class="modal-mask" @click.self="showDoc = false">
        <div class="modal modal-doc">
          <h3>{{ $t('新建文档') }}</h3>
          <input
            ref="docNameInput"
            v-model="docName"
            class="modal-input"
            :placeholder="$t('文档名称（如 笔记.md）')"
            @keydown.enter="confirmDoc"
          />
          <textarea
            v-model="docContent"
            class="modal-textarea"
            :placeholder="$t('初始内容（可留空，创建后随时编辑）')"
            rows="6"
          ></textarea>
          <div class="modal-actions">
            <button class="btn" @click="showDoc = false">{{ $t('取消') }}</button>
            <button class="btn btn-primary" :disabled="!docName.trim()" @click="confirmDoc">{{ $t('创建') }}</button>
          </div>
        </div>
      </div>
    </Transition>

    <!-- OCR 识别结果弹窗 -->
    <Transition name="fade">
      <div v-if="ocrState" class="modal-mask" @click.self="ocrState = null">
        <div class="modal modal-ocr">
          <h3>{{ $t('OCR 识别 —') }} {{  ocrState.name  }}</h3>
          <p v-if="ocrLoading" class="modal-desc">{{ $t('正在识别图片文字…（首次约 5-15 秒）') }}</p>
          <div v-else class="ocr-body">
            <div class="ocr-text">{{  ocrState.text || $t('未识别到文字')  }}</div>
          </div>
          <div class="modal-actions">
            <button class="btn" @click="ocrState = null">{{ $t('关闭') }}</button>
            <button
              v-if="ocrState.text && !ocrLoading"
              class="btn"
              :disabled="ocrCleaning"
              @click="cleanOcrText"
            >{{  ocrCleaning ? $t('整理中…') : $t('AI 整理')  }}</button>
            <button
              v-if="ocrState.text && !ocrLoading"
              class="btn btn-primary"
              :disabled="ocrSaving"
              @click="saveOcrDoc"
            >{{  ocrSaving ? $t('保存中…') : $t('保存为文档')  }}</button>
          </div>
        </div>
      </div>
    </Transition>

    <!-- 重命名弹窗 -->
    <Transition name="fade">
      <div v-if="showRename" class="modal-mask" @click.self="showRename = false">
        <div class="modal">
          <h3>{{ $t('重命名') }}</h3>
          <input
            ref="renameInput"
            v-model="renameName"
            class="modal-input"
            :placeholder="$t('新名称')"
            @keydown.enter="confirmRename"
          />
          <div class="modal-actions">
            <button class="btn" @click="showRename = false">{{ $t('取消') }}</button>
            <button class="btn btn-primary" @click="confirmRename">{{ $t('确定') }}</button>
          </div>
        </div>
      </div>
    </Transition>

    <!-- 分享/发布弹窗 -->
    <Transition name="fade">
      <div v-if="shareState" class="modal-mask" @click.self="shareState = null">
        <div class="modal modal-share">
          <h3>{{ $t('分享 / 发布 —') }} {{  shareState.name  }}</h3>
          <!-- 文件夹整体分享：强授权确认（必须先显式确认，服务端同步校验） -->
          <template v-if="shareState.kind === 'dir' && !shareState.url">
            <div class="dir-share-warn">
              <AikIcon name="alert" :size="16" />
              <div>
                <p class="dir-share-title">{{ $t('文件夹整体分享（强授权）') }}</p>
                <p class="dir-share-desc">
                  {{ $t('此操作将把该文件夹') }}<b>{{ $t('及全部子文件夹') }}</b>{{ $t('中的文件整体公开到对外博客（不含草稿）， 任何人拿到链接即可阅读。') }}<b>{{ $t('目录后续新增文件会自动公开') }}</b>{{ $t('（撤销分享后全部下线）。') }}
                </p>
                <p class="dir-share-tip">{{ $t('这是一次公开范围较大的操作，需要你显式确认后才能创建。') }}</p>
              </div>
            </div>
            <div v-if="shareState.fileCount != null" class="dir-share-count">
              {{ $t('将公开') }} <b>{{  shareState.fileCount  }}</b> {{ $t('个文件') }}
            </div>
            <div class="share-actions">
              <button class="btn" @click="shareState = null">{{ $t('取消') }}</button>
              <button class="btn btn-danger" :disabled="shareState.creating" @click="createShareUrl">
                {{  shareState.creating ? $t('正在创建…') : $t('确认整体分享')  }}
              </button>
            </div>
          </template>
          <!-- 单文件分享 / 已创建 -->
          <template v-else>
            <p class="modal-desc">{{ $t('任何人拿到链接即可阅读（公开访问，无需登录）。发布后可作为博客文章展示。') }}</p>
            <div v-if="shareState.creating" class="share-loading">{{ $t('正在创建分享链接…') }}</div>
            <template v-else-if="shareState.url">
              <input ref="shareInput" :value="shareState.url" class="modal-input share-url" readonly @click="copyShareUrl" />
              <div v-if="shareState.kind === 'dir'" class="dir-share-count">{{ $t('已整体分享，目录内公开文件：') }}{{  shareState.fileCount  }}</div>
              <div class="share-actions">
                <button class="btn" :disabled="shareState.revoking" @click="openSharePage">{{ $t('打开预览') }}</button>
                <button class="btn" @click="copyShareUrl">{{ $t('复制链接') }}</button>
                <button class="btn btn-danger-ghost" :disabled="shareState.revoking" @click="revokeShare">{{ $t('撤销') }}</button>
              </div>
            </template>
            <template v-else>
              <div class="share-actions">
                <button class="btn" @click="shareState = null">{{ $t('关闭') }}</button>
                <button class="btn btn-primary" :disabled="shareState.creating" @click="createShareUrl">{{ $t('创建链接') }}</button>
              </div>
            </template>
          </template>
        </div>
      </div>
    </Transition>

    <!-- 删除确认弹窗 -->
    <Transition name="fade">
      <div v-if="confirmState" class="modal-mask" @click.self="confirmState = null">
        <div class="modal modal-sm">
          <h3>{{  confirmState.title  }}</h3>
          <p class="modal-desc">{{  confirmState.message  }}</p>
          <div class="modal-actions">
            <button class="btn" @click="confirmState = null">{{ $t('取消') }}</button>
            <button class="btn btn-danger" @click="confirmOk">{{ $t('删除') }}</button>
          </div>
        </div>
      </div>
    </Transition>

    <!-- 同名冲突对话框（资源管理器式：覆盖 / 重命名 / 跳过） -->
    <Transition name="fade">
      <div v-if="conflict" class="modal-mask" @click.self="closeConflict('skip')">
        <div class="modal modal-sm">
          <h3>{{ $t('目标位置已存在同名文件') }}</h3>
          <p class="modal-desc">
“{{ conflict.names[0] }}”{{ conflict.names.length > 1 ? ' 等 ' + conflict.names.length + $t('个文件') : '' }} {{ $t('在目标位置已存在同名文件') }}
          </p>
          <label class="conflict-all">
            <input type="checkbox" v-model="conflict.applyAll" />
            {{ $t('为剩余冲突执行相同操作') }}
          </label>
          <div class="modal-actions">
            <button class="btn btn-danger" @click="closeConflict('overwrite')">{{ $t('覆盖') }}</button>
            <button class="btn" @click="closeConflict('skip')">{{ $t('跳过') }}</button>
            <button class="btn btn-primary" @click="closeConflict('rename')">{{ $t('重命名') }}</button>
          </div>
        </div>
      </div>
    </Transition>

    <!-- 轻提示 -->
  </div>
</template>

<script setup>
import { t as i18t } from '@/i18n'
import { ref, computed, onMounted, onUnmounted, watch, reactive } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import AikIcon from '@/components/AikIcon.vue'
import FileMenu from '@/components/FileMenu.vue'
import TagDialog from '@/components/TagDialog.vue'
import OrganizeDialog from '@/components/OrganizeDialog.vue'
import { useFilesStore } from '@/stores/files'
import * as api from '@/api'
import { contentUrl } from '@/api'

const router = useRouter()
const route = useRoute()
const files = useFilesStore()

// 一键导出全站 MD（含 博客/ 目录）：POST /exports source=all → 轮询 → 下载
const exporting = ref(false)
async function exportAll() {
  if (exporting.value) return
  exporting.value = true
  try {
    const job = await api.requestJSON('/exports', { method: 'POST', body: JSON.stringify({ source: 'all', name: t('aiklog-全站') }) })
    const jid = job.id
    for (let i = 0; i < 120; i++) {
      await new Promise((r) => setTimeout(r, 1000))
      const d = await api.requestJSON(`/exports/${jid}`)
      if (d.status === 'done' || d.status === 'partial') {
        const a = document.createElement('a')
        a.href = `/api/v1/exports/${jid}/download`
        a.download = ''
        document.body.appendChild(a)
        a.click()
        a.remove()
        showToast(d.status === 'done' ? i18t('全站导出完成（含博客目录）') : i18t('导出完成（部分失败，见详情）'))
        return
      }
      if (d.status === 'failed') {
        showToast(i18t('导出失败：') + (d.last_error || t('未知错误')))
        return
      }
    }
    showToast(t('导出超时，请稍后在导入导出中心查看'))
  } catch (e) {
    showToast(t(i18t('导出失败：')) + (e.message || e))
  } finally {
    exporting.value = false
  }
}

const dragging = ref(false)
const showMkdir = ref(false)
const newDirName = ref('')
const emptyInput = ref(null)
// 新建文档
const showDoc = ref(false)
const docName = ref('')
const docContent = ref('')
const docNameInput = ref(null)
// 行操作菜单
const menuFor = ref('')          // 当前打开的菜单对应文件 id
const menuPos = ref({ top: 0, left: 0 })
const clipboard = ref(null)      // { ids: [], cut: boolean, names: [], parents: [] } 多选剪贴板（parents 用于原地移动检测）
// 资源管理器式同名冲突对话框：{ names, applyAll, resolve }——resolve({action, applyAll})
const conflict = ref(null)
function showConflictDialog(names) {
  return new Promise((resolve) => {
    conflict.value = { names: [...names], applyAll: false, resolve }
  })
}
function closeConflict(action) {
  const c = conflict.value
  if (!c) return
  conflict.value = null
  c.resolve({ action, applyAll: c.applyAll })
}
const showRename = ref(false)
const renameTarget = ref(null)
const renameName = ref('')
const renameInput = ref(null)
// 标签筛选（跨目录）
const allTags = ref([])
const tagFilter = ref('')
// 行内标签最多显示 2 个（多余折叠为 +N，避免覆盖文件名）
const MAX_INLINE_TAGS = 2
function visibleTags(f) {
  return (f.tags || []).slice(0, MAX_INLINE_TAGS)
}
function moreTagsTitle(f) {
  return t('更多标签：') + (f.tags || []).slice(MAX_INLINE_TAGS).map((t) => t.path).join('、')
}
const moreOpen = ref(null)
function moreTags(f) {
  return (f.tags || []).slice(MAX_INLINE_TAGS)
}
function toggleMore(id) {
  moreOpen.value = moreOpen.value === id ? null : id
}
// 点击文件行内标签徽章 → 按该标签筛选（行内其他区域维持原有选取/双击逻辑）
function filterByTag(t) {
  tagFilter.value = t.id
  applyTagFilter()
}
async function applyTagFilter() {
  files.selected = []
  tagPage.value = 1
  // 同步路由 query（侧栏标签树高亮/直达一致）
  const want = tagFilter.value || undefined
  if (route.query.tag !== want) {
    router.replace({ path: '/files', query: want ? { tag: want } : {} })
  }
  if (!tagFilter.value) {
    await files.refresh()
    return
  }
  files.loading = true
  files.error = ''
  try {
    files.items = await api.listFilesByTag(tagFilter.value)
  } catch (e) {
    files.error = e.message
  } finally {
    files.loading = false
  }
}
// 标签筛选结果前端分页（一页 N 个 + 页码换页）
const tagPage = ref(1)
const tagPageSize = ref(20)
const visibleItems = computed(() => {
  if (!tagFilter.value) return sortedItems.value
  const start = (tagPage.value - 1) * tagPageSize.value
  return sortedItems.value.slice(start, start + tagPageSize.value)
})
function tagTotalPages() {
  return Math.max(1, Math.ceil(sortedItems.value.length / tagPageSize.value))
}
function tagGo(p) {
  tagPage.value = Math.min(Math.max(1, p), tagTotalPages())
  const scroller = document.querySelector('.files-body')
  if (scroller) scroller.scrollTop = 0
}
// 删除确认（自定义 modal）与轻提示 toast
import { useToastStore } from '@/stores/toast'
const toastStore = useToastStore()
import { tryCopy } from '@/utils/clipboard'
import { t } from '@/i18n'
const confirmState = ref(null)

function askConfirm(title, message, onOk) {
  confirmState.value = { title, message, onOk }
}
function confirmOk() {
  const cb = confirmState.value?.onOk
  confirmState.value = null
  if (cb) cb()
}
function showToast(text) {
  toastStore.push(text)
}

const IMAGE_EXT = ['png', 'jpg', 'jpeg', 'gif', 'webp', 'svg', 'bmp', 'avif']

function ext(name) {
  const i = name.lastIndexOf('.')
  return i >= 0 ? name.slice(i + 1).toLowerCase() : ''
}

function isImage(f) {
  return f.kind === 'file' && IMAGE_EXT.includes(ext(f.name))
}

// ---- 缩略图（B15）----
// 能出缩略图的类型，与后端 thumbMimeOf 同口径：图片（**排除 svg**，活性文档不解码）+ 视频。
const VIDEO_EXT = ['mp4', 'mov', 'mkv', 'webm', 'avi']
const AUDIO_EXT = ['mp3', 'wav', 'm4a', 'flac', 'ogg']
const THUMB_EXT = IMAGE_EXT.filter((e) => e !== 'svg').concat(VIDEO_EXT)

function canThumb(f) {
  return f.kind === 'file' && THUMB_EXT.includes(ext(f.name))
}

// 缓存是普通 Map（非响应式）→ 异步到达后必须显式自增来触发重渲染，否则会白屏占位。
// ⚠️ 这正是「资产 200 ≠ 页面渲染成功」那类坑：请求成功但视图不更新。
const thumbTick = ref(0)
const thumbMiss = new Set() // 本地失败集：同一列表里不反复打同一个 404

// 缩略图：优先派生对象；拿不到就退回图标。
// 有意**不**退回原图 —— 列表里拉整张原图正是 B15 要收掉的开销（大图会把首屏拖垮）。
function thumbUrl(f) {
  void thumbTick.value // 建立响应式依赖
  if (!canThumb(f)) return ''
  const cached = api.fetchThumbUrlSync(f.id)
  if (cached) return cached
  if (thumbMiss.has(f.id)) return ''
  api
    .fetchThumbUrl(f.id)
    .then((u) => {
      if (u) thumbTick.value++
      else thumbMiss.add(f.id)
    })
    .catch(() => thumbMiss.add(f.id))
  return ''
}

// 图片解码失败（服务端缩略图坏了）→ 让位给图标，而不是留个破图
function onThumbError(f) {
  thumbMiss.add(f.id)
  api.invalidateThumb(f.id)
  thumbTick.value++
}

// ---- 批量媒体元信息（B15）：只读缓存，不触发探测/生成 ----
const mediaInfo = ref({})
const mediaAsked = new Set()

function fmtDuration(ms) {
  const s = Math.round(ms / 1000)
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  const ss = s % 60
  const pad = (n) => String(n).padStart(2, '0')
  return h > 0 ? `${h}:${pad(m)}:${pad(ss)}` : `${m}:${pad(ss)}`
}

// 列表行尾的「分辨率 · 时长」：只在拿到缓存值后显示，避免闪一串占位
function mediaMeta(f) {
  if (f.kind !== 'file') return ''
  const e = ext(f.name)
  if (!VIDEO_EXT.includes(e) && !AUDIO_EXT.includes(e)) return ''
  const m = mediaInfo.value[f.id]
  if (!m) return ''
  const parts = []
  if (m.width && m.height) parts.push(`${m.width}×${m.height}`)
  if (m.duration_ms > 0) parts.push(fmtDuration(m.duration_ms))
  return parts.join(' · ')
}

// 按可见项补拉元信息：去重 + 限批量 100（与后端上限一致）+ 失败静默
async function loadMediaMeta() {
  const ids = visibleItems.value
    .filter((f) => f.kind === 'file')
    .map((f) => f.id)
    .filter((id) => !mediaAsked.has(id))
  if (!ids.length) return
  const batch = ids.slice(0, 100)
  batch.forEach((id) => mediaAsked.add(id))
  try {
    const d = await api.fetchMediaBatch(batch)
    if (d && d.items && Object.keys(d.items).length) {
      mediaInfo.value = { ...mediaInfo.value, ...d.items }
    }
  } catch (_) {
    /* 元信息是增强项：失败不影响列表本身 */
  }
}
// ---- 视频转码（B17）：触发 / 状态 / 取消 ----
// 状态域与后端同构：queued|running|done|failed|canceled；空串=从未发起。
// 复用 thumbTick 的响应式思路：transcodeTick 自增触发重渲染（缓存非响应式，必须手动 tick）。
const transcodeTick = ref(0)
const transcodeData = reactive({}) // id -> { status, progress, message, bytes, height, signature }
const transcodeAsked = new Set()
const transcodeTimers = {}

function isVideo(f) {
  return f.kind === 'file' && VIDEO_EXT.includes(ext(f.name))
}
// 渲染期惰性拉取状态（与 thumbUrl 同思路：模板反复调用收敛成一次请求）。
function tcStatus(f) {
  void transcodeTick.value
  const j = transcodeData[f.id]
  if (!j) {
    if (!transcodeAsked.has(f.id)) {
      transcodeAsked.add(f.id)
      api
        .transcodeStatus(f.id)
        .then((r) => {
          transcodeData[f.id] = r || { status: '' }
          transcodeTick.value++
          const s = r && r.status
          if (s === 'queued' || s === 'running') pollTranscode(f.id)
          else transcodeAsked.delete(f.id)
        })
        .catch(() => transcodeAsked.delete(f.id))
    }
    return ''
  }
  return j.status || ''
}
function tcProgress(f) {
  const j = transcodeData[f.id]
  if (!j) return '0%'
  const p = j.progress
  if (p > 0) return Math.round(p * 100) + '%'
  if (p < 0) return '…'
  return '0%'
}
function pollTranscode(id) {
  if (transcodeTimers[id]) return
  transcodeTimers[id] = setInterval(() => {
    api
      .transcodeStatus(id)
      .then((r) => {
        transcodeData[id] = r || { status: '' }
        transcodeTick.value++
        const s = r && r.status
        if (s === 'done' || s === 'failed' || s === 'canceled') {
          clearInterval(transcodeTimers[id])
          delete transcodeTimers[id]
          transcodeAsked.delete(id)
        }
      })
      .catch(() => {
        clearInterval(transcodeTimers[id])
        delete transcodeTimers[id]
      })
  }, 1000)
}
function triggerTranscodeFor(f) {
  transcodeAsked.delete(f.id)
  api
    .triggerTranscode(f.id)
    .then((r) => {
      transcodeData[f.id] = r || { status: 'queued' }
      transcodeTick.value++
      pollTranscode(f.id)
    })
    .catch((e) => {
      toastStore.push((e && e.message) || t('转码发起失败'))
    })
}
function cancelTranscodeFor(f) {
  api
    .cancelTranscode(f.id)
    .then(() => {
      if (transcodeData[f.id]) transcodeData[f.id].status = 'canceled'
      transcodeTick.value++
    })
    .catch(() => {})
}

// 首次加载与监听必须放到 onMounted，**不能**在这里直接 watch(visibleItems)：
// 🔴 watch(source) 在**创建时**就会求值一次源（与 immediate 无关），而 visibleItems 依赖的
// sortedItems 在本文件里声明得更靠后 → setup 期求值直接踩 TDZ
// （ReferenceError: Cannot access 'X' before initialization，症状是整页白屏 —— 已实测踩到）。
// onMounted 里 setup 已跑完，所有 const 都已初始化，安全。
onMounted(() => {
  loadMediaMeta()
  watch(visibleItems, () => { loadMediaMeta() })
})

function fileKind(f) {
  if (f.kind === 'dir') return 'dir'
  const e = ext(f.name)
  if (['md', 'markdown', 'txt', 'log', 'html', 'json'].includes(e)) return 'text'
  if (IMAGE_EXT.includes(e)) return 'image'
  if (['mp3', 'wav', 'm4a', 'flac', 'ogg'].includes(e)) return 'audio'
  if (['mp4', 'mov', 'mkv', 'webm', 'avi'].includes(e)) return 'video'
  if (['zip', 'rar', '7z', 'tar', 'gz'].includes(e)) return 'archive'
  if (['pdf'].includes(e)) return 'pdf'
  if (['js', 'ts', 'py', 'go', 'java', 'c', 'cpp', 'css', 'vue', 'sql', 'sh'].includes(e)) return 'code'
  return 'file'
}

function kindIcon(f) {
  if (f.kind === 'dir') return 'folder'
  const map = {
    text: 'fileText',
    image: 'fileImage',
    audio: 'fileAudio',
    video: 'fileVideo',
    archive: 'fileArchive',
    pdf: 'filePdf',
    code: 'fileCode'
  }
  return map[fileKind(f)] || 'file'
}

function formatSize(n) {
  if (!n && n !== 0) return ''
  if (n < 1024) return n + ' B'
  if (n < 1024 * 1024) return (n / 1024).toFixed(1) + ' KB'
  if (n < 1024 * 1024 * 1024) return (n / 1024 / 1024).toFixed(1) + ' MB'
  return (n / 1024 / 1024 / 1024).toFixed(2) + ' GB'
}

function formatTime(ts) {
  if (!ts) return ''
  const d = new Date(ts * 1000)
  const now = new Date()
  const sameDay = d.toDateString() === now.toDateString()
  const pad = (x) => String(x).padStart(2, '0')
  const hm = `${pad(d.getHours())}:${pad(d.getMinutes())}`
  if (sameDay) return hm
  return `${d.getMonth() + 1}月${d.getDate()}日 ${hm}`
}

// ---- 导航 ----
// 单击 = 单选选中（低调高亮，KodExplorer 式）；双击 = 打开
// 全选本页（表头方格，邮箱样式）
const allSelected = computed(
  () => files.items.length > 0 && files.items.every((f) => files.selected.includes(f.id))
)
const someSelected = computed(() => files.items.some((f) => files.selected.includes(f.id)))
function toggleSelectAll() {
  if (allSelected.value) files.selected = []
  else files.selected = files.items.map((f) => f.id)
}

// 单击 / Ctrl / Shift 选择
let lastClickIndex = -1
function onItemClick(f, e) {
  if (cDrag.suppressClick) {
    cDrag.suppressClick = false // 拖拽结束后的残留 click，忽略
    return
  }
  const idx = sortedItems.value.findIndex((x) => x.id === f.id)
  if (e && e.shiftKey && lastClickIndex >= 0 && idx >= 0) {
    // Shift：从锚点到当前行的范围选择
    const from = Math.min(lastClickIndex, idx)
    const to = Math.max(lastClickIndex, idx)
    files.selected = sortedItems.value.slice(from, to + 1).map((x) => x.id)
    return
  }
  if (e && (e.ctrlKey || e.metaKey)) {
    // Ctrl/⌘：逐个切换
    files.toggleSelect(f.id)
    lastClickIndex = idx
    return
  }
  files.selectOne(f.id)
  lastClickIndex = idx
}

// 方格点击：普通/Ctrl=切换；Shift=范围选择（锚点同上一次点选）
function onCheckClick(f, e) {
  const idx = sortedItems.value.findIndex((x) => x.id === f.id)
  if (e && e.shiftKey) {
    if (lastClickIndex >= 0 && idx >= 0) {
      const from = Math.min(lastClickIndex, idx)
      const to = Math.max(lastClickIndex, idx)
      files.selected = sortedItems.value.slice(from, to + 1).map((x) => x.id)
    }
    return
  }
  files.toggleSelect(f.id)
  lastClickIndex = idx
}

function onItemDblClick(f) {
  if (f.kind === 'dir') {
    files.openDir(f.id, f)
    return
  }
  router.push({ path: `/read/${f.id}` })
}

// 类型列文案（KodExplorer 式：文件夹 / md文件 / 文本 / 图片…）
const TYPE_LABELS = {
  dir: t(i18t('文件夹')),
  text: t(i18t('文本')),
  image: t('图片'),
  audio: t('音频'),
  video: t('视频'),
  archive: t('压缩包'),
  pdf: t('PDF 文档'),
  code: t('代码'),
  file: t('文件')
}

function fileTypeLabel(f) {
  if (f.kind === 'dir') return TYPE_LABELS.dir
  const t = fileKind(f)
  if (t === 'text') {
    const e = ext(f.name)
    return e === 'md' || e === 'markdown' ? 'Markdown' : i18t('文本')
  }
  return TYPE_LABELS[t] || t('文件')
}

// ---- 上传 ----
function pickFiles() {
  if (emptyInput.value) emptyInput.value.click()
}

function onPickFiles(e) {
  const list = Array.from(e.target.files || [])
  e.target.value = ''
  if (list.length) doUpload(list.map((f) => ({ path: f.name, file: f })))
}

async function doUpload(list) {
  const n = list.length
  uploadState.value = { percent: 0 }
  const onProg = (loaded, total, pct) => {
    uploadState.value.percent = pct
  }
  try {
    // 资源管理器式：当前目录已有同名的顶层文件 → 暂缓弹窗；其余（含子目录内文件）先传不中断
    const curNames = new Set((files.items || []).map((f) => f.name))
    const dupList = list.filter((x) => !x.path.includes('/') && curNames.has(x.path))
    const okList = list.filter((x) => x.path.includes('/') || !curNames.has(x.path))
    if (okList.length) await files.upload(okList, onProg)
    if (dupList.length) {
      let choice = null
      for (let i = 0; i < dupList.length; i++) {
        if (i === 0 || !choice.applyAll) choice = await showConflictDialog([dupList[i].path])
        if (choice.action === 'skip') continue
        if (choice.action === 'overwrite') {
          // upload 列表元素无 name 字段（onPickFiles 映射 {path,file}），顶层文件 path==name
          const dup = (files.items || []).find((f) => f.name === dupList[i].path)
          if (dup) await api.deleteFile(dup.id)
        }
        await files.upload([dupList[i]], onProg)
      }
    }
    if (n > 1) showToast(`已上传 ${n} 个文件`)
    else showToast(t('已上传'))
  } catch (err) {
    files.error = err.message
  } finally {
    uploadState.value = null
  }
}

// ---- 自定义拖拽移动（纯鼠标事件，兼容 WebView/桌面，不依赖 HTML5 DnD） ----
// 行/卡片 mousedown 启动；移动超阈值进入拖拽（悬浮卡片跟随 + 目标高亮）；mouseup 落在文件夹/树节点即移动。
let cDrag = { ids: null, moved: false, startX: 0, startY: 0, ghost: null }
// 文件拖拽期间禁止文字选择与原生文本/图片拖拽（img 缩略图除外——不启动文件拖拽，保留原生图片拖出）
function preventSelectStart(e) {
  e.preventDefault()
}
function preventNativeDrag(e) {
  if (cDrag.ids && cDrag.ids.length && !cDrag.moved) return
  if (cDrag.ids && cDrag.ids.length) e.preventDefault()
}
function onRowMouseDown(e, f) {
  if (e.button !== 0) return
  // 交互控件（按钮/勾选/菜单/列宽）不启动拖拽
  if (e.target.closest('button, a, input, .col-resize, .ctx-menu, .grid-ops, .row-more')) return
  // 缩略图保留浏览器原生图片拖拽（拖出保存）；其余行内任意位置（含文件名）按住拖 = 移动文件
  if (e.target.closest('img')) return
  // 支持多选拖拽：当前行已在选中集合 → 拖整个集合；否则拖当前行
  const inGroup = files.selected.includes(f.id) && files.selected.length > 0
  if (inGroup) {
    // 组拖：行已勾选，意图明确是移动文件 → 本操作直接禁止文字选择（不产生蓝底）
    e.preventDefault()
    document.addEventListener('selectstart', preventSelectStart, true)
    cDrag.ids = [...files.selected]
  } else {
    cDrag.ids = [f.id]
  }
  cDrag.moved = false
  cDrag.startX = e.clientX
  cDrag.startY = e.clientY
  document.addEventListener('mousemove', onDocMouseMove)
  document.addEventListener('mouseup', onDocMouseUp)
}
function onDocMouseMove(e) {
  if (!cDrag.ids || !cDrag.ids.length) return
  if (!cDrag.moved) {
    // 位移阈值：正常点击的手抖/微小移动不视为拖拽
    if (Math.abs(e.clientX - cDrag.startX) < 15 && Math.abs(e.clientY - cDrag.startY) < 15) return
    cDrag.moved = true
    // 清除移动过程中产生的文字选择蓝底，并禁止拖拽期间的原生文本/图片拖拽
    if (window.getSelection && window.getSelection().removeAllRanges) {
      window.getSelection().removeAllRanges()
    }
    document.addEventListener('selectstart', preventSelectStart, true)
    document.addEventListener('dragstart', preventNativeDrag, true)
    const ghost = document.createElement('div')
    ghost.className = 'drag-ghost'
    ghost.innerHTML =
      '<span class="dg-ic"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"/></svg></span><span class="dg-name"></span>'
    if (cDrag.ids.length > 1) {
      ghost.querySelector('.dg-name').textContent = `${cDrag.ids.length} 个项目`
    } else {
      const f = files.items.find((x) => x.id === cDrag.ids[0])
      ghost.querySelector('.dg-name').textContent = f ? f.name : ''
    }
    document.body.appendChild(ghost)
    cDrag.ghost = ghost
    document.body.classList.add('dragging')
  }
  if (cDrag.ghost) {
    cDrag.ghost.style.left = e.clientX + 14 + 'px'
    cDrag.ghost.style.top = e.clientY + 10 + 'px'
  }
  // 命中检测：列表/网格文件夹行 或 侧栏目录树节点 → 高亮
  const el = document.elementFromPoint(e.clientX, e.clientY)
  let target = null
  if (el) {
    const row = el.closest('[data-kind="dir"]')
    if (row) target = row.getAttribute('data-fid')
  }
  if (dropTargetId.value !== target) dropTargetId.value = target
}
function onDocMouseUp() {
  document.removeEventListener('mousemove', onDocMouseMove)
  document.removeEventListener('mouseup', onDocMouseUp)
  document.removeEventListener('selectstart', preventSelectStart, true)
  document.removeEventListener('dragstart', preventNativeDrag, true)
  const ids = cDrag.ids
  const moved = cDrag.moved
  cDrag.ids = null
  cDrag.moved = false
  if (cDrag.ghost) {
    cDrag.ghost.remove()
    cDrag.ghost = null
    document.body.classList.remove('dragging')
  }
  const target = dropTargetId.value
  dropTargetId.value = null
  if (!moved) return // 未移动视为点击，交给 click 事件
  cDrag.suppressClick = true // 拖拽后残留 click 忽略（防误选中），80ms 后自动清空防残留
  setTimeout(() => {
    cDrag.suppressClick = false
  }, 80)
  if (!target || !ids || !ids.length) return
  if (ids.length === 1 && target === ids[0]) return
  moveFiles(ids, target)
}
// 批量转移（剪切/复制粘贴核心）。返回 { done, failed, skipped }：
//   skipped：剪切且目标==源所在位置（原地移动，不调用接口，避免"提示成功却无变化"）；
//   failed：单项失败（收集错误但不中断后续项）。
async function batchTransfer(ids, names, targetParentId, op, srcParents) {
  let targetItems = []
  try {
    targetItems = await api.listFiles(targetParentId)
  } catch (err) {
    targetItems = files.items || []
  }
  const targetNames = new Set(targetItems.map((f) => f.name))
  const pend = [] // 冲突待询问 { id, name, dup }
  const ok = [] // 不冲突直接处理
  for (let i = 0; i < ids.length; i++) {
    const f = files.items.find((x) => x.id === ids[i])
    const name = (names && names[i]) || (f && f.name) || ''
    if (!name) {
      ok.push(ids[i])
      continue
    }
    if (targetNames.has(name)) {
      // 同名是源自身（复制/移动到当前位置）不算冲突，交给后端自动重命名
      const dup = targetItems.find((x) => x.name === name)
      if (dup && ids.includes(dup.id)) {
        ok.push(ids[i])
      } else {
        pend.push({ id: ids[i], name, dup })
      }
    } else {
      ok.push(ids[i])
    }
  }
  let done = 0
  let failed = 0
  let skipped = 0
  const errs = []
  // 不冲突的先处理（不中断）
  for (const id of ok) {
    // 原地移动检测：剪切到自身所在目录 → 跳过
    if (srcParents) {
      const idx = ids.indexOf(id)
      const src = (srcParents[idx] || '') === (targetParentId || '')
      if (src) {
        skipped++
        continue
      }
    }
    try {
      await op(id)
      done++
    } catch (err) {
      failed++
      errs.push(err.message)
    }
  }
  // 冲突的弹窗询问（资源管理器式：逐个问，勾选"应用到全部"后同一选择处理剩余）
  let choice = null
  for (let i = 0; i < pend.length; i++) {
    const p = pend[i]
    if (i === 0 || !choice.applyAll) {
      choice = await showConflictDialog([p.name])
    }
    if (choice.action === 'skip') continue
    if (choice.action === 'overwrite' && p.dup) {
      try {
        await api.deleteFile(p.dup.id)
      } catch (err) {
        errs.push(err.message)
      }
    }
    try {
      await op(p.id)
      done++
    } catch (err) {
      failed++
      errs.push(err.message)
    }
  }
  await files.refresh()
  await files.loadTree()
  if (errs.length && !files.error) files.error = errs[0]
  return { done, failed, skipped }
}
async function moveFiles(ids, targetParentId) {
  const names = ids
    .map((id) => {
      const f = files.items.find((x) => x.id === id)
      return f ? f.name : null
    })
    .filter((n) => n !== null)
  const st = await batchTransfer(ids, names, targetParentId, (id) => api.moveFile(id, targetParentId), ids.map((id) => { const f = files.items.find((x) => x.id === id); return f ? (f.parent_id || '') : '' }))
  if (st.failed > 0) {
    showToast(`移动完成：${st.done} 项成功，${st.failed} 项失败`)
  } else if (st.done > 0) {
    showToast(
      st.done > 1
        ? `已移动 ${st.done} 个项目到目标文件夹`
        : `已移动${names[0] ? `「${names[0]}」` : ''}到目标文件夹`
    )
  } else if (st.skipped > 0) {
    showToast(`所选项目已在目标位置（${st.skipped} 项）`)
  }
}
async function moveFileTo(id, targetParentId) {
  return moveFiles([id], targetParentId)
}

// ---- 拖拽上传（window 级监听 + 文件夹递归） ----
let dragCounter = 0
const uploadState = ref(null) // { percent }
const dropTargetId = ref(null) // 拖拽移动悬停目标文件夹行
function onDragOver(e) {
  e.preventDefault()
  dragCounter++
  dragging.value = true
}
function onDragLeave(e) {
  e.preventDefault()
  dragCounter--
  if (dragCounter <= 0) {
    dragCounter = 0
    dragging.value = false
  }
}
function onDrop(e) {
  e.preventDefault()
  dragCounter = 0
  dragging.value = false
  collectDropped(e.dataTransfer).then((list) => {
    if (list.length) doUpload(list)
  })
}
// 递归收集拖入的文件/文件夹（保留目录结构，webkitGetAsEntry；不支持时降级 files）
function collectDropped(dt) {
  return new Promise((resolve) => {
    const out = []
    const items = dt && dt.items ? Array.from(dt.items).filter((i) => i.kind === 'file') : []
    if (!items.length) {
      // 旧浏览器降级：仅文件
      resolve(Array.from((dt && dt.files) || []).map((f) => ({ path: f.name, file: f })))
      return
    }
    let pending = items.length
    if (!pending) {
      resolve(out)
      return
    }
    const done = () => {
      if (--pending <= 0) resolve(out)
    }
    items.forEach((it) => {
      const entry = it.webkitGetAsEntry ? it.webkitGetAsEntry() : null
      if (entry && entry.isDirectory) {
        // 保留文件夹名作为路径前缀（拖入文件夹 → 当前目录出现该文件夹）
        walkDir(entry, entry.name, out, done)
      } else if (entry && entry.isFile) {
        entry.file((f) => {
          out.push({ path: f.name, file: f })
          done()
        }, done)
      } else {
        const f = it.getAsFile && it.getAsFile()
        if (f) out.push({ path: f.name, file: f })
        done()
      }
    })
  })
}
function walkDir(entry, base, out, done) {
  const reader = entry.createReader()
  const all = []
  const readBatch = () => {
    reader.readEntries(
      (batch) => {
        if (!batch.length) {
          let sub = 0
          const subDone = () => {
            if (--sub <= 0) done()
          }
          all.forEach((e) => {
            const p = base ? base + '/' + e.name : e.name
            if (e.isDirectory) {
              sub++
              walkDir(e, p, out, subDone)
            } else {
              sub++
              e.file((f) => {
                out.push({ path: p, file: f })
                subDone()
              }, subDone)
            }
          })
          if (!sub) done()
          return
        }
        all.push(...batch)
        readBatch()
      },
      () => done()
    )
  }
  readBatch()
}
function bindDragListeners() {
  window.addEventListener('dragover', onDragOver)
  window.addEventListener('dragleave', onDragLeave)
  window.addEventListener('drop', onDrop)
}
function unbindDragListeners() {
  window.removeEventListener('dragover', onDragOver)
  window.removeEventListener('dragleave', onDragLeave)
  window.removeEventListener('drop', onDrop)
}

// ---- 其他操作 ----
function openMkdir() {
  newDirName.value = ''
  showMkdir.value = true
}

async function confirmMkdir() {
  const name = newDirName.value.trim()
  if (!name) return
  try {
    await files.mkdir(name)
    showMkdir.value = false
  } catch (err) {
    files.error = err.message
  }
}

// ---- 新建文档 ----
function openCreateDoc() {
  docName.value = ''
  docContent.value = ''
  showDoc.value = true
  setTimeout(() => docNameInput.value?.focus(), 50)
}

async function confirmDoc() {
  const name = docName.value.trim()
  if (!name) return
  try {
    const f = await api.createDoc(name, files.currentParent, docContent.value)
    showDoc.value = false
    await files.refresh()
    await files.loadTree()
    files.error = ''
    showToast(`已创建「${f.name}」`)
    // 有内容直接进编辑/阅读；空文档也进阅读页便于继续写
    router.push({ path: `/read/${f.id}` })
  } catch (err) {
    files.error = err.message
  }
}

// ---- 行操作菜单 ----
function toggleMenu(id) {
  if (menuFor.value === id) {
    menuFor.value = ''
    return
  }
  menuFor.value = id
  // fixed 定位：按按钮真实位置放置（避开 .list overflow:hidden 裁切）
  // 列表/网格按钮 DOM 不同（.row-menu 与 .grid-ops），全局按 [data-mid] 查找
  const btn = document.querySelector('.icon-btn[data-mid="' + id + '"]')
  if (btn) {
    const r = btn.getBoundingClientRect()
    menuPos.value = { top: r.bottom + 6, left: Math.max(12, r.right - 170) }
  }
}
// 真·鼠标右键（contextmenu）弹出行菜单：定位到光标，按视口收边。
// 与「⋯」按钮共享 menuFor / menuPos / FileMenu，仅定位来源不同。
function openMenuAt(id, x, y) {
  menuFor.value = id
  const w = 184
  const h = 400
  let left = x
  let top = y
  if (left + w > window.innerWidth) left = Math.max(8, window.innerWidth - w - 8)
  if (top + h > window.innerHeight) top = Math.max(8, window.innerHeight - h - 8)
  menuPos.value = { top: Math.max(8, top), left: Math.max(8, left) }
}
function onRowContextMenu(f, e) {
  e.preventDefault()
  // 右键即选中：目标不在当前选中集合时替换为单选项；落在多选组内则保留整组选中
  if (!files.selected.includes(f.id)) {
    files.selected = [f.id]
  }
  openMenuAt(f.id, e.clientX, e.clientY)
}
function closeMenu() {
  menuFor.value = ''
}
function menuOpen(f) {
  closeMenu()
  onItemDblClick(f)
}
function menuDownload(f) {
  closeMenu()
  if (f.kind === 'dir') return
  api.fetchFileBlob(f.id).then((blob) => {
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = f.name
    a.click()
    setTimeout(() => URL.revokeObjectURL(url), 30000)
  }).catch((e) => showToast(e.message || t('下载失败')))
}
function menuCopy(f) {
  closeMenu()
  if (f.kind === 'dir') {
    files.error = i18t('目录复制将在阶段 3 提供')
    return
  }
  clipboard.value = { ids: [f.id], cut: false, names: [f.name] }
  files.error = ''
  showToast(`已复制「${f.name}」，去目标位置粘贴`)
}
function menuCut(f) {
  closeMenu()
  clipboard.value = { ids: [f.id], cut: true, names: [f.name] }
  files.error = ''
  showToast(`已剪切「${f.name}」，去目标位置粘贴`)
}
// 标记为草稿（进草稿箱；文本/文档类才有意义，图片等也可标记但草稿箱语义以内容为主）
async function menuToDraft(f) {
  closeMenu()
  try {
    await api.setFileStatus(f.id, 'draft')
    files.error = ''
    showToast(`「${f.name}」已转为草稿，可在草稿箱查看`)
  } catch (err) {
    files.error = t('操作失败：') + err.message
  }
}
// 标签 / 分类（网盘打标 → 知识库/博客/检索共用）
const tagFile = ref(null)
function menuTags(f) {
  closeMenu()
  tagFile.value = f
}
function onTagsSaved() {
  files.refresh()
}
function menuRename(f) {
  closeMenu()
  renameTarget.value = f
  renameName.value = f.name
  showRename.value = true
  setTimeout(() => renameInput.value?.focus(), 50)
}
function menuShare(f) {
  closeMenu()
  files.error = ''
  shareState.value = { id: f.id, name: f.name, kind: f.kind || 'file', url: '', creating: false, revoking: false, fileCount: null }
}
// 已整体分享的目录 id 集合（列表角标：该目录文件已公开到博客）
const sharedDirIds = ref(new Set())
async function refreshSharedDirs() {
  try {
    const items = await api.sharesList()
    const s = new Set()
    for (const it of items) {
      if (it.scope === 'dir' && it.status === 'active' && it.file?.id) s.add(it.file.id)
    }
    sharedDirIds.value = s
  } catch (e) { /* 静默：角标是增强信息 */ }
}
// 分享/发布（博客公开页）：创建链接 → 复制/预览/撤销
const shareState = ref(null)
async function createShareUrl() {
  const st = shareState.value
  if (!st || st.creating) return
  st.creating = true
  try {
    const r = st.kind === 'dir'
      ? await api.createDirShare(st.id, true)
      : await api.createShare(st.id)
    st.url = shareUrl(r.url)
    st.fileCount = r.file_count != null ? r.file_count : null
    if (st.kind === 'dir') refreshSharedDirs()
  } catch (err) {
    files.error = t('创建分享失败：') + (err.message || err)
  } finally {
    st.creating = false
  }
}
function shareUrl(path) {
  return `${location.protocol}//${location.host}${location.pathname}#${path}`
}
function copyShareUrl() {
  const st = shareState.value
  if (!st?.url) return
  const input = st.url
  // 剪贴板 API；失败时选中链接提示手动复制（不触发系统级弹窗）
  tryCopy(input).then((ok) => {
    if (ok) {
      showToast(t('分享链接已复制'))
    } else {
      const el = shareInput.value
      if (el) {
        el.focus()
        el.select()
      }
      showToast(t('浏览器限制了自动复制，请按 Ctrl+C 复制'))
    }
  })
}
function openSharePage() {
  const st = shareState.value
  if (!st?.url) return
  window.open(st.url, '_blank')
}
async function revokeShare() {
  const st = shareState.value
  if (!st?.url || st.revoking) return
  st.revoking = true
  try {
    const token = new URL(st.url).hash.replace(/^#\/p\//, '')
    await api.revokeShare(token)
    showToast(t('已撤销分享，链接已失效'))
    if (st.kind === 'dir') refreshSharedDirs()
    shareState.value = null
  } catch (err) {
    files.error = t('撤销失败：') + (err.message || err)
  } finally {
    if (st.revoking !== undefined) st.revoking = false
  }
}
// OCR 识别（模块组件：图片 → 文字，可一键存为文档进知识库）
const ocrState = ref(null)
const ocrLoading = ref(false)
const ocrSaving = ref(false)
const ocrCleaning = ref(false)
async function menuOcr(f) {
  closeMenu()
  ocrState.value = { id: f.id, name: f.name, text: '' }
  ocrLoading.value = true
  try {
    const r = await api.ocrImage({ file_id: f.id })
    ocrState.value.text = r.text || ''
  } catch (err) {
    ocrState.value.text = ''
    files.error = t('OCR 识别失败：') + (err.message || err)
  } finally {
    ocrLoading.value = false
  }
}
// AI 整理：去 HTML/代码噪声，整理为干净 Markdown（可选，知识库入库更整洁）
async function cleanOcrText() {
  const st = ocrState.value
  if (!st || !st.text) return
  ocrCleaning.value = true
  try {
    const r = await api.cleanText(st.text, 'ocr')
    st.text = r.text || st.text
  } catch (err) {
    files.error = t('整理失败：') + (err.message || err)
  } finally {
    ocrCleaning.value = false
  }
}
async function saveOcrDoc() {
  const st = ocrState.value
  if (!st || !st.text) return
  ocrSaving.value = true
  try {
    const base = st.name.replace(/\.\w+$/, '')
    const r = await api.createDoc(`${base}-OCR.md`, files.currentParent, st.text)
    ocrSaving.value = false
    ocrState.value = null
    await files.refresh()
    showToast(`已保存为文档「${r.name}」，可编辑 / 加入知识库`)
  } catch (err) {
    ocrSaving.value = false
    files.error = t('保存失败：') + (err.message || err)
  }
}
async function menuDelete(f) {
  closeMenu()
  askConfirm(
    t('删除文件'),
    `确定删除「${f.name}」？删除后可在回收站恢复。`,
    async () => {
      try {
        await files.remove([f.id])
      } catch (err) {
        files.error = err.message
      }
    }
  )
}

// 粘贴（剪贴板）—— 工具栏按钮调用；支持多选 ids
function hasClipboard() {
  return !!clipboard.value
}
async function pasteClipboard() {
  if (!clipboard.value) return
  const c = clipboard.value
  const ids = c.ids || []
  if (!ids.length) {
    clipboard.value = null
    return
  }
  try {
    const st = await batchTransfer(ids, c.names || [], files.currentParent, (id) =>
      c.cut ? api.moveFile(id, files.currentParent) : api.copyFile(id, files.currentParent),
      c.cut ? (c.parents || []) : null
    )
    clipboard.value = null
    files.error = ''
    if (st.failed > 0) {
      showToast(`粘贴完成：${st.done} 项成功，${st.failed} 项失败`)
    } else if (st.done > 0) {
      showToast(`粘贴完成：${st.done} 项`)
    } else if (st.skipped > 0) {
      showToast(`所选项目已在目标位置（${st.skipped} 项）`)
    } else {
      showToast(t('粘贴完成'))
    }
  } catch (err) {
    files.error = err.message
  }
}

// ---- 列表排序（资源管理器式：目录优先 + 点击表头切换） ----
const sortKey = ref('name')
const sortDir = ref(1) // 1 升序，-1 降序

function toggleSort(key) {
  if (sortKey.value === key) {
    sortDir.value *= -1
  } else {
    sortKey.value = key
    sortDir.value = 1
  }
}

const sortedItems = computed(() => {
  const arr = [...files.items]
  const dir = sortDir.value
  arr.sort((a, b) => {
    if (a.kind !== b.kind) return a.kind === 'dir' ? -1 : 1
    let r = 0
    if (sortKey.value === 'name') r = a.name.localeCompare(b.name, 'zh')
    else if (sortKey.value === 'size') r = (a.size || 0) - (b.size || 0)
    else if (sortKey.value === 'time') r = (a.updated_at || 0) - (b.updated_at || 0)
    else if (sortKey.value === 'type') r = fileTypeLabel(a).localeCompare(fileTypeLabel(b), 'zh')
    return r * dir
  })
  return arr
})

// ---- 列宽拖拽调节（资源管理器式，持久化） ----
const DEFAULTS_COLS = { name: 200, type: 110, size: 80, time: 130 }
const colWidths = ref(loadColWidths())

function loadColWidths() {
  try {
    return { ...DEFAULTS_COLS, ...JSON.parse(localStorage.getItem('aikmap.colwidths') || '{}') }
  } catch (_) {
    return { ...DEFAULTS_COLS }
  }
}

function colStyle() {
  return {
    '--col-name': colWidths.value.name + 'px',
    '--col-type': colWidths.value.type + 'px',
    '--col-size': colWidths.value.size + 'px',
    '--col-time': colWidths.value.time + 'px'
  }
}

let resizeCtx = null
const resizingCol = ref('')
const resizeLineX = ref(0)
function startResize(e, key) {
  const startX = e.clientX
  const startW = colWidths.value[key]
  resizeCtx = { key, startX, startW }
  resizingCol.value = key
  resizeLineX.value = e.clientX
  document.body.style.cursor = 'col-resize'
  document.body.style.userSelect = 'none'
  document.addEventListener('mousemove', onResizeMove)
  document.addEventListener('mouseup', stopResize)
}
function onResizeMove(e) {
  if (!resizeCtx) return
  const { key, startX, startW } = resizeCtx
  const w = Math.min(420, Math.max(60, startW + (e.clientX - startX)))
  colWidths.value[key] = w
  resizeLineX.value = e.clientX
  localStorage.setItem('aikmap.colwidths', JSON.stringify(colWidths.value))
}
function stopResize() {
  resizeCtx = null
  resizingCol.value = ''
  document.body.style.cursor = ''
  document.body.style.userSelect = ''
  document.removeEventListener('mousemove', onResizeMove)
  document.removeEventListener('mouseup', stopResize)
}

// ---- 工具栏选取后操作（KodExplorer 式：选中时出现） ----
function selectedItems() {
  return files.items.filter((f) => files.selected.includes(f.id))
}
function toolbarCopy() {
  const sel = selectedItems()
  if (!sel.length) return
  const dirs = sel.filter((f) => f.kind === 'dir')
  const filesSel = sel.filter((f) => f.kind !== 'dir')
  if (dirs.length) files.error = `目录「${dirs[0].name}」复制将在阶段 3 提供`
  if (!filesSel.length) return
  clipboard.value = { ids: filesSel.map((f) => f.id), cut: false, names: filesSel.map((f) => f.name), parents: filesSel.map((f) => f.parent_id || '') }
  files.clearSelect()
  showToast(`已复制 ${filesSel.length} 项，去目标位置粘贴`)
}
function toolbarCut() {
  const sel = selectedItems()
  if (!sel.length) return
  clipboard.value = { ids: sel.map((f) => f.id), cut: true, names: sel.map((f) => f.name), parents: sel.map((f) => f.parent_id || '') }
  files.clearSelect()
  showToast(`已剪切 ${sel.length} 项，去目标位置粘贴`)
}
function toolbarRename() {
  const sel = selectedItems()
  if (sel.length !== 1) return
  menuRename(sel[0])
}
async function toolbarDelete() {
  const sel = selectedItems()
  if (!sel.length) return
  askConfirm(
    t('删除选中项'),
    `确定删除选中的 ${sel.length} 项？删除后可在回收站恢复。`,
    async () => {
      try {
        await files.remove(sel.map((f) => f.id))
        showToast(t(i18t('已删除')))
      } catch (err) {
        files.error = err.message
      }
    }
  )
}

// ---- AI 整理（P0-3）----
const showOrganize = ref(false)
const organizeIds = ref([])
function openOrganize() {
  const sel = selectedItems()
  if (!sel.length) return
  organizeIds.value = sel.map((f) => f.id)
  showOrganize.value = true
}
function onOrganizeApplied() {
  showToast(t('AI 整理已应用，正在刷新…'))
  showOrganize.value = false
  files.refresh()
  files.loadTree()
}
function openOrganizeTarget(id) {
  showOrganize.value = false
  router.push({ path: `/read/${id}` })
}

async function confirmRename() {
  const name = renameName.value.trim()
  if (!name || !renameTarget.value) return
  try {
    await api.moveFile(renameTarget.value.id, files.currentParent, name)
    showRename.value = false
    renameTarget.value = null
    await files.refresh()
    await files.loadTree()
    files.error = ''
  } catch (err) {
    files.error = err.message
  }
}

onMounted(async () => {
  bindDragListeners()
  document.addEventListener('click', onDocClick)
  // 键盘快捷键（捕获阶段，先于行内元素处理）
  window.addEventListener('keydown', onGlobalKeydown, true)
  if (!files.items.length && !files.loading) {
    await files.openDir(files.currentParent)
  }
  if (!files.tree.length) await files.loadTree()
  refreshSharedDirs()
  // 标签筛选下拉
  api
    .listTags()
    .then((ts) => { allTags.value = ts || [] })
    .catch(() => {})
  // 侧栏标签树等 ?tag= 直达：路由 query 驱动标签筛选
  const qtag = route.query.tag
  if (qtag) {
    tagFilter.value = String(qtag)
    await applyTagFilter()
  }
})

// 侧栏标签树点击 / 其他入口跳 /files?tag=xxx → 即时切换筛选
watch(
  () => route.query.tag,
  (v) => {
    const id = v ? String(v) : ''
    if (id === tagFilter.value) return
    tagFilter.value = id
    if (id) applyTagFilter()
    else files.refresh()
  }
)

// ---- 键盘快捷键（对齐上游「文件管理右键菜单与键盘快捷键」）----
// Ctrl/⌘ + C 复制、X 剪切、V 粘贴、A 全选；Delete 删除、F2 重命名、Enter 打开、Esc 逐层关闭。
// 任何弹窗打开时不抢快捷键（交由弹窗自身处理），输入类元素聚焦时完全不拦截。
function anyModalOpen() {
  return !!(
    confirmState.value ||
    conflict.value ||
    shareState.value ||
    ocrState.value ||
    showOrganize.value ||
    showRename.value ||
    showMkdir.value ||
    showDoc.value
  )
}
function onGlobalKeydown(e) {
  const t = e.target
  // 输入框 / 文本域 / 下拉 / 可编辑元素聚焦时不拦截（保留浏览器与输入法默认行为）
  if (t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.tagName === 'SELECT' || t.isContentEditable)) return
  const mod = e.ctrlKey || e.metaKey
  if (mod) {
    const k = (e.key || '').toLowerCase()
    if (k !== 'c' && k !== 'x' && k !== 'v' && k !== 'a') return
    if (anyModalOpen()) return
    if (k === 'a') {
      const list = visibleItems.value.map((f) => f.id)
      if (!list.length) return
      e.preventDefault()
      files.selected = list
      return
    }
    if (k === 'v') {
      if (!hasClipboard()) return
      e.preventDefault()
      pasteClipboard()
      return
    }
    // C / X：未选中任何文件时不抢占浏览器默认的文本复制
    if (!files.selected.length) return
    e.preventDefault()
    if (k === 'c') toolbarCopy()
    else toolbarCut()
    return
  }
  switch (e.key) {
    case 'Delete':
      if (anyModalOpen() || !files.selected.length) return
      e.preventDefault()
      toolbarDelete()
      break
    case 'F2':
      if (anyModalOpen() || files.selected.length !== 1) return
      e.preventDefault()
      toolbarRename()
      break
    case 'Enter': {
      if (anyModalOpen() || files.selected.length !== 1) return
      const f = files.items.find((x) => x.id === files.selected[0])
      if (!f) return
      e.preventDefault()
      onItemDblClick(f)
      break
    }
    case 'Escape':
      // 弹窗优先于菜单/选中：Esc 逐层关闭
      if (confirmState.value) confirmState.value = null
      else if (conflict.value) closeConflict('skip')
      else if (shareState.value) shareState.value = null
      else if (ocrState.value) ocrState.value = null
      else if (showOrganize.value) showOrganize.value = false
      else if (showRename.value) showRename.value = false
      else if (showMkdir.value) showMkdir.value = false
      else if (showDoc.value) showDoc.value = false
      else if (menuFor.value) menuFor.value = ''
      else if (moreOpen.value) moreOpen.value = null
      else if (files.selected.length) files.clearSelect()
      break
  }
}

function onDocClick(e) {
  // 点击 +N 弹层外部 → 关闭
  if (moreOpen.value && !(e.target.closest && e.target.closest('.row-tag.more'))) {
    moreOpen.value = null
  }
  if (!menuFor.value) return
  // 点击菜单/按钮内部不关闭
  if (e.target.closest && e.target.closest('.row-menu')) return
  menuFor.value = ''
}

onUnmounted(() => {
  unbindDragListeners()
  window.removeEventListener('keydown', onGlobalKeydown, true)
  document.removeEventListener('mousemove', onDocMouseMove)
  document.removeEventListener('mouseup', onDocMouseUp)
  // F1 修复：转码轮询定时器此前未清理 —— 离开文件页后仍每秒打 /files/{id}/transcode，
  // 既泄漏定时器又持续产生无效请求（登出后还会反复触发 401）。
  for (const id of Object.keys(transcodeTimers)) {
    clearInterval(transcodeTimers[id])
    delete transcodeTimers[id]
  }
  if (cDrag.ghost) {
    cDrag.ghost.remove()
    cDrag.ghost = null
  }
  document.body.classList.remove('dragging')
  document.removeEventListener('click', onDocClick)
  dragCounter = 0
})
</script>

<style scoped>
.files-view {
  position: relative;
  min-height: 100%;
  max-width: 1200px;
  margin: 0 auto;
  padding: 20px 24px 40px;
}

/* 工具栏（面包屑已移到顶栏，这里仅操作按钮） */
.toolbar {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 16px;
  padding: 14px 0 12px;
  position: sticky;
  top: 0;
  background: var(--bg);
  z-index: 5;
}

.toolbar-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
  margin-left: auto;
}

.toolbar-sep {
  width: 1px;
  height: 18px;
  background: var(--border);
  margin: 0 4px;
}

.tag-filter {
  max-width: 190px;
  height: 30px;
  padding: 0 8px;
  border: 1px solid var(--border);
  border-radius: 6px;
  background: var(--surface-2);
  color: var(--text-2);
  font-size: 12.5px;
  outline: none;
  cursor: pointer;
}
.tag-filter:focus {
  border-color: var(--primary);
}

.btn-danger-ghost {
  color: var(--danger);
  border-color: var(--border);
}

.btn-danger-ghost:hover {
  background: var(--danger-soft);
  border-color: var(--danger);
}

.btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 7px 14px;
  border-radius: var(--radius-sm);
  border: 1px solid var(--border);
  background: var(--surface);
  color: var(--text);
  font-size: var(--fs-base);
  transition: background 0.15s ease, border-color 0.15s ease;
}

.btn:hover {
  background: var(--surface-2);
  border-color: var(--text-3);
}

.btn-primary {
  background: var(--primary);
  border-color: var(--primary);
  color: #fff;
}

.btn-primary:hover {
  opacity: 0.92;
  background: var(--primary);
}

.btn-danger {
  color: var(--danger);
  border-color: rgba(239, 68, 68, 0.35);
}

.btn-danger:hover {
  background: var(--danger-soft);
  border-color: var(--danger);
}

.btn-sm {
  padding: 5px 10px;
  font-size: var(--fs-small);
}

.view-switch {
  display: flex;
  gap: 2px;
  padding: 3px;
  border-radius: var(--radius-sm);
  background: var(--surface);
  border: 1px solid var(--border);
}

.view-switch .icon-btn.active {
  background: var(--primary-soft);
  color: var(--primary);
}

/* 错误 */
.error-banner {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 14px;
  margin-bottom: 12px;
  border-radius: var(--radius-sm);
  background: var(--danger-soft);
  color: var(--danger);
  font-size: var(--fs-small);
}

.btn-link {
  color: var(--danger);
  text-decoration: underline;
  margin-left: 4px;
}

/* 上传进度条 */
.upload-progress {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 14px;
  margin-bottom: 12px;
  border-radius: var(--radius-sm);
  background: var(--surface-2);
  border: 1px solid var(--border);
  font-size: var(--fs-small);
  color: var(--text-2);
}

.up-bar {
  flex: 1;
  height: 6px;
  border-radius: 3px;
  background: var(--border);
  overflow: hidden;
}

.up-fill {
  height: 100%;
  border-radius: 3px;
  background: var(--primary);
  transition: width 0.15s ease;
}

.up-text {
  min-width: 42px;
  text-align: right;
  font-variant-numeric: tabular-nums;
  color: var(--primary);
}

/* 加载/空态 */
.loading-state,
.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 90px 20px;
  color: var(--text-3);
  gap: 10px;
}

.spinner {
  width: 26px;
  height: 26px;
  border: 3px solid var(--border);
  border-top-color: var(--primary);
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

.empty-icon {
  width: 64px;
  height: 64px;
  border-radius: 50%;
  background: var(--primary-soft);
  color: var(--primary);
  display: flex;
  align-items: center;
  justify-content: center;
}

.empty-title {
  font-size: var(--fs-large);
  font-weight: 600;
  color: var(--text);
}

.empty-desc {
  color: var(--text-3);
  max-width: 320px;
  text-align: center;
}

/* 网格 */
.grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
  gap: 12px;
}

.grid-item {
  position: relative;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: var(--surface);
  padding: 12px;
  cursor: pointer;
  transition: box-shadow 0.15s ease, border-color 0.15s ease, transform 0.1s ease;
}

.grid-item:hover {
  box-shadow: var(--shadow);
  border-color: var(--text-3);
}

.grid-item.selected {
  border-color: var(--primary);
  box-shadow: 0 0 0 1px var(--primary);
}

/* 网格拖拽移动：目标文件夹高亮 */
.grid-item.drop-target {
  border-color: var(--primary);
  outline: 2px dashed var(--primary);
  outline-offset: 1px;
  background: var(--primary-soft);
}

/* 自定义拖拽悬浮卡片 */
.drag-ghost {
  position: fixed;
  z-index: 99999;
  display: flex;
  align-items: center;
  gap: 8px;
  max-width: 260px;
  padding: 7px 12px 7px 8px;
  border-radius: 8px;
  background: var(--surface);
  border: 1px solid var(--primary);
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.28);
  color: var(--text);
  font-size: 13px;
  pointer-events: none;
  opacity: 0.92;
}
.drag-ghost .dg-ic {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 26px;
  height: 26px;
  border-radius: 6px;
  background: var(--primary-soft);
  color: var(--primary);
  flex-shrink: 0;
}
.drag-ghost .dg-ic svg {
  width: 15px;
  height: 15px;
}
.drag-ghost .dg-name {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
body.dragging {
  cursor: grabbing !important;
  user-select: none;
}

.thumb {
  position: relative;
  height: 96px;
  display: flex;
  align-items: center;
  justify-content: center;
  overflow: hidden;
  border-radius: var(--radius-sm);
  background: var(--surface-2);
  margin-bottom: 10px;
}

/* 视频缩略图角标：纯 CSS 三角，不引图标资源（图标名未登记时会渲染成空白） */
.thumb-play {
  position: absolute;
  right: 6px;
  bottom: 6px;
  width: 0;
  height: 0;
  border-left: 9px solid rgba(255, 255, 255, 0.92);
  border-top: 6px solid transparent;
  border-bottom: 6px solid transparent;
  filter: drop-shadow(0 1px 2px rgba(0, 0, 0, 0.45));
}

/* 视频转码控制（B17）：网格卡片底部一行，不抢缩略图 */
.grid-transcode {
  display: flex;
  align-items: center;
  gap: 6px;
  min-height: 18px;
  padding: 0 8px 6px;
  font-size: var(--fs-xs, 12px);
}
.mini-btn {
  border: 1px solid var(--border-2, #d0d4dc);
  background: var(--surface-2, #f3f4f6);
  color: var(--text-1, #222);
  border-radius: 6px;
  padding: 1px 8px;
  font-size: var(--fs-xs, 12px);
  cursor: pointer;
  line-height: 1.5;
}
.mini-btn:hover {
  background: var(--surface-3, #e6e8ec);
}
.mini-btn.ghost {
  color: var(--text-3, #888);
}
.tc-progress {
  color: var(--text-3, #888);
  white-space: nowrap;
}
.tc-bar {
  flex: 1;
  height: 4px;
  border-radius: 2px;
  background: var(--surface-3, #e6e8ec);
  overflow: hidden;
}
.tc-bar > i {
  display: block;
  height: 100%;
  background: var(--accent, #3b82f6);
}
.tc-done {
  color: var(--ok, #16a34a);
}

/* 列表行里的「分辨率 · 时长」：低对比、不抢文件名 */
.row-media {
  margin-left: 8px;
  font-size: var(--fs-xs, 12px);
  color: var(--text-3);
  white-space: nowrap;
}

.thumb img {
  max-width: 100%;
  max-height: 100%;
  object-fit: contain;
}

.thumb-icon {
  color: var(--text-3);
}

.thumb-icon.kind-dir {
  color: var(--warning);
}
.thumb-icon.kind-text,
.thumb-icon.kind-code {
  color: var(--primary);
}
.thumb-icon.kind-pdf {
  color: var(--danger);
}
.thumb-icon.kind-audio,
.thumb-icon.kind-video {
  color: var(--accent);
}

.item-name {
  font-size: var(--fs-base);
  color: var(--text);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  margin-bottom: 2px;
}

.item-meta {
  font-size: var(--fs-micro);
  color: var(--text-3);
}

/* 选取框：小方框（邮箱式），列表常驻可见 */
.check {
  position: absolute;
  top: 8px;
  right: 8px;
  width: 17px;
  height: 17px;
  border-radius: 4px;
  background: var(--surface);
  border: 1.5px solid var(--border);
  display: flex;
  align-items: center;
  justify-content: center;
  opacity: 0;
  transition: opacity 0.15s ease, background 0.15s ease, border-color 0.15s ease;
  color: #fff;
  cursor: pointer;
}

.grid-item:hover .check,
.grid-item.selected .check,
.grid-item:hover .grid-more {
  opacity: 1;
}

/* 表头全选方格（邮箱样式）：常显、半选横杠 */
.list-head .select-all {
  position: static;
  opacity: 1;
  margin: 1px auto 0;
}
.check.half::before {
  content: '';
  width: 8px;
  height: 2px;
  border-radius: 1px;
  background: #fff;
}

.grid-ops {
  position: absolute;
  top: 6px;
  right: 6px;
  display: flex;
  gap: 4px;
  z-index: 2;
}

.grid-ops .check {
  position: static;
  top: auto;
  right: auto;
}

.grid-more {
  width: 22px;
  height: 22px;
  border-radius: 50%;
  background: var(--surface);
  border: 1.5px solid var(--border);
  color: var(--text-2);
  opacity: 0;
  transition: opacity 0.15s ease, background 0.15s ease, color 0.15s ease;
  display: flex;
  align-items: center;
  justify-content: center;
}

.grid-more:hover {
  background: var(--surface-2);
  color: var(--text);
}

/* 列表视图的选取框：位于独立 col-check 列，常驻可见，hover 加深，勾选主色填充 */
.col-check {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
}

.col-check .check {
  position: static;
  top: auto;
  right: auto;
  width: 17px;
  height: 17px;
  opacity: 0;
  border-color: var(--text-3);
}

.list-row:hover .col-check .check,
.col-check .check.on {
  opacity: 1;
  border-color: var(--primary);
}

.check.on {
  background: var(--primary);
  border-color: var(--primary);
  opacity: 1;
}

/* 列表 */
.list {
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: var(--surface);
  overflow-x: auto;
}

.list-head,
.list-row {
  display: grid;
  grid-template-columns: 26px var(--col-name, 200px) var(--col-type, 110px) var(--col-size, 80px) var(--col-time, 130px) 36px minmax(0, 1fr);
  align-items: center;
  gap: 12px;
  padding: 0 16px;
}

/* 拖拽移动：目标文件夹行高亮 */
.list-row.drop-target {
  background: var(--primary-soft);
  outline: 2px dashed var(--primary);
  outline-offset: -2px;
}

.list-row[draggable="true"] {
  cursor: default;
}

.list-head {
  position: sticky;
  top: 0;
  z-index: 4;
  background: var(--bg);
  padding-top: 2px;
  padding-bottom: 2px;
}

/* 表头排序按钮 */
.th-btn {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  background: none;
  border: none;
  padding: 4px 2px;
  font-size: 12.5px;
  font-weight: 600;
  color: var(--text-2);
  cursor: pointer;
  border-radius: 4px;
  transition: color 0.12s ease;
}

.th-btn:hover {
  color: var(--text);
}

.th-btn.active {
  color: var(--primary);
}

/* 列宽拖拽手柄 */
.col-name,
.col-type,
.col-size,
.col-time {
  position: relative;
}

.col-resize {
  position: absolute;
  top: 0;
  right: -7px;
  width: 14px;
  height: 100%;
  cursor: col-resize;
  z-index: 2;
}

.col-resize::after {
  content: '';
  position: absolute;
  top: 18%;
  bottom: 18%;
  left: 6px;
  width: 2px;
  border-radius: 1px;
  background: transparent;
  transition: background 0.12s ease;
}

.col-resize:hover::after {
  background: var(--primary);
}

/* 拖拽中：当前列高亮 + 贯穿列表的边界线（明确方向与目标列） */
.col-name.resizing,
.col-type.resizing,
.col-size.resizing,
.col-time.resizing {
  background: var(--primary-soft);
  border-radius: 6px;
}

.resize-line {
  position: fixed;
  top: 0;
  bottom: 0;
  width: 2px;
  background: var(--primary);
  pointer-events: none;
  z-index: var(--z-modal);
  box-shadow: 0 0 0 0.5px rgba(0, 0, 0, 0.08);
}

.list-head {
  height: 38px;
  background: var(--surface-2);
  border-bottom: 1px solid var(--border);
  font-size: var(--fs-micro);
  color: var(--text-3);
  text-transform: uppercase;
  letter-spacing: 0.04em;
}

.list-row {
  height: 46px;
  cursor: pointer;
  border-bottom: 1px solid var(--border);
  transition: background 0.12s ease;
}

.list-row:last-child {
  border-bottom: none;
}

.list-row:hover {
  background: var(--surface-2);
}

.list-row.selected {
  background: var(--primary-soft);
}

.col-name {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
  color: var(--text);
}

.col-name .aik-icon {
  color: var(--text-3);
  flex-shrink: 0;
}

.row-name {
  flex: 1 1 auto;
  min-width: 40px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 列表行标签徽章（行内最多 2 个，多余折叠为 +N，避免覆盖文件名） */
.row-tag {
  flex-shrink: 0;
  font-size: 10.5px;
  line-height: 1;
  color: var(--primary);
  background: var(--primary-soft);
  border-radius: 999px;
  padding: 3px 7px;
  max-width: 110px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.row-tag.more {
  position: relative;
  overflow: visible; /* 覆盖 .row-tag 的 overflow:hidden，否则 absolute 弹层被裁剪 */
  max-width: none;
  color: var(--text-2);
  background: var(--bg-2);
  cursor: pointer;
}
/* 折叠标签弹出层：悬停显示可点击标签；::before 桥接区防抖 */
.tag-pop {
  display: none;
  position: absolute;
  top: calc(100% + 8px);
  left: 0;
  z-index: 60;
  width: max-content; /* 按内容自适应：+4 窄、+14 宽，不挤 */
  min-width: 130px;
  max-width: 380px;
  max-height: 220px;
  overflow-y: auto;
  flex-direction: column;
  gap: 2px;
  padding: 5px;
  background: var(--surface, #fff);
  border: 1px solid var(--border, #e4e7ee);
  border-radius: 8px;
  box-shadow: 0 6px 20px rgba(15, 23, 42, 0.14);
}
/* 桥接区：从弹层顶部向上覆盖 8px 间隙+徽章下沿，鼠标滑向弹层不丢 hover */
.tag-pop::before {
  content: '';
  position: absolute;
  top: -22px;
  left: -16px;
  right: -16px;
  height: 22px;
}
.row-tag.more:hover .tag-pop,
.row-tag.more.open .tag-pop {
  display: flex;
}
.tag-pop-item {
  display: block;
  width: 100%;
  text-align: left;
  background: none;
  border: none;
  cursor: pointer;
  font-size: 12px;
  color: var(--text-1, #20242c);
  padding: 5px 9px;
  border-radius: 5px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.tag-pop-item:hover {
  background: var(--primary-soft, #eaf1ff);
  color: var(--primary, #2f6bff);
}

/* 标签筛选分页条 */
.tag-pager {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 16px;
  border-top: 1px solid var(--border, #ececf1);
  font-size: 12.5px;
  color: var(--text-2, #4b5563);
  flex-wrap: wrap;
}
.tag-pager-info { margin-right: auto; }
.tag-pager-size {
  height: 28px;
  padding: 0 8px;
  border-radius: 6px;
  border: 1px solid var(--border, #ececf1);
  background: var(--surface-2, #f5f6f8);
  color: var(--text, #20242c);
  font-size: 12px;
  outline: none;
}
.pager-btn {
  height: 28px;
  padding: 0 12px;
  border-radius: 6px;
  border: 1px solid var(--border, #ececf1);
  background: var(--bg-2, #fff);
  color: var(--text, #20242c);
  font-size: 12px;
  cursor: pointer;
}
.pager-btn:hover:not(:disabled) { border-color: var(--primary, #2f6bff); color: var(--primary, #2f6bff); }
.pager-btn:disabled { opacity: 0.45; cursor: default; }
.tag-pager-cur { font-variant-numeric: tabular-nums; }

.col-type {
  font-size: var(--fs-small);
  color: var(--text-2);
}

.col-size,
.col-time {
  font-size: var(--fs-small);
  color: var(--text-2);
}

.col-action {
  display: flex;
  justify-content: flex-end;
}

/* 弹窗 */
.modal-mask {
  position: fixed;
  inset: 0;
  background: rgba(15, 17, 23, 0.45);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: var(--z-modal);
}

.modal {
  width: 360px;
  background: var(--surface);
  border-radius: var(--radius-lg);
  padding: 22px;
  box-shadow: var(--shadow-lg);
}

.modal h3 {
  font-size: var(--fs-medium);
  margin-bottom: 14px;
}

.modal-input {
  width: 100%;
  padding: 9px 12px;
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  background: var(--surface-2);
  color: var(--text);
  outline: none;
  margin-bottom: 16px;
}

.modal-input:focus {
  border-color: var(--primary);
}

.modal-doc {
  width: 440px;
}

.modal-ocr {
  width: 560px;
}

.modal-share {
  width: 480px;
}

.share-loading {
  padding: 12px 0;
  font-size: 13px;
  color: var(--text-3);
}

.share-url {
  font-family: var(--font-mono);
  font-size: 13px;
  cursor: text;
}

.share-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 16px;
}

/* 文件夹整体分享（强授权） */
.dir-share-warn {
  display: flex;
  gap: 10px;
  margin-top: 12px;
  padding: 12px 14px;
  border: 1px solid color-mix(in srgb, var(--warn) 45%, transparent);
  background: color-mix(in srgb, var(--warn) 10%, transparent);
  border-radius: 8px;
  color: var(--text-1);
  font-size: 13px;
  line-height: 1.65;
}
.dir-share-warn > svg { flex: 0 0 auto; margin-top: 2px; color: var(--warn); }
.dir-share-title { font-weight: 600; margin-bottom: 4px; }
.dir-share-desc { color: var(--text-2); }
.dir-share-tip { margin-top: 6px; color: var(--warn); font-weight: 500; }
.dir-share-count {
  margin-top: 12px;
  padding: 8px 12px;
  border-radius: 6px;
  background: color-mix(in srgb, var(--accent) 8%, transparent);
  color: var(--text-2);
  font-size: 13px;
}
.dir-share-count b { color: var(--text-1); }
.dir-share-badge {
  display: inline-block;
  margin-left: 6px;
  padding: 1px 6px;
  border-radius: 4px;
  font-size: 11px;
  line-height: 1.6;
  color: var(--warn);
  border: 1px solid color-mix(in srgb, var(--warn) 50%, transparent);
  background: color-mix(in srgb, var(--warn) 12%, transparent);
  vertical-align: 1px;
  white-space: nowrap;
}

.btn-danger-ghost {
  color: var(--danger);
  border-color: var(--danger);
  background: transparent;
}

.btn-danger-ghost:hover {
  background: var(--danger-soft, rgba(220, 38, 38, 0.1));
}

.modal-ocr .ocr-body {
  max-height: 380px;
  overflow-y: auto;
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  padding: 12px 14px;
  background: var(--surface-2);
  margin-bottom: 16px;
}

.ocr-text {
  font-size: 14px;
  line-height: 1.75;
  color: var(--text);
  white-space: pre-wrap;
  word-break: break-word;
}

.modal-textarea {
  width: 100%;
  padding: 9px 12px;
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  background: var(--surface-2);
  color: var(--text);
  outline: none;
  margin-bottom: 16px;
  font-family: inherit;
  font-size: var(--fs-base);
  line-height: 1.6;
  resize: vertical;
  min-height: 120px;
}

.modal-textarea:focus {
  border-color: var(--primary);
}

.modal-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}

.conflict-all {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  color: var(--text-2);
  margin: 10px 0 14px;
  cursor: pointer;
}
.conflict-all input {
  cursor: pointer;
}

/* 拖拽遮罩 */
.drop-overlay {
  position: fixed;
  inset: 0;
  z-index: var(--z-modal);
  background: rgba(79, 70, 229, 0.08);
  display: flex;
  align-items: center;
  justify-content: center;
  pointer-events: none;
}

.drop-hint {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
  padding: 36px 52px;
  border-radius: var(--radius-lg);
  background: var(--surface);
  border: 2px dashed var(--primary);
  color: var(--primary);
  font-size: var(--fs-medium);
  font-weight: 600;
  box-shadow: var(--shadow-lg);
}

.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.18s ease;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}

/* 响应式 */
@media (max-width: 767px) {
  .files-view {
    padding: 0 12px 24px;
  }
  .toolbar {
    flex-direction: column;
    align-items: stretch;
    gap: 8px;
  }
  .list-head {
    grid-template-columns: minmax(0, 1fr) 70px 32px;
  }
  .list-head .col-time {
    display: none;
  }
  .list-row {
    grid-template-columns: minmax(0, 1fr) 70px 32px;
  }
  .col-time {
    display: none;
  }
}
</style>
