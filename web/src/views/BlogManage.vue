<template>
  <div class="bm-view">
    <!-- 说明条：博客目录在存储层不挂载 -->
    <div class="bm-tip">
      <AikIcon name="info" :size="16" />
      <span>{{  $t('「博客」目录已对文件管理可见：可在此管理，也可在文件库直接拖拽 md 到博客目录发布。根目录「博客」不可删除/改名。')  }}</span>
    </div>

    <!-- 博客状态 -->
    <div class="bm-status" v-if="status">
      <span class="bm-status-dot" :class="{ off: !status.active }"></span>
      <span>{{  status.active ? $t('博客公开中') : $t('博客已关闭')  }}</span>
      <button v-if="status.active" class="btn btn-sm bm-toggle" :disabled="busy" @click="toggleBlog(false)">{{  $t('关闭博客')  }}</button>
      <button v-else class="btn btn-sm bm-toggle on" :disabled="busy" @click="toggleBlog(true)">{{  $t('开启博客')  }}</button>
      <a class="bm-open" :href="status.url" target="_blank">{{  $t('打开博客')  }}</a>
    </div>

    <!-- 顶部功能标签（?tab= 深链可直达） -->
    <nav class="bm-nav-tabs">
      <button v-for="t in navTabs" :key="t.key" class="bm-tab-btn" :class="{ on: tab === t.key }" @click="switchTab(t.key)">
        {{  t.label  }}<span v-if="t.count" class="bm-tab-count">{{  t.count  }}</span>
      </button>
    </nav>

    <!-- 站点设置（博客名称/简介/页脚/SEO 默认描述；RSS/OG/公开页同源） -->
    <section v-show="tab === 'overview'" class="bm-panel bm-site">
      <div class="bm-panel-head">
        <h3>{{  $t('站点设置')  }}</h3>
        <button class="btn btn-sm" :disabled="busy || !siteDirty" @click="saveSite">{{  $t('保存站点设置')  }}</button>
      </div>
      <div class="bm-site-grid">
        <label class="bm-field">
          <span>{{  $t('博客名称（站点标题）')  }}</span>
          <input v-model="site.title" class="bm-input" maxlength="60" :placeholder="$t('爱库录博客')" @input="siteDirty = true" />
        </label>
        <label class="bm-field">
          <span>{{  $t('站点简介')  }}</span>
          <input v-model="site.description" class="bm-input" maxlength="200" :placeholder="$t('由爱库录知识库发布的公开文章')" @input="siteDirty = true" />
        </label>
        <label class="bm-field">
          <span>{{  $t('页脚文案')  }}</span>
          <input v-model="site.footer" class="bm-input" maxlength="200" :placeholder="$t('由爱库录发布 · 数据主权在发布者')" @input="siteDirty = true" />
        </label>
        <label class="bm-field">
          <span>{{  $t('SEO 默认描述（留空=站点简介）')  }}</span>
          <input v-model="site.seo_default" class="bm-input" maxlength="300" :placeholder="$t('留空则使用站点简介')" @input="siteDirty = true" />
        </label>
        <label class="bm-field bm-field-full">
          <span>{{  $t('自定义 CSS（前台公开页全主题生效）')  }}</span>
          <textarea v-model="site.custom_css" class="bm-input bm-textarea" rows="3" maxlength="20000" :placeholder="$t('直接粘贴 CSS，如 .ph-title { color: #e5484d }')" @input="siteDirty = true"></textarea>
        </label>
        <label class="bm-field bm-field-full">
          <span>{{  $t('自定义 JS（前台公开页全主题生效）')  }}</span>
          <textarea v-model="site.custom_js" class="bm-input bm-textarea" rows="3" maxlength="20000" :placeholder="$t('直接粘贴 JS（如统计脚本），公开页加载时执行')" @input="siteDirty = true"></textarea>
        </label>
        <label class="bm-field">
          <span>{{  $t('站点基础域名（RSS/canonical/OG 用；留空=跟随请求域）')  }}</span>
          <input v-model="site.base_url" class="bm-input" maxlength="200" placeholder="https://blog.example.com" @input="siteDirty = true" />
        </label>
        <label class="bm-field">
          <span>{{  $t('站点语言（RSS language / 前端 locale 同源）')  }}</span>
          <input v-model="site.locale" class="bm-input" maxlength="20" placeholder="zh-cn" @input="siteDirty = true" />
        </label>
        <label class="bm-field bm-field-check">
          <span>{{  $t('上传后自动发布')  }}</span>
          <input type="checkbox" v-model="autoPublish" @change="saveAutoPublish" />
          <span class="bm-check-hint">{{  $t('开启：拖拽 md 上传即公开；关闭：先存草稿，确认后再发')  }}</span>
        </label>
        <label class="bm-field bm-field-check">
          <span>{{  $t('访客评论（免登录）')  }}</span>
          <input type="checkbox" v-model="guestComments" @change="saveGuestComments" />
          <span class="bm-check-hint">{{  $t('开启：未登录读者可评论（默认待审核，通过后展示；IP 限流防刷）；关闭：仅登录用户可评论')  }}</span>
        </label>
        <label class="bm-field">
          <span>{{  $t('默认主题（访客默认看到，可自行切换）')  }}</span>
          <!-- 动态渲染：主题选项来自注册表（themes/*/index.js 注册即出现，无需改此处） -->
          <select v-model="site.theme" class="bm-input" @change="siteDirty = true">
            <option v-for="t in themeOptions" :key="t.id" :value="t.id">{{  t.label  }}</option>
          </select>
          <span class="bm-check-hint" style="font-size:12px;color:var(--text-3)">{{ $t('访客默认看到该主题；访客可用页头切换器换成自己习惯的主题（仅本机记住，不影响其他访客）。仅作用于交互版公开页；静态页 /blog 固定阅读壳（SEO）。外置主题部署到服务器 data/themes/<id>/ 即自动出现在本下拉，无需改白名单或重编译') }}</span>
        </label>
      </div>
    </section>

    <!-- 内容：分类 + 文章（双栏） -->
    <div v-show="tab === 'content'" class="bm-cols">
      <!-- 左：分类 -->
      <section class="bm-panel bm-cats">
        <div class="bm-panel-head">
          <h3>{{  $t('分类（子目录）')  }}</h3>
          <button class="btn btn-sm" :disabled="busy || savingOrder" @click="saveCatOrder">{{  $t('保存顺序')  }}</button>
        </div>
        <div class="bm-cat-filter">
          <button class="bm-cat-mini" :class="{ active: curCat === '' }" @click="curCat = ''">{{  $t('全部')  }}</button>
          <button class="bm-cat-mini" :class="{ active: curCat === '__none' }" @click="curCat = '__none'">{{  $t('未分类')  }}</button>
        </div>
        <div class="bm-cat-list">
          <div
            v-for="c in cats"
            :key="c.id"
            class="bm-cat-row"
            :class="{ dragging: dragId === c.id, dragover: dragOverId === c.id }"
            draggable="true"
            @dragstart="onDragStart(c, $event)"
            @dragover.prevent="onDragOver(c)"
            @dragleave="onDragLeave(c)"
            @drop.prevent="onDrop(c)"
            @dragend="onDragEnd"
          >
            <span class="bm-cat-handle" :title="$t('拖拽调整顺序')">⠿</span>
            <button class="bm-cat" :class="{ active: curCat === c.id }" @click="curCat = c.id">{{  c.name  }}</button>
            <span class="bm-cat-n">{{  catCount(c)  }}</span>
<span class="bm-cat-so" :title="c.sort_order >= 0 ? $t('手动权重 ') + c.sort_order : $t('未设置（按最新活动动态排序）')">{{ c.sort_order >= 0 ? '#' + c.sort_order : $t('自动') }}</span>
            <span class="bm-cat-la" :title="$t('该分类最近一篇文章更新时间')">{{  fmtDate(c.last_activity)  }}</span>
          </div>
        </div>
        <div class="bm-cat-actions">
          <button class="btn btn-sm bm-reset" :disabled="busy || savingOrder" @click="resetCatOrder">{{  $t('恢复动态排序')  }}</button>
        </div>
        <div class="bm-cat-new">
          <input v-model="newCat" class="bm-input" :placeholder="$t('新分类名称，如：设计')" @keyup.enter="addCat" />
          <button class="btn btn-sm" :disabled="busy" @click="addCat">{{  $t('新建')  }}</button>
        </div>
      </section>

      <!-- 右：文章 -->
      <section class="bm-panel bm-posts">
        <div class="bm-panel-head">
          <h3>{{  $t('文章')  }}</h3>
          <span class="bm-count">{{  shownPosts.length  }} {{  $t('篇')  }}</span>
        </div>

        <!-- 批量编辑（A-G 批量编辑入口）：多选后统一设置内容类型 -->
        <div class="bm-batch-bar" v-if="shownPosts.length">
          <label class="bm-cbx"><input type="checkbox" :checked="allSelected" @change="toggleAll" /> {{  $t('全选本页')  }}</label>
          <span class="bm-batch-n">{{  $t('已选')  }} {{  selectedCount  }} {{  $t('篇')  }}</span>
          <select v-model="batchNodeType" class="bm-input bm-select">
            <option value="">{{  $t('内容类型（不选=不改）')  }}</option>
            <option value="article">{{  $t('文章')  }}</option>
            <option value="resource">{{  $t('资源')  }}</option>
            <option value="link">{{  $t('外链')  }}</option>
          </select>
          <button class="btn btn-sm bm-primary" :disabled="busy || !selectedCount || !batchNodeType" @click="applyBatch">{{  $t('批量设置类型')  }}</button>
          <button class="btn btn-sm" :disabled="busy || !selectedCount" @click="clearSel">{{  $t('清除选择')  }}</button>
        </div>

        <div class="bm-new">
          <div class="bm-new-row">
            <input v-model="newTitle" class="bm-input" :placeholder="$t('文章标题（不含扩展名）')" />
            <select v-model="newCatFor" class="bm-input bm-select">
              <option value="">{{  $t('博客根（未分类）')  }}</option>
              <option v-for="c in cats" :key="c.id" :value="c.id">{{  c.name  }}</option>
            </select>
          </div>
          <textarea v-model="newBody" class="bm-textarea" rows="5" :placeholder="$t('Markdown 正文…')"></textarea>
          <div class="bm-new-actions">
            <button class="btn btn-sm bm-primary" :disabled="busy" @click="addPost">{{  $t('发布')  }}</button>
          </div>
        </div>

        <!-- 上传区：拖拽（含文件夹）/ 选文件 / 选文件夹，上传到当前分类 -->
        <div
          class="bm-upzone"
          :class="{ over: dragOver }"
          @dragover.prevent="dragOver = true"
          @dragleave.prevent="dragOver = false"
          @drop.prevent="onDropZone"
        >
          <div class="bm-upzone-inner">
            <AikIcon name="upload" :size="20" />
            <span class="bm-upzone-text">{{  $t('拖拽文件 / 文件夹到此处上传')  }}</span>
            <span class="bm-upzone-sep">{{  $t('或')  }}</span>
            <label class="btn btn-sm bm-up">
              <input type="file" multiple @change="onPick" />{{  $t('选文件')  }}
            </label>
            <label class="btn btn-sm bm-up">
              <input type="file" webkitdirectory @change="onPickDir" />{{  $t('选文件夹')  }}
            </label>
          </div>
          <div class="bm-upzone-meta">
            <span v-if="dragOver" class="bm-upzone-hint">{{  $t('松开即上传')  }}</span>
            <span v-else-if="autoPublish">{{  $t('上传至：')  }}{{  uploadTargetLabel  }} {{  $t('· md 将自动发布')  }}</span>
            <span v-else>{{  $t('上传至：')  }}{{  uploadTargetLabel  }} {{  $t('· 默认草稿（可在站点设置开启自动发布）')  }}</span>
            <span v-if="upQueue.length" class="bm-upzone-q">{{  upQueue.length  }} {{  $t('个文件待上传…')  }}</span>
          </div>
        </div>

        <div v-if="loading" class="bm-hint">{{  $t('加载中…')  }}</div>
        <div v-else-if="err" class="bm-hint err">{{  err  }}</div>
        <div v-else-if="!shownPosts.length" class="bm-hint">{{  $t('当前分类还没有文章')  }}</div>
        <div v-else class="bm-post-list">
          <div v-for="p in shownPosts" :key="p.id" class="bm-post">
            <input type="checkbox" class="bm-row-cbx" :checked="!!selMap[p.id]" @change="onSel(p, $event)" :title="$t('选择 {name}', { name: p.name })" />
            <a class="bm-post-name" :title="p.path" :href="'#/p/blog?path=' + encodeURIComponent(p.path)" target="_blank">{{  p.name  }}</a>
            <span v-if="nodeTypeOf(p)" class="bm-type-tag" :title="$t('内容类型：{type}', { type: nodeTypeOf(p) })">{{  typeLabel(nodeTypeOf(p))  }}</span>
            <span class="bm-post-meta">{{  p.path  }}</span>
            <span class="bm-post-meta bm-views" :title="$t('累计阅读（PV，1 小时内同一读者不重复计数）')">{{  $t('阅读')  }} {{  p.view_count ?? 0  }}</span>
            <span class="bm-pin" :title="$t('置顶范围：无 / 分类置顶 / 全站置顶')">
              <select v-model="pinDraft[p.id].scope" :disabled="busy">
                <option value="none">{{  $t('不置顶')  }}</option>
                <option value="category">{{  $t('分类置顶')  }}</option>
                <option value="global">{{  $t('全站置顶')  }}</option>
              </select>
              <input type="number" min="0" v-model.number="pinDraft[p.id].order" :disabled="busy" :placeholder="$t('序')" :title="$t('置顶顺序（越小越前，留空=按更新时间）')" />
              <button class="bm-pin-apply" :disabled="busy" @click="applyPin(p)" :title="$t('保存置顶设置')">{{  $t('置顶')  }}</button>
            </span>
            <span class="bm-pin" :title="pwdDraft[p.id] ? $t('设置文章访问密码（留空=清除）') : ''">
              <input :type="pwdDraft[p.id] === '' && p.has_pwd ? 'text' : 'password'" v-model="pwdDraft[p.id]" :disabled="busy" :placeholder="p.has_pwd ? $t('已加密') : $t('密码')" :title="$t('文章访问密码（留空保存=清除）')" />
              <button class="bm-pin-apply" :disabled="busy" @click="applyPwd(p)" :title="$t('保存访问密码')">{{  $t('密码')  }}</button>
            </span>
            <span class="bm-pin" :title="$t('定时发布（{state}）', { state: p.publish_at ? $t('已定时') : $t('留空=立即发布') })">
              <input type="datetime-local" v-model="schDraft[p.id]" :disabled="busy" :title="$t('定时发布（留空保存=立即发布）')" />
              <button class="bm-pin-apply" :disabled="busy" @click="applySch(p)" :title="$t('保存定时发布')">{{  $t('定时')  }}</button>
            </span>
            <button class="bm-del" :title="$t('删除（文章下线）')" @click="askDel(p)">{{  $t('删除')  }}</button>
          </div>
        </div>
      </section>
    </div>

    <!-- 统计看板（浏览/评论/分类/标签聚合；Chart.js 按需分包加载，切入统计标签时自动加载） -->
    <section v-show="tab === 'stats'" class="bm-panel bm-stats">
      <div class="bm-panel-head">
        <h3>{{  $t('统计看板')  }}</h3>
        <span class="bm-hook-hint">{{  $t('基于文章浏览、评论、标签聚合（无逐日流水）')  }}</span>
        <span class="bm-spacer"></span>
      </div>
      <template v-if="statsOpen">
        <div v-if="statsLoading && !stats" class="bm-hint">{{  $t('统计加载中…')  }}</div>
        <template v-else-if="stats">
          <div class="bm-stat-cards">
            <div class="bm-stat-card"><span class="n">{{  stats.totals.posts  }}</span><span class="l">{{  $t('文章')  }}</span></div>
            <div class="bm-stat-card"><span class="n">{{  stats.totals.published  }}</span><span class="l">{{  $t('已发布')  }}</span></div>
            <div class="bm-stat-card"><span class="n">{{  stats.totals.views  }}</span><span class="l">{{  $t('总浏览')  }}</span></div>
            <div class="bm-stat-card"><span class="n">{{  stats.totals.comments_total  }}</span><span class="l">{{  $t('评论')  }}</span></div>
            <div class="bm-stat-card"><span class="n">{{  stats.totals.comments_pending  }}</span><span class="l">{{  $t('待审')  }}</span></div>
          </div>
          <div v-if="stats.categories.length" class="bm-stat-charts">
            <canvas ref="catChartEl"></canvas>
          </div>
          <div class="bm-stat-cols">
            <div class="bm-stat-col">
              <p class="bm-stat-sub">{{  $t('热门文章 TOP10')  }}</p>
              <div v-if="!stats.top_posts.length" class="bm-hint">{{  $t('暂无数据')  }}</div>
              <div v-for="t in stats.top_posts" :key="t.id" class="bm-top-row">
                <span class="bm-top-name" :title="t.name">{{  (t.name || '').replace(/\.(md|markdown)$/i, '')  }}</span>
                <span class="bm-top-n">{{  t.views  }} {{  $t('浏览')  }}</span>
              </div>
            </div>
            <div class="bm-stat-col">
              <p class="bm-stat-sub">{{  $t('热门标签')  }}</p>
              <div v-if="!stats.tags.length" class="bm-hint">{{  $t('暂无标签')  }}</div>
              <div class="bm-tag-cloud">
                <span v-for="t in stats.tags" :key="t.name" class="bm-tag-chip2">#{{  t.name  }}<i>{{  t.count  }}</i></span>
              </div>
            </div>
          </div>
        </template>
      </template>
    </section>

    <!-- 评论管理（待审/已通过；访客评论审核） -->
    <section v-show="tab === 'comments'" class="bm-panel bm-cmts">
      <div class="bm-panel-head">
        <h3>{{ $t('评论管理') }}</h3>
        <span class="bm-cmt-filter">
          <button class="bm-cat-mini" :class="{ active: cmtFilter === 'pending' }" @click="cmtFilter = 'pending'; loadCmts()">{{ $t('待审') }}</button>
          <button class="bm-cat-mini" :class="{ active: cmtFilter === 'approved' }" @click="cmtFilter = 'approved'; loadCmts()">{{ $t('已通过') }}</button>
          <button class="bm-cat-mini" :class="{ active: cmtFilter === '' }" @click="cmtFilter = ''; loadCmts()">{{ $t('全部') }}</button>
        </span>
      </div>
      <div v-if="cmtSel.length" class="bm-cmt-bar">
        <span class="bm-hint">{{ $t('已选') }} {{  cmtSel.length  }} {{ $t('条已通过评论') }}</span>
        <button class="btn btn-sm bm-primary" :disabled="busy" @click="fuseSelected">{{ $t('AI 融合收录（') }}{{  cmtSel.length  }}）</button>
        <button class="bm-cat-mini" :disabled="busy" @click="cmtSel = []">{{ $t('清空') }}</button>
        <span class="bm-hint">{{ $t('融合式：AI 只把多条回复整理成一段，先出草稿、确认后才写入正文') }}</span>
      </div>
      <div v-if="!cmts.length" class="bm-hint">{{  cmtFilter === 'pending' ? $t('没有待审评论') : $t('暂无评论')  }}</div>
      <div v-else class="bm-cmt-list">
        <div v-for="c in cmts" :key="c.id" class="bm-cmt-row">
          <input v-if="c.status === 'approved'" v-model="cmtSel" class="bm-cmt-ck" type="checkbox" :value="c.id" />
          <span v-else class="bm-cmt-ck-ph"></span>
          <span class="bm-cmt-status" :class="c.status">{{  c.status === 'pending' ? $t('待审') : $t('已通过')  }}</span>
          <span class="bm-cmt-author">{{  c.author  }}</span>
          <span class="bm-cmt-body" :title="c.body">{{  c.body  }}</span>
          <span class="bm-cmt-file" :title="c.file_name">{{  c.file_name  }}</span>
          <button v-if="c.status === 'approved'" class="btn btn-sm" :disabled="busy" @click="ingestCmt(c)">{{ $t('收录') }}</button>
          <button v-if="c.status === 'pending'" class="btn btn-sm" :disabled="busy" @click="approveCmt(c)">{{ $t('通过') }}</button>
          <button class="bm-del" :disabled="busy" @click="delCmt(c)">{{ $t('删除') }}</button>
        </div>
      </div>
    </section>

    <!-- 收录（B3 内容引用与归因：评论带署名与回指链接补录进正文）
         后端 7 端点：quote / fuse-draft / {id}/accept / {id}·DELETE / list / my-count / pending -->
    <section v-show="tab === 'ingest'" class="bm-panel bm-ingests">
      <div class="bm-panel-head">
        <h3>{{ $t('收录') }}</h3>
        <span class="bm-hint">{{ $t('评论区高价值内容带署名 + 回指原评论链接补录进正文（旧文自生长；只追加、不改既有人工内容）') }}</span>
        <button class="bm-cat-mini" :disabled="busy" @click="loadIngests()">{{ $t('刷新') }}</button>
      </div>

      <div class="bm-ingest-block">
        <div class="bm-ingest-sub">{{ $t('待采纳的 AI 融合草稿（') }}{{  ingestPendings.length  }}）</div>
        <div v-if="!ingestPendings.length" class="bm-hint">{{ $t('没有待采纳草稿。到「评论」页勾选同篇文章的已通过评论 → 「AI 融合收录」即生成。') }}</div>
        <div v-else class="bm-cmt-list">
          <div v-for="g in ingestPendings" :key="g.id" class="bm-ingest-card">
            <div class="bm-ingest-card-head">
              <span class="bm-ingest-tag">{{ $t('融合草稿') }}</span>
              <span class="bm-cmt-file" :title="g.title">{{  g.title || $t('（文章）')  }}</span>
              <span class="bm-ingest-time">{{  fmtIngestTime(g.created_at)  }}</span>
              <button class="btn btn-sm bm-primary" :disabled="busy" @click="acceptIngest(g)">{{ $t('采纳写入正文') }}</button>
              <button class="bm-del" :disabled="busy" @click="discardIngest(g)">{{ $t('放弃') }}</button>
            </div>
            <pre class="bm-ingest-draft">{{  g.draft  }}</pre>
          </div>
        </div>
      </div>

      <div class="bm-ingest-block">
        <div class="bm-ingest-sub">{{ $t('收录记录（最近 100 条）') }}</div>
        <div v-if="!ingestRecords.length" class="bm-hint">{{ $t('暂无收录记录') }}</div>
        <div v-else class="bm-cmt-list">
          <div v-for="g in ingestRecords" :key="g.id" class="bm-cmt-row">
            <span class="bm-cmt-status" :class="ingestStatusClass(g.status)">{{  ingestStatusText(g.status)  }}</span>
            <span class="bm-ingest-tag">{{  g.mode === 'fuse' ? $t('融合') : $t('引用')  }}</span>
            <span class="bm-cmt-author">{{  g.title || '—'  }}</span>
            <span class="bm-cmt-body">{{  (g.comments || []).length  }} {{ $t('条评论 · v') }}{{  g.version_before  }}→v{{  g.version_after  }} · {{  fmtIngestTime(g.created_at)  }}</span>
            <button v-if="g.status === 'accepted'" class="bm-del" :disabled="busy" @click="revertIngest(g)">{{ $t('撤销') }}</button>
          </div>
        </div>
      </div>
    </section>

    <!-- Webhook 推送（事件契约 v1 投递器：发文/评论等事件外发） -->
    <section v-show="tab === 'webhook'" class="bm-panel bm-hooks">
      <div class="bm-panel-head">
        <h3>{{ $t('Webhook 推送') }}</h3>
        <span class="bm-hook-hint">{{ $t('订阅内核事件（发文、评论…），HMAC 签名投递到你的地址') }}</span>
      </div>
      <div class="bm-hook-new">
        <input v-model="hookForm.url" class="bm-input" :placeholder="$t('https://your-server.com/hook（接收 POST）')" />
        <input v-model="hookForm.secret" class="bm-input bm-hook-secret" :placeholder="$t('签名密钥（可空）')" maxlength="128" />
        <select v-model="hookForm.topic" class="bm-input bm-select">
          <option v-for="t in kernelTopics" :key="t" :value="t">{{  t  }}</option>
        </select>
        <button class="btn btn-sm bm-primary" :disabled="busy" @click="addHook">{{ $t('添加') }}</button>
      </div>
      <div v-if="!hooks.length" class="bm-hint">{{ $t('尚未配置 webhook') }}</div>
      <div v-else class="bm-cmt-list">
        <div v-for="h in hooks" :key="h.id" class="bm-cmt-row">
          <span class="bm-cmt-status approved" :class="{ off: !h.enabled }">{{  h.enabled ? $t('启用') : $t('停用')  }}</span>
          <span class="bm-cmt-body" :title="h.url">{{  h.url  }}</span>
          <span class="bm-cmt-file">{{  (h.topics || []).join(', ')  }}</span>
          <button class="btn btn-sm" :disabled="busy" @click="testHook(h)">{{ $t('测试') }}</button>
          <button class="bm-del" :disabled="busy" @click="delHook(h)">{{ $t('删除') }}</button>
        </div>
      </div>
    </section>

    <!-- 删除确认（自定义弹窗，替代系统 confirm） -->
    <ConfirmDialog
      v-if="delTarget"
      :title="$t('删除文章')"
      :message="$t('删除「{v0}」？文章将从博客下线（可在回收站恢复）。', { v0: delTarget.name })"
      confirm-text="删除"
      cancel-text="取消"
      danger
      @confirm="doDel"
      @cancel="delTarget = null"
    />
  </div>
</template>

<script setup>
import { t as i18t } from '@/i18n'
import { ref, computed, nextTick, onMounted, onUnmounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AikIcon from '@/components/AikIcon.vue'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import { useToastStore } from '@/stores/toast'
import { requestJSON, mkdir, createDoc, uploadFiles, deleteFile, setFileStatus, blogPostsBatch } from '@/api'
import { listThemes, applyServerTheme } from '@/themes' // 主题选项来自注册表（BlogView 已静态 import 全部主题）
import { t } from '@/i18n'
const tr = t // 别名：部分作用域用 t 作局部变量（theme 对象/类型键），避免遮蔽

const BLOG_DIR = '00000000-0000-0000-0000-000000000010'

// ===== 顶部功能标签（概览/内容/统计/评论/Webhook；?tab= 深链直达） =====
const route = useRoute()
const router = useRouter()
const BM_TAB_KEYS = ['overview', 'content', 'stats', 'comments', 'ingest', 'webhook']
const tab = ref(BM_TAB_KEYS.includes(route.query.tab) ? route.query.tab : 'overview')
function switchTab(k) {
  tab.value = k
  router.replace({ query: { ...route.query, tab: k } })
}

// ===== 收录（B3 内容引用与归因：评论区 → 正文；只 append 不 mutate）=====
const ingestPendings = ref([]) // 待采纳的 AI 融合草稿（GET /ingests/pending）
const ingestRecords = ref([]) // 收录记录（GET /ingests，跨文章、站点隔离）
const cmtSel = ref([]) // 评论页多选（仅「已通过」可选，用于 AI 融合）

const navTabs = computed(() => [
  { key: 'overview', label: t('概览') },
  { key: 'content', label: t(i18t('内容')) },
  { key: 'stats', label: t('统计') },
  { key: 'comments', label: t('评论') },
  { key: 'ingest', label: t('收录'), count: ingestPendings.value.length || 0 },
  { key: 'webhook', label: 'Webhook', count: hooks.value.length || 0 }
])

// ===== 站点设置（blog.* 白名单；RSS/OG/公开页同源） =====
const site = ref({ title: '', description: '', logo: '', footer: '', seo_default: '', theme: 'aiklog', custom_css: '', custom_js: '', base_url: '', locale: 'zh-cn' })
const siteDirty = ref(false)
const autoPublish = ref(false)

// 主题下拉选项：由服务端 /public/site 的 theme_options 驱动
// （内置白名单 ∪ data/themes 下已安装外置主题）——装主题包即出现，无需改前端或重新编译。
// 交互版（SPA）主题须随主系统构建注册；外置主题仅作用于公网静态页 /blog。
const srvThemeOptions = ref(null)
const themeOptions = computed(() => {
  const reg = listThemes()
  const list = srvThemeOptions.value
  if (Array.isArray(list) && list.length) {
    return list.map((o) => {
      const t = reg.find((x) => x.id === o.id)
      const desc = t ? (t.desc || t.id) : tr(i18t('外置主题 · 仅静态页 /blog（交互版回退默认）'))
      return { id: o.id, label: `${o.title || o.id}（${desc}）` }
    })
  }
  // 兜底：服务端未返回候选时退回本地注册表
  return reg.map((t) => ({ id: t.id, label: `${t.title}（${t.desc || t.id}）` }))
})

async function loadSite() {
  try {
    const d = await requestJSON('/public/site')
    srvThemeOptions.value = Array.isArray(d?.theme_options) ? d.theme_options : null
    if (d?.site) {
      site.value = {
        title: d.site.title || '',
        description: d.site.description || '',
        logo: d.site.logo || '',
        footer: d.site.footer || '',
        seo_default: d.site.seo_default || '',
        theme: d.site.theme || 'aiklog',
        custom_css: d.site.custom_css || '',
        custom_js: d.site.custom_js || '',
        base_url: d.site.base_url || '',
        locale: d.site.locale || 'zh-cn',
      }
      siteDirty.value = false
    }
  } catch (_) { /* 离线时保持默认 */ }
}

