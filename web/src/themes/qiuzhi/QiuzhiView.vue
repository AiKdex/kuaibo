<template>
  <div class="qz-root">
    <!-- ===== 顶栏 ===== -->
    <header class="qz-topbar">
      <div class="qz-container">
        <div class="qz-topbar-inner">
          <div class="qz-brand">
            <div class="qz-brand-mark"></div>
            <span class="qz-brand-name">{{ $t('求智') }}</span>
            <span class="qz-brand-badge" @click="toggleCity">
              {{ city }}
              <div class="qz-city-dropdown" :class="{ show: cityOpen }" @click.stop>
                <div v-for="c in cities" :key="c" class="qz-city-option" :class="{ active: c === city }" @click="switchCity(c)">{{ c }}</div>
              </div>
            </span>
          </div>
          <div class="qz-search-box">
            <svg class="qz-search-icon" viewBox="0 0 16 16" fill="none"><circle cx="7" cy="7" r="5" stroke="currentColor" stroke-width="1.5"/><path d="M11 11l3 3" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/></svg>
            <input class="qz-search-input" :placeholder="$t('搜索岗位 / 公司 / 区域')" v-model.trim="searchQ" @input="onSearch">
          </div>
          <nav class="qz-top-nav">
            <span class="qz-top-nav-item" :class="{ active: nav === 'jobs' }" @click="switchNav('jobs')">{{ $t('📋 岗位情报') }}</span>
            <span class="qz-top-nav-item" :class="{ active: nav === 'training' }" @click="switchNav('training')">{{ $t('🎓 培训补贴') }}</span>
            <span class="qz-top-nav-item" @click="openTrend">{{ $t('📊 趋势') }}</span>
            <span class="qz-top-nav-item" @click="graphTip">{{ $t('🔗 图谱') }}</span>
          </nav>
          <span class="qz-source-badge">{{ $t('聚合') }} {{ sourceCount }} {{ $t('个官办源 ·') }} {{ jobCount }} {{ $t('条') }}</span>
        </div>
      </div>
    </header>

    <!-- ===== 主布局 ===== -->
    <div class="qz-container">
      <div class="qz-app-layout">

        <!-- 左侧筛选 -->
        <aside class="qz-sidebar">
          <div class="qz-sidebar-group">
            <div class="qz-sidebar-title">{{ $t('地区') }}</div>
            <div class="qz-filter-list">
              <div class="qz-filter-item" :class="{ active: filter.area === 'all' }" @click="setFilter('area', 'all')">{{ $t('全部') }} <span class="qz-filter-count">{{ countFor('area', 'all') }}</span></div>
              <div v-for="v in areaOptions" :key="v" class="qz-filter-item" :class="{ active: filter.area === v }" @click="setFilter('area', v)">{{ v }} <span class="qz-filter-count">{{ countFor('area', v) }}</span></div>
            </div>
          </div>
          <div class="qz-sidebar-group">
            <div class="qz-sidebar-title">{{ $t('用工类型') }}</div>
            <div class="qz-filter-list">
              <div class="qz-filter-item" :class="{ active: filter.type === 'all' }" @click="setFilter('type', 'all')">{{ $t('全部') }} <span class="qz-filter-count">{{ countFor('type', 'all') }}</span></div>
              <div v-for="v in typeOptions" :key="v" class="qz-filter-item" :class="{ active: filter.type === v }" @click="setFilter('type', v)">{{ v }} <span class="qz-filter-count">{{ countFor('type', v) }}</span></div>
            </div>
          </div>
          <div class="qz-sidebar-group">
            <div class="qz-sidebar-title">{{ $t('薪资') }}</div>
            <div class="qz-filter-list">
              <div class="qz-filter-item" :class="{ active: filter.salary === 'all' }" @click="setFilter('salary', 'all')">{{ $t('不限') }}</div>
              <div class="qz-filter-item" :class="{ active: filter.salary === 'low' }" @click="setFilter('salary', 'low')">{{ $t('4k 以下') }}</div>
              <div class="qz-filter-item" :class="{ active: filter.salary === 'mid' }" @click="setFilter('salary', 'mid')">{{ $t('4k - 8k') }}</div>
              <div class="qz-filter-item" :class="{ active: filter.salary === 'high' }" @click="setFilter('salary', 'high')">{{ $t('8k 以上') }}</div>
            </div>
          </div>
          <div class="qz-sidebar-group qz-sidebar-tip" @click="switchNav('training')">
            <div class="qz-sidebar-title">{{ $t('🎓 补贴培训') }} <span class="qz-tip-badge">{{ $t('官办') }}</span></div>
            <div class="qz-tip-text">{{ $t('岗位要什么证 → 哪家机构教、有没有补贴，一眼看清') }}</div>
          </div>
        </aside>

        <!-- 主内容 -->
        <main class="qz-main-content">

          <!-- 岗位情报页 -->
          <template v-if="nav === 'jobs'">
            <!-- 条件匹配栏 -->
            <div class="qz-match-bar">
              <div class="qz-match-row">
                <span class="qz-match-label">{{ $t('我的条件') }}</span>
                <select class="qz-cond-select" v-model="cond.edu">
                  <option value="">{{ $t('学历') }}</option>
                  <option value="本科">{{ $t('本科') }}</option>
                  <option value="硕士">{{ $t('硕士及以上') }}</option>
                  <option value="大专">{{ $t('大专') }}</option>
                  <option value="高中">{{ $t('高中/中专') }}</option>
                </select>
                <select class="qz-cond-select" v-model="cond.major">
                  <option value="">{{ $t('专业') }}</option>
                  <option value="机械">{{ $t('机械/材料/电气') }}</option>
                  <option value="计算机">{{ $t('计算机/软件') }}</option>
                  <option value="财会">{{ $t('财会/管理') }}</option>
                  <option value="教育">{{ $t('教育/文史') }}</option>
                </select>
                <select class="qz-cond-select" v-model="cond.exp">
                  <option value="">{{ $t('年限') }}</option>
                  <option :value="$t('3内')">{{ $t('3年以内') }}</option>
                  <option value="3-8">3-8年</option>
                  <option :value="$t('8上')">{{ $t('8年以上') }}</option>
                </select>
                <select class="qz-cond-select" v-model="cond.pref">
                  <option value="">{{ $t('诉求') }}</option>
                  <option value="安稳">{{ $t('安稳优先') }}</option>
                  <option :value="$t('薪资')">{{ $t('薪资优先') }}</option>
                  <option value="对口">{{ $t('专业对口') }}</option>
                </select>
                <button class="qz-match-btn" @click="runMatch">{{ $t('匹配') }}</button>
              </div>
              <div v-if="matched" class="qz-match-result">
                <span class="pass">✅ {{ passCount }} {{ $t('条走得通') }}</span>
                <span class="sep">·</span>
                <span class="hard">⚠️ {{ hardCount }} {{ $t('条吃力') }}</span>
                <span class="sep">·</span>
                <span class="nomatch">{{ missCount }} {{ $t('条已错过') }}</span>
                <span class="hint">{{ $t('点击岗位卡片查看匹配详情和补齐建议') }}</span>
              </div>
              <div v-else class="qz-match-result">
                <span class="hint" style="color:var(--th-ink-3);">{{ $t('填入条件点「匹配」，立刻看哪些路走得通、差在哪——不画饼') }}</span>
              </div>
            </div>

            <!-- 状态标签栏 + 排序 -->
            <div class="qz-tab-bar">
              <span class="qz-tab" :class="{ active: filter.status === 'all' }" @click="setFilter('status', 'all')">{{ $t('全部') }} <span class="qz-count">{{ statusCount('all') }}</span></span>
              <span class="qz-tab" :class="{ active: filter.status === 'live' }" @click="setFilter('status', 'live')">{{ $t('进行中') }} <span class="qz-count">{{ statusCount('live') }}</span></span>
              <span class="qz-tab" :class="{ active: filter.status === 'soon' }" @click="setFilter('status', 'soon')">{{ $t('即将开启') }} <span class="qz-count">{{ statusCount('soon') }}</span></span>
              <span class="qz-tab" :class="{ active: filter.status === 'ended' }" @click="setFilter('status', 'ended')">{{ $t('已结束') }} <span class="qz-count">{{ statusCount('ended') }}</span></span>
              <span class="qz-tab-spacer"></span>
              <div class="qz-sort-wrap">
                <button class="qz-sort-btn" :class="{ active: sortOpen }" @click="toggleSort" type="button">
                  {{ sortLabel }} <svg width="10" height="10" viewBox="0 0 12 12" fill="none"><path d="M3 5l3 3 3-3" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/></svg>
                </button>
                <div class="qz-sort-dropdown" :class="{ show: sortOpen }">
                  <div v-for="(label, key) in sortOptions" :key="key" class="qz-sort-option" :class="{ active: sortBy === key }" @click="setSort(key)">{{ label }}</div>
                </div>
              </div>
              <button v-if="matched" class="qz-sort-btn" style="margin-left:6px;" @click="resetMatch" type="button">{{ $t('清除匹配') }}</button>
            </div>

            <!-- 岗位列表 -->
            <div class="qz-job-feed">
              <div v-if="dataLoading" class="qz-empty-state">
                <div class="qz-empty-icon">⏳</div>
                <p>{{ $t('正在从文件库加载采集数据…') }}</p>
              </div>
              <div v-else-if="!visibleJobs.length" class="qz-empty-state">
                <div class="qz-empty-icon">🔍</div>
                <p>{{ $t('没有符合条件的岗位') }}</p>
                <p class="qz-empty-clear"><a href="#" @click.prevent="clearAllFilters" style="color:var(--th-c-main-l);">{{ $t('清除筛选') }}</a></p>
              </div>

              <div v-for="j in visibleJobs" :key="j.id" class="qz-job-card" :class="[cardClass(j), { matched: matched && j.match === 'high', gap: matched && (j.match === 'gap' || j.match === 'mid') }]" role="button" tabindex="0" @click="openDetail(j)" @keydown.enter.prevent="openDetail(j)" @keydown.space.prevent="openDetail(j)">
                <div class="qz-card-left">
                  <span class="qz-card-dot" :class="j.status"></span>
                  <span class="qz-card-type-badge" :class="typeClass(j.type)">{{ j.type }}</span>
                  <span class="qz-card-deadline-short">{{ j.detail.deadline }}</span>
                </div>
                <div class="qz-card-body">
                  <div class="qz-card-row1">
                    <div class="qz-card-row1-left">
                      <span class="qz-card-position">{{ j.position }}</span>
                      <span class="qz-card-company">{{ j.company }}</span>
                    </div>
                    <div class="qz-card-salary-big">{{ j.salary }}<span class="unit">{{ $t('/月') }}</span></div>
                  </div>
                  <div class="qz-card-row2">
                    <span class="qz-card-meta">📍 {{ j.area }}</span>
                    <span class="qz-card-meta">🎓 {{ j.detail.edu }}</span>
                    <span class="qz-card-meta">⏱ {{ j.detail.exp }}</span>
                    <a class="qz-card-source" :href="j.sourceUrl" target="_blank" rel="noopener" @click.stop>🔗 {{ j.source }}</a>
                  </div>
                  <div class="qz-card-row3">
                    <div class="qz-card-tags">
                      <span v-for="(t, i) in j.detail.tags" :key="i" class="qz-card-tag" :class="t.color">{{ t.text }}</span>
                    </div>
                    <div class="qz-card-row3-right">
                      <span v-if="j.status !== 'ended'" class="qz-card-deadline" :class="deadlineClass(j.status)">⏰ {{ j.detail.deadline }}</span>
                      <span class="qz-card-badge" :class="j.status">{{ statusLabel(j.status) }}</span>
                      <span v-if="matchBadge(j)" class="qz-card-match" :class="matchBadge(j).cls">{{ matchBadge(j).text }}</span>
                    </div>
                  </div>
                </div>
                <div class="qz-card-right">
                  <div class="qz-card-training">
                    <div class="qz-ct-label">
                      <span class="qz-ct-icon">🎓</span>
                      <span>{{ $t('相关培训补贴') }}</span>
                    </div>
                    <div class="qz-ct-name">{{ j.detail.relatedTraining ? j.detail.relatedTraining.name : $t("暂无关联培训") }}</div>
                    <template v-if="j.detail.relatedTraining">
                      <div class="qz-ct-subsidy">
                        <span class="qz-ct-subsidy-amount">¥{{ j.detail.relatedTraining.subsidy }}</span>
                        <span class="qz-ct-subsidy-unit">{{ $t('/人') }}</span>
                      </div>
                      <div class="qz-ct-link" @click.stop="goToTraining(j.detail.relatedTraining.trainingId)">{{ $t('查看补贴详情 →') }}</div>
                    </template>
                    <div v-else class="qz-ct-none">{{ $t('暂无') }}</div>
                  </div>
                </div>
              </div>
            </div>
          </template>

          <!-- 培训补贴页 -->
          <template v-else>
            <div class="qz-training-header">
              <div class="qz-training-title"><span class="qz-training-icon">🎓</span> {{ $t('培训补贴 · 官办目录') }}</div>
              <div class="qz-training-tags">
                <span v-for="(d, i) in districtOptions" :key="d" class="qz-training-tag" :class="{ active: trainingDistrict === d }" @click="trainingDistrict = d">{{ d }}</span>
              </div>
            </div>
            <div class="qz-training-list">
              <div v-if="dataLoading" class="qz-empty-state">
                <div class="qz-empty-icon">⏳</div>
                <p>{{ $t('正在从文件库加载培训目录…') }}</p>
              </div>
              <div v-else-if="!visibleTraining.length" class="qz-empty-state">
                <div class="qz-empty-icon">🎓</div>
                <p>{{ $t('暂无培训补贴目录') }}</p>
              </div>
              <div v-for="t in visibleTraining" :key="t.id" class="qz-training-card" :id="'qz-training-' + t.id">
                <div class="qz-tc-top">
                  <div class="qz-tc-title">{{ t.title }}</div>
                  <span class="qz-tc-badge" :class="{ new: t.isNew }">{{ t.isNew ? $t("最新发布") : $t('官办目录') }}</span>
                </div>
                <div class="qz-tc-meta">
                  <span class="qz-tc-meta-item">📍 {{ t.district }}</span>
                  <span class="qz-tc-meta-item">🏛 {{ t.org }}</span>
                  <span class="qz-tc-meta-item">📅 {{ t.date }}</span>
                  <span class="qz-tc-meta-item">📚 {{ t.count }} {{ $t('个工种') }}</span>
                </div>
                <div class="qz-tc-desc">{{ t.desc }}</div>
                <div class="qz-tc-bottom">
                  <div class="qz-tc-subsidy">{{ $t('补贴标准') }} <span class="amount">{{ t.subsidy > 0 ? '¥' + t.subsidy : $t("见目录") }}</span>{{ t.subsidy > 0 ? ' /人' : '' }}</div>
                  <span class="qz-tc-detail-link" @click="trainingDetail(t)">{{ $t('查看详情') }} <svg width="12" height="12" viewBox="0 0 16 16" fill="none"><path d="M6 3l5 5-5 5" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg></span>
                </div>
              </div>
            </div>
          </template>
        </main>
      </div>
    </div>

    <!-- ===== 趋势条 ===== -->
    <div class="qz-trend-strip">
      <div class="qz-container qz-trend-strip-inner">
        <span class="qz-trend-strip-label">{{ $t('📊 趋势速览') }}</span>
        <div class="qz-trend-strip-items">
          <span v-for="(t, i) in trendItems" :key="i" class="qz-trend-strip-item" :class="t.up ? 'up' : 'down'">
            {{ t.label }} <b>{{ t.value }}</b>
          </span>
          <span class="qz-trend-strip-expand" @click="openTrend">{{ $t('展开 →') }}</span>
        </div>
      </div>
    </div>

    <!-- ===== 详情面板 ===== -->
    <div class="qz-detail-overlay" :class="{ show: detailJob }" @click.self="closeDetail">
      <div class="qz-detail-panel" v-if="detailJob">
        <div class="qz-detail-header">
          <button class="qz-detail-close" @click="closeDetail">×</button>
          <div class="qz-detail-company">{{ detailJob.company }}</div>
          <div class="qz-detail-position">{{ detailJob.position }}</div>
          <div class="qz-detail-badges">
            <span class="qz-card-badge" :class="detailJob.status">{{ statusLabel(detailJob.status) }}</span>
            <span class="qz-card-badge" style="background:rgba(232,168,56,0.15);color:var(--th-c-coral-d);">{{ detailJob.type }}</span>
            <span class="qz-card-badge" style="background:rgba(74,144,184,0.15);color:var(--th-c-sky);">{{ detailJob.area }}</span>
          </div>
        </div>
        <div class="qz-detail-body">
          <div class="qz-detail-section">
            <div class="qz-detail-section-title">{{ $t('📋 基本信息') }}</div>
            <div class="qz-detail-grid">
              <div class="qz-detail-cell"><div class="qz-detail-cell-label">{{ $t('学历要求') }}</div><div class="qz-detail-cell-value">{{ detailJob.detail.edu }}</div></div>
              <div class="qz-detail-cell"><div class="qz-detail-cell-label">{{ $t('经验要求') }}</div><div class="qz-detail-cell-value">{{ detailJob.detail.exp }}</div></div>
              <div class="qz-detail-cell"><div class="qz-detail-cell-label">{{ $t('用工性质') }}</div><div class="qz-detail-cell-value">{{ detailJob.detail.nature }}</div></div>
              <div class="qz-detail-cell"><div class="qz-detail-cell-label">{{ $t('截止/开启') }}</div><div class="qz-detail-cell-value">{{ detailJob.detail.deadline }}</div></div>
              <div class="qz-detail-cell"><div class="qz-detail-cell-label">{{ $t('薪资区间') }}</div><div class="qz-detail-cell-value" style="color:var(--th-c-coral-d);">{{ detailJob.detail.salaryRange }}</div></div>
              <div class="qz-detail-cell"><div class="qz-detail-cell-label">{{ $t('数据来源') }}</div><div class="qz-detail-cell-value" style="color:var(--th-c-main-l);">{{ detailJob.source }}</div></div>
            </div>
          </div>

          <div v-if="matched" class="qz-detail-section">
            <div class="qz-detail-section-title">{{ $t('🎯 能力匹配诊断') }}</div>
            <div class="qz-match-box" :class="detailJob.detail.matchType === 'pass' ? '' : (detailJob.detail.matchType === 'hard' ? 'gap' : 'nomatch')">
              <div class="qz-match-box-title" :class="detailJob.detail.matchType === 'pass' ? 'pass' : 'hard'">{{ detailJob.detail.matchTitle }}</div>
              <div class="qz-match-box-desc">{{ detailJob.detail.matchDesc }}</div>
              <div v-if="detailJob.detail.tags" class="qz-card-tags" style="margin-top:8px;">
                <span v-for="(t, i) in detailJob.detail.tags" :key="i" class="qz-card-tag" :class="t.color">{{ t.text }}</span>
              </div>
            </div>
            <div v-if="detailJob.detail.gap" class="qz-gap-suggestion" style="margin-top:10px;">
              <strong>{{ $t('💡 怎么补：') }}</strong>{{ detailJob.detail.gap }}
            </div>
          </div>

          <div class="qz-detail-section">
            <div class="qz-detail-section-title">{{ $t('🔗 溯源') }}</div>
            <div class="qz-source-link-box">
              <span style="font-size:13px;color:var(--th-ink-2);">{{ $t('每条结论可点回官方原文') }}</span>
              <a :href="detailJob.sourceUrl" target="_blank">{{ detailJob.source }} →</a>
            </div>
          </div>

          <div v-if="detailJob.detail.related" class="qz-detail-section">
            <div class="qz-detail-section-title">{{ $t('📌 相关岗位') }}</div>
            <div class="qz-related-list">
              <div v-for="(r, i) in detailJob.detail.related" :key="i" class="qz-related-item" @click="gotoRelated(r)">
                <span class="r-title">{{ r.title }}</span>
                <span class="r-meta">{{ r.meta }}</span>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- ===== 趋势弹窗 ===== -->
    <div class="qz-trend-overlay" :class="{ show: trendOpen }" @click.self="closeTrend">
      <div class="qz-trend-modal">
        <div class="qz-trend-modal-header">
          <span>{{ $t('📊 本地岗位趋势') }}</span>
          <button class="qz-trend-modal-close" @click="closeTrend">×</button>
        </div>
        <div class="qz-trend-modal-body">
          <div v-for="t in trendItems" :key="t.label" class="qz-trend-bar-group">
            <div class="qz-trend-bar-label">{{ t.label }} <b>{{ t.value }}</b></div>
            <div class="qz-trend-bar-track"><div class="qz-trend-bar-fill" :class="t.up ? 'up' : 'down'" :style="{ width: t.width + '%' }"></div></div>
          </div>
          <div class="qz-detail-section-title" style="margin-top:20px;">{{ $t('🧭 该怎么走') }}</div>
          <div class="qz-trend-card">
            <div class="qz-trend-card-title">{{ $t('① 先看匹配高的岗位') }}</div>
            <div class="qz-trend-card-desc">{{ $t('填学历/专业点「匹配」，系统给出能直接投的岗位，别在够不着的岗位上空耗。') }}</div>
          </div>
          <div class="qz-trend-card">
            <div class="qz-trend-card-title">{{ $t('② 差一点 = 有补法') }}</div>
            <div class="qz-trend-card-desc">{{ $t('「差:学历」「差:经验」的岗位，详情页给了补齐路径（读研/实习/考证），别直接放弃。') }}</div>
          </div>
          <div class="qz-trend-card">
            <div class="qz-trend-card-title">{{ $t('③ 培训补贴是加速器') }}</div>
            <div class="qz-trend-card-desc">{{ $t('岗位要的证，正好有官办培训补贴（¥1200-3000/人）。先考证再投，命中率翻倍。') }}</div>
          </div>
        </div>
      </div>
    </div>
  </div>
    <AskWidget />
