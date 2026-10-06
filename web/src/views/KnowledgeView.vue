<template>
  <div class="kb-view">
    <div class="kb-head">
      <div>
        <h2>{{  $t('知识库')  }}</h2>
        <p class="kb-sub">{{  $t('聚合你的文件、标签、摘要与关系 —— 概念、图谱、集合在此汇流')  }}</p>
      </div>
      <div class="kb-tools">
        <button class="btn" @click="loadAll" :disabled="loading">
          <AikIcon name="refresh" :size="14" />{{  $t('刷新')  }}
        </button>
      </div>
    </div>

    <!-- Tabs -->
    <div class="kb-tabs">
      <button v-for="t in tabs" :key="t.key" class="kb-tab" :class="{ active: tab === t.key }" @click="switchTab(t.key)">
        <AikIcon :name="t.icon" :size="14" />{{  t.label  }}
      </button>
    </div>

    <!-- ===== 总览 ===== -->
    <div v-show="tab === 'overview'">
      <div v-if="loading" class="kb-empty">{{  $t('加载中…')  }}</div>
      <template v-else>
        <div class="kb-cards">
          <div v-for="c in cards" :key="c.label" class="kb-card">
            <div class="kb-card-num">{{  c.value  }}</div>
            <div class="kb-card-label">{{  c.label  }}</div>
          </div>
        </div>

        <div class="kb-cols">
          <!-- 标签聚合 -->
          <div class="kb-panel">
            <div class="kb-panel-head">
              <span>{{  $t('标签 · 概念')  }}</span>
              <span class="muted">{{  tagAggs.length  }} {{  $t('个')  }}</span>
            </div>
            <div v-if="!tagAggs.length" class="kb-empty-sm">
              {{  $t('还没有标签。在文件库中对文件打标签，这里会自动聚合。')  }}
            </div>
            <div v-else class="kb-tag-list">
              <div v-for="t in tagAggs" :key="t.id" class="kb-tag-item" @click="openConcept(t.id)">
                <span class="kb-tag-path">{{  t.path  }}</span>
                <span class="kb-tag-count">{{  t.count  }}</span>
                <span v-if="t.summary" class="kb-tag-snippet">{{  clip(t.summary, 60)  }}</span>
              </div>
            </div>
          </div>

          <!-- 最近文件 -->
          <div class="kb-panel">
            <div class="kb-panel-head"><span>{{  $t('最近文件')  }}</span></div>
            <div v-if="!overview || !overview.recent_files?.length" class="kb-empty-sm">{{  $t('暂无文件')  }}</div>
            <div v-else class="kb-file-list">
              <div v-for="f in overview.recent_files" :key="f.id" class="kb-file-item" @click="openFile(f)">
                <AikIcon :name="f.kind === 'dir' ? 'folder' : 'file'" :size="14" class="kb-file-ico" />
                <span class="kb-file-name">{{  f.name  }}</span>
                <span class="kb-file-meta">{{  f.kind === 'dir' ? $t('目录') : fmtSize(f.size)  }}</span>
              </div>
            </div>
          </div>

          <!-- 最近 AI 摘要 -->
          <div class="kb-panel">
            <div class="kb-panel-head">
              <span>{{  $t('最近 AI 解读')  }}</span>
              <span v-if="overview" class="muted kb-qstat">
                {{  $t('已完成')  }} {{  overview.summaries  }} {{  $t('· 待解读')  }} {{  overview.summary_pending  }} {{  $t('· 失败')  }} {{  overview.summary_error  }}
                <button v-if="overview.summary_error > 0" class="btn btn-sm kb-retry" :title="$t('把失败解读重新加入队列（按当前模型配置重跑）')"
                  :disabled="retrying" @click="retrySummaries">
                  <AikIcon name="refresh" :size="12" />{{  retrying ? $t('重试中…') : $t('重试失败')  }}
                </button>
                <select v-model="requeueExt" class="input kb-requeue-ext" :title="$t('按文件类型批量解读（精读策略）')">
                  <option value="">{{  $t('全部类型')  }}</option>
                  <option value="md">Markdown</option>
                  <option value="doc">Word</option>
                  <option value="pdf">PDF</option>
                  <option value="txt">{{  $t('文本')  }}</option>
                </select>
                <button class="btn btn-sm kb-retry" :title="$t('把未解读/失败的文件按所选类型批量加入解读队列（已解读不重跑）')"
                  :disabled="requeuing" @click="requeueSummaries">
                  <AikIcon name="refresh" :size="12" />{{  requeuing ? $t('排队中…') : $t('批量解读')  }}
                </button>
              </span>
            </div>
            <div v-if="!overview || !overview.recent_summaries?.length" class="kb-empty-sm">{{  $t('尚无 AI 解读')  }}</div>
            <div v-else class="kb-sum-list">
              <div v-for="s in overview.recent_summaries" :key="s.file_id" class="kb-sum-item">
                <div class="kb-sum-name">{{  s.file_name  }}</div>
                <div class="kb-sum-text">{{  clip(s.summary, 70)  }}</div>
              </div>
            </div>
          </div>
        </div>
      </template>
    </div>

    <!-- ===== 图谱 ===== -->
    <div v-show="tab === 'graph'">
      <div v-if="loadingGraph" class="kb-empty">{{ $t('加载中…') }}</div>
      <div v-else-if="!graph.nodes.length" class="kb-empty">
        <AikIcon name="graph" :size="30" />
        <p>{{ $t('图谱还是空的') }}</p>
        <p class="muted">{{ $t('给文件打上标签后，文件-标签关系会自动出现在这里') }}</p>
      </div>
      <div v-else class="kb-graph-wrap">
        <div class="kb-graph-bar">
          <input v-model="graphQuery" class="kb-graph-search" :placeholder="$t('搜索节点，如 WebDAV、SQLite…')"
            @input="onGraphQuery" @keyup.enter="onGraphQuery" />
          <button v-if="graphQuery" class="btn btn-sm btn-ghost" :title="$t('清除搜索')" @click="clearGraphQuery">{{ $t('清除') }}</button>
          <select v-model="graphTag" class="input kb-graph-tag" :title="$t('按标签过滤')" @change="onGraphQuery">
            <option value="">{{ $t('全部标签') }}</option>
            <option v-for="t in tagAggs" :key="t.id" :value="t.id">{{  t.path  }}（{{  t.count  }}）</option>
          </select>
          <label class="kb-graph-filelab" :title="$t('显示/隐藏文件节点文字')">
            <input type="checkbox" v-model="showFileLabels" @change="onGraphQuery" /> {{ $t('文件标签') }}
          </label>
          <button class="btn btn-sm btn-ghost" :title="showAllGraph ? t('切换回精简骨架（标签+每标签少量文件）') : t('显示全部节点（可能密集）')"
            @click="toggleGraphScope">{{  showAllGraph ? $t('精简') : $t('全部')  }}</button>
          <button v-if="focusNodeId" class="btn btn-sm btn-ghost" :title="$t('取消聚焦')" @click="clearGraphFocus">{{ $t('取消聚焦') }}</button>
          <button class="btn btn-sm btn-ghost" :title="$t('重新布局')" @click="relayout"><AikIcon name="refresh" :size="13" /></button>
        </div>
        <div ref="graphEl" class="kb-graph-echarts"></div>
        <span class="kb-zoom-hint">{{ $t('滚轮缩放 · 拖拽平移 · 搜索自动定位 · 单击选中 · 双击打开') }}</span>
        <div v-if="selectedNode" class="kb-graph-ops">
          <span class="kb-ops-name">{{  selectedNode.label  }}</span>
          <span class="kb-ops-type">{{  selectedNode.type === 'tag' ? $t('标签/实体') : $t('文件')  }}<template v-if="selectedNode.type === 'tag' && selectedNode.size"> · {{  selectedNode.size  }} {{ $t('个文件') }}</template></span>
          <span class="kb-ops-spacer"></span>
          <button class="btn btn-sm btn-ghost" @click="focusGraphNode(selectedNode)">
            <AikIcon name="eye" :size="13" />{{ $t('聚焦') }}
          </button>
          <button v-if="selectedNode.type !== 'tag'" class="btn btn-sm" @click="openGraphNode(selectedNode)">
            <AikIcon name="fileText" :size="13" />{{ $t('打开阅读') }}
          </button>
          <button v-else class="btn btn-sm" @click="openGraphNode(selectedNode)">
            <AikIcon name="tag" :size="13" />{{ $t('查看关联文件') }}
          </button>
          <button class="btn btn-sm btn-ghost" @click="aiSearchNode(selectedNode)">
            <AikIcon name="sparkles" :size="13" />{{ $t('AI 检索') }}
          </button>
          <button class="btn btn-sm btn-ghost" :title="$t('取消选择')" @click="selectedNode = null">{{ $t('取消') }}</button>
        </div>
        <div class="kb-legend">
          <span><i class="dot dot-file"></i>{{ $t('文件') }}</span>
          <span><i class="dot dot-tag"></i>{{ $t('标签') }}</span>
          <span class="muted">{{ $t('实体与关系随 AI 解读自动生成') }}</span>
        </div>
      </div>
    </div>

    <!-- ===== 集合 ===== -->
    <div v-show="tab === 'collections'">
      <div class="kb-panel">
        <div class="kb-panel-head">
          <span>{{ $t('集合（手动收藏 / 智能筛选）') }}</span>
        </div>
        <div class="kb-col-new">
          <input v-model="newColName" class="input kb-col-input" :placeholder="$t('集合名称…')" @keyup.enter="createCol" />
          <div class="kb-kind-switch">
            <button v-for="k in colKinds" :key="k.key" class="kb-kind-btn" :class="{ active: newColKind === k.key }"
              @click="newColKind = k.key">{{  k.label  }}</button>
          </div>
          <template v-if="newColKind === 'smart'">
            <select v-model="smartTag" class="input kb-col-input">
              <option value="">{{ $t('全部标签') }}</option>
              <option v-for="t in tagAggs" :key="t.id" :value="t.path">{{  t.path  }}</option>
            </select>
            <select v-model="smartKind" class="input kb-col-input">
              <option value="">{{ $t('全部类型') }}</option>
              <option value="file">{{ $t('仅文件') }}</option>
              <option value="dir">{{ $t('仅目录') }}</option>
            </select>
            <input v-model="smartQ" class="input kb-col-input" :placeholder="$t('文件名关键词…')" @keyup.enter="createCol" />
          </template>
          <button class="btn btn-sm btn-primary" :disabled="!newColName.trim()" @click="createCol">
            <AikIcon name="plus" :size="13" />{{ $t('新建') }}
          </button>
        </div>
        <div v-if="!collections.length" class="kb-empty-sm">{{ $t('还没有集合。手动收藏重要文件，或用标签/关键词建一个智能集合。') }}</div>
        <div v-else class="kb-col-list">
          <div v-for="c in collections" :key="c.id" class="kb-col-item">
            <div class="kb-col-main" @click="toggleCol(c)">
              <AikIcon :name="c.kind === 'smart' ? 'filter' : 'layers'" :size="15" class="kb-file-ico" />
              <span class="kb-col-name">{{  c.name  }}</span>
              <span class="kb-col-count">{{  c.file_count  }} {{ $t('项') }}</span>
              <span class="kb-col-kind">{{  c.kind === 'smart' ? $t('智能') : $t('手动')  }}</span>
              <span v-if="c.kind === 'smart' && c.query" class="kb-col-query">{{  describeQuery(c.query)  }}</span>
            </div>
            <button class="btn btn-sm btn-ghost" :title="$t('删除集合')" @click="confirmRemoveCol = c">{{ $t('删除') }}</button>
            <div v-if="openColId === c.id" class="kb-col-files">
              <div v-if="colFilesLoading" class="kb-empty-sm">{{ $t('加载中…') }}</div>
              <div v-else-if="!colFiles.length" class="kb-empty-sm">
                {{  c.kind === 'smart' ? $t('当前条件没有匹配的文件') : $t('空集合。在文件库右键「加入集合」')  }}
              </div>
              <div v-for="f in colFiles" :key="f.id" class="kb-file-item" @click="openFile(f)">
                <AikIcon :name="f.kind === 'dir' ? 'folder' : 'file'" :size="14" class="kb-file-ico" />
                <span class="kb-file-name">{{  f.name  }}</span>
                <span class="kb-file-meta">{{  fmtSize(f.size)  }}</span>
                <button v-if="c.kind !== 'smart'" class="btn btn-sm btn-ghost" :title="$t('移出集合')" @click.stop="removeColFile(c, f)">{{ $t('移出') }}</button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- ===== 质量 lint ===== -->
    <div v-show="tab === 'lint'">
      <div v-if="lintLoading" class="kb-empty">{{ $t('检查中…') }}</div>
      <template v-else-if="lint">
        <!-- 统计条 -->
        <div class="kb-cards kb-cards-sm">
          <div class="kb-card"><div class="kb-card-num">{{  lint.total_files  }}</div><div class="kb-card-label">{{ $t('文件总数') }}</div></div>
          <div class="kb-card" :class="{ 'kb-warn': lint.orphans.length }"><div class="kb-card-num">{{  lint.orphans.length  }}</div><div class="kb-card-label">{{ $t('孤儿文件') }}</div></div>
          <div class="kb-card" :class="{ 'kb-warn': lint.empty_tags.length }"><div class="kb-card-num">{{  lint.empty_tags.length  }}</div><div class="kb-card-label">{{ $t('空标签') }}</div></div>
          <div class="kb-card" :class="{ 'kb-warn': lint.no_summary.length }"><div class="kb-card-num">{{  lint.no_summary.length  }}</div><div class="kb-card-label">{{ $t('未解读') }}</div></div>
          <div class="kb-card" :class="{ 'kb-warn': lint.duplicates.length }"><div class="kb-card-num">{{  lint.duplicates.length  }}</div><div class="kb-card-label">{{ $t('重复组') }}</div></div>
          <div class="kb-card" :class="{ 'kb-warn': lint.broken_links.length }"><div class="kb-card-num">{{  lint.broken_links.length  }}</div><div class="kb-card-label">{{ $t('断链双链') }}</div></div>
          <div class="kb-card" :class="{ 'kb-warn': lint.summary_error }"><div class="kb-card-num">{{  lint.summary_error  }}</div><div class="kb-card-label">{{ $t('解读失败') }}</div></div>
        </div>

        <div class="kb-cols">
          <!-- 孤儿文件 -->
          <div class="kb-panel">
            <div class="kb-panel-head">
              <span>{{ $t('孤儿文件（无标签 · 无集合）') }}<span class="muted">{{  lint.orphans.length  }} {{ $t('个') }}</span></span>
              <span class="kb-orphan-bar" v-if="lint.orphans.length">
                <label class="kb-orphan-all"><input type="checkbox" :checked="orphanSelAll" @change="orphanToggleAll" /> {{ $t('全选') }}</label>
                <span v-if="orphanSel.size" class="kb-orphan-cnt">{{ $t('已选') }} {{  orphanSel.size  }}</span>
                <button class="btn btn-sm" :disabled="!orphanSel.size" @click="openBatchTag">{{ $t('批量打标签') }}</button>
                <button class="btn btn-sm" :disabled="!orphanSel.size" @click="openBatchCol">{{ $t('批量入集合') }}</button>
              </span>
            </div>
            <div v-if="!lint.orphans.length" class="kb-empty-sm">{{ $t('没有孤儿文件') }}</div>
            <div v-else class="kb-file-list">
              <div v-for="f in lint.orphans" :key="f.file_id" class="kb-file-item" :class="{ on: orphanSel.has(f.file_id) }" @click="openFileById(f.file_id)">
                <input type="checkbox" class="kb-file-chk" :checked="orphanSel.has(f.file_id)" @click.stop @change="orphanToggle(f.file_id)" />
                <AikIcon name="file" :size="14" class="kb-file-ico" />
                <span class="kb-file-name">{{  f.file_name  }}</span>
                <span class="kb-file-meta">{{  fmtSize(f.size)  }}</span>
                <span class="kb-file-ops">
                  <button class="btn btn-sm btn-ghost" :title="$t('打标签（知识库/博客/频道都能用）')" @click.stop="orphanTagFile = f">{{ $t('打标签') }}</button>
                  <button class="btn btn-sm btn-ghost" :title="$t('加入集合')" @click.stop="openOrphanCol(f)">{{ $t('入集合') }}</button>
                </span>
              </div>
            </div>
          </div>

          <!-- 重复文件 -->
          <div class="kb-panel">
            <div class="kb-panel-head"><span>{{ $t('重复文件（sha256 相同）') }}</span><span class="muted">{{  lint.duplicates.length  }} {{ $t('组 · 保留一份，其余软删可恢复') }}</span></div>
            <div v-if="!lint.duplicates.length" class="kb-empty-sm">{{ $t('没有重复文件') }}</div>
            <div v-else class="kb-lint-dups">
              <div v-for="(g, gi) in lint.duplicates" :key="g.sha256" class="kb-dup-group">
                <div class="kb-dup-bar">
                  <span class="kb-dup-sha">#{{  gi + 1  }} · {{  g.sha256.slice(0, 10)  }}…</span>
                  <button v-if="g.files.length > 1" class="btn btn-sm btn-ghost kb-dup-del"
                    :disabled="dedupBusy[g.sha256]"
                    @click="askDedup(g)">{{ $t('删除其余') }} {{  g.files.length - 1  }} {{ $t('份') }}</button>
                </div>
                <div v-for="f in g.files" :key="f.file_id" class="kb-file-item"
                  :class="{ 'kb-dup-keep': dedupSel[g.sha256] === f.file_id }" @click="pickDedup(g, f.file_id)">
                  <input type="radio" :name="'dup-' + g.sha256" class="kb-dup-radio" :checked="dedupSel[g.sha256] === f.file_id"
                    @click.stop="pickDedup(g, f.file_id)" />
                  <AikIcon name="file" :size="13" class="kb-file-ico" />
                  <span class="kb-file-name">{{  f.file_name  }}</span>
                  <span class="kb-file-meta">{{  fmtSize(f.size)  }}</span>
                  <span v-if="dedupSel[g.sha256] === f.file_id" class="kb-dup-flag">{{ $t('保留') }}</span>
                </div>
              </div>
            </div>
          </div>

          <!-- 断链双链 -->
          <div class="kb-panel">
            <div class="kb-panel-head"><span>{{ $t('断链双链（[[目标]] 未匹配到文件）') }}</span><span class="muted">{{ $t('来源可点击') }}</span></div>
            <div v-if="!lint.broken_links.length" class="kb-empty-sm">{{ $t('没有断链') }}</div>
            <div v-else class="kb-file-list">
              <div v-for="(b, bi) in lint.broken_links" :key="bi" class="kb-file-item" @click="openFileById(b.file_id)">
                <AikIcon name="link" :size="13" class="kb-file-ico" />
                <span class="kb-file-name">{{  b.file_name  }}</span>
                <span class="kb-file-meta kb-broken-target">→ {{  clip(b.target, 24)  }}</span>
              </div>
            </div>
          </div>

          <!-- 空标签 / 未解读 / 解读失败 -->
          <div class="kb-panel">
            <div class="kb-panel-head"><span>{{ $t('空标签') }}</span><span class="muted">{{  lint.empty_tags.length  }} {{ $t('个 · 点概念或删除') }}</span></div>
            <div v-if="!lint.empty_tags.length" class="kb-empty-sm">{{ $t('没有空标签') }}</div>
            <div v-else class="kb-rel-tags kb-wrap kb-lint-tags">
              <span v-for="t in lint.empty_tags.slice(0, 24)" :key="t.tag_id" class="kb-lint-tag">
                <button class="kb-rel-tag" @click="openConcept(t.tag_id)">{{  t.path  }}</button>
                <button class="kb-lint-tag-del" :title="$t('删除空标签')" @click="confirmEmptyTag = t">×</button>
              </span>
              <span v-if="lint.empty_tags.length > 24" class="muted">{{ $t('等') }} {{  lint.empty_tags.length - 24  }} {{ $t('个') }}</span>
            </div>
          </div>

          <div class="kb-panel">
            <div class="kb-panel-head"><span>{{ $t('未解读文件') }}</span><span class="muted">{{ $t('文本/文档类') }}</span></div>
            <div v-if="!lint.no_summary.length" class="kb-empty-sm">{{ $t('都已解读') }}</div>
            <div v-else class="kb-file-list">
              <div v-for="f in lint.no_summary" :key="f.file_id" class="kb-file-item" @click="openFileById(f.file_id)">
                <AikIcon name="fileText" :size="13" class="kb-file-ico" />
                <span class="kb-file-name">{{  f.file_name  }}</span>
                <span class="kb-file-meta">{{  fmtSize(f.size)  }}</span>
              </div>
            </div>
          </div>

          <div class="kb-panel">
            <div class="kb-panel-head"><span>{{ $t('解读失败') }}</span><span class="muted">{{ $t('可去总览重试') }}</span></div>
            <div v-if="!lint.summary_error" class="kb-empty-sm">{{ $t('无失败记录') }}</div>
            <div v-else class="kb-empty-sm">
              {{  lint.summary_error  }} {{ $t('条解读失败。切换到「总览」→ 最近 AI 解读右上角「重试失败」可重新入队。') }}
            </div>
          </div>
        </div>
        <div class="kb-lint-note muted">{{ $t('质量检查只读不改数据 · 断链基于已索引内容匹配文件名（全名/去扩展名）') }}</div>
      </template>
    </div>

    <!-- 概念页弹窗 -->
    <div v-if="concept" class="kb-modal-mask" @click.self="concept = null">
      <div class="kb-modal">
        <div class="kb-modal-head">
          <div>
            <h3>{{ $t('概念 ·') }} {{  concept.tag.path  }}</h3>
            <p class="muted">{{  concept.tag.count  }} {{ $t('个文件') }}</p>
          </div>
          <div class="kb-modal-actions">
            <button class="btn btn-sm" :disabled="aiBusy" @click="summarizeConcept">
              {{ aiBusy ? $t('AI 生成中…') : (concept.tag.summary_source === 'ai' ? $t('重新生成摘要') : $t('AI 生成摘要')) }}
            </button>
            <button class="btn btn-sm btn-ghost" @click="concept = null">{{ $t('关闭') }}</button>
          </div>
        </div>
        <div v-if="concept.tag.summary" class="kb-concept-sum">
          {{  concept.tag.summary  }}
          <div v-if="concept.tag.summary_source === 'ai'" class="kb-sum-meta">
            <span class="kb-sum-badge">AI</span>
            <span v-if="concept.tag.summary_updated_at">{{ fmtTime(concept.tag.summary_updated_at) }}</span>
          </div>
          <div v-if="concept.tag.summary_sources?.length" class="kb-sum-srcs">
            <span class="muted">{{ $t('来源：') }}</span>
            <button v-for="s in concept.tag.summary_sources" :key="s.file_id" class="kb-sum-src" @click="openFileById(s.file_id)">{{ s.name }}</button>
          </div>
        </div>
        <div v-if="concept.related_tags?.length" class="kb-rel-tags">
          <span class="muted">{{ $t('关联概念：') }}</span>
          <button v-for="rt in concept.related_tags" :key="rt.id" class="kb-rel-tag" @click="openConcept(rt.id)">
            {{  rt.name  }}
            <span class="kb-tag-count">{{  rt.count  }}</span>
          </button>
        </div>
        <div class="kb-concept-files">
          <div v-if="concept.inbound?.length" class="kb-concept-sec">
            <div class="kb-concept-sec-head"><span>{{ $t('被这些文件引用') }}</span><span class="muted">{{ $t('双链反向链接') }}</span></div>
            <div v-for="f in concept.inbound" :key="f.id" class="kb-file-item" @click="openFile(f)">
              <AikIcon name="link" :size="13" class="kb-file-ico" />
              <span class="kb-file-name">{{  f.name  }}</span>
              <span class="kb-file-meta">{{  fmtSize(f.size)  }}</span>
            </div>
          </div>
          <div v-if="concept.related_files?.length" class="kb-concept-sec">
            <div class="kb-concept-sec-head"><span>{{ $t('相关文件') }}</span><span class="muted">{{ $t('标签共现推荐') }}</span></div>
            <div v-for="f in concept.related_files" :key="f.id" class="kb-file-item" @click="openFile(f)">
              <AikIcon name="fileText" :size="13" class="kb-file-ico" />
              <span class="kb-file-name">{{  f.name  }}</span>
              <span class="kb-file-meta">{{  fmtSize(f.size)  }}</span>
            </div>
          </div>
          <div class="kb-concept-sec">
            <div class="kb-concept-sec-head"><span>{{ $t('本概念文件') }}</span><span class="muted">{{  concept.tag.count  }} {{ $t('个') }}</span></div>
            <div v-for="f in concept.tag.files" :key="f.id" class="kb-file-item" @click="openFile(f)">
              <AikIcon :name="f.kind === 'dir' ? 'folder' : 'file'" :size="14" class="kb-file-ico" />
              <span class="kb-file-name">{{  f.name  }}</span>
              <span class="kb-file-meta">{{  fmtSize(f.size)  }}</span>
            </div>
          </div>
        </div>
      </div>
    </div>

    <TagDialog
      v-if="orphanTagFile"
      :file="{ id: orphanTagFile.file_id, name: orphanTagFile.file_name }"
      @close="orphanTagFile = null"
      @saved="afterOrphanTag"
    />
    <TagDialog
      v-if="orphanTagBatch"
      :files="orphanTagBatch"
      @close="orphanTagBatch = null"
      @saved="afterOrphanTag"
    />
    <div v-if="orphanColFile" class="modal-mask" @click.self="orphanColFile = null">
      <div class="tag-dialog">
        <div class="td-head">
          <h3>{{ $t('加入集合') }}</h3>
          <button class="icon-btn" :title="$t('关闭')" @click="orphanColFile = null"><AikIcon name="close" :size="16" /></button>
        </div>
        <div class="td-file" :title="orphanColFile.file_name">{{  orphanColFile.file_name  }}</div>
        <div class="td-new">
          <select v-model="orphanColSel" class="input">
            <option value="">{{ $t('选择集合…') }}</option>
            <option v-for="c in colList" :key="c.id" :value="c.id">{{  c.name  }}（{{  c.file_count  }} {{ $t('项）') }}</option>
          </select>
          <button class="btn btn-sm btn-primary" :disabled="!orphanColSel" @click="doOrphanCol">{{ $t('加入') }}</button>
        </div>
        <div v-if="!colList.length" class="kb-empty-sm">{{ $t('还没有集合，去「集合」tab 建一个') }}</div>
      </div>
    </div>
    <div v-if="orphanColBatch" class="modal-mask" @click.self="orphanColBatch = null">
      <div class="tag-dialog">
        <div class="td-head">
          <h3>{{ $t('批量加入集合') }}</h3>
          <button class="icon-btn" :title="$t('关闭')" @click="orphanColBatch = null"><AikIcon name="close" :size="16" /></button>
        </div>
        <div class="td-file">{{ $t('已选') }} {{  orphanColBatch.length  }} {{ $t('个文件') }}</div>
        <div class="td-new">
          <select v-model="orphanColSel" class="input">
            <option value="">{{ $t('选择集合…') }}</option>
            <option v-for="c in colList" :key="c.id" :value="c.id">{{  c.name  }}（{{  c.file_count  }} {{ $t('项）') }}</option>
          </select>
          <button class="btn btn-sm btn-primary" :disabled="!orphanColSel" @click="doOrphanColBatch">{{ $t('加入') }}</button>
        </div>
        <div v-if="!colList.length" class="kb-empty-sm">{{ $t('还没有集合，去「集合」tab 建一个') }}</div>
      </div>
    </div>
    <ConfirmDialog
      v-if="confirmEmptyTag"
      :title="$t('删除空标签')"