async function saveAutoPublish() {
  try {
    await requestJSON('/admin/settings', {
      method: 'PUT',
      body: JSON.stringify({
        key: 'blog.auto_publish_on_upload',
        value: autoPublish.value ? 'true' : 'false',
      }),
    })
    toast.success(autoPublish.value ? t('已开启：上传 md 将自动发布') : t('已关闭：上传后保持草稿'))
  } catch (e) {
    toast.error(t('保存失败：') + (e.message || e))
  }
}

// ===== 统计看板（Chart.js 动态分包；切入统计标签时才加载） =====
const statsOpen = computed(() => tab.value === 'stats')
const statsLoading = ref(false)
const stats = ref(null)
const catChartEl = ref(null)
let catChart = null
watch(statsOpen, (on) => {
  if (on && !stats.value) loadStats()
  if (on && stats.value) nextTick(renderCatChart) // v-show 隐藏时 canvas 无尺寸，重进需重渲染
})
async function loadStats() {
  statsLoading.value = true
  try {
    stats.value = await requestJSON('/blog/stats')
    nextTick(renderCatChart)
  } catch (e) {
    toast.error(t('统计加载失败：') + (e.message || e))
  } finally {
    statsLoading.value = false
  }
}
async function renderCatChart() {
  const el = catChartEl.value
  const s = stats.value
  if (!el || !s?.categories?.length) return
  try {
    const { Chart } = await import('chart.js/auto')
    catChart?.destroy()
    catChart = new Chart(el, {
      type: 'bar',
      data: {
        labels: s.categories.map((c) => c.name),
        datasets: [
          { label: t('文章数'), data: s.categories.map((c) => c.posts), backgroundColor: 'rgba(79,124,255,.72)', borderRadius: 4 },
          { label: t('浏览量'), data: s.categories.map((c) => c.views), backgroundColor: 'rgba(46,168,122,.72)', borderRadius: 4 }
        ]
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        scales: { y: { beginAtZero: true, ticks: { precision: 0 } } },
        plugins: { legend: { position: 'bottom' } }
      }
    })
  } catch (_) { /* 图表加载失败不阻断数字展示 */ }
}
onUnmounted(() => catChart?.destroy())