</template>

<script setup>
import { t as i18t } from '@/i18n'
import AskWidget from '../AskWidget.vue'
import { ref, reactive, computed, onMounted } from 'vue'
import { loadData } from './data'

const jobList = ref([])
const trainingList = ref([])
const dataLoading = ref(true)
const cities = ['蚌埠', '合肥', '芜湖', '安庆']
const city = ref('蚌埠')
const cityOpen = ref(false)
const nav = ref('jobs') // jobs | training
const searchQ = ref('')
const matched = ref(false)
const sortOpen = ref(false)
const trendOpen = ref(false)
const detailJob = ref(null)
const trainingDistrict = ref('全部')

const filter = reactive({ area: 'all', type: 'all', salary: 'all', status: 'live' })
const cond = reactive({ edu: '', major: '', exp: '', pref: '' })
const sortBy = ref('urgent')

const statusMap = { live: i18t('进行中'), soon: i18t('即将开启'), ended: i18t('已结束') }
const sortOptions = { urgent: i18t('紧急度'), salary: i18t('薪资高→低'), match: i18t('匹配度'), salaryAsc: i18t('薪资低→高'), newest: i18t('最新发布') }
const trendItems = [
  { label: '商业航天', value: '↑210%', up: true, width: 90 },
  { label: '新能源材料', value: '↑160%', up: true, width: 78 },
  { label: '智能装备', value: '↑120%', up: true, width: 66 },
  { label: '半导体封测', value: '↑85%', up: true, width: 55 },
  { label: '传统制造业', value: '↓18%', up: false, width: 38 },
  { label: '低端服务岗', value: '↓25%', up: false, width: 30 },
  { label: '纯文职', value: '↓30%', up: false, width: 24 }
]
const sourceCount = computed(() => new Set([...jobList.value, ...trainingList.value].map(x => x.source).filter(Boolean)).size || 0)
const jobCount = computed(() => jobList.value.length)

