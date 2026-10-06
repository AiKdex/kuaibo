// 内容状态解析工具：从 file.content_state（JSON 字符串）安全提取
// 生产站底座 A-G 扩展字段 node_type / fields，兼容旧 visibility/status 双轨。
//
// 旧字段（AiKlog 原有）：visibility / status
// 新字段（底座 A-G 对接）：node_type（内容类型）/ fields（自定义业务字段）
// 两类字段共存于同一 content_state JSON，互不影响。

// 已知内容类型（驱动差异化渲染）
export const NODE_TYPES = ['article', 'resource', 'link', 'doc', 'tool']

// 字段中文标签（用于信息卡/筛选卡渲染）
const LABELS = {
  download_url: '下载地址',
  file_name: '文件名',
  version: '版本',
  platform: '平台',
  size_hint: '大小',
  author_name: '作者',
  license: '许可',
  homepage: '主页',
  url: '链接',
  site_name: '站点',
  desc: '简介',
  price: '价格',
  currency: '币种',
  access: '访问方式',
  paid_preview: '试读',
  source: '来源',
  updated: '更新日期',
  language: '语言'
}

export function humanLabel(key) {
  return LABELS[key] || String(key).replace(/_/g, ' ')
}

export function formatFieldValue(v) {
  if (v == null) return ''
  if (typeof v === 'boolean') return v ? '是' : '否'
  if (Array.isArray(v)) return v.join('、')
  if (typeof v === 'object') return JSON.stringify(v)
  return String(v)
}

// 安全解析 content_state → 结构化对象（任何异常都回退到默认值）
export function parseContentState(file) {
  const raw = file && (file.content_state || file.contentState)
  let st = {}
  if (typeof raw === 'string' && raw.trim()) {
    try {
      st = JSON.parse(raw)
    } catch (_) {
      st = {}
    }
  } else if (raw && typeof raw === 'object') {
    st = raw
  }
  return {
    visibility: st.visibility || 'public',
    status: st.status || 'published',
    nodeType: st.node_type || '',
    fields: st.fields && typeof st.fields === 'object' ? st.fields : {}
  }
}

export function nodeTypeOf(file) {
  return parseContentState(file).nodeType
}

// 内容类型中文标签（用于列表卡角标/详情页标题）
const NODE_TYPE_LABELS = {
  article: '文章',
  resource: '资源',
  link: '外链',
  doc: '文档',
  tool: '工具'
}

export function nodeTypeLabel(nodeType) {
  if (!nodeType) return ''
  return NODE_TYPE_LABELS[nodeType] || String(nodeType)
}

// 将 fields 转为有序 [{key,label,value}] 列表（供信息卡/筛选卡渲染）
// opts.order: 字段显示顺序（键名数组）；opts.hide: 需要隐藏的键名
export function fieldList(file, opts = {}) {
  const { fields } = parseContentState(file)
  const order = opts.order || []
  const hide = new Set(opts.hide || [])
  const entries = Object.entries(fields).filter(([k]) => !hide.has(k))
  const sorted = [...entries].sort((a, b) => {
    const ia = order.indexOf(a[0])
    const ib = order.indexOf(b[0])
    if (ia === -1 && ib === -1) return 0
    if (ia === -1) return 1
    if (ib === -1) return -1
    return ia - ib
  })
  return sorted.map(([key, value]) => ({
    key,
    label: humanLabel(key),
    value: formatFieldValue(value)
  }))
}

export function isResource(file) {
  return nodeTypeOf(file) === 'resource'
}

export function isLink(file) {
  return nodeTypeOf(file) === 'link'
}