// ===== 评论管理（待审/已通过） =====
const cmts = ref([])
const cmtFilter = ref('pending')
const guestComments = ref(false)
const pendingCmts = ref(0)

async function saveGuestComments() {
  try {
    await requestJSON('/admin/settings', {
      method: 'PUT',
      body: JSON.stringify({ key: 'blog.comments_guest', value: guestComments.value ? 'true' : 'false' }),
    })
    toast.success(guestComments.value ? t('已开放访客评论（默认待审）') : t('已关闭访客评论（仅登录可评）'))
  } catch (e) {
    toast.error(t('保存失败：') + (e.message || e))
  }
}

async function loadCmts() {
  cmtSel.value = [] // 过滤/刷新后旧选择失效，避免对着看不见的行做融合
  try {
    const d = await requestJSON('/blog/comments?status=' + encodeURIComponent(cmtFilter.value) + '&limit=200')
    cmts.value = d?.items || []
  } catch (_) {
    cmts.value = []
  }
}

async function approveCmt(c) {
  try {
    await requestJSON(`/blog/comments/${c.id}/approve`, { method: 'POST' })
    toast.success(t('评论已通过并公开'))
    loadCmts()
  } catch (e) {
    toast.error(t('审核失败：') + (e.message || e))
  }
}

async function delCmt(c) {
  try {
    await requestJSON(`/blog/comments/${c.id}`, { method: 'DELETE' })
    toast.success(t('评论已删除'))
    loadCmts()
  } catch (e) {
    toast.error(t(i18t('删除失败：')) + (e.message || e))
  }
}