const areaOptions = computed(() => [...new Set(jobList.value.map(j => j.area))])
const typeOptions = computed(() => [...new Set(jobList.value.map(j => j.type))])
const districtOptions = computed(() => [i18t('全部'), ...new Set(trainingList.value.map(t => t.district))])

function salaryOk(j, band) {
  if (band === 'all') return true
  const n = j.salaryNum || 0
  if (n === 0) return false
  if (band === 'low') return n < 4
  if (band === 'mid') return n >= 4 && n < 8
  if (band === 'high') return n >= 8
  return true
}

function filtered() {
  const q = searchQ.value.toLowerCase()
  return jobList.value.filter(j => {
    if (filter.status !== 'all' && j.status !== filter.status) return false
    if (filter.area !== 'all' && j.area !== filter.area) return false
    if (filter.type !== 'all' && j.type !== filter.type) return false
    if (!salaryOk(j, filter.salary)) return false
    if (q) {
      const s = (j.company + j.position + j.area + j.type + j.source).toLowerCase()
      if (!s.includes(q)) return false
    }
    return true
  })
}

const visibleJobs = computed(() => {
  const arr = filtered()
  const statusRank = { live: 0, soon: 1, ended: 2 }
  const matchRank = { high: 0, mid: 1, gap: 2 }
  arr.sort((a, b) => {
    if (sortBy.value === 'match' || matched.value) {
      const m = (matchRank[a.match] ?? 3) - (matchRank[b.match] ?? 3)
      if (m !== 0) return m
      if (sortBy.value === 'match') {
        const s = (statusRank[a.status] ?? 9) - (statusRank[b.status] ?? 9)
        if (s !== 0) return s
        return (b.salaryNum || 0) - (a.salaryNum || 0)
      }
    }
    if (sortBy.value === 'salary') return (b.salaryNum || 0) - (a.salaryNum || 0)
    if (sortBy.value === 'salaryAsc') return (a.salaryNum || 0) - (b.salaryNum || 0)
    if (sortBy.value === 'newest') return (b.id || 0) - (a.id || 0)
    const s = (statusRank[a.status] ?? 9) - (statusRank[b.status] ?? 9)
    if (s !== 0) return s
    return (b.salaryNum || 0) - (a.salaryNum || 0)
  })
  return arr
})