:message="$t('删除标签「{v0}」？它名下没有文件，删除不影响任何内容。', { v0: confirmEmptyTag.path })"
      confirm-text="删除"
      danger
      @confirm="doDeleteEmptyTag"
      @cancel="confirmEmptyTag = null"
    />
    <ConfirmDialog
      v-if="confirmDedup"
      :title="$t('删除重复文件')"
:message="$t('保留「{keep}」，删除其余 {count} 份？\n\n{names}\n\n删除为软删，可在回收站恢复。', { keep: confirmDedup.keepName, count: confirmDedup.delNames.length, names: confirmDedup.delNames.join('\n') })"
      confirm-text="删除"
      danger
      @confirm="doDedup"
      @cancel="confirmDedup = null"
    />
    <ConfirmDialog
      v-if="confirmRemoveCol"
      :title="$t('删除集合')"
:message="$t('删除集合「{v0}」？集合内文件不会被删除。', { v0: confirmRemoveCol.name })"
      confirm-text="删除"
      danger
      @confirm="doRemoveCol(confirmRemoveCol)"
      @cancel="confirmRemoveCol = null"
    />
  </div>
</template>

<script setup>
import { t as i18t } from '@/i18n'
import { ref, computed, onMounted, onBeforeUnmount, nextTick } from 'vue'
import { useRouter } from 'vue-router'
import AikIcon from '@/components/AikIcon.vue'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import TagDialog from '@/components/TagDialog.vue'
import * as api from '@/api'
import { useToastStore } from '@/stores/toast'
import { t } from '@/i18n'

