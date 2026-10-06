/**
 * 求智主题数据 —— 数据源为「文件库管道」：采集只是入库手段，主题直接消费
 * 已入库文件（标签 情报/就业、情报/培训 下的采集产物）及其 front matter / 标签关联。
 *
 * 管道：采集 → 入库为带 front matter 的 md 文件（kind/status/city/source…）→
 *       本文件 loadData() 按标签取文件 → 解析 front matter → 组装 jobs/trainingData。
 * 样例数据仅作「文件库为空」时的兜底展示。
 *
 * 数据结构契约：
 *   jobs[].{ id, company, position, area, type, salary, salaryNum, status(live|soon|ended),
 *            source, sourceUrl, match(high|mid|gap), matchLabel,
 *            detail.{ edu, exp, nature, deadline, salaryRange, matchTitle, matchType,
 *                     matchDesc, tags[{text,color}], gap, related[{title,meta}],
 *                     relatedTraining{name,subsidy,trainingId} } }
 *   trainingData[].{ id, title, org, date, source, sourceUrl, isNew, district,
 *                    count, subsidy, careers[], desc }
 */

import { listTags, listFilesByTag, fetchTextContent } from '@/api'

// ============ 样例数据（兜底：文件库无数据时展示） ============

const sampleTrainingData = [
  {
    id: 1,
    title: "2026年度龙子湖区第二批补贴性职业技能培训机构目录",
    org: "龙子湖区人社局",
    date: "2026-05-11",
    source: "职业培训信息发布",
    sourceUrl: "#",
    isNew: true,
    district: "龙子湖区",
    count: 3,
    subsidy: 1500,
    careers: ["无人机", "电工", "焊工"],
    desc: "公布蚌埠智飞无人机技能培训学校等3家承担补贴性职业技能培训的机构及其培训职业、等级和联系方式。机构需加强基础能力建设、师资配备和规范管理。"
  },
  {
    id: 2,
    title: "2026年度龙子湖区第一批补贴性职业技能培训机构目录",
    org: "龙子湖区人社局",
    date: "2026-02-10",
    source: "职业培训信息发布",
    sourceUrl: "#",
    isNew: false,
    district: "龙子湖区",
    count: 19,
    subsidy: 2000,
    careers: ["电工", "焊工", "家政服务", "中式烹调"],
    desc: "确定安徽机电技师学院等19家单位为第一批承担补贴性职业技能培训的机构。深入实施「技能照亮前程」培训行动，经机构自主申报、市人社局审核等程序。"
  },
  {
    id: 3,
    title: "蚌埠市市区2026年度职业技能提升行动补贴标准公示",
    org: "蚌埠市人社局",
    date: "2026-03-20",
    source: "市人社局官网",
    sourceUrl: "#",
    isNew: false,
    district: "全市",
    count: 42,
    subsidy: 3000,
    careers: ["高级工", "技师", "高级技师"],
    desc: "对取得高级工、技师、高级技师职业资格证书的，分别给予个人补贴。企业新型学徒制培训按规定给予补贴。"
  },
  {
    id: 4,
    title: "禹会区企业职工岗位技能提升培训补贴申报指南",
    org: "禹会区人社局",
    date: "2026-04-05",
    source: "禹会区政府网",
    sourceUrl: "#",
    isNew: false,
    district: "禹会区",
    count: 8,
    subsidy: 1800,
    careers: ["设备运维", "品质管理", "数控加工"],
    desc: "企业组织职工参加岗位技能提升培训，取得职业资格证书的，按规定给予企业培训补贴。个人也可自主参加培训并申领补贴。"
  }
]