const visibleTraining = computed(() => {
  if (trainingDistrict.value === '全部') return trainingList.value
  return trainingList.value.filter(t => t.district === trainingDistrict.value)
})

function countFor(field, value) {
  const ov = { ...filter }
  ov[field] = value
  if (field !== 'status') ov.status = 'all'
  return jobList.value.filter(j => {
    if (ov.status !== 'all' && j.status !== ov.status) return false
    if (ov.area !== 'all' && j.area !== ov.area) return false
    if (ov.type !== 'all' && j.type !== ov.type) return false
    if (!salaryOk(j, ov.salary)) return false
    return true
  }).length
}

function statusCount(s) {
  return countFor('status', s)
}

function setFilter(field, value) {
  filter[field] = value
  if (nav.value !== 'jobs') nav.value = 'jobs'
}

function setSort(key) {
  sortBy.value = key
  sortOpen.value = false
}

function toggleSort() { sortOpen.value = !sortOpen.value }

const sortLabel = computed(() => sortOptions[sortBy.value])

function onSearch() { /* 响应式已驱动 */ }

function clearAllFilters() {
  filter.area = 'all'; filter.type = 'all'; filter.salary = 'all'; filter.status = 'live'
  searchQ.value = ''
}

// ===== 匹配 =====
function runMatch() {
  if (!cond.edu && !cond.major) {
    alert(i18t('请至少选择学历或专业'))
    return
  }
  matched.value = true
  jobList.value.forEach(j => {
    const need = (j.detail && j.detail.edu) || ''
    if ((cond.edu === '高中' || cond.edu === '大专') && /硕士|博士/.test(need) && j.match === 'high') {
      j.match = 'gap'
      j.matchLabel = '差:学历'
    }
  })
}