const router = useRouter()
const toast = useToastStore()

const tab = ref('overview')
const tabs = [
  { key: 'overview', label: t('总览'), icon: 'grid' },
  { key: 'graph', label: t('图谱'), icon: 'graph' },
  { key: 'collections', label: t('集合'), icon: 'layers' },
  { key: 'lint', label: t('质量'), icon: 'alert' }
]

const loading = ref(false)
const overview = ref(null)
const tagAggs = ref([])
const loadingGraph = ref(false)
const graph = ref({ nodes: [], links: [] })
const collections = ref([])
const openColId = ref('')
const colFiles = ref([])
const colFilesLoading = ref(false)
const newColName = ref('')
const newColKind = ref('manual')
const smartTag = ref('')
const smartKind = ref('')
const smartQ = ref('')
const colKinds = [
  { key: 'manual', label: t('手动') },
  { key: 'smart', label: t('智能') }
]
const concept = ref(null)

// ===== 质量 lint（A08）=====
const lintLoading = ref(false)
const lint = ref(null)
const dedupSel = ref({})
const dedupBusy = ref({})
const confirmDedup = ref(null)
async function loadLint() {
  lintLoading.value = true
  try {
    lint.value = await api.kbLint()
    // 默认保留每组第一份（updated_at 最早 = 原始版）
    const sel = {}
    lint.value.duplicates.forEach((g) => {
      sel[g.sha256] = g.files[0].file_id
    })
    dedupSel.value = sel
  } catch (e) {
    toast.push(t('质量检查失败：') + (e.message || e))
  } finally {
    lintLoading.value = false
  }
}
function openFileById(id) {
  router.push('/read/' + id)
}
function pickDedup(g, fileId) {
  dedupSel.value = { ...dedupSel.value, [g.sha256]: fileId }
}
function askDedup(g) {
  const keepId = dedupSel.value[g.sha256]
  const del = g.files.filter((f) => f.file_id !== keepId)
  const keep = g.files.find((f) => f.file_id === keepId)
  confirmDedup.value = {
    sha256: g.sha256,
    keepName: keep ? keep.file_name : keepId,
    delNames: del.map((f) => f.file_name)
  }
}
async function doDedup() {
  const c = confirmDedup.value
  confirmDedup.value = null
  if (!c) return
  const del = lint.value.duplicates.find((g) => g.sha256 === c.sha256).files
    .filter((f) => f.file_id !== dedupSel.value[c.sha256])
    .map((f) => f.file_id)
  dedupBusy.value = { ...dedupBusy.value, [c.sha256]: true }
  try {
    const d = await api.kbDedupResolve(c.sha256, dedupSel.value[c.sha256], del)
    toast.push(`已删除 ${d.deleted} 份重复（软删，可在回收站恢复）`)
    await loadLint()
  } catch (e) {
    toast.push(t('去重失败：') + (e.message || e))
  } finally {
    dedupBusy.value = { ...dedupBusy.value, [c.sha256]: false }
  }
}
const confirmEmptyTag = ref(null)
const orphanTagFile = ref(null)
const orphanColFile = ref(null)
const orphanColSel = ref('')
// 批量处理
const orphanTagBatch = ref(null)
const orphanColBatch = ref(null)
const orphanSel = ref(new Set())
function orphanToggle(id) {
  const s = new Set(orphanSel.value)
  if (s.has(id)) s.delete(id)
  else s.add(id)
  orphanSel.value = s
}
const orphanSelAll = computed(() => {
  const n = lint.value?.orphans?.length || 0
  return n > 0 && orphanSel.value.size === n
})
function orphanToggleAll() {
  const all = (lint.value?.orphans || []).map((f) => f.file_id)
  orphanSel.value = orphanSel.value.size === all.length && all.length > 0 ? new Set() : new Set(all)
}
function openBatchTag() {
  orphanTagBatch.value = (lint.value?.orphans || [])
    .filter((f) => orphanSel.value.has(f.file_id))
    .map((f) => ({ id: f.file_id, name: f.file_name }))
}
function openBatchCol() {
  orphanColBatch.value = (lint.value?.orphans || []).filter((f) => orphanSel.value.has(f.file_id))
  orphanColSel.value = ''
}
async function doOrphanColBatch() {
  const cid = orphanColSel.value
  if (!cid || !orphanColBatch.value?.length) return
  try {
    for (const f of orphanColBatch.value) {
      await api.addCollectionFile(cid, f.file_id)
    }
    toast.push(`已将 ${orphanColBatch.value.length} 个文件加入集合`)
    orphanColBatch.value = null
    orphanSel.value = new Set()
    await loadLint()
  } catch (e) {
    toast.push(t('批量入集合失败：') + (e.message || e))
  }
}
const colList = ref([])
function afterOrphanTag() {
  // 提示已由 TagDialog 内部给出（单文件/批量各有文案），这里只刷新数据
  orphanSel.value = new Set()
  loadLint()
}
async function openOrphanCol(f) {
  orphanColFile.value = f
  orphanColSel.value = ''
  try {
    colList.value = await api.listCollections()
  } catch (e) {
    colList.value = []
  }
}
async function doOrphanCol() {
  const f = orphanColFile.value
  if (!f || !orphanColSel.value) return
  try {
    await api.addCollectionFile(orphanColSel.value, f.file_id)
    toast.push(`「${f.file_name}」已加入集合`)
    orphanColFile.value = null
    await loadLint()
  } catch (e) {
    toast.push(t('加入集合失败：') + (e.message || e))
  }
}
async function doDeleteEmptyTag() {
  const t = confirmEmptyTag.value
  confirmEmptyTag.value = null
  if (!t) return
  try {
    await api.deleteTag(t.tag_id)
    toast.push(`已删除空标签「${t.path}」`)
    await loadLint()
  } catch (e) {
    toast.push(t(i18t('删除失败：')) + (e.message || e))
  }
}