const sampleJobs = [
  {
    id: 1, company: "城投融资控股", position: "合同制招聘",
    area: "全市", type: "合同制", salary: "面议", salaryNum: 0,
    status: "live", source: "蚌埠人事考试网", sourceUrl: "#",
    match: "high", matchLabel: "高匹配",
    detail: {
      edu: "本科", exp: "3年内达门槛", nature: "合同制",
      deadline: "招满即止", salaryRange: "面议（参考4-6k）",
      matchTitle: "走得通 · 匹配高", matchType: "pass",
      matchDesc: "本科+3年经验达门槛，安稳无业绩压力，适合求稳。本地城投系统长期稳定。",
      tags: [{ text: "安稳", color: "green" }, { text: "无业绩压力", color: "green" }],
      gap: null,
      related: [{ title: "丰原集团·工艺技术员", meta: "禹会区 · 国企 · 4.3-4.8k" }, { title: "淮上区月度清单", meta: "淮上区 · 基层 · 4-6k" }],
      relatedTraining: { name: "企业人力资源管理师", subsidy: 1500, trainingId: 2 }
    }
  },
  {
    id: 2, company: "中建材玻璃院", position: "研发工程师",
    area: "全市", type: "国企", salary: "8k-15k", salaryNum: 11.5,
    status: "live", source: "蚌埠人事考试网", sourceUrl: "#",
    match: "gap", matchLabel: "差:学历",
    detail: {
      edu: "硕士/博士", exp: "3年以上", nature: "国企正式",
      deadline: "招满即止", salaryRange: "8k-15k",
      matchTitle: "吃力 · 差在学历", matchType: "hard",
      matchDesc: "要求硕/博，本科暂不匹配。但可以走替代路径。",
      tags: [{ text: "国企", color: "amber" }, { text: "高薪", color: "amber" }],
      gap: "玻璃院研发差在学历，可走在职读研或转本科工艺岗——同单位有工艺岗不要求硕士。",
      related: [{ title: "丰原集团·工艺技术员", meta: "禹会区 · 国企 · 4.3-4.8k" }, { title: "蚌埠卷烟厂·操作类", meta: "全市 · 国企 · 5-7k" }],
      relatedTraining: { name: "材料物理性能检验员", subsidy: 2000, trainingId: 3 }
    }
  },
  {
    id: 3, company: "丰原集团", position: "工艺技术员",
    area: "禹会区", type: "国企", salary: "4.3k-4.8k", salaryNum: 4.55,
    status: "live", source: "安徽公共招聘网", sourceUrl: "#",
    match: "high", matchLabel: "高匹配",
    detail: {
      edu: "本科", exp: "不限", nature: "国企正式",
      deadline: "招满即止", salaryRange: "4350-4850",
      matchTitle: "走得通 · 匹配高", matchType: "pass",
      matchDesc: "本科·机械材料电气完全对口，本地老牌国企安稳，起薪4350-4850。专业对口+本地+安稳。",
      tags: [{ text: "专业对口", color: "green" }, { text: "安稳", color: "green" }, { text: "本地", color: "green" }],
      gap: null,
      related: [{ title: "城投融资控股·合同制", meta: "全市 · 合同制 · 面议" }, { title: "中建材玻璃院·工艺岗", meta: "全市 · 国企 · 6-9k" }],
      relatedTraining: { name: "无人机驾驶员", subsidy: 1800, trainingId: 1 }
    }
  },
  {
    id: 4, company: "轨道交通职业学院", position: "事业编教师",
    area: "全市", type: "事业编", salary: "5k-8k", salaryNum: 6.5,
    status: "soon", source: "安徽公共招聘网", sourceUrl: "#",
    match: "mid", matchLabel: "需招考",
    detail: {
      edu: "本科+教师资格", exp: "不限", nature: "事业编",
      deadline: "预计10月开启", salaryRange: "5k-8k",
      matchTitle: "吃力 · 需招考", matchType: "hard",
      matchDesc: "需招考季报名+笔试，非随时可进。建议按考季准备，不冲突。",
      tags: [{ text: "事业编", color: "amber" }, { text: "需笔试", color: "amber" }],
      gap: "需招考季报名+笔试，建议按考季准备。目前不冲突，可先走企业岗，考季再报。",
      related: [{ title: "城投融资控股·合同制", meta: "全市 · 合同制" }, { title: "丰原集团·工艺技术员", meta: "禹会区 · 国企" }],
      relatedTraining: { name: "电工（中级）", subsidy: 1500, trainingId: 2 }
    }
  },
  {
    id: 5, company: "高新区·九州云箭", position: "商业航天专场",
    area: "高新区", type: "国企", salary: "7k-12k", salaryNum: 9.5,
    status: "soon", source: "高新区人社局", sourceUrl: "#",
    match: "mid", matchLabel: "需关注",
    detail: {
      edu: "本科及以上", exp: "1年以上", nature: "国企/合资",
      deadline: "预计11月开启", salaryRange: "7k-12k",
      matchTitle: "值得准备", matchType: "hard",
      matchDesc: "商业航天是本地增长最快赛道，相关能力需求↑210%。需提前准备相关项目经历。",
      tags: [{ text: "商业航天", color: "amber" }, { text: "高增长", color: "amber" }, { text: "缺口大", color: "coral" }],
      gap: "失效分析/工艺工程能力是关键。现在补免费公开课+本地企业实习，11月前攒出项目证明。",
      related: [{ title: "中建材玻璃院·研发", meta: "全市 · 国企 · 8-15k" }, { title: "丰原集团·工艺技术员", meta: "禹会区 · 国企" }],
      relatedTraining: { name: "化工总控工", subsidy: 1800, trainingId: 4 }
    }
  },
  {
    id: 6, company: "蚌埠卷烟厂", position: "生产操作类社招",
    area: "全市", type: "国企", salary: "5k-7k", salaryNum: 6,
    status: "ended", source: "蚌埠人事考试网", sourceUrl: "#",
    match: "high", matchLabel: "已结束",
    detail: {
      edu: "本科", exp: "不限", nature: "国企正式",
      deadline: "已结束", salaryRange: "5k-7k",
      matchTitle: "本该走得通 · 已错过", matchType: "nomatch",
      matchDesc: "条件完全匹配，但已结束。本地烟厂待遇好、最安稳，下次秋招别再错过。",
      tags: [{ text: "国企", color: "amber" }, { text: "最安稳", color: "amber" }],
      gap: null,
      related: [{ title: "丰原集团·工艺技术员", meta: "禹会区 · 国企 · 4.3-4.8k" }, { title: "城投融资控股", meta: "全市 · 合同制" }],
      relatedTraining: { name: "烟草制品检验员", subsidy: 2000, trainingId: 3 }
    }
  },
  {
    id: 7, company: "淮上区月度清单", position: "品质/运维",
    area: "全市", type: "基层", salary: "5k-10k", salaryNum: 7.5,
    status: "ended", source: "淮上区人社局", sourceUrl: "#",
    match: "mid", matchLabel: "需平移",
    detail: {
      edu: "高中/中专以上", exp: "1年以上", nature: "企业直招",
      deadline: "已结束", salaryRange: "5k-10k",
      matchTitle: "走得通 · 需能力平移", matchType: "hard",
      matchDesc: "需从纯操作向质检/设备运维平移。门槛适中，下月清单会再来。",
      tags: [{ text: "需平移", color: "amber" }, { text: "门槛适中", color: "amber" }],
      gap: "从操作岗向设备运维/质检平移，需补设备基础+质检经验。下月清单再来时可直接报。",
      related: [{ title: "丰原集团·工艺技术员", meta: "禹会区 · 国企" }, { title: "高新区·九州云箭", meta: "高新区 · 国企 · 7-12k" }],
      relatedTraining: { name: "数控车工（高级）", subsidy: 2500, trainingId: 4 }
    }
  },
  {
    id: 8, company: "蚌埠卷烟厂", position: "去年秋招",
    area: "全市", type: "国企", salary: "5k-7k", salaryNum: 6,
    status: "ended", source: "蚌埠人事考试网", sourceUrl: "#",
    match: "high", matchLabel: "已结束",
    detail: {
      edu: "本科", exp: "不限", nature: "国企正式",
      deadline: "已结束", salaryRange: "5k-7k",
      matchTitle: "本该走得通 · 已错过", matchType: "nomatch",
      matchDesc: "去年秋招已结束。今年秋招预计10-11月，条件完全匹配，务必提前关注。",
      tags: [{ text: "国企", color: "amber" }, { text: "最安稳", color: "amber" }],
      gap: null,
      related: [{ title: "轨道交通职业学院", meta: "全市 · 事业编 · 预计10月" }, { title: "高新区·九州云箭", meta: "高新区 · 预计11月" }],
      relatedTraining: { name: "中式烹调师", subsidy: 1200, trainingId: 2 }
    }
  }
]