const passCount = computed(() => jobList.value.filter(j => j.match === 'high').length)
const hardCount = computed(() => jobList.value.filter(j => j.match === 'mid' || j.match === 'gap').length)
const missCount = computed(() => jobList.value.filter(j => j.status === 'ended' && j.match !== 'high').length)

function resetMatch() {
  matched.value = false
  const defaults = { 1: 'high', 2: 'gap', 3: 'high', 4: 'mid', 5: 'mid', 6: 'high', 7: 'mid', 8: 'high' }
  jobList.value.forEach(j => {
    if (defaults[j.id]) j.match = defaults[j.id]
    j.matchLabel = { high: i18t('高匹配'), gap: i18t('差:学历'), mid: i18t('需招考') }[j.match] || j.matchLabel
    if (j.id === 5) j.matchLabel = i18t('需关注')
    if (j.id === 6 || j.id === 8) j.matchLabel = i18t('已结束')
    if (j.id === 7) j.matchLabel = i18t('需平移')
  })
}

// ===== 卡片辅助 =====
function statusLabel(s) { return statusMap[s] || s }
function typeClass(t) { return { 事业编: 'state', 国企: 'gov', 合同制: 'contract', 基层: 'grass' }[t] || 'grass' }
function deadlineClass(s) { return s === 'ended' ? 'ended' : (s === 'soon' ? 'soon' : 'normal') }
function cardClass(j) { return 's-' + j.status }
function matchBadge(j) {
  if (matched.value && j.status !== 'ended') {
    const cls = j.match === 'high' ? 'high' : (j.match === 'gap' ? 'gap' : 'mid')
    return { cls, text: j.matchLabel }
  }
  if (j.status === 'ended' && j.match === 'high') return { cls: 'fomo', text: i18t('来晚了') }
  return null
}