// ===== 图谱：爱库录精简发行已移除 ECharts（~1MB）；图谱 tab 显示占位 =====
const graphEl = ref(null)
let chart = null
const graphQuery = ref('')
const graphTag = ref('')
const showFileLabels = ref(false)
const focusNodeId = ref('')
const selectedNode = ref(null)
const showAllGraph = ref(false) // 默认骨架视图：标签 + 每标签少量文件（避免密集糊图）

// 过滤管线：标签过滤 → 聚焦（1 跳邻接）→ 关键词搜索
function filteredGraph() {
  const q = graphQuery.value.trim().toLowerCase()
  let nodes = graph.value.nodes
  let links = graph.value.links
  // 骨架截断：无搜索/聚焦/标签过滤时，只展示"大标签 + 每标签少量文件"，控制整体密度
  if (!q && !focusNodeId.value && !graphTag.value && !showAllGraph.value) {
    const MAX_TAGS = 40 // 按文件数降序保留的大标签数
    const MAX_PER_TAG = 6
    const tagNodes = nodes
      .filter((n) => n.type === 'tag')
      .sort((a, b) => (b.size || 0) - (a.size || 0))
      .slice(0, MAX_TAGS)
    const keepTags = new Set(tagNodes.map((n) => n.id))
    const keepFiles = new Set()
    const perTag = {}
    links.forEach((l) => {
      let tagId = '', fid = ''
      if (String(l.source).startsWith('tag:') && String(l.target).startsWith('file:')) { tagId = l.source; fid = l.target }
      else if (String(l.target).startsWith('tag:') && String(l.source).startsWith('file:')) { tagId = l.target; fid = l.source }
      if (tagId && keepTags.has(tagId) && fid) {
        perTag[tagId] = perTag[tagId] || []
        if (perTag[tagId].length < MAX_PER_TAG) keepFiles.add(fid)
      }
    })
    nodes = nodes.filter((n) => keepTags.has(n.id) || keepFiles.has(n.id))
    links = links.filter((l) => {
      const ids = new Set([String(l.source), String(l.target)])
      return nodes.some((n) => ids.has(n.id))
    })
  }
  if (graphTag.value) {
    const tagNodeId = 'tag:' + graphTag.value
    const keep = new Set([tagNodeId])
    links.forEach((l) => {
      if (l.source === tagNodeId) keep.add(l.target)
      if (l.target === tagNodeId) keep.add(l.source)
    })
    const idSet = new Set(keep)
    nodes = nodes.filter((n) => idSet.has(n.id))
    links = links.filter((l) => idSet.has(l.source) && idSet.has(l.target))
  }
  if (focusNodeId.value) {
    const keep = new Set([focusNodeId.value])
    links.forEach((l) => {
      if (l.source === focusNodeId.value) keep.add(l.target)
      if (l.target === focusNodeId.value) keep.add(l.source)
    })
    const idSet = new Set(keep)
    nodes = nodes.filter((n) => idSet.has(n.id))
    links = links.filter((l) => idSet.has(l.source) && idSet.has(l.target))
  }
  if (q) {
    const matched = new Set()
    nodes.forEach((n) => {
      if (n.label.toLowerCase().includes(q)) matched.add(n.id)
    })
    const keep = new Set(matched)
    links.forEach((l) => {
      if (matched.has(l.source) && !keep.has(l.target)) keep.add(l.target)
      if (matched.has(l.target) && !keep.has(l.source)) keep.add(l.source)
    })
    const idSet = new Set(keep)
    nodes = nodes.filter((n) => idSet.has(n.id))
    links = links.filter((l) => idSet.has(l.source) && idSet.has(l.target))
  }
  return { nodes, links }
}
function focusGraphNode(n) {
  if (!n) return
  focusNodeId.value = n.id
  selectedNode.value = n
  onGraphQuery()
}
function clearGraphFocus() {
  focusNodeId.value = ''
  onGraphQuery()
}
function toggleGraphScope() {
  showAllGraph.value = !showAllGraph.value
  onGraphQuery()
}
function onGraphQuery() {
  if (chart) chart.setOption(buildGraphOption(), true)
  autoFocusHit()
}
// 搜索定位闭环：命中第一个节点 → 自动选中 + 聚焦（1 跳邻接），操作面板随即出现打开入口
function autoFocusHit() {
  const q = graphQuery.value.trim().toLowerCase()
  if (!q) return
  const hit = graph.value.nodes.find((n) => n.label.toLowerCase().includes(q))
  if (hit && hit.id !== focusNodeId.value) {
    focusNodeId.value = hit.id
    selectedNode.value = hit
    if (chart) chart.setOption(buildGraphOption(), true)
  }
}
function clearGraphQuery() {
  graphQuery.value = ''
  focusNodeId.value = ''
  selectedNode.value = null
  onGraphQuery()
}
function buildGraphOption() {
  const { nodes: fNodes, links: fLinks } = filteredGraph()
  const q = graphQuery.value.trim().toLowerCase()
  const nodes = fNodes.map((n) => {
    const hit = q && n.label.toLowerCase().includes(q)
    const isFocus = focusNodeId.value === n.id
    return {
      id: n.id,
      name: n.label,
      category: n.type === 'tag' ? 0 : 1,
      symbolSize: n.type === 'tag' ? Math.min(10 + (n.size || 0) * 4, 26) : 6,
      label: { show: n.type === 'tag' || hit || showFileLabels.value },
      itemStyle: hit
        ? { color: n.type === 'tag' ? '#19b394' : '#4e7bff', borderColor: '#ff8f2b', borderWidth: 3 }
        : isFocus
          ? { color: n.type === 'tag' ? '#19b394' : '#4e7bff', borderColor: '#ff8f2b', borderWidth: 2 }
          : { color: n.type === 'tag' ? '#19b394' : '#4e7bff' }
    }
  })
  const links = fLinks.map((l) => ({
    source: l.source,
    target: l.target,
    lineStyle: { width: 0.6, opacity: 0.38, curveness: 0.08 }
  }))
  return {
    tooltip: {
      trigger: 'item',
      confine: true,
      formatter: (p) => (p.dataType === 'edge' ? '' : `<b>${p.name}</b>`)
    },
    legend: {
      show: true,
      top: 0,
      right: 8,
      itemWidth: 9,
      itemHeight: 9,
      textStyle: { fontSize: 11, color: '#565d6d' },
      data: [
        { name: t('标签'), itemStyle: { color: '#19b394' } },
        { name: t('文件'), itemStyle: { color: '#4e7bff' } }
      ]
    },
    series: [
      {
        type: 'graph',
        layout: 'force',
        roam: true,
        draggable: true,
        focusNodeAdjacency: true,
        edgeSymbol: ['none', 'none'],
        categories: [
          { name: t('标签'), itemStyle: { color: '#19b394' } },
          { name: t('文件'), itemStyle: { color: '#4e7bff' } }
        ],
        force: { repulsion: 190, edgeLength: [30, 80], gravity: 0.12, friction: 0.6, layoutAnimation: false },
        label: {
          show: true,
          fontSize: 11,
          color: '#565d6d',
          formatter: (p) => (p.name.length > 10 ? p.name.slice(0, 10) + '…' : p.name)
        },
        lineStyle: { color: '#c6cbda' },
        data: nodes,
        links
      }
    ]
  }
}
function graphNodeById(id) {
  return graph.value.nodes.find((n) => n.id === id) || null
}
function bindGraphEvents() {
  if (!chart) return
  chart.off('click')
  chart.off('dblclick')
  chart.on('click', (p) => {
    if (p.dataType !== 'node' || !p.data) return
    selectedNode.value = graphNodeById(p.data.id)
  })
  chart.on('dblclick', (p) => {
    if (p.dataType !== 'node' || !p.data) return
    openGraphNode(graphNodeById(p.data.id))
  })
}
function openGraphNode(n) {
  if (!n) return
  const rawId = String(n.id).replace(/^(file|dir|tag):/, '')
  if (n.type === 'tag') {
    openConcept(rawId)
    return
  }
  if (n.type === 'dir') router.push({ path: '/files', query: { parent: rawId } })
  else router.push('/read/' + rawId)
}
function aiSearchNode(n) {
  if (!n) return
  router.push({ path: '/search', query: { q: n.label } })
}
function renderGraph() {
  if (!graphEl.value) return
  graphEl.value.innerHTML =
    t('<div style="display:flex;align-items:center;justify-content:center;height:100%;color:#6b7a8d;font-size:14px;padding:24px;text-align:center">爱库录精简版：知识图谱已移除（ECharts）。标签树与检索仍可用。</div>')
}
function relayout() {
  if (chart) chart.setOption(buildGraphOption(), true)
}
function onWindowResize() {
  if (chart) chart.resize()
}