/**
 * 数据提供者：loadData() 返回 { jobs, trainingData }。
 *
 * 数据源 = 文件库管道：按标签「情报/就业」「情报/培训」取采集入库文件，
 * 解析 front matter（kind/status/city/city_resolved/source_name/source_url/collected_at）
 * 与文件名（日期前缀）组装；字段缺失给默认值（—），保证组件零改动。
 * 文件库无数据 / 读取失败 → 返回样例数据兜底（console 提示）。
 */
const JOB_TAG = '情报/就业'
const TRAIN_TAG = '情报/培训'
// 回退简名：层级标签未命中时用简名标签兜底（历史数据打标差异）
const JOB_TAG_FALLBACK = '就业'
const TRAIN_TAG_FALLBACK = '培训'

function parseFrontMatter(content) {
  const fm = {}
  const m = content.match(/^---\s*\n([\s\S]*?)\n---/)
  if (m) {
    for (const line of m[1].split('\n')) {
      const i = line.indexOf(':')
      if (i > 0) fm[line.slice(0, i).trim()] = line.slice(i + 1).trim().replace(/^["']|["']$/g, '')
    }
  }
  return fm
}

function stripDatePrefix(name) {
  return (name || '').replace(/^\d{4}-\d{2}-\d{2}-/, '').replace(/\.md$/i, '').trim()
}

function bodyPreview(content) {
  const s = (content || '').replace(/^---[\s\S]*?---/, '').replace(/^#\s.*$/m, '').replace(/[#>*\-\[\]()|`\n]+/g, ' ').replace(/\s+/g, ' ').trim()
  return s.slice(0, 100) + (s.length > 100 ? '…' : '')
}

function extractCompany(title) {
  const m = (title || '').match(/([\u4e00-\u9fa5A-Za-z0-9（）()]+?(?:有限公司|集团|公司|学校|学院|中心))/)
  return m ? m[1] : ''
}

function dedupFiles(files) {
  const seen = new Set()
  const out = []
  for (const f of files) {
    if (!f || seen.has(f.id)) continue
    seen.add(f.id)
    out.push(f)
  }
  return out
}

function mapJob(f, content, idx) {
  const fm = parseFrontMatter(content)
  const title = stripDatePrefix(f.name) || '（未命名岗位）'
  const company = extractCompany(title)
  const area = fm.city_resolved || fm.city || '—'
  const status = fm.status === 'ended' ? 'ended' : (fm.status === 'soon' ? 'soon' : 'live')
  const source = fm.source_name || '—'
  return {
    id: idx + 1,
    company: company || '—',
    position: title,
    area,
    type: '—',
    salary: '—',
    salaryNum: 0,
    status,
    source,
    sourceUrl: fm.source_url || '#',
    match: 'mid',
    matchLabel: '待匹配',
    detail: {
      edu: '—',
      exp: '—',
      nature: '—',
      deadline: status === 'ended' ? '已结束' : '招满即止',
      salaryRange: '—',
      matchTitle: status === 'ended' ? '已结束收录' : '已收录 · 可查看',
      matchType: 'mid',
      matchDesc: bodyPreview(content) || '（暂无摘要）',
      tags: [
        { text: area !== '—' ? area : '本地', color: 'green' },
        { text: source !== '—' ? source.slice(0, 8) : '官办', color: 'sky' }
      ],
      gap: null,
      related: [],
      relatedTraining: null
    }
  }
}

function mapTraining(f, content, idx) {
  const fm = parseFrontMatter(content)
  const title = stripDatePrefix(f.name) || '（未命名目录）'
  const date = fm.collected_at || ''
  const district = fm.city_resolved || '全市'
  const subsidyM = (content || '').match(/(?:补贴|补助|培训费)[^\d]{0,24}(\d{3,5})\s*元(?:\s*[/／]?\s*人)?/) || (content || '').match(/(\d{3,5})\s*元\s*[/／]\s*人/)
  const subsidy = subsidyM ? parseInt(subsidyM[1], 10) : 0
  let isNew = false
  if (date) {
    const days = (Date.now() - new Date(date).getTime()) / 86400000
    isNew = days >= 0 && days <= 45
  }
  return {
    id: idx + 1,
    title,
    org: fm.source_name || '—',
    date: date || '—',
    source: fm.source_name || '—',
    sourceUrl: fm.source_url || '#',
    isNew,
    district,
    count: 0,
    subsidy,
    careers: [],
    desc: bodyPreview(content) || '（暂无摘要）'
  }
}

async function loadFromLibrary() {
  let tags = []
  try { tags = await listTags() } catch (e) { return null }
  const jobTags = tags.filter(t => t.name === JOB_TAG || t.name === JOB_TAG_FALLBACK)
  const trainTags = tags.filter(t => t.name === TRAIN_TAG || t.name === TRAIN_TAG_FALLBACK)
  if (!jobTags.length && !trainTags.length) return null
  // 双标签并集（按文件 id 去重）
  const jobSets = await Promise.all(jobTags.map(t => listFilesByTag(t.id).catch(() => [])))
  const trainSets = await Promise.all(trainTags.map(t => listFilesByTag(t.id).catch(() => [])))
  const jobFiles = dedupFiles(jobSets.flat())
  const trainFiles = dedupFiles(trainSets.flat())
  if (!jobFiles.length && !trainFiles.length) return null
  const jobs = []
  const trainingData = []
  let i = 0
  for (const f of jobFiles) {
    if (f.kind === 'dir') continue
    const content = await fetchTextContent(f.id).catch(() => '')
    jobs.push(mapJob(f, content, i++))
  }
  let j = 0
  for (const f of trainFiles) {
    if (f.kind === 'dir') continue
    const content = await fetchTextContent(f.id).catch(() => '')
    trainingData.push(mapTraining(f, content, j++))
  }
  return { jobs, trainingData }
}

export async function loadData() {
  const real = await loadFromLibrary().catch(() => null)
  if (real && (real.jobs.length || real.trainingData.length)) {
    return real
  }
  console.warn('[qiuzhi-theme] 文件库无采集数据（标签 情报/就业、情报/培训），回退样例数据')
  return { jobs: sampleJobs, trainingData: sampleTrainingData }
}