// ===== 详情 =====
function openDetail(j) { detailJob.value = j }
function closeDetail() { detailJob.value = null }

function gotoRelated(r) {
  const key = r.title.split('·')[0].trim()
  const hit = jobList.value.find(j => j.company === key || j.company.includes(key))
  closeDetail()
  if (hit) openDetail(hit)
}

// ===== 培训 =====
function goToTraining(id) {
  nav.value = 'training'
  trainingDistrict.value = '全部'
  setTimeout(() => {
    const el = document.getElementById('qz-training-' + id)
    if (el) {
      el.scrollIntoView({ behavior: 'smooth', block: 'center' })
      el.style.transition = 'box-shadow 0.3s, transform 0.3s'
      el.style.boxShadow = '0 8px 24px rgba(0, 183, 100, 0.25)'
      el.style.transform = 'scale(1.01)'
      setTimeout(() => { el.style.boxShadow = ''; el.style.transform = '' }, 1500)
    }
  }, 80)
}

function trainingDetail(t) { alert(i18t('查看详情：') + t.title) }

// ===== 导航 / 城市 / 趋势 =====
function switchNav(n) { nav.value = n }
function toggleCity() { cityOpen.value = !cityOpen.value }
function switchCity(c) {
  city.value = c
  cityOpen.value = false
}
function openTrend() { trendOpen.value = true }
function closeTrend() { trendOpen.value = false }
function graphTip() { alert(i18t('溯源图谱功能开发中')) }

// Esc 关闭浮层
onMounted(async () => {
  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape') { closeDetail(); closeTrend() }
  })
  const d = await loadData().catch(() => ({ jobs: [], trainingData: [] }))
  jobList.value = d.jobs || []
  trainingList.value = d.trainingData || []
  dataLoading.value = false
})
</script>

<style scoped src="./style.css"></style>