const cards = computed(() => {
  const o = overview.value || {}
  return [
    { label: t('文件'), value: o.files ?? '—' },
    { label: t(i18t('文件夹')), value: o.dirs ?? '—' },
    { label: t('标签'), value: o.tags ?? '—' },
    { label: t('集合'), value: o.collections ?? '—' },
    { label: t('AI 解读'), value: o.summaries ?? '—' },
    { label: t('分块'), value: o.chunks ?? '—' },
    { label: t('关系边'), value: o.edges ?? '—' }
  ]
})

async function loadAll() {
  loading.value = true
  try {
    const [o, t, cols] = await Promise.all([api.kbOverview(), api.kbTags(), api.listCollections()])
    overview.value = o
    tagAggs.value = t
    collections.value = cols
    startPolling()
  } catch (e) {
    toast.push(t('加载知识库失败：') + (e.message || e))
  } finally {
    loading.value = false
  }
}

const retrying = ref(false)
async function retrySummaries() {
  retrying.value = true
  try {
    const d = await api.kbSummariesRetry()
    const parts = []
    if (d.retried) parts.push(`重试 ${d.retried} 条`)
    if (d.cleaned) parts.push(`清理 ${d.cleaned} 条失效记录`)
    toast.push(parts.length ? `${parts.join('，')}，后台队列处理中` : '没有需要重试的失败记录')
    await loadAll()
  } catch (e) {
    toast.push(t('重试失败：') + (e.message || e))
  } finally {
    retrying.value = false
  }
}