// ===== 收录（B3 内容引用与归因）=====
const INGEST_STATUS_TEXT = { draft: i18t('草稿'), accepted: t('已收录'), reverted: t('已撤销'), discarded: t('已放弃') }

function ingestStatusText(s) {
  return INGEST_STATUS_TEXT[s] || s
}
function ingestStatusClass(s) {
  if (s === 'accepted') return 'approved'
  if (s === 'draft') return 'pending'
  return 'off'
}
function fmtIngestTime(ms) {
  if (!ms) return ''
  const d = new Date(Number(ms))
  if (isNaN(d.getTime())) return ''
  const p = (n) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`
}

async function loadIngests() {
  try {
    const d = await requestJSON('/ingests/pending')
    ingestPendings.value = d?.items || []
  } catch (_) {
    ingestPendings.value = []
  }
  try {
    const d2 = await requestJSON('/ingests')
    ingestRecords.value = d2?.items || []
  } catch (_) {
    ingestRecords.value = []
  }
}

// 引用式：单条已通过评论一键收录（原文照录 + 归因区块 append 进正文）
// 注：request() 抛的是后端中文 message（非错误码），故按文案判别分支。
async function ingestCmt(c) {
  if (busy.value) return
  busy.value = true
  try {
    await requestJSON('/ingests/quote', {
      method: 'POST',
      body: JSON.stringify({ file_id: c.file_id, comment_id: c.id })
    })
    toast.success(t('已带署名与回指链接补录进正文'))
    loadIngests()
  } catch (e) {
    const m = String(e?.message || e)
    if (m.includes('已被收录')) toast.error(i18t('该评论已被收录过'))
    else if (m.includes('冻结收录')) toast.error(i18t('该文章已冻结收录（生长策略=frozen）'))
    else if (m.includes('仅文本')) toast.error(i18t('仅文本文章可收录'))
    else toast.error(t('收录失败：') + m)
  } finally {
    busy.value = false
  }
}

// 融合式：多条评论 → AI 草稿（两段式，先草稿；站长确认才写正文）
async function fuseSelected() {
  if (busy.value || !cmtSel.value.length) return
  if (cmtSel.value.length > 8) {
    toast.error(t(i18t('AI 融合一次最多 8 条评论')))
    return
  }
  // 后端按单个 file_id 取素材 —— 必须同一篇文章
  const picked = cmts.value.filter((c) => cmtSel.value.includes(c.id))
  const fileIds = [...new Set(picked.map((c) => c.file_id))]
  if (fileIds.length !== 1) {
    toast.error(i18t('AI 融合只能选同一篇文章下的评论'))
    return
  }
  busy.value = true
  try {
    const d = await requestJSON('/ingests/fuse-draft', {
      method: 'POST',
      body: JSON.stringify({ file_id: fileIds[0], comment_ids: cmtSel.value })
    })
    cmtSel.value = []
    await loadIngests()
    switchTab('ingest')
    toast.success(`已生成融合草稿（${d?.comments ?? 0} 条评论），确认后再写入正文`)
  } catch (e) {
    const m = String(e?.message || e)
    if (m.includes('AI 模型未配置')) toast.error(i18t('AI 模型未配置，融合式不可用（可改用逐条「收录」）'))
    else if (m.includes('最多聚合')) toast.error(i18t('AI 融合一次最多 8 条评论'))
    else toast.error(t('生成草稿失败：') + m)
  } finally {
    busy.value = false
  }
}

async function acceptIngest(g) {
  if (busy.value) return
  busy.value = true
  try {
    await requestJSON(`/ingests/${g.id}/accept`, { method: 'POST' })
    toast.success(t('已采纳并写入正文'))
    loadIngests()
  } catch (e) {
    toast.error(t('采纳失败：') + (e.message || e))
  } finally {
    busy.value = false
  }
}

async function discardIngest(g) {
  if (busy.value) return
  busy.value = true
  try {
    await requestJSON(`/ingests/${g.id}`, { method: 'DELETE' })
    toast.success(t('已放弃草稿'))
    loadIngests()
  } catch (e) {
    toast.error(t('放弃失败：') + (e.message || e))
  } finally {
    busy.value = false
  }
}

async function revertIngest(g) {
  if (busy.value) return
  busy.value = true
  try {
    await requestJSON(`/ingests/${g.id}`, { method: 'DELETE' })
    toast.success(t('已撤销收录（正文按锚点精确摘除）'))
    loadIngests()
  } catch (e) {
    const m = String(e?.message || e)
    if (m.includes('锚点缺失')) toast.error(i18t('正文锚点已丢失，请人工处理'))
    else toast.error(t('撤销失败：') + m)
  } finally {
    busy.value = false
  }
}

// ===== Webhook 推送管理 =====
const hooks = ref([])
const kernelTopics = ref([])
const hookForm = ref({ url: '', secret: '', topic: 'file.status' })

async function loadHooks() {
  try {
    const d = await requestJSON('/blog/webhooks')
    hooks.value = d?.items || []
    if (Array.isArray(d?.kernel_topics) && d.kernel_topics.length) kernelTopics.value = d.kernel_topics
  } catch (_) {
    hooks.value = []
  }
}

async function addHook() {
  const url = (hookForm.value.url || '').trim()
  if (!url) {
    toast.error(t('请填写 webhook 地址'))
    return
  }
  try {
    await requestJSON('/blog/webhooks', {
      method: 'POST',
      body: JSON.stringify({ url, secret: hookForm.value.secret, topics: [hookForm.value.topic] }),
    })
    toast.success(t('Webhook 已添加'))
    hookForm.value = { url: '', secret: '', topic: hookForm.value.topic }
    loadHooks()
  } catch (e) {
    toast.error(t('添加失败：') + (e.message || e))
  }
}

async function delHook(h) {
  try {
    await requestJSON(`/blog/webhooks/${h.id}`, { method: 'DELETE' })
    toast.success(t('Webhook 已删除'))
    loadHooks()
  } catch (e) {
    toast.error(t(i18t('删除失败：')) + (e.message || e))
  }
}

async function testHook(h) {
  try {
    const d = await requestJSON(`/blog/webhooks/${h.id}/test`, { method: 'POST' })
    if (d?.ok) toast.success(t('测试事件已投递成功（topic: ') + d.topic + '）')
    else toast.error(t('投递失败：') + (d?.error || t('目标无响应')))
  } catch (e) {
    toast.error(t('测试失败：') + (e.message || e))
  }
}

async function saveSite() {
  const pairs = [
    ['blog.title', site.value.title],
    ['blog.description', site.value.description],
    ['blog.logo', site.value.logo],
    ['blog.footer', site.value.footer],
    ['blog.seo_default', site.value.seo_default],
    ['blog.theme', site.value.theme || 'aiklog'],
    ['blog.custom_css', site.value.custom_css],
    ['blog.custom_js', site.value.custom_js],
    ['blog.base_url', site.value.base_url],
    ['blog.locale', site.value.locale],
  ]
  for (const [k, v] of pairs) {
    try {
      await requestJSON('/admin/settings', { method: 'PUT', body: JSON.stringify({ key: k, value: v }) })
    } catch (e) {
      toast.error(`保存 ${k} 失败：${e.message}`)
      return
    }
  }
  // 立即生效：清掉本地预览覆盖，套用刚保存的站点主题
  applyServerTheme(site.value.theme || 'aiklog', { force: true })
  siteDirty.value = false
  toast.success(t('站点设置已保存'))
}

const loading = ref(true)
const err = ref('')
const busy = ref(false)
const status = ref(null)
const cats = ref([])
const posts = ref([])
const curCat = ref('')
const newCat = ref('')
const newTitle = ref('')
const newBody = ref('')
const newCatFor = ref('')
const delTarget = ref(null)
const toast = useToastStore()
const dragOver = ref(false)
const upQueue = ref([])

// 2.1 置顶底座：每行置顶草稿 { [postId]: { scope, order } }
const pinDraft = ref({})
// 2.2/2.3：每行密码草稿 / 定时草稿（datetime-local 字符串）
const pwdDraft = ref({})
const schDraft = ref({})

// A-G 批量编辑：多选 + 批量设置内容类型底座
const selMap = ref({})
const batchNodeType = ref('')
const selectedCount = computed(() => Object.values(selMap.value).filter(Boolean).length)
const allSelected = computed(() => shownPosts.value.length > 0 && shownPosts.value.every((p) => selMap.value[p.id]))
function onSel(p, e) {
  const v = e.target.checked
  const m = { ...selMap.value }
  if (v) m[p.id] = true
  else delete m[p.id]
  selMap.value = m
}
function toggleAll(e) {
  const v = e.target.checked
  const m = {}
  if (v) for (const p of shownPosts.value) m[p.id] = true
  selMap.value = m
}
function clearSel() {
  selMap.value = {}
}
// content_state → node_type（旧 visibility/status 双轨兼容）
function nodeTypeOf(p) {
  try {
    const cs = p.content_state ? JSON.parse(p.content_state) : {}
    return cs && cs.node_type ? cs.node_type : ''
  } catch (_) {
    return ''
  }
}
function typeLabel(t) {
  return { article: tr(i18t('文章')), resource: tr(i18t('资源')), link: tr(i18t('外链')) }[t] || t
}
async function applyBatch() {
  if (busy.value || !selectedCount.value || !batchNodeType.value) return
  busy.value = true
  try {
    const ids = shownPosts.value.filter((p) => selMap.value[p.id]).map((p) => p.id)
    const items = ids.map((id) => ({ id, node_type: batchNodeType.value }))
    const r = await blogPostsBatch(items)
    const ok = (r && r.ok_count) || 0
    toast.success(`批量设置完成：成功 ${ok} / ${items.length} 篇`)
    selMap.value = {}
    batchNodeType.value = ''
    await load()
  } catch (e) {
    toast.error(t('批量设置失败：') + (e.message || e))
  } finally {
    busy.value = false
  }
}

// 毫秒时间戳 → 本地 datetime-local 值（YYYY-MM-DDTHH:mm）
function localDT(ms) {
  if (!ms) return ''
  const d = new Date(Number(ms))
  const p = (n) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}T${p(d.getHours())}:${p(d.getMinutes())}`
}
// datetime-local 值 → 毫秒（空返回 null）
function dtToMs(v) {
  return v ? new Date(v).getTime() : null
}