const requeueExt = ref('')
const requeuing = ref(false)
async function requeueSummaries() {
  requeuing.value = true
  try {
    const d = await api.kbSummariesRequeue(requeueExt.value ? { exts: [requeueExt.value] } : {})
    toast.push(`已把 ${d.queued} 个文件加入解读队列${requeueExt.value ? t('（类型：') + requeueExt.value + '）' : ''}，后台分批处理中`)
    await loadAll()
  } catch (e) {
    toast.push(t('批量解读失败：') + (e.message || e))
  } finally {
    requeuing.value = false
  }
}

async function loadGraph() {
  loadingGraph.value = true
  try {
    graph.value = await api.kbGraph()
  } catch (e) {
    toast.push(t('加载图谱失败：') + (e.message || e))
  } finally {
    loadingGraph.value = false
  }
  // loadingGraph=false 后 v-else 的图谱容器才挂载，nextTick 等 ref 绑定后再初始化 ECharts
  await nextTick()
  renderGraph()
}

async function openConcept(id) {
  try {
    concept.value = await api.kbConcept(id)
  } catch (e) {
    toast.push(t('加载概念失败：') + (e.message || e))
  }
}

const aiBusy = ref(false)
async function summarizeConcept() {
  if (aiBusy.value || !concept.value) return
  aiBusy.value = true
  try {
    const id = concept.value.tag.id
    await api.kbConceptSummarize(id)
    concept.value = await api.kbConcept(id)
    toast.push(t('摘要已生成'))
  } catch (e) {
    toast.push(t('AI 摘要生成失败：') + (e.message || e))
  } finally {
    aiBusy.value = false
  }
}
function fmtTime(ms) {
  if (!ms) return ''
  const d = new Date(ms)
  const p = (n) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`
}

async function createCol() {
  const name = newColName.value.trim()
  if (!name) return
  let kind = 'manual'
  let query = ''
  if (newColKind.value === 'smart') {
    kind = 'smart'
    query = JSON.stringify({
      tag: smartTag.value || '',
      kind: smartKind.value || '',
      q: smartQ.value.trim() || ''
    })
    if (!query || query === '{}') query = ''
  }
  try {
    await api.createCollection(name, kind, query)
    newColName.value = ''
    smartTag.value = ''
    smartKind.value = ''
    smartQ.value = ''
    collections.value = await api.listCollections()
    toast.push(kind === 'smart' ? `已创建智能集合「${name}」` : `已创建集合「${name}」`)
  } catch (e) {
    toast.push(t('创建失败：') + (e.message || e))
  }
}

function describeQuery(query) {
  try {
    const q = JSON.parse(query)
    const parts = []
    if (q.tag) parts.push(t('标签: ') + q.tag)
    if (q.kind) parts.push(q.kind === 'dir' ? i18t('仅目录') : i18t('仅文件'))
    if (q.q) parts.push(t('含「') + q.q + '」')
    return parts.join(' · ') || i18t('全部文件')
  } catch {
    return t('动态')
  }
}

const confirmRemoveCol = ref(null)

function doRemoveCol(c) {
  confirmRemoveCol.value = null
  api
    .deleteCollection(c.id)
    .then(async () => {
      collections.value = await api.listCollections()
      if (openColId.value === c.id) { openColId.value = ''; colFiles.value = [] }
      toast.push(t('已删除集合'))
    })
    .catch((e) => toast.push(t(i18t('删除失败：')) + (e.message || e)))
}

async function toggleCol(c) {
  if (openColId.value === c.id) {
    openColId.value = ''
    return
  }
  openColId.value = c.id
  colFilesLoading.value = true
  try {
    colFiles.value = await api.listCollectionFiles(c.id)
  } catch (e) {
    toast.push(t('加载集合文件失败：') + (e.message || e))
  } finally {
    colFilesLoading.value = false
  }
}

function removeColFile(c, f) {
  api
    .removeCollectionFile(c.id, f.id)
    .then(async () => {
      colFiles.value = colFiles.value.filter((x) => x.id !== f.id)
      const cc = collections.value.find((x) => x.id === c.id)
      if (cc) cc.file_count = Math.max(0, cc.file_count - 1)
    })
    .catch((e) => toast.push(t('移出失败：') + (e.message || e)))
}

function switchTab(k) {
  tab.value = k
  if (k === 'graph') {
    if (!graph.value.nodes.length) loadGraph()
    else if (chart) setTimeout(() => chart.resize(), 50) // v-show 切换后容器尺寸恢复，重算
  }
  if (k === 'lint' && !lint.value) loadLint()
}

function openFile(f) {
  if (f.kind === 'dir') router.push({ path: '/files', query: { parent: f.id } })
  else router.push('/read/' + f.id)
}

function clip(s, n) {
  if (!s) return ''
  return s.length > n ? s.slice(0, n) + '…' : s
}
function fmtSize(n) {
  if (!n) return '—'
  if (n < 1024) return n + ' B'
  if (n < 1048576) return (n / 1024).toFixed(1) + ' KB'
  return (n / 1048576).toFixed(1) + ' MB'
}

onMounted(() => {
  loadAll()
  window.addEventListener('resize', onWindowResize)
})
onBeforeUnmount(() => {
  stopPolling()
  window.removeEventListener('resize', onWindowResize)
  if (chart) {
    chart.dispose()
    chart = null
  }
})

// 解读队列实时轮询：有待解读/失败时每 15s 静默刷新总览，进度自动可见
let pollTimer = null
function startPolling() {
  stopPolling()
  const o = overview.value
  if (!o || (o.summary_pending === 0 && o.summary_error === 0)) return
  pollTimer = setInterval(async () => {
    try {
      const o2 = await api.kbOverview()
      overview.value = o2
      if (o2.summary_pending === 0 && o2.summary_error === 0) stopPolling()
    } catch (e) { /* 静默，下轮再试 */ }
  }, 15000)
}
function stopPolling() {
  if (pollTimer) { clearInterval(pollTimer); pollTimer = null }
}
</script>

<style scoped>
.kb-view { max-width: 1200px; margin: 0 auto; padding: 20px 24px 40px; }
.kb-head { display: flex; justify-content: space-between; align-items: flex-start; margin-bottom: 14px; }
.kb-head h2 { font-size: 20px; margin: 0 0 4px; }
.kb-sub { color: var(--text-3, #8a919f); font-size: 13px; margin: 0; }
.kb-tools { display: flex; gap: 8px; }
.kb-tabs { display: flex; gap: 6px; margin-bottom: 16px; border-bottom: 1px solid var(--border, #ececf1); }
.kb-tab { display: inline-flex; align-items: center; gap: 6px; padding: 8px 14px; background: none; border: none;
  border-bottom: 2px solid transparent; color: var(--text-2, #565d6d); cursor: pointer; font-size: 13px; }
.kb-tab.active { color: var(--accent, #2f6bff); border-bottom-color: var(--accent, #2f6bff); font-weight: 600; }

.kb-cards { display: grid; grid-template-columns: repeat(auto-fit, minmax(120px, 1fr)); gap: 10px; margin-bottom: 16px; }
.kb-card { background: var(--bg-2, #fff); border: 1px solid var(--border, #ececf1); border-radius: 10px; padding: 14px; }
.kb-card-num { font-size: 22px; font-weight: 700; }
.kb-card-label { font-size: 12px; color: var(--text-3, #8a919f); margin-top: 2px; }

.kb-cols { display: grid; grid-template-columns: repeat(auto-fit, minmax(300px, 1fr)); gap: 12px; }
.kb-panel { background: var(--bg-2, #fff); border: 1px solid var(--border, #ececf1); border-radius: 10px; padding: 14px; }
.kb-panel-head { display: flex; justify-content: space-between; align-items: center; font-weight: 600; font-size: 13px; margin-bottom: 10px; }
.kb-panel-ops { display: flex; gap: 6px; }
.kb-col-input { width: 160px; padding: 5px 8px; border: 1px solid var(--border, #ececf1); border-radius: 6px; font-size: 12px; }

.kb-tag-list, .kb-file-list, .kb-sum-list, .kb-col-list { display: flex; flex-direction: column; gap: 4px; }
.kb-tag-item { display: flex; align-items: center; gap: 8px; padding: 6px 8px; border-radius: 6px; cursor: pointer; }
.kb-tag-item:hover { background: var(--bg-3, #f5f6f8); }
.kb-tag-path { font-size: 13px; flex: 0 0 auto; }
.kb-tag-count { font-size: 11px; color: var(--text-3, #8a919f); background: var(--bg-3, #f5f6f8); padding: 1px 7px; border-radius: 8px; }
.kb-tag-snippet { font-size: 12px; color: var(--text-3, #8a919f); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.kb-file-item { display: flex; align-items: center; gap: 8px; padding: 6px 8px; border-radius: 6px; cursor: pointer; }
.kb-file-item:hover { background: var(--bg-3, #f5f6f8); }
.kb-file-item.on { background: var(--primary-soft, #eaf1ff); }
.kb-file-chk { flex: 0 0 auto; cursor: pointer; accent-color: var(--primary, #2f6bff); }
.kb-orphan-bar { display: flex; align-items: center; gap: 8px; font-weight: 400; font-size: 12px; color: var(--text-2, #4b5563); flex-wrap: wrap; }
.kb-orphan-all { display: inline-flex; align-items: center; gap: 4px; cursor: pointer; }
.kb-orphan-all input { accent-color: var(--primary, #2f6bff); cursor: pointer; }
.kb-orphan-cnt { color: var(--primary, #2f6bff); font-weight: 600; }
.kb-file-ico { flex: 0 0 auto; color: var(--text-3, #8a919f); }
.kb-file-name { flex: 1; font-size: 13px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.kb-file-meta { font-size: 12px; color: var(--text-3, #8a919f); flex: 0 0 auto; }
.kb-sum-item { padding: 6px 8px; border-radius: 6px; }
.kb-sum-item:hover { background: var(--bg-3, #f5f6f8); }
.kb-sum-name { font-size: 13px; font-weight: 600; }
.kb-sum-text { font-size: 12px; color: var(--text-3, #8a919f); margin-top: 2px; }

.kb-graph-wrap { position: relative; background: var(--bg-2, #fff); border: 1px solid var(--border, #ececf1); border-radius: 10px; padding: 12px; }
.kb-graph-bar { display: flex; align-items: center; gap: 6px; margin-bottom: 8px; }
.kb-graph-search { flex: 0 0 200px; padding: 5px 10px; border: 1px solid var(--border, #ececf1); border-radius: 6px; font-size: 12px; outline: none; background: var(--bg-3, #f5f6f8); }
.kb-graph-search:focus { border-color: var(--accent, #2f6bff); background: var(--bg-1, #fff); }
.kb-graph-tag { flex: 0 0 170px; max-width: 170px; font-size: 12px; }
.kb-graph-filelab { display: inline-flex; align-items: center; gap: 4px; font-size: 12px; color: var(--text-2, #565d6d); white-space: nowrap; cursor: pointer; }
.kb-graph-filelab input { margin: 0; accent-color: var(--accent, #2f6bff); }
.kb-graph-echarts { width: 100%; height: 66vh; min-height: 420px; }
.kb-zoom-hint { position: absolute; top: 48px; right: 20px; z-index: 5; pointer-events: none; font-size: 11px; color: var(--text-2, #565d6d); background: rgba(255, 255, 255, 0.72); padding: 2px 8px; border-radius: 6px; white-space: nowrap; }
.kb-graph-ops { display: flex; align-items: center; gap: 6px; margin-top: 10px; padding: 8px 10px; border: 1px solid var(--border, #ececf1); border-radius: 8px; background: var(--bg-3, #f5f6f8); font-size: 12px; }
.kb-ops-name { font-weight: 600; max-width: 260px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.kb-ops-type { font-size: 11px; color: var(--text-3, #8a919f); border: 1px solid var(--border, #ececf1); border-radius: 8px; padding: 0 6px; }
.kb-ops-spacer { flex: 1; }
.kb-legend { display: flex; gap: 16px; align-items: center; font-size: 12px; margin-top: 10px; color: var(--text-2, #565d6d); }
.dot { display: inline-block; width: 9px; height: 9px; border-radius: 50%; margin-right: 4px; }
.dot-file { background: #4e7bff; }
.dot-tag { background: #19b394; }

.kb-col-item { border: 1px solid var(--border, #ececf1); border-radius: 8px; margin-bottom: 8px; padding: 8px 10px; }
.kb-col-main { display: flex; align-items: center; gap: 8px; cursor: pointer; }
.kb-col-name { font-size: 13px; font-weight: 600; flex: 1; }
.kb-col-count, .kb-col-kind { font-size: 11px; color: var(--text-3, #8a919f); }
.kb-col-query { font-size: 11px; color: var(--accent, #2f6bff); background: var(--bg-3, #f5f6f8); padding: 1px 8px; border-radius: 8px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; max-width: 300px; }
.kb-col-files { margin-top: 8px; padding-top: 8px; border-top: 1px dashed var(--border, #ececf1); }

.kb-col-new { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; margin-bottom: 12px; }
.kb-col-new .kb-col-input { flex: 0 0 auto; }
.kb-kind-switch { display: inline-flex; border: 1px solid var(--border, #ececf1); border-radius: 8px; overflow: hidden; }
.kb-kind-btn { padding: 5px 14px; font-size: 12px; background: none; border: none; cursor: pointer; color: var(--text-2, #565d6d); }
.kb-kind-btn.active { background: var(--accent, #2f6bff); color: #fff; }
.btn-primary { background: var(--accent, #2f6bff); border-color: var(--accent, #2f6bff); color: #fff; }
.btn-primary:disabled { opacity: .5; cursor: default; }

.kb-modal-mask { position: fixed; inset: 0; background: rgba(20, 22, 28, .45); display: flex; align-items: center; justify-content: center; z-index: 100; }
.kb-modal { background: var(--bg-1, #fff); border-radius: 12px; width: min(640px, 92vw); max-height: 80vh; display: flex; flex-direction: column; }
.kb-modal-head { display: flex; justify-content: space-between; align-items: flex-start; padding: 16px 18px 0; }
.kb-modal-head h3 { margin: 0 0 4px; font-size: 16px; }
.kb-concept-sum { margin: 12px 18px 0; padding: 10px 12px; background: var(--bg-3, #f5f6f8); border-radius: 8px; font-size: 13px; line-height: 1.6; }
.kb-modal-actions { display: flex; align-items: center; gap: 8px; }
.kb-sum-meta { display: flex; align-items: center; gap: 6px; margin-top: 8px; font-size: 11px; color: var(--text-2, #565d6d); }
.kb-sum-badge { padding: 1px 6px; border-radius: 4px; background: var(--primary, #4b6bfb); color: #fff; font-size: 10px; font-weight: 600; }
.kb-sum-srcs { display: flex; flex-wrap: wrap; align-items: center; gap: 6px; margin-top: 8px; font-size: 12px; }
.kb-sum-src { border: none; background: none; color: var(--primary, #4b6bfb); cursor: pointer; padding: 0; font-size: 12px; }
.kb-sum-src:hover { text-decoration: underline; }
.kb-rel-tags { margin: 10px 18px 0; display: flex; flex-wrap: wrap; align-items: center; gap: 6px; font-size: 12px; }
.kb-rel-tag { display: inline-flex; align-items: center; gap: 5px; padding: 3px 9px; border-radius: 12px; border: 1px solid var(--border, #ececf1); background: none; cursor: pointer; font-size: 12px; }
.kb-rel-tag:hover { border-color: var(--accent, #2f6bff); color: var(--accent, #2f6bff); }
.kb-concept-files { padding: 12px 18px 18px; overflow-y: auto; display: flex; flex-direction: column; gap: 12px; }
.kb-concept-sec { display: flex; flex-direction: column; gap: 4px; }
.kb-concept-sec-head { display: flex; justify-content: space-between; align-items: center; font-weight: 600; font-size: 12px; margin-bottom: 2px; color: var(--text-2, #565d6d); }

.kb-empty { text-align: center; color: var(--text-3, #8a919f); padding: 40px 0; font-size: 13px; }
.kb-empty-sm { text-align: center; color: var(--text-3, #8a919f); padding: 16px 0; font-size: 12px; }
.kb-qstat { display: inline-flex; align-items: center; gap: 6px; flex-wrap: wrap; }
.kb-requeue-ext { width: auto; min-width: 88px; font-size: 12px; padding: 2px 6px; height: 24px; }
.btn-ghost { background: none; }

/* ---- 质量 lint ---- */
.kb-cards-sm .kb-card { padding: 10px 12px; }
.kb-cards-sm .kb-card-num { font-size: 18px; }
.kb-warn { border-color: #f0a14b !important; }
.kb-warn .kb-card-num { color: #e08a2e; }
.kb-lint-dups { display: flex; flex-direction: column; gap: 8px; }
.kb-dup-group { border: 1px solid var(--border, #ececf1); border-radius: 8px; padding: 6px 8px; }
.kb-dup-bar { display: flex; align-items: center; justify-content: space-between; gap: 8px; margin-bottom: 2px; }
.kb-dup-sha { font-size: 11px; color: var(--text-3, #8a919f); font-family: monospace; }
.kb-dup-del { color: #e08a2e; }
.kb-dup-radio { margin: 0; flex: 0 0 auto; accent-color: var(--accent, #2f6bff); }
.kb-dup-keep { background: rgba(47, 107, 255, .06); }
.kb-dup-flag { font-size: 11px; color: var(--accent, #2f6bff); font-weight: 600; flex: 0 0 auto; }
.kb-broken-target { max-width: 180px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: #e08a2e; }
.kb-wrap { flex-wrap: wrap; }
.kb-lint-note { margin-top: 10px; font-size: 11px; }
.kb-lint-tags { gap: 8px; }
.kb-lint-tag { display: inline-flex; align-items: center; gap: 2px; }
.kb-lint-tag-del { border: none; background: none; color: var(--text-3, #8a919f); cursor: pointer; font-size: 14px; padding: 0 2px; line-height: 1; border-radius: 4px; }
.kb-lint-tag-del:hover { color: #e03e3e; background: rgba(224, 62, 62, .1); }
</style>