// 上传目标：当前分类（未选/未分类 → 博客根）
const uploadTargetLabel = computed(() => {
  if (curCat.value && curCat.value !== '__none') {
    const c = cats.value.find((x) => x.id === curCat.value)
    return c ? i18t('分类：') + c.name : t('分类')
  }
  return t('博客根（未分类）')
})

function uploadParent() {
  return curCat.value && curCat.value !== '__none' ? curCat.value : BLOG_DIR
}

const catOptions = computed(() => {
  const opts = [{ v: '', label: t('全部') }, { v: '__none', label: t('未分类') }]
  for (const c of cats.value) {
    opts.push({ v: c.id, label: c.name, n: posts.value.filter((p) => catOf(p) === c.id).length })
  }
  return opts
})

const shownPosts = computed(() => {
  if (curCat.value === '') return posts.value
  if (curCat.value === '__none') return posts.value.filter((p) => !catOf(p))
  return posts.value.filter((p) => catOf(p) === curCat.value)
})

function catOf(p) {
  const c = cats.value.find((x) => p.path && p.path.startsWith(x.name + '/'))
  return c ? c.id : ''
}

function catCount(c) {
  return posts.value.filter((p) => catOf(p) === c.id).length
}
function fmtDate(ts) {
  if (!ts) return '—'
  const d = new Date(Number(ts))
  if (isNaN(d.getTime())) return '—'
  const p2 = (n) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p2(d.getMonth() + 1)}-${p2(d.getDate())} ${p2(d.getHours())}:${p2(d.getMinutes())}`
}
const savingOrder = ref(false)
const dragId = ref('')
const dragOverId = ref('')
function onDragStart(c, ev) {
  dragId.value = c.id
  if (ev && ev.dataTransfer) {
    ev.dataTransfer.effectAllowed = 'move'
    ev.dataTransfer.setData('text/plain', c.id)
  }
}
function onDragOver(c) {
  if (dragId.value && dragId.value !== c.id) dragOverId.value = c.id
}
function onDragLeave(c) {
  if (dragOverId.value === c.id) dragOverId.value = ''
}
function onDrop(c) {
  const from = cats.value.findIndex((x) => x.id === dragId.value)
  const to = cats.value.findIndex((x) => x.id === c.id)
  if (from < 0 || to < 0 || from === to) { dragOverId.value = ''; return }
  const arr = cats.value.slice()
  const [moved] = arr.splice(from, 1)
  arr.splice(to, 0, moved)
  cats.value = arr
  dragOverId.value = ''
}
function onDragEnd() {
  dragId.value = ''
  dragOverId.value = ''
}
async function saveCatOrder() {
  if (busy.value || savingOrder.value) return
  savingOrder.value = true
  try {
    const orders = cats.value.map((c, i) => ({ id: c.id, sort_order: i * 10 }))
    await requestJSON('/blog/categories/sort', { method: 'POST', body: JSON.stringify({ orders }) })
    toast.success(t('分类顺序已保存'))
    await load()
  } catch (e) {
    toast.error(t('保存失败：') + (e.message || e))
  } finally {
    savingOrder.value = false
  }
}
async function resetCatOrder() {
  if (busy.value || savingOrder.value) return
  savingOrder.value = true
  try {
    const orders = cats.value.map((c) => ({ id: c.id, sort_order: null }))
    await requestJSON('/blog/categories/sort', { method: 'POST', body: JSON.stringify({ orders }) })
    toast.success(t('已恢复为动态排序（按最新活动）'))
    await load()
  } catch (e) {
    toast.error(t('操作失败：') + (e.message || e))
  } finally {
    savingOrder.value = false
  }
}

async function load() {
  loading.value = true
  err.value = ''
  try {
    const d = await requestJSON('/blog/manage')
    cats.value = d.categories || []
    posts.value = d.posts || []
    selMap.value = {}
    status.value = d.blog_status || null
    autoPublish.value = !!d.auto_publish_on_upload
    guestComments.value = !!d.comments_guest
    pendingCmts.value = d.pending_comments || 0
    // 2.1 置顶底座 + 2.2/2.3 密码/定时：初始化每行草稿
    pinDraft.value = {}
    pwdDraft.value = {}
    schDraft.value = {}
    for (const p of posts.value) {
      pinDraft.value[p.id] = {
        scope: p.pin_scope && p.pin_scope !== 'none' ? p.pin_scope : 'none',
        order: typeof p.pin_order === 'number' ? p.pin_order : null,
      }
      // 2.2/2.3：密码草稿（''=不动；已加密留空 placeholder）与定时草稿（datetime-local 回填）
      pwdDraft.value[p.id] = ''
      schDraft.value[p.id] = p.publish_at ? localDT(p.publish_at) : ''
    }
  } catch (e) {
    err.value = t('加载失败：') + (e.message || e)
  } finally {
    loading.value = false
  }
}

async function toggleBlog(open) {
  if (busy.value) return
  busy.value = true
  try {
    await requestJSON('/admin/settings', {
      method: 'PUT',
      body: JSON.stringify({ key: 'blog.open', value: open ? 'true' : 'false' }),
    })
    await load()
    toast.success(open ? t('博客已开启') : t('博客已关闭（公开页/RSS 下线，数据保留）'))
  } catch (e) {
    toast.error(t('切换失败：') + (e.message || e))
  } finally {
    busy.value = false
  }
}

// 2.1 保存置顶设置（不重排列表；下次刷新按 pin 生效）
async function applyPin(p) {
  if (busy.value) return
  busy.value = true
  try {
    const r = await requestJSON('/blog/posts/pin', {
      method: 'POST',
      body: JSON.stringify({ id: p.id, scope: pinDraft.value[p.id].scope, order: pinDraft.value[p.id].order }),
    })
    if (!r || !r.ok) throw new Error((r && r.error) || t('置顶设置失败'))
    p.pin_scope = pinDraft.value[p.id].scope
    p.pin_order = pinDraft.value[p.id].order
    toast.success(t('已保存置顶设置'))
  } catch (e) {
    toast.error(t('置顶设置失败：') + (e.message || e))
  } finally {
    busy.value = false
  }
}

// 2.2 保存文章访问密码（留空=清除）
async function applyPwd(p) {
  if (busy.value) return
  busy.value = true
  try {
    const v = (pwdDraft.value[p.id] || '').trim()
    if (!v && !p.has_pwd) {
      toast.error(t('未输入新密码（留空保存仅用于清除已有密码）'))
      return
    }
    const r = await requestJSON('/blog/posts/access', { method: 'POST', body: JSON.stringify({ id: p.id, password: v }) })
    if (!r || !r.ok) throw new Error((r && r.error) || t('密码设置失败'))
    p.has_pwd = v !== ''
    pwdDraft.value[p.id] = ''
    toast.success(v ? t('已设置访问密码') : t('已清除访问密码'))
  } catch (e) {
    toast.error(t('密码设置失败：') + (e.message || e))
  } finally {
    busy.value = false
  }
}

// 2.3 保存定时发布（留空=立即发布/清除定时）
async function applySch(p) {
  if (busy.value) return
  busy.value = true
  try {
    const ms = dtToMs(schDraft.value[p.id])
    if (ms && ms <= Date.now()) {
      toast.error(t('定时时间必须晚于当前时间（立即发布请留空）'))
      return
    }
    const r = await requestJSON('/blog/posts/schedule', { method: 'POST', body: JSON.stringify({ id: p.id, publish_at: ms }) })
    if (!r || !r.ok) throw new Error((r && r.error) || t('定时设置失败'))
    p.publish_at = ms
    toast.success(ms ? t('已设置定时发布') : t('已设为立即发布'))
  } catch (e) {
    toast.error(t('定时设置失败：') + (e.message || e))
  } finally {
    busy.value = false
  }
}

async function addCat() {  const name = newCat.value.trim()
  if (!name || busy.value) return
  busy.value = true
  try {
    await mkdir(name, BLOG_DIR)
    newCat.value = ''
    await load()
  } catch (e) {
    toast.error(t('新建分类失败：') + (e.message || e))
  } finally {
    busy.value = false
  }
}

async function addPost() {
  const name = newTitle.value.trim()
  if (!name || busy.value) return
  busy.value = true
  try {
    // 新建默认草稿；博客管理面板"发布"= 创建后置为 published（博客公开页只展示已发布）
    const f = await createDoc(name.endsWith('.md') ? name : name + '.md', newCatFor.value || BLOG_DIR, newBody.value)
    await setFileStatus(f.id, 'published')
    newTitle.value = ''
    newBody.value = ''
    await load()
  } catch (e) {
    toast.error(t(i18t('发布失败：')) + (e.message || e))
  } finally {
    busy.value = false
  }
}

// ---- 上传：拖拽（含文件夹递归）/ 选文件 / 选文件夹 ----
async function onPick(ev) {
  const files = Array.from(ev.target.files || [])
  ev.target.value = ''
  if (!files.length) return
  await doUpload(files.map((f) => ({ path: f.name, file: f })))
}

function onPickDir(ev) {
  const files = Array.from(ev.target.files || [])
  ev.target.value = ''
  if (!files.length) return
  // webkitdirectory：webkitRelativePath 自带目录结构（如 设计规范/图标/check.svg）
  const list = files.map((f) => ({
    path: f.webkitRelativePath || f.name,
    file: f,
  }))
  doUpload(list)
}

async function onDropZone(e) {
  dragOver.value = false
  const list = await collectDropped(e.dataTransfer)
  if (list.length) doUpload(list)
}

// 递归收集拖入的文件/文件夹（保留目录结构，webkitGetAsEntry；不支持时降级 files）
function collectDropped(dt) {
  return new Promise((resolve) => {
    const out = []
    const items = dt && dt.items ? Array.from(dt.items).filter((i) => i.kind === 'file') : []
    if (!items.length) {
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

async function doUpload(list) {
  busy.value = true
  upQueue.value = list
  try {
    await uploadFiles(list, uploadParent())
    await load()
    // 站长设置：上传后自动发布（仅 md；关闭时保持草稿，由站长确认后发布）
    if (autoPublish.value && posts.value?.length) {
      const names = new Set(list.map((x) => (x.path || x.file?.name || '').split('/').pop().toLowerCase()))
      let n = 0
      for (const p of posts.value) {
        if (!p.id || !p.name) continue
        if (!/\.(md|markdown)$/i.test(p.name)) continue
        if (!names.has(p.name.toLowerCase())) continue
        try {
          await setFileStatus(p.id, 'published')
          n++
        } catch (_) { /* 单篇失败不阻断 */ }
      }
      if (n > 0) {
        await load()
        toast.success(`已自动发布 ${n} 篇 Markdown`)
      }
    }
  } catch (e) {
    toast.error(t('上传失败：') + (e.message || e))
  } finally {
    busy.value = false
    upQueue.value = []
  }
}

// ---- 删除：自定义确认弹窗 ----
function askDel(p) {
  delTarget.value = p
}

async function doDel() {
  const p = delTarget.value
  if (!p || busy.value) return
  delTarget.value = null
  busy.value = true
  try {
    await deleteFile(p.id)
    await load()
  } catch (e) {
    toast.error(t(i18t('删除失败：')) + (e.message || e))
  } finally {
    busy.value = false
  }
}

onMounted(() => {
  load()
  loadSite()
  loadCmts()
  loadHooks()
  loadIngests()
  // 深链直达统计标签（?tab=stats）时 watch 不触发，这里补一次加载
  if (statsOpen.value && !stats.value) loadStats()
})
</script>

<style scoped>
.bm-view {
  width: 100%;
  max-width: 1200px;
  margin: 0 auto;
  padding: 20px 24px 40px;
  font-size: 14px;
}
/* 顶部功能标签栏 */
.bm-nav-tabs {
  display: flex;
  gap: 6px;
  margin: 10px 0 16px;
  border-bottom: 1px solid var(--border);
  padding-bottom: 10px;
  flex-wrap: wrap;
}
.bm-tab-btn {
  border: 1px solid transparent;
  background: none;
  cursor: pointer;
  font-size: 13px;
  color: var(--muted, #666);
  padding: 6px 14px;
  border-radius: 999px;
  transition: all 0.15s;
}
.bm-tab-btn:hover {
  color: var(--text, #222);
  background: var(--hover, rgba(127, 127, 127, 0.08));
}
.bm-tab-btn.on {
  color: var(--primary, #4f7cff);
  background: var(--primary-soft, rgba(79, 124, 255, 0.1));
  border-color: var(--primary-soft, rgba(79, 124, 255, 0.2));
  font-weight: 600;
}
.bm-tab-count {
  margin-left: 5px;
  font-size: 11px;
  opacity: 0.75;
}

.bm-tip {
  display: flex;
  gap: 8px;
  align-items: flex-start;
  padding: 10px 14px;
  border-radius: var(--radius, 8px);
  background: rgba(76, 125, 240, 0.08);
  color: var(--text-3, #6b7280);
  line-height: 1.6;
  margin-bottom: 12px;
}

.bm-status {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 16px;
  color: var(--text-3, #6b7280);
}

.bm-status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #34c759;
}

.bm-status-dot.off {
  background: #ff9500;
}

.bm-open {
  margin-left: auto;
  color: var(--accent, #4c7df0);
  text-decoration: none;
}

.bm-toggle {
  margin-left: 8px;
  color: #e5484d;
  border-color: color-mix(in srgb, #e5484d 45%, transparent);
}

.bm-toggle.on {
  color: #2f9e44;
  border-color: color-mix(in srgb, #2f9e44 45%, transparent);
}

/* 站点设置 */
.bm-site {
  margin-bottom: 14px;
}

.bm-site-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(260px, 1fr));
  gap: 10px 14px;
  margin-top: 10px;
}

.bm-field {
  display: flex;
  flex-direction: column;
  gap: 5px;
  font-size: 12px;
  color: var(--text-3, #6b7280);
}

.bm-field input {
  font-size: 13px;
  color: var(--text, #1f2329);
}

.bm-textarea {
  resize: vertical;
  min-height: 56px;
  font-family: ui-monospace, 'SFMono-Regular', Consolas, monospace;
  font-size: 12px;
  line-height: 1.5;
}
.bm-field-full {
  grid-column: 1 / -1;
}

.bm-field-check {
  flex-direction: row;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px 10px;
  grid-column: 1 / -1;
  padding: 10px 12px;
  border: 1px solid var(--border, #e5e7eb);
  border-radius: 10px;
  background: var(--bg-2, #f8fafb);
}

.bm-field-check > span:first-child {
  font-weight: 600;
  color: var(--text, #1f2329);
  font-size: 13px;
}

.bm-field-check input[type='checkbox'] {
  width: 16px;
  height: 16px;
  accent-color: var(--accent, #0d7a6a);
}

.bm-check-hint {
  flex: 1;
  min-width: 200px;
  font-size: 12px;
  color: var(--text-3, #6b7280);
}

.bm-cols {
  display: grid;
  grid-template-columns: 240px 1fr;
  gap: 16px;
  align-items: start;
}

@media (max-width: 720px) {
  .bm-cols {
    grid-template-columns: 1fr;
  }
}

.bm-panel {
  background: var(--surface, #fff);
  border: 1px solid var(--border, #e3e6eb);
  border-radius: var(--radius, 10px);
  padding: 14px;
}

.bm-panel-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 10px;
}

.bm-panel-head h3 {
  margin: 0;
  font-size: 15px;
}

.bm-count {
  font-size: 12px;
  color: var(--muted, #8a919f);
}

.bm-cat-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
  margin-bottom: 10px;
}

.bm-cat {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 7px 10px;
  border-radius: 6px;
  border: none;
  background: transparent;
  color: var(--text, #1f2329);
  cursor: pointer;
  text-align: left;
}

.bm-cat:hover {
  background: var(--bg-hover, #f2f4f7);
}

.bm-cat.active {
  background: rgba(76, 125, 240, 0.12);
  color: var(--accent, #4c7df0);
  font-weight: 600;
}

.bm-cat-n {
  font-size: 11px;
  color: var(--muted, #8a919f);
}

.bm-cat-new {
  display: flex;
  gap: 6px;
}

.bm-input {
  flex: 1;
  min-width: 0;
  padding: 7px 10px;
  border: 1px solid var(--border, #e3e6eb);
  border-radius: 6px;
  font-size: 13px;
  background: var(--surface, #fff);
  color: var(--text, #1f2329);
}

.bm-select {
  flex: 0 0 140px;
}

.bm-new {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 12px;
  background: var(--bg, #f7f8fa);
  border-radius: 8px;
  margin-bottom: 14px;
}

.bm-new-row {
  display: flex;
  gap: 8px;
}

.bm-textarea {
  padding: 8px 10px;
  border: 1px solid var(--border, #e3e6eb);
  border-radius: 6px;
  font-size: 13px;
  font-family: inherit;
  resize: vertical;
  background: var(--surface, #fff);
  color: var(--text, #1f2329);
}

.bm-new-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}

.bm-up {
  position: relative;
  overflow: hidden;
  font-weight: 500;
}

.bm-up input {
  position: absolute;
  inset: 0;
  opacity: 0;
  cursor: pointer;
}

/* 拖拽上传区 */
.bm-upzone {
  border: 1.5px dashed var(--border, #d4d8de);
  border-radius: 8px;
  padding: 14px 16px;
  margin-bottom: 14px;
  background: var(--bg, #fafbfc);
  transition: all 0.15s ease;
}

.bm-upzone.over {
  border-color: var(--accent, #4c7df0);
  background: rgba(76, 125, 240, 0.06);
}

.bm-upzone-inner {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  color: var(--muted, #8a919f);
}

.bm-upzone-text {
  font-weight: 500;
}

.bm-upzone-sep {
  opacity: 0.6;
}

.bm-upzone-meta {
  margin-top: 6px;
  font-size: 12px;
  color: var(--muted, #8a919f);
  display: flex;
  align-items: center;
  gap: 10px;
}

.bm-upzone-hint {
  color: var(--accent, #4c7df0);
  font-weight: 600;
}

.bm-upzone-q {
  color: var(--accent, #4c7df0);
}

.bm-primary {
  background: var(--accent, #4c7df0);
  border-color: var(--accent, #4c7df0);
  color: #fff;
}

.bm-post-list {
  display: flex;
  flex-direction: column;
}

.bm-post {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 4px;
  border-bottom: 1px solid var(--border, #eef0f3);
}

/* A-G 批量编辑：多选工具栏 / 行内勾选 / 内容类型角标 */
.bm-batch-bar {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  padding: 8px 10px;
  margin-bottom: 10px;
  background: var(--bg-2, #f7f9fb);
  border: 1px solid var(--border, #e6e9ee);
  border-radius: 8px;
  font-size: 13px;
  color: var(--text-3, #6b7280);
}

.bm-cbx {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  cursor: pointer;
  user-select: none;
}

.bm-batch-n {
  font-variant-numeric: tabular-nums;
}

.bm-select {
  width: auto;
  min-width: 150px;
}

.bm-row-cbx {
  flex: none;
  width: 16px;
  height: 16px;
  accent-color: var(--accent, #4c7df0);
  cursor: pointer;
}

.bm-type-tag {
  flex: none;
  font-size: 11px;
  line-height: 1;
  padding: 3px 7px;
  border-radius: 999px;
  background: rgba(76, 125, 240, 0.12);
  color: var(--accent, #4c7df0);
  font-weight: 600;
}

.bm-post-name {
  flex-shrink: 0;
  max-width: 260px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--text, #1f2329);
  text-decoration: none;
  font-weight: 500;
}

.bm-post-name:hover {
  color: var(--accent, #4c7df0);
}

.bm-post-meta {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 12px;
  color: var(--muted, #8a919f);
}

/* 2.1 置顶底座：行内置顶控件 */
.bm-pin {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  flex-shrink: 0;
}
.bm-pin select,
.bm-pin input[type='number'] {
  height: 26px;
  font-size: 12px;
  border: 1px solid var(--border, #eef0f3);
  border-radius: 6px;
  background: var(--card, #fff);
  color: inherit;
  padding: 0 4px;
}
.bm-pin input[type='number'] {
  width: 46px;
}
.bm-pin input[type='password'],
.bm-pin input[type='text'] {
  width: 70px;
}
.bm-pin input[type='datetime-local'] {
  width: 132px;
}
.bm-pin-apply {
  height: 26px;
  border: 1px solid var(--border, #eef0f3);
  background: none;
  color: var(--accent, #4c6ef5);
  font-size: 12px;
  border-radius: 6px;
  cursor: pointer;
}

.bm-del {
  flex-shrink: 0;
  border: none;
  background: none;
  color: #e5484d;
  font-size: 12px;
  cursor: pointer;
  opacity: 0.7;
}

.bm-del:hover {
  opacity: 1;
  text-decoration: underline;
}

.bm-hint {
  padding: 20px 0;
  text-align: center;
  color: var(--muted, #8a919f);
}

.bm-hint.err {
  color: #e5484d;
}

.bm-cat-filter {
  display: flex;
  gap: 6px;
  margin-bottom: 8px;
}
.bm-cat-mini {
  flex: 1;
  padding: 5px 8px;
  border-radius: 6px;
  border: 1px solid var(--border, #e3e6eb);
  background: transparent;
  color: var(--text-3, #6b7280);
  cursor: pointer;
  font-size: 12px;
}
.bm-cat-mini.active {
  background: rgba(76, 125, 240, 0.12);
  color: var(--accent, #4c7df0);
  font-weight: 600;
}
.bm-cat-row {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 5px 6px;
  border-radius: 6px;
  border: 1px solid transparent;
  cursor: grab;
}
.bm-cat-row:hover {
  background: var(--bg-hover, #f2f4f7);
}
.bm-cat-row.dragging {
  opacity: 0.5;
  cursor: grabbing;
}
.bm-cat-row.dragover {
  border-color: var(--accent, #4c7df0);
  background: rgba(76, 125, 240, 0.08);
}
.bm-cat-handle {
  color: var(--muted, #8a919f);
  cursor: grab;
  font-size: 14px;
  user-select: none;
}
.bm-cat-row .bm-cat {
  flex: 0 0 auto;
  padding: 2px 6px;
}
.bm-cat-so {
  font-size: 11px;
  color: var(--accent, #4c7df0);
  font-weight: 600;
}
.bm-cat-la {
  margin-left: auto;
  font-size: 11px;
  color: var(--muted, #8a919f);
}
.bm-cat-actions {
  margin: 8px 0;
}
.bm-reset {
  color: var(--muted, #8a919f);
  border-color: var(--border, #e3e6eb);
}

/* ===== 评论管理 / Webhook ===== */
.bm-cmts,
.bm-hooks {
  margin-top: 16px;
}
.bm-cmt-filter {
  display: inline-flex;
  gap: 6px;
}
.bm-cmt-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.bm-cmt-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 10px;
  border: 1px solid var(--border, #e3e6eb);
  border-radius: 8px;
  font-size: 13px;
}
.bm-cmt-status {
  flex: none;
  font-size: 12px;
  padding: 2px 8px;
  border-radius: 999px;
  background: var(--warn-bg, #fff4e0);
  color: var(--warn, #b7791f);
}
.bm-cmt-status.approved {
  background: var(--ok-bg, #e6f6ec);
  color: var(--ok, #2f9e63);
}
.bm-cmt-status.off {
  background: var(--bg-3, #f0f1f3);
  color: var(--text-3, #8a919f);
}
.bm-cmt-author {
  flex: none;
  font-weight: 600;
  color: var(--text-2, #4b5563);
}
.bm-cmt-body {
  flex: 1 1 auto;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--text-2, #4b5563);
}
.bm-cmt-file {
  flex: none;
  max-width: 220px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 12px;
  color: var(--text-3, #8a919f);
}
.bm-cmt-ck {
  flex: none;
  width: 14px;
  height: 14px;
  margin: 0;
  cursor: pointer;
}
.bm-cmt-ck-ph {
  flex: none;
  width: 14px;
}
.bm-cmt-bar {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  padding: 8px 10px;
  margin-bottom: 8px;
  border: 1px dashed var(--border, #e3e6eb);
  border-radius: 8px;
  background: var(--bg-3, #f7f8fa);
}
/* ===== 收录（B3 内容引用与归因）===== */
.bm-ingests {
  margin-top: 16px;
}
.bm-ingest-block {
  margin-top: 14px;
}
.bm-ingest-sub {
  font-size: 13px;
  font-weight: 600;
  color: var(--text-2, #4b5563);
  margin-bottom: 8px;
}
.bm-ingest-card {
  border: 1px solid var(--border, #e3e6eb);
  border-radius: 8px;
  padding: 10px;
}
.bm-ingest-card-head {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 8px;
}
.bm-ingest-tag {
  flex: none;
  font-size: 12px;
  padding: 2px 8px;
  border-radius: 999px;
  background: var(--bg-3, #f0f1f3);
  color: var(--text-3, #8a919f);
}
.bm-ingest-time {
  flex: none;
  font-size: 12px;
  font-variant-numeric: tabular-nums;
  color: var(--text-3, #8a919f);
}
.bm-ingest-draft {
  margin: 0;
  padding: 8px 10px;
  border-radius: 6px;
  background: var(--bg-3, #f7f8fa);
  font-size: 12.5px;
  line-height: 1.7;
  white-space: pre-wrap;
  word-break: break-word;
  color: var(--text-2, #4b5563);
  max-height: 240px;
  overflow: auto;
}
.bm-hook-hint {
  font-size: 12px;
  color: var(--text-3, #8a919f);
}
.bm-hook-new {
  display: flex;
  gap: 8px;
  margin-bottom: 10px;
}
.bm-hook-secret {
  max-width: 200px;
}
.bm-views {
  flex: none;
  font-variant-numeric: tabular-nums;
}

/* ===== 统计看板 ===== */
.bm-spacer {
  flex: 1;
}
.bm-stat-cards {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(110px, 1fr));
  gap: 10px;
  margin-bottom: 12px;
}
.bm-stat-card {
  display: flex;
  flex-direction: column;
  gap: 2px;
  border: 1px solid var(--border, #e3e6eb);
  border-radius: 10px;
  padding: 10px 14px;
  background: var(--bg-1, #fff);
}
.bm-stat-card .n {
  font-size: 20px;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
}
.bm-stat-card .l {
  font-size: 12px;
  color: var(--text-3, #8a919f);
}
.bm-stat-charts {
  height: 220px;
  margin-bottom: 12px;
  border: 1px solid var(--border, #e3e6eb);
  border-radius: 10px;
  padding: 10px;
  background: var(--bg-1, #fff);
}
.bm-stat-cols {
  display: grid;
  grid-template-columns: 1.2fr 1fr;
  gap: 14px;
}
.bm-stat-sub {
  margin: 0 0 6px;
  font-size: 13px;
  font-weight: 600;
  color: var(--text-2, #4a5568);
}
.bm-top-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 0;
  font-size: 13px;
  border-bottom: 1px dashed var(--border, #e9ecf1);
}
.bm-top-name {
  flex: 1;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.bm-top-n {
  flex: none;
  color: var(--text-3, #8a919f);
  font-variant-numeric: tabular-nums;
}
.bm-tag-cloud {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.bm-tag-chip2 {
  border: 1px solid var(--border, #e3e6eb);
  border-radius: 999px;
  padding: 2px 10px;
  font-size: 12px;
  background: var(--bg-1, #fff);
}
.bm-tag-chip2 i {
  font-style: normal;
  margin-left: 4px;
  color: var(--text-3, #8a919f);
}
@media (max-width: 900px) {
  .bm-stat-cols {
    grid-template-columns: 1fr;
  }
}
</style>

