// AiKlog（爱库录）API 客户端：对接后端 /api/v1 文件 API
// 阶段 1 覆盖：文件列表/树/上传/下载/目录/移动/删除/预览内容

import { reactive } from 'vue'

const BASE = '/api/v1'

// 响应式登录态：cookie 会话（HttpOnly）无法被 isAuthed 的 localStorage/sessionStorage 判定捕捉，
// 故以 /auth/me 实探结果作为权威，所有 computed(() => isAuthed()) 自动跟随。
export const authState = reactive({ loggedIn: false, ready: false, user: null })
let _authProbe = null

// ---- 会话（安全加固：登录鉴权） ----
const TOKEN_KEY = 'aikmap_token'
const AUTH_FLAG = 'aikmap_auth' // 登录标记（sessionStorage 布尔，不含凭证；凭证走 HttpOnly cookie）
export function getToken() {
  return localStorage.getItem(TOKEN_KEY) || ''
}
export function setToken(t) {
  if (t) localStorage.setItem(TOKEN_KEY, t)
  else localStorage.removeItem(TOKEN_KEY)
}
export function clearToken() {
  localStorage.removeItem(TOKEN_KEY)
}
export function setAuthFlag() {
  sessionStorage.setItem(AUTH_FLAG, '1')
}
export function clearAuthFlag() {
  sessionStorage.removeItem(AUTH_FLAG)
}
// 鉴权判定：响应式实探结果（权威）或旧 token 或登录标记
export function isAuthed() {
  return authState.loggedIn || !!getToken() || sessionStorage.getItem(AUTH_FLAG) === '1'
}
// 启动 / 需要时调用一次：实探 /auth/me 校准响应式登录态。带幂等保护，只探一次。
// 匿名态 401 不会触发 handle401 跳转（这里直接 fetch，不经由 requestJSON）。
export function refreshAuth() {
  if (_authProbe) return _authProbe
  _authProbe = (async () => {
    if (getToken() || sessionStorage.getItem(AUTH_FLAG) === '1') {
      authState.loggedIn = true
      authState.ready = true
      return true
    }
    try {
      const r = await fetch(BASE + '/auth/me', { headers: authHeaders(), credentials: 'same-origin' })
      if (r.ok) {
        try { authState.user = await r.json() } catch (_) {}
        authState.loggedIn = true
      } else {
        authState.loggedIn = false
      }
    } catch (_) {
      authState.loggedIn = false
    }
    authState.ready = true
    return authState.loggedIn
  })()
  return _authProbe
}
// 登录态探测（供组件按需调用；已确认登录则直接返回 true）
export async function probeAuthed() {
  if (isAuthed()) return true
  return refreshAuth()
}
function authHeaders(headers = {}) {
  const tok = getToken()
  return tok ? { ...headers, Authorization: `Bearer ${tok}` } : headers
}
// 401 统一处理：清 token 回控制台（登录接口自身除外，避免循环）
// 401 统一处理：清 token 回登录页（登录接口自身除外，避免循环）。
// F6 修复：竞态锁 —— 页面并行请求同时失效时会连续触发多次 handle401，
// 每次都改写 location.hash，路由守卫会把上一个 next 二次处理成错误目标（重定向竞态）。
// 已处于登录/工作台页说明竞态窗口结束，重置锁。
let _authRedirecting = false
function handle401(url) {
  // 免登录/登录页自身接口不触发跳转（避免循环）：登录、注册、邮箱验证码、能力探测
  if (url.includes('/auth/login') || url.includes('/auth/register') || url.includes('/auth/email-code') || url.includes('/auth/config')) return
  clearToken()
  clearAuthFlag()
  authState.loggedIn = false
  if (location.hash.startsWith('#/desk') || location.hash.startsWith('#/login')) {
    _authRedirecting = false
    return
  }
  if (_authRedirecting) return
  _authRedirecting = true
  const cur = location.hash.slice(1) || '/files'
  location.hash = `#/desk?next=${encodeURIComponent(cur)}`
}
export function authLogin(username, password) {
  const d = requestJSON('/auth/login', { method: 'POST', body: JSON.stringify({ username, password }) })
  authState.loggedIn = true
  return d
}
export function authLogout() {
  const d = requestJSON('/auth/logout', { method: 'POST', body: JSON.stringify({}) })
  authState.loggedIn = false
  clearAuthFlag()
  clearToken()
  return d
}
export function authMe() {
  return requestJSON('/auth/me')
}
export function authPassword(oldPassword, newPassword) {
  return requestJSON('/auth/password', {
    method: 'POST',
    body: JSON.stringify({ old_password: oldPassword, new_password: newPassword })
  })
}
// 能力探测（公开）：多用户/开放注册/邮箱验证开关，登录页据此决定是否展示注册入口
export function authConfig() {
  return requestJSON('/auth/config')
}
// 开放注册（受 site.registration_open 门禁；email_required 时强制邮箱+验证码）；注册成功即登录
export function authRegister(username, password, displayName = '', email = '', code = '') {
  const d = requestJSON('/auth/register', {
    method: 'POST',
    body: JSON.stringify({ username, password, display_name: displayName || '', email: email || '', code: code || '' })
  })
  authState.loggedIn = true
  return d
}
// 发送邮箱验证码（purpose: register|bind；debug 模式响应含 debug_code）
export function emailCode(email, purpose = 'register') {
  return requestJSON('/auth/email-code', { method: 'POST', body: JSON.stringify({ email, purpose }) })
}

// 请求超时（AI 类接口可能慢，默认 45s；其余 20s）
const TIMEOUT = 45000
const GET_TIMEOUT = 20000

// 带超时的 fetch：AbortController，超时抛 TimeoutError
async function fetchWithTimeout(url, options = {}, timeout = GET_TIMEOUT) {
  const ctrl = new AbortController()
  const t = setTimeout(() => ctrl.abort(), timeout)
  try {
    return await fetch(url, { ...options, credentials: 'same-origin', signal: ctrl.signal })
  } finally {
    clearTimeout(t)
  }
}

async function request(url, options = {}) {
  // GET 失败重试 1 次（指数退避）；写操作不重试（防重复副作用）
  const maxTry = (!options.method || options.method === 'GET') ? 2 : 1
  let lastErr
  for (let i = 0; i < maxTry; i++) {
    try {
      const res = await fetchWithTimeout(BASE + url, { ...options, headers: authHeaders(options.headers) }, options.method && options.method !== 'GET' ? TIMEOUT : GET_TIMEOUT)
      if (res.status === 401) handle401(url)
      if (!res.ok) {
        let msg = `HTTP ${res.status}`
        try {
          const body = await res.json()
          if (body?.error?.message) msg = body.error.message
          else if (body?.error?.code) msg = body.error.code
        } catch (_) {
          /* 非 JSON 响应 */
        }
        throw new Error(msg)
      }
      return res
    } catch (e) {
      lastErr = e
      if (e.name === 'AbortError') {
        throw new Error('请求超时，请稍后重试')
      }
      if (i < maxTry - 1) await new Promise((r) => setTimeout(r, 600 * (i + 1)))
    }
  }
  throw lastErr
}

export async function requestJSON(url, options = {}) {
  const res = await request(url, {
    ...options,
    headers: { 'Content-Type': 'application/json', ...(options.headers || {}) }
  })
  return res.json()
}

// 读取文件正文（文本类；写作工作台"载入已有文章"用）
export async function fetchFileText(id) {
  const res = await request(`/files/${id}/content`)
  return res.text()
}

// ---- 文件 ----

// 全库检索：关键词+向量 RRF 融合（语义检索）；scope=all|file|blog（默认 all 全部；file 排除博客；blog 仅博客）
export function searchFiles(q, limit = 30, scope = '') {
  const s = scope ? `&scope=${scope}` : ''
  return requestJSON(`/search?q=${encodeURIComponent(q)}&limit=${limit}${s}`).then((d) => d.items || [])
}

export function listFiles(parent = '') {
  const q = parent ? `?parent=${encodeURIComponent(parent)}` : ''
  return requestJSON(`/files${q}`).then((d) => d.items || [])
}

// 全空间文件名清单（[[ 联想数据源）
export function listWikiNames() {
  return requestJSON('/files?names=1').then((d) => d.items || [])
}

// 双链标题解析：[[标题]] → 文件 id（未匹配抛错）
export function resolveWikiTitle(name) {
  return requestJSON(`/files/resolve?name=${encodeURIComponent(name)}`)
}

// ---- AI 整理（P0-3）：批量建议 + 一键应用 ----
export function organizeSuggest(fileIds) {
  return requestJSON('/ai/organize/suggest', {
    method: 'POST',
    body: JSON.stringify({ file_ids: fileIds })
  }).then((d) => d.items || [])
}

export function organizeApply(payload) {
  return requestJSON('/ai/organize/apply', {
    method: 'POST',
    body: JSON.stringify(payload)
  })
}

// 草稿箱：全空间 status=draft 的文件（content_state.status）
export function listDrafts() {
  return requestJSON('/files?scope=drafts').then((d) => d.items || [])
}

// 更新内容状态：draft | published（草稿箱 ↔ 正式）
export function setFileStatus(id, status) {
  return requestJSON(`/files/${id}/status`, { method: 'PUT', body: JSON.stringify({ status }) })
}

// ---- 回收站 ----
export function listTrash() {
  return requestJSON('/files?scope=trash').then((d) => d.items || [])
}
export function restoreFile(id) {
  return requestJSON(`/files/${id}/restore`, { method: 'POST' })
}
export function purgeFile(id) {
  return requestJSON(`/files/${id}/purge`, { method: 'DELETE' })
}
export function emptyTrash() {
  return requestJSON('/files/trash/empty', { method: 'POST' })
}

// ---- 标签 / 分类（树状） ----
export function listTags() {
  return requestJSON('/tags').then((d) => d.items || [])
}
export function createTag(name, parentId = '') {
  return requestJSON('/tags', { method: 'POST', body: JSON.stringify({ name, parent_id: parentId }) })
}
export function renameTag(id, name) {
  return requestJSON(`/tags/${id}`, { method: 'PUT', body: JSON.stringify({ name }) })
}
export function deleteTag(id) {
  return requestJSON(`/tags/${id}`, { method: 'DELETE' })
}
export function getFileTags(id) {
  return requestJSON(`/files/${id}/tags`).then((d) => d.items || [])
}
export function setFileTags(id, tagIds) {
  return requestJSON(`/files/${id}/tags`, { method: 'PUT', body: JSON.stringify({ tag_ids: tagIds }) })
}
export function listFilesByTag(tagId) {
  return requestJSON(`/files?tag=${encodeURIComponent(tagId)}`).then((d) => d.items || [])
}

// ---- 知识库聚合层 ----
export function kbOverview() {
  return requestJSON('/kb/overview')
}
export function kbTags() {
  return requestJSON('/kb/tags').then((d) => d.items || [])
}
export function kbConcept(tagId) {
  return requestJSON(`/kb/concepts/${tagId}`)
}
export function kbConceptSummarize(tagId) {
  return requestJSON(`/kb/concepts/${tagId}/summarize`, { method: 'POST' })
}
export function kbGraph() {
  return requestJSON('/kb/graph')
}
export function kbSummariesRetry() {
  return requestJSON('/kb/summaries/retry', { method: 'POST' })
}
export function kbSummariesRequeue(body) {
  return requestJSON('/kb/summaries/requeue', { method: 'POST', body: JSON.stringify(body || {}) })
}
export function kbLint() {
  return requestJSON('/kb/lint')
}
export function kbDedupResolve(sha256, keepFileId, deleteFileIds) {
  return requestJSON('/kb/dedup/resolve', {
    method: 'POST',
    body: JSON.stringify({ sha256, keep_file_id: keepFileId, delete_file_ids: deleteFileIds })
  })
}

// ---- 集合 ----
export function listCollections() {
  return requestJSON('/collections').then((d) => d.items || [])
}
export function createCollection(name, kind = 'manual', query = '') {
  return requestJSON('/collections', { method: 'POST', body: JSON.stringify({ name, kind, query }) })
}
export function deleteCollection(id) {
  return requestJSON(`/collections/${id}`, { method: 'DELETE' })
}
export function listCollectionFiles(id) {
  return requestJSON(`/collections/${id}/files`).then((d) => d.items || [])
}
export function addCollectionFile(id, fileId) {
  return requestJSON(`/collections/${id}/files`, { method: 'POST', body: JSON.stringify({ file_id: fileId }) })
}
export function removeCollectionFile(id, fileId) {
  return requestJSON(`/collections/${id}/files/${fileId}`, { method: 'DELETE' })
}

export function getFile(id) {
  return requestJSON(`/files/${id}`)
}

// AI 入库解读（摘要+标签+元数据 K19）；无记录返回 {status:'none'}
export function getFileSummary(id) {
  return requestJSON(`/files/${id}/summary`)
}

export function getTree() {
  return requestJSON('/files/tree').then((d) => d.items || [])
}

export function mkdir(name, parent = '') {
  return requestJSON('/files/mkdir', { method: 'POST', body: JSON.stringify({ name, parent_id: parent }) })
}

// 新建文档（文本文件，带初始内容）
export function createDoc(name, parent = '', content = '') {
  return requestJSON('/files/doc', { method: 'POST', body: JSON.stringify({ name, parent_id: parent, content }) })
}

// 更新文本文件内容
export function updateDocContent(id, content) {
  return requestJSON(`/files/${id}/content`, { method: 'PUT', body: JSON.stringify({ content }) })
}

export function moveFile(id, parentId, name = '') {
  return requestJSON(`/files/${id}/move`, { method: 'POST', body: JSON.stringify({ parent_id: parentId, name }) })
}

export function copyFile(id, parentId, name = '') {
  return requestJSON(`/files/${id}/copy`, { method: 'POST', body: JSON.stringify({ parent_id: parentId, name }) })
}

export function deleteFile(id) {
  return request('/files/' + id, { method: 'DELETE' }).then(() => true)
}

// OCR 识别图片文字（模块组件：知识库入库/IM/阅读器复用）
export function ocrImage(payload) {
  return requestJSON('/ai/ocr', { method: 'POST', body: JSON.stringify(payload) })
}

// 文本整理（OCR 结果去噪声 → 干净 Markdown；purpose: ocr|raw）
export function cleanText(text, purpose = 'ocr') {
  return requestJSON('/ai/clean', { method: 'POST', body: JSON.stringify({ text, purpose }) })
}

// 上传（支持多文件与文件夹拖拽；list 为 [{ path, file }]，path 为相对路径，后端自动建目录；同名自动改名）
// onProgress: (done, total, percent) 回调（XMLHttpRequest 上报）
export function uploadFiles(list, parent = '', onProgress) {
  const fd = new FormData()
  for (const it of list) {
    fd.append('file', it.file)
    fd.append('path', it.path || it.file.name)
  }
  const q = parent ? `?parent=${encodeURIComponent(parent)}` : ''
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest()
    xhr.open('POST', `${BASE}/files/upload${q}`)
    const tok = getToken()
    if (tok) xhr.setRequestHeader('Authorization', `Bearer ${tok}`)
    if (onProgress) {
      xhr.upload.onprogress = (e) => {
        if (e.lengthComputable) onProgress(e.loaded, e.total, Math.round((e.loaded / e.total) * 100))
      }
    }
    xhr.onload = () => {
      if (xhr.status >= 200 && xhr.status < 300) {
        try {
          const d = JSON.parse(xhr.responseText)
          resolve(d.results || [])
        } catch (_) {
          reject(new Error('上传响应解析失败'))
        }
      } else {
        let msg = `HTTP ${xhr.status}`
        try {
          const body = JSON.parse(xhr.responseText)
          if (body?.error?.message) msg = body.error.message
        } catch (_) { /* 非 JSON */ }
        reject(new Error(msg))
      }
    }
    xhr.onerror = () => reject(new Error('网络错误，上传失败'))
    xhr.send(fd)
  })
}

// 文件内容 URL（预览/下载）
export function contentUrl(id, download = false) {
  const q = download ? '?download=1' : ''
  return `${BASE}/files/${id}/content${q}`
}

// 媒体 blob 加载（鉴权后 objectURL）：img/video/audio/pdf 用
// 带内存缓存（同 id 只拉一次），组件卸载时对象 URL 由浏览器 GC。
const _mediaCache = new Map()
export async function fetchMediaUrl(id, { force = false } = {}) {
  if (!force && _mediaCache.has(id)) return _mediaCache.get(id)
  const res = await request(`/files/${id}/content`)
  const blob = await res.blob()
  // F5 修复：force 刷新覆盖旧缓存前先 revoke 旧 objectURL，否则旧 blob URL 无人释放
  const old = _mediaCache.get(id)
  if (old) URL.revokeObjectURL(old)
  const url = URL.createObjectURL(blob)
  _mediaCache.set(id, url)
  return url
}
// F5：组件卸载/文件切换时手动释放媒体 objectURL（长会话下 blob 内存只增不减）
export function revokeMediaUrl(id) {
  const old = _mediaCache.get(id)
  if (old) URL.revokeObjectURL(old)
  _mediaCache.delete(id)
}
export function fetchMediaUrlSync(id) {
  return _mediaCache.get(id) || ''
}

// 缩略图 blob（B15）：走 /files/{id}/thumb 派生对象（≤480px），不再把原图整张拉进列表。
// 与 fetchMediaUrl 分开缓存，因为它是**可失败**的：未装 ffmpeg / 能力开关关闭 / 坏文件都会 404。
// 失败进负缓存（同一列表里不反复打），force=true 可重试（站长刚打开开关时用）。
// 超时 30s：首次请求要就地跑 ffmpeg 抽帧，比普通 GET 慢；生成一次后由 Cache-Control 命中。
const _thumbCache = new Map()
const _thumbFailed = new Set()
const _thumbInflight = new Map() // 在途请求：模板会对同一 id 调多次 thumbUrl，必须收敛成一次
const THUMB_TIMEOUT = 30000
export function fetchThumbUrl(id, { force = false } = {}) {
  if (!force && _thumbCache.has(id)) return Promise.resolve(_thumbCache.get(id))
  if (!force && _thumbFailed.has(id)) return Promise.resolve('')
  if (_thumbInflight.has(id)) return _thumbInflight.get(id)
  const task = (async () => {
    try {
      const res = await fetchWithTimeout(`${BASE}/files/${id}/thumb`, { headers: authHeaders() }, THUMB_TIMEOUT)
      if (res.status === 401) handle401(`/files/${id}/thumb`)
      if (!res.ok) {
        _thumbFailed.add(id)
        return ''
      }
      const blob = await res.blob()
      if (!blob || blob.size === 0) {
        _thumbFailed.add(id)
        return ''
      }
      const url = URL.createObjectURL(blob)
      // F5 修复：force 刷新覆盖旧缓存前先 revoke，防旧 objectURL 泄漏
      const old = _thumbCache.get(id)
      if (old) URL.revokeObjectURL(old)
      _thumbCache.set(id, url)
      return url
    } catch (_) {
      _thumbFailed.add(id)
      return ''
    } finally {
      _thumbInflight.delete(id)
    }
  })()
  _thumbInflight.set(id, task)
  return task
}
export function fetchThumbUrlSync(id) {
  return _thumbCache.get(id) || ''
}
// 文件被替换/删除时清缓存：objectURL 必须显式 revoke，否则整个页面会话内泄漏
export function invalidateThumb(id) {
  const url = _thumbCache.get(id)
  if (url) URL.revokeObjectURL(url)
  _thumbCache.delete(id)
  _thumbFailed.delete(id)
  _thumbInflight.delete(id)
}

// 批量媒体元信息（B15）：POST /files/media/batch，只回**已缓存**的探测结果，不触发探测/生成。
// 未命中者在 missing 里，由调用方按需再取 —— 与 B13「懒探测」同一立场（避免一次列表拉上百个 ffprobe）。
export function fetchMediaBatch(ids) {
  return requestJSON('/files/media/batch', { method: 'POST', body: JSON.stringify({ ids }) })
}

// 视频转码（B17）：触发 / 状态 / 取消 —— 与缩略图同口径，走登录态 /files/{id}/transcode*。
export function triggerTranscode(id) {
  return requestJSON(`/files/${id}/transcode`, { method: 'POST' })
}
export function transcodeStatus(id) {
  return requestJSON(`/files/${id}/transcode`, { method: 'GET' })
}
export function cancelTranscode(id) {
  return requestJSON(`/files/${id}/transcode/cancel`, { method: 'POST' })
}

// 原始文件 blob 下载（带鉴权）
export async function fetchFileBlob(id) {
  const res = await request(`/files/${id}/content?download=1`)
  return res.blob()
}

// 文本内容（阅读视图用）
export async function fetchTextContent(id) {
  const res = await request(`/files/${id}/content`)
  return res.text()
}

// ---- 系统 ----

export function health() {
  return requestJSON('/health')
}

export function settings() {
  return requestJSON('/admin/settings')
}

export async function updateSetting(key, value) {
  return requestJSON('/admin/settings', {
    method: 'PUT',
    body: JSON.stringify({ key, value })
  })
}

// 数据备份动态管理（B28，管理员）：读/写 ops.backup_* 配置
export function systemBackupGet() {
  return requestJSON('/system/backup')
}

export function systemBackupPut(patch) {
  return requestJSON('/system/backup', { method: 'PUT', body: JSON.stringify(patch) })
}

// 能力模块生效态与判定来源（设置页运维面板，只读）
// source ∈ config | plugin | plugin-capability | default；restart_required 恒 true（装配在启动阶段）
export function capabilityStates() {
  return requestJSON('/admin/capabilities')
}

// 公开能力开关（只读布尔）：侧栏按应用中心安装状态渲染 family/org 导航入口
export function publicCapabilities() {
  return requestJSON('/public/capabilities')
}

// 派生资产运维（B16）：占用/孤儿统计 + 一键清理。
// 响应含 stats（对象数/字节/孤儿）、options（当前生效参数）、ffmpeg、capability_enabled，
// 供面板自解释「为什么没有缩略图」（能力关了？没装 ffmpeg？参数配错？）。
export function fetchDerivedStats() {
  return requestJSON('/admin/derived/stats')
}

// fileId 留空 = 清全部（用于清孤儿，或改完尺寸参数后强制按新参数重生成）。
export function purgeDerived(fileId = '') {
  return requestJSON('/admin/derived/purge', {
    method: 'POST',
    body: JSON.stringify({ file_id: fileId })
  })
}

// 孤儿派生对象立即清理（B18）：源文件已不存在的派生对象删掉，正常缓存不动。
// 与定时兜底清理（media.derived_sweep_interval_min）同口径。
export function sweepDerived() {
  return requestJSON('/admin/derived/sweep', { method: 'POST' })
}

// ---- AI 对话（Agent + 工具链 + 多会话）----

export async function aiChat(messages, context = {}, conversationId = '') {
  return requestJSON('/ai/chat', {
    method: 'POST',
    body: JSON.stringify({
      messages,
      ...(Object.keys(context).length ? { context } : {}),
      ...(conversationId ? { conversation_id: conversationId } : {})
    })
  })
}

export async function aiConversations() {
  return requestJSON('/ai/conversations')
}

export async function aiCreateConversation(title = '') {
  return requestJSON('/ai/conversations', {
    method: 'POST',
    body: JSON.stringify({ title })
  })
}

export async function aiDeleteConversation(id) {
  return requestJSON(`/ai/conversations/${id}`, { method: 'DELETE' })
}

export async function aiConversationMessages(id) {
  return requestJSON(`/ai/conversations/${id}/messages`)
}

// ---- 分享/发布（博客公开页底层：token 访问）----
export function sharesList() {
  return requestJSON('/shares').then((d) => d.items || [])
}

// 对外博客首页：已发布文章列表（含摘要）
export function publicPosts() {
  return requestJSON('/public/posts')
}

// 博客站点设置（公开读：title/description/logo/footer/seo_default，RSS/OG/公开页同源）
export function publicSite() {
  return requestJSON('/public/site')
}

// 公开标签聚合（标签云与标签内页数据源）：{ items: [{ name, count }] }
// 口径与 public/posts 一致（仅博客目录分享内非草稿文章），未开启博客时为空数组
export function publicTags() {
  return requestJSON('/public/tags')
}

// 已安装博客主题列表（公开读）：{ items: [{ id, title, version }] }
// 来源 = blog_plugins kind=theme 且 enabled=1；应用中心「主题」类别装/卸驱动。
// 前端懒加载基座据此决定哪些主题注册进切换器（未安装不注册不请求 chunk）。
export function publicBlogThemes() {
  return requestJSON('/public/blog/themes')
}

// 已安装文章编辑器列表（公开读）：{ items: [{ id, title, description }] }
// 来源 = 常驻兜底 plain + blog_plugins kind=editor 且 enabled=1。
// 写作轨与阅读页快编轨共用此列表决定「编辑器选择」下拉项（两轨一致）。
export function publicBlogEditors() {
  return requestJSON('/public/blog/editors')
}

// 读者 AI 问答（公开；仅公开文章上下文，进程内限流）
export function publicBlogAsk(question, path) {
  return requestJSON('/public/blog/ask', {
    method: 'POST',
    body: JSON.stringify({ question, path: path || '' }),
  })
}

// 文章 PV 计数（公开）
export function publicBlogPV(pathOrSlug) {
  return requestJSON('/public/blog/pv', {
    method: 'POST',
    body: JSON.stringify({ path: pathOrSlug, slug: pathOrSlug }),
  })
}

// 相关文章语义推荐（向量；无向量/冷启动时后端返回空 items，调用方降级到同分类排序）
export function publicBlogRelated(slug, limit = 5) {
  return requestJSON(`/public/blog/related?slug=${encodeURIComponent(slug)}&limit=${limit}`)
}

// 博客文章语音朗读：返回音频 Blob（合成可能较慢，超时放宽到 60s；命中服务端缓存即时返回）
export async function publicBlogTTSBlob(slug, voice) {
  let url = BASE + `/public/blog/tts?slug=${encodeURIComponent(slug)}`
  if (voice) url += `&voice=${encodeURIComponent(voice)}`
  const ctrl = new AbortController()
  const t = setTimeout(() => ctrl.abort(), 60000)
  try {
    const res = await fetch(url, { method: 'GET', credentials: 'same-origin', signal: ctrl.signal })
    if (!res.ok) {
      let msg = `HTTP ${res.status}`
      try {
        const b = await res.json()
        if (b?.error?.message) msg = b.error.message
        else if (b?.error?.code) msg = b.error.code
      } catch (_) { /* 非 JSON 响应 */ }
      throw new Error(msg)
    }
    const truncated = res.headers.get('X-TTS-Truncated') === '1'
    const blob = await res.blob()
    return { blob, truncated }
  } finally {
    clearTimeout(t)
  }
}

export function publicBlogPVGet(key) {
  return requestJSON(`/public/blog/pv?key=${encodeURIComponent(key)}`)
}

// 博客文章批量编辑（管理端；A-G 批量编辑）：items=[{id, node_type?, fields?, cover?, excerpt?, seo_title?, seo_desc?}]
export function blogPostsBatch(items) {
  return requestJSON('/blog/posts/batch', {
    method: 'POST',
    body: JSON.stringify({ items }),
  })
}

// 评论公开读（按 token+path 定位文章；公开面不暴露内部 file_id）
export function publicComments(token, path) {
  const q = path ? `?token=${encodeURIComponent(token)}&path=${encodeURIComponent(path)}` : `?token=${encodeURIComponent(token)}`
  return requestJSON(`/public/comments${q}`)
}

// 评论写入（需登录；body ≤2000 字符）
export function createComment(token, path, body, parentId = '') {
  return requestJSON('/comments', {
    method: 'POST',
    body: JSON.stringify({ token, path, body, parent_id: parentId }),
  })
}

// 博客功能插件：已启用插件列表（公开，供前端按挂载点渲染）
export function publicPlugins() {
  return requestJSON('/blog/plugins')
}

// 插件 KV 全部键（公开读；调试页抽样用）
export function pluginKVList(pluginId) {
  return requestJSON(`/blog/plugins/${pluginId}/kv`)
}

// 应用市场：zip 安装（登录）
export async function installAppZip(file) {
  const fd = new FormData()
  fd.append('file', file)
  const res = await fetch(BASE + '/admin/apps/install-zip', {
    method: 'POST',
    headers: authHeaders(),
    body: fd,
  })
  if (res.status === 401) {
    handle401('/admin/apps/install-zip')
    throw new Error('需要登录')
  }
  const data = await res.json().catch(() => ({}))
  if (!res.ok) throw new Error(data.message || data.error || '安装失败')
  return data
}

// 应用/插件：启用禁用
export function pluginToggle(id, enable) {
  return requestJSON(`/blog/plugins/${id}/toggle?enable=${enable ? '1' : '0'}`, { method: 'POST', body: JSON.stringify({}) })
}

export function pluginDelete(id) {
  return requestJSON(`/blog/plugins/${id}`, { method: 'DELETE' })
}

// 应用中心：在线目录列表（登录）→ { plugins:[], themes:[], index_url, shell, edition }
export function appsMarketList() {
  return requestJSON('/admin/apps/market')
}

// 应用中心：一键安装（按市场 id，或直接 url+sha256）
export async function appsMarketInstall({ id, url, sha256 } = {}) {
  return requestJSON('/admin/apps/market/install', {
    method: 'POST',
    body: JSON.stringify({ id: id || '', url: url || '', sha256: sha256 || '' }),
  })
}

// 插件动态设置 schema（公开读）→ { id, settings_schema:[...] }
export function pluginSettingsSchema(id) {
  return requestJSON(`/blog/plugins/${id}/settings-schema`)
}

// 插件 KV 写入（登录；upsert）→ { ok:true, key }
export function pluginKVSet(id, key, value) {
  return requestJSON(`/blog/plugins/${id}/kv`, {
    method: 'POST',
    body: JSON.stringify({ key, value: value == null ? '' : String(value) }),
  })
}

export function createShare(fileId, expiresAt = 0) {
  return requestJSON('/shares', { method: 'POST', body: JSON.stringify({ file_id: fileId, expires_at: expiresAt }) })
}

// 文件夹整体分享（强授权：confirm 必须显式 true，服务端拒绝未确认的目录分享）
export function createDirShare(dirId, confirm = false, expiresAt = 0) {
  return requestJSON('/shares', { method: 'POST', body: JSON.stringify({ dir_id: dirId, confirm, expires_at: expiresAt }) })
}

// 目录分享公开内容（path=文件相对目录路径）
export async function fetchDirShareContent(token, path) {
  const res = await request(`/shares/${token}/content?path=${encodeURIComponent(path)}`)
  return res.text()
}

export function dirShareContentUrl(token, path, download = false) {
  const q = download ? 'download=1' : ''
  return `${BASE}/shares/${token}/content?path=${encodeURIComponent(path)}${q ? '&' + q : ''}`
}

// 公开分享缩略图地址（B19）：分享通道专属公开缩略图端点，按 token 鉴权、不强制博客子树。
// dir 分享需 ?path= 指定目录内文件相对路径；file 分享忽略 path。
export function shareThumbUrl(token, path = '') {
  const q = path ? '?path=' + encodeURIComponent(path) : ''
  return `${BASE}/shares/${token}/thumb${q}`
}

export function getShare(token) {
  return requestJSON(`/shares/${token}`)
}

export async function fetchShareContent(token) {
  const res = await request(`/shares/${token}/content`)
  return res.text()
}

export function shareContentUrl(token, download = false) {
  const q = download ? '?download=1' : ''
  return `${BASE}/shares/${token}/content${q}`
}

export function revokeShare(token) {
  return request(`/shares/${token}`, { method: 'DELETE' }).then(() => true)
}

// ---- 公开分享正文：裸 fetch（绕过 request 的 401 跳登录）----
// 公开页密码/付费闸门需捕获 401 PASSWORD_REQUIRED，不能使用统一的 request（会跳登录）。
// 返回 { ok, status, text }，调用方据此决定是否进入解锁流程。
export async function fetchShareContentRaw(token, opts = {}) {
  const { unlock = '', path = '' } = opts
  const q = []
  if (path) q.push('path=' + encodeURIComponent(path))
  if (unlock) q.push('unlock=' + encodeURIComponent(unlock))
  const url = `${BASE}/shares/${token}/content${q.length ? '?' + q.join('&') : ''}`
  const res = await fetch(url, { credentials: 'same-origin' })
  return { ok: res.ok, status: res.status, text: await res.text() }
}

// 公开分享目录内文件正文（同上，支持 ?path= 与解锁令牌）
export async function fetchDirShareContentRaw(token, path, unlock = '') {
  const q = []
  if (path) q.push('path=' + encodeURIComponent(path))
  if (unlock) q.push('unlock=' + encodeURIComponent(unlock))
  const url = `${BASE}/shares/${token}/content${q.length ? '?' + q.join('&') : ''}`
  const res = await fetch(url, { credentials: 'same-origin' })
  return { ok: res.ok, status: res.status, text: await res.text() }
}

// 公开文章解锁（密码保护 2.2）：校验通过后签发 24h 会话级解锁凭证
export async function publicUnlock(fileID, password) {
  return requestJSON('/public/unlock', {
    method: 'POST',
    body: JSON.stringify({ file_id: fileID, password })
  })
}

// 公开链接存活体检（生产站底座 E 的本地实现）：服务端 HEAD 探测
export async function publicLinkCheck(url) {
  return requestJSON('/public/link-check?url=' + encodeURIComponent(url))
}

// ---- 模型提供方管理（能力级热切换）----

export async function aiProviders() {
  return requestJSON('/admin/ai/providers')
}

export async function aiProviderSwitch(cap, provider, model = '') {
  return requestJSON('/admin/ai/providers/switch', {
    method: 'POST',
    body: JSON.stringify({ cap, provider, model })
  })
}

// 自备模型（custom）：endpoint/model/api_key
export async function aiCustomSave(cap, endpoint, model, apiKey) {
  return requestJSON('/admin/ai/providers/custom', {
    method: 'POST',
    body: JSON.stringify({ cap, endpoint, model, api_key: apiKey })
  })
}

export async function aiCustomClear(cap) {
  return requestJSON('/admin/ai/providers/custom', {
    method: 'DELETE',
    body: JSON.stringify({ cap })
  })
}

// 模型提供方管理：全量注册表（增删改 + token 池）
export async function aiProvidersManage() {
  return requestJSON('/admin/ai/providers/all')
}

export async function aiProviderUpsert(payload) {
  return requestJSON('/admin/ai/providers', {
    method: 'PUT',
    body: JSON.stringify(payload)
  })
}

export async function aiProviderDelete(name) {
  return requestJSON('/admin/ai/providers', {
    method: 'DELETE',
    body: JSON.stringify({ name })
  })
}

export async function aiProviderTest(name, cap = 'llm') {
  return requestJSON('/admin/ai/providers/test', {
    method: 'POST',
    body: JSON.stringify({ name, cap })
  })
}

// ---- 平台模型用量管控（配额 + 增值 token 计费，ai_billing.go）----

// 当前策略 + 今日按主体用量 + 余额列表
export async function aiQuotaPolicy() {
  return requestJSON('/admin/ai/quota-policy')
}

// 保存策略（0=不限；billing_enabled 开启后平台模型按 token 扣余额）
export async function aiQuotaPolicySave(payload) {
  return requestJSON('/admin/ai/quota-policy', {
    method: 'PUT',
    body: JSON.stringify(payload)
  })
}

// 某主体余额 + 流水（subject: "user:<uid>" | "public"）
export async function aiBalance(subject, ledgerLimit = 30) {
  return requestJSON(`/admin/ai/balance?subject=${encodeURIComponent(subject)}&ledger_limit=${ledgerLimit}`)
}

// 充值/扣减（delta 正=充值 负=扣减）
export async function aiBalanceGrant(subject, delta, reason = '') {
  return requestJSON('/admin/ai/balance', {
    method: 'POST',
    body: JSON.stringify({ subject, delta, reason })
  })
}


// 语音合成试听（设置页）：POST 二进制音频，返回 { blob, model }
export async function aiTTSListen({ text, style, voice, format } = {}) {
  const res = await fetch(`${BASE}/admin/ai/tts`, {
    method: 'POST',
    headers: authHeaders({ 'Content-Type': 'application/json' }),
    credentials: 'include',
    body: JSON.stringify({ text, style, voice, format: format || 'mp3' })
  })
  if (!res.ok) {
    let msg = `HTTP ${res.status}`
    try {
      const j = await res.json()
      msg = j.error || j.message || msg
    } catch (_) { /* 非 JSON 错误体 */ }
    throw new Error(msg)
  }
  const blob = await res.blob()
  return { blob, model: res.headers.get('X-Voice-Model') || '' }
}

// ---- IM 网关（Telegram 等平台对话接入）----

export async function imStatus() {
  return requestJSON('/im/status')
}

export async function imTelegramConfig({ botToken = '', allowedChats = '' } = {}) {
  return requestJSON('/im/telegram/config', {
    method: 'PUT',
    body: JSON.stringify({ bot_token: botToken, allowed_chats: allowedChats })
  })
}

export async function imWeComConfig({ webhookUrl = '', corpId = '', agentId = '', secret = '', token = '', encodingAesKey = '' } = {}) {
  return requestJSON('/im/wecom/config', {
    method: 'PUT',
    body: JSON.stringify({
      webhook_url: webhookUrl,
      corp_id: corpId,
      agent_id: agentId,
      secret,
      token,
      encoding_aes_key: encodingAesKey
    })
  })
}

// ---- 导入导出中心（R11/R12：异步任务 + 进度 + 失败清单可重试）----

const impexBase = (kind) => (kind === 'export' ? '/exports' : '/imports')

export function impexList(kind) {
  return requestJSON(impexBase(kind)).then((d) => d.items || [])
}

export function impexJob(kind, id) {
  return requestJSON(`${impexBase(kind)}/${id}`)
}

export function impexRetry(kind, id) {
  return requestJSON(`${impexBase(kind)}/${id}/retry`, { method: 'POST' })
}

// ZIP / Obsidian vault 导入（multipart，带上传进度）
// onProgress: (percent) => void
export function impexImportZip(file, { parent = '', source = 'zip', onConflict = 'rename', onProgress } = {}) {
  const fd = new FormData()
  fd.append('source', source)
  fd.append('parent', parent)
  fd.append('on_conflict', onConflict)
  fd.append('file', file)
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest()
    xhr.open('POST', `${BASE}/imports`)
    const tok = getToken()
    if (tok) xhr.setRequestHeader('Authorization', `Bearer ${tok}`)
    if (onProgress) {
      xhr.upload.onprogress = (e) => {
        if (e.lengthComputable) onProgress(Math.round((e.loaded / e.total) * 100))
      }
    }
    xhr.onload = () => {
      if (xhr.status >= 200 && xhr.status < 300) {
        try {
          resolve(JSON.parse(xhr.responseText))
        } catch (_) {
          reject(new Error('导入响应解析失败'))
        }
      } else {
        let msg = `HTTP ${xhr.status}`
        try {
          const body = JSON.parse(xhr.responseText)
          if (body?.error?.message) msg = body.error.message
        } catch (_) {
          /* 非 JSON */
        }
        reject(new Error(msg))
      }
    }
    xhr.onerror = () => reject(new Error('网络错误，导入失败'))
    xhr.send(fd)
  })
}

// URL 网页抓取导入
export function impexImportURL(url, { parent = '', ua = '' } = {}) {
  return requestJSON('/imports', { method: 'POST', body: JSON.stringify({ source: 'url', url, parent, ua }) })
}

// 剪贴板粘贴导入（text 文本 / image base64）
export function impexImportClipboard({ parent = '', name = '', text = '', image = '' } = {}) {
  return requestJSON('/imports', {
    method: 'POST',
    body: JSON.stringify({ source: 'clipboard', parent, name, text, image })
  })
}

// 创建导出（source: folder|obsidian|tag|collection）
export function impexCreateExport(source, targetId, name = '') {
  return requestJSON('/exports', {
    method: 'POST',
    body: JSON.stringify({ source, target_id: targetId, name })
  })
}

// 导出产物下载 URL
export function impexDownloadUrl(id) {
  return `${BASE}/exports/${id}/download`
}

// 导出产物下载（带鉴权：fetch blob 保存）
export async function impexDownload(id) {
  const res = await request(`/exports/${id}/download`, { method: 'GET' })
  const blob = await res.blob()
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `aiklog-export-${id}.zip`
  document.body.appendChild(a)
  a.click()
  a.remove()
  setTimeout(() => URL.revokeObjectURL(url), 30000)
}

// 溯源：取单个内容块（阅读页定位高亮用）
export function getChunk(id) {
  return requestJSON(`/chunks/${id}`)
}

// 双链/反向链接：本文引用了谁（outgoing）、谁引用了本文（incoming）
export function getFileLinks(id) {
  return requestJSON(`/files/${id}/links`)
}

// 段落级双链：在目标文件中定位含锚点文本的 chunk（阅读页锚点跳转高亮用）
export function getLocateChunk(id, q) {
  return requestJSON(`/files/${id}/locate?q=${encodeURIComponent(q)}`)
}

// ---- 消息通知中心（R-notify：采集/导入导出终态 → 铃铛未读红点 + 面板） ----
export function listNotifications(limit = 50, unreadOnly = false) {
  return requestJSON(`/notifications?limit=${limit}${unreadOnly ? '&unread=1' : ''}`)
}
export function notificationsUnread() {
  return requestJSON('/notifications/unread-count')
}
export function notificationsRead(id) {
  return requestJSON('/notifications/read', { method: 'POST', body: JSON.stringify({ id }) })
}
export function notificationsReadAll() {
  return requestJSON('/notifications/read', { method: 'POST', body: JSON.stringify({ all: true }) })
}

// ---- AI 用量（配额分层规范 v1.0：站长侧用量看板） ----
export function aiUsage(days = 7) {
  return requestJSON(`/admin/ai/usage?days=${days}`)
}


// ---- 多站点（SPEC-MS-001）：站点管理后台（owner/admin） ----
// 站点列表 + 站点数上限汇总 + 授权列表 → { site_cap:{base,permanent,subscription,total,used}, licenses:[], sites:[] }
export function adminSitesList() {
  return requestJSON('/admin/sites')
}
// 新建站点（受 cap 闸门；超额后端返回 402 SITE_LIMIT_REACHED，前端提示购买）
export function adminSiteCreate(payload) {
  return requestJSON('/admin/sites', { method: 'POST', body: JSON.stringify(payload || {}) })
}
export function adminSiteGet(id) {
  return requestJSON(`/admin/sites/${encodeURIComponent(id)}`)
}
export function adminSiteUpdate(id, payload) {
  return requestJSON(`/admin/sites/${encodeURIComponent(id)}`, { method: 'PUT', body: JSON.stringify(payload || {}) })
}
export function adminSiteDelete(id) {
  return requestJSON(`/admin/sites/${encodeURIComponent(id)}`, { method: 'DELETE' })
}
// 站点设置（per-site：默认主题 / 访客切换 / SEO / 标题等）
export function adminSiteSettingsGet(id) {
  return requestJSON(`/admin/sites/${encodeURIComponent(id)}/settings`)
}
export function adminSiteSettingsUpdate(id, payload) {
  return requestJSON(`/admin/sites/${encodeURIComponent(id)}/settings`, { method: 'PUT', body: JSON.stringify(payload || {}) })
}
// 站点数授权（license：feature:multisite + seats）
export function adminSiteLicenses() {
  return requestJSON('/admin/sites/licenses')
}
export function adminSiteLicenseGrant(licenseKey) {
  return requestJSON('/admin/sites/licenses', { method: 'POST', body: JSON.stringify({ license_key: licenseKey }) })
}

// ---- 收件箱（采集聚合：未读 / 已归档；inbox_state 0=未读 1=已归档） ----
export function inboxList(filter = 'all', limit = 50) {
  return requestJSON(`/inbox?filter=${encodeURIComponent(filter)}&limit=${limit}`)
}
export function inboxUnread() {
  return requestJSON('/inbox/unread-count')
}
export function inboxArchive(id) {
  return requestJSON(`/inbox/${encodeURIComponent(id)}/archive`, { method: 'POST', body: JSON.stringify({}) })
}
export function inboxArchiveAll() {
  return requestJSON('/inbox/archive-all', { method: 'POST', body: JSON.stringify({}) })
}

// ---- 每日知识日报（预览不推送 / 立即推送） ----
export function digestPreview() {
  return requestJSON('/digest/preview')
}
export function digestRun() {
  return requestJSON('/digest/run', { method: 'POST', body: JSON.stringify({}) })
}

// ---- 外部 WebDAV 挂载（浏览远端 / 导入知识库） ----
export function webdavMounts() {
  return requestJSON('/webdav/mounts')
}
export function webdavMountCreate(payload) {
  return requestJSON('/webdav/mounts', { method: 'POST', body: JSON.stringify(payload || {}) })
}
export function webdavMountUpdate(id, payload) {
  return requestJSON(`/webdav/mounts/${encodeURIComponent(id)}`, { method: 'PUT', body: JSON.stringify(payload || {}) })
}
export function webdavMountDelete(id) {
  return requestJSON(`/webdav/mounts/${encodeURIComponent(id)}`, { method: 'DELETE' })
}
export function webdavMountTest(id) {
  return requestJSON(`/webdav/mounts/${encodeURIComponent(id)}/test`, { method: 'POST', body: JSON.stringify({}) })
}
export function webdavMountList(id, path = '') {
  return requestJSON(`/webdav/mounts/${encodeURIComponent(id)}/list?path=${encodeURIComponent(path)}`)
}
export function webdavMountImport(id, path, parentId = '') {
  return requestJSON(`/webdav/mounts/${encodeURIComponent(id)}/import`, {
    method: 'POST',
    body: JSON.stringify({ path, parent_id: parentId })
  })
}

// ---- 组织架构（组织树 / 岗位 / 部门空间 / 移交流） ----
export function orgSettings() {
  return requestJSON('/org/settings')
}
export function orgTreeList() {
  return requestJSON('/org/tree')
}
export function orgTreeCreate(name, parentId = '') {
  return requestJSON('/org/tree', { method: 'POST', body: JSON.stringify({ name, parent_id: parentId }) })
}
export function orgTreeUpdate(id, payload) {
  return requestJSON(`/org/tree/${encodeURIComponent(id)}`, { method: 'PUT', body: JSON.stringify(payload || {}) })
}
export function orgTreeDelete(id) {
  return requestJSON(`/org/tree/${encodeURIComponent(id)}`, { method: 'DELETE' })
}
export function orgTreePaths() {
  return requestJSON('/org/tree/paths')
}
export function orgMembershipsSet(payload) {
  return requestJSON('/org/memberships', { method: 'POST', body: JSON.stringify(payload || {}) })
}
export function orgNodeMembers(nodeId) {
  return requestJSON(`/org/members?node_id=${encodeURIComponent(nodeId)}`)
}
export function orgMembershipsHistory(userId = '') {
  const q = userId ? `?user_id=${encodeURIComponent(userId)}` : ''
  return requestJSON(`/org/memberships${q}`)
}
export function orgDepartments() {
  return requestJSON('/org/departments')
}
export function orgDepartmentCreate(nodeId, name) {
  return requestJSON('/org/departments', { method: 'POST', body: JSON.stringify({ node_id: nodeId, name }) })
}
export function orgDepartmentMembers(id, payload) {
  return requestJSON(`/org/departments/${encodeURIComponent(id)}/members`, {
    method: 'POST',
    body: JSON.stringify(payload || {})
  })
}
export function orgTransfers(scope = 'out') {
  return requestJSON(`/org/transfers?scope=${encodeURIComponent(scope)}`)
}
export function orgTransferCreate(payload) {
  return requestJSON('/org/transfers', { method: 'POST', body: JSON.stringify(payload || {}) })
}
export function orgTransferItems(id) {
  return requestJSON(`/org/transfers/${encodeURIComponent(id)}/items`)
}
export function orgTransferPreview(id) {
  return requestJSON(`/org/transfers/${encodeURIComponent(id)}/preview`, { method: 'POST', body: JSON.stringify({}) })
}
export function orgTransferItemSkip(id, itemId, skip) {
  return requestJSON(`/org/transfers/${encodeURIComponent(id)}/items/${encodeURIComponent(itemId)}`, {
    method: 'POST',
    body: JSON.stringify({ skip: !!skip })
  })
}
export function orgTransferExecute(id) {
  return requestJSON(`/org/transfers/${encodeURIComponent(id)}/execute`, { method: 'POST', body: JSON.stringify({}) })
}
export function orgTransferAccept(id) {
  return requestJSON(`/org/transfers/${encodeURIComponent(id)}/accept`, { method: 'PUT', body: JSON.stringify({}) })
}
export function orgTransferCancel(id) {
  return requestJSON(`/org/transfers/${encodeURIComponent(id)}/cancel`, { method: 'POST', body: JSON.stringify({}) })
}
export function orgCustodianCount() {
  return requestJSON('/org/custodian-count')
}

// ---- 用户管理（owner/admin：列表 / 改角色与状态） ----
export function adminUsersList() {
  return requestJSON('/admin/users')
}
export function adminUserUpdate(id, payload) {
  return requestJSON(`/admin/users/${encodeURIComponent(id)}`, { method: 'PUT', body: JSON.stringify(payload || {}) })
}

// ---- 阅读数据中心（A5：站级概览 + 单篇热力 + 热门榜） ----
export function adminAnalyticsOverview() {
  return requestJSON('/admin/analytics/overview')
}
export function fileHeatmap(id) {
  return requestJSON(`/files/${encodeURIComponent(id)}/heatmap`)
}
export function publicPopular(limit = 10) {
  return requestJSON(`/public/popular?limit=${limit}`)
}

// ---- B5 写作增强：文章版本历史（列表 / 下载 / 恢复） ----

export function listFileVersions(id) {
  return requestJSON(`/files/${encodeURIComponent(id)}/versions`)
}

// 历史版本正文（带鉴权 fetch → blob；<a href> 不会带 Authorization，故必须走 fetch）
export async function fetchVersionBlob(id, version) {
  const res = await request(`/files/${encodeURIComponent(id)}/versions/${version}/content`)
  return res.blob()
}

// 下载历史版本为本地文件
export async function downloadFileVersion(id, version, name = '') {
  const blob = await fetchVersionBlob(id, version)
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `${name || 'version'}-v${version}`
  document.body.appendChild(a)
  a.click()
  a.remove()
  setTimeout(() => URL.revokeObjectURL(url), 30000)
}

// 恢复历史版本（产生新版本，不回退版本号；恢复前的内容也会被快照，故可再恢复回去）
export function restoreFileVersion(id, version) {
  return requestJSON(`/files/${encodeURIComponent(id)}/versions/${version}/restore`, {
    method: 'POST',
    body: '{}'
  })
}

// ---- B5 写作增强：AI 写作提示词模板（提示词参数化，后台可配置） ----

export function listPrompts(category = '') {
  const q = category ? `?category=${encodeURIComponent(category)}` : ''
  return requestJSON('/prompts' + q)
}

export function getPrompt(id) {
  return requestJSON(`/prompts/${encodeURIComponent(id)}`)
}

export function createPrompt(payload) {
  return requestJSON('/prompts', { method: 'POST', body: JSON.stringify(payload || {}) })
}

export function updatePrompt(id, payload) {
  return requestJSON(`/prompts/${encodeURIComponent(id)}`, { method: 'PUT', body: JSON.stringify(payload || {}) })
}

export function deletePrompt(id) {
  return requestJSON(`/prompts/${encodeURIComponent(id)}`, { method: 'DELETE' })
}

// 渲染模板：把 {{变量}} 替换为 values（缺省回落模板默认值）
export function renderPrompt(id, values = {}) {
  return requestJSON(`/prompts/${encodeURIComponent(id)}/render`, {
    method: 'POST',
    body: JSON.stringify({ values })
  })
}

// ---- 内容订阅（B6 订阅与通知）：订阅文件或目录，内容更新时收到通知 ----
export function listSubscriptions() {
  return requestJSON('/subscriptions')
}

export function subscriptionStatus(targetType, targetId) {
  const q = `target_type=${encodeURIComponent(targetType)}&target_id=${encodeURIComponent(targetId)}`
  return requestJSON(`/subscriptions/status?${q}`)
}

export function subscribeTarget(targetType, targetId) {
  return requestJSON('/subscriptions', {
    method: 'POST',
    body: JSON.stringify({ target_type: targetType, target_id: targetId })
  })
}

export function unsubscribeTarget(targetType, targetId) {
  return requestJSON(`/subscriptions/${encodeURIComponent(targetType)}/${encodeURIComponent(targetId)}`, {
    method: 'DELETE'
  })
}

// ---- @提及（B6）：被提及记录与未读数 ----
export function listMentions(limit = 50) {
  return requestJSON(`/mentions?limit=${limit}`)
}

export function mentionsUnread() {
  return requestJSON('/mentions/unread-count')
}

export function mentionsReadAll() {
  return requestJSON('/mentions/read', { method: 'POST', body: JSON.stringify({ all: true }) })
}

// ---- IM 绑定（B7）：把 IM 账号绑到站内身份（Web 生成码 → IM 端 /bind <码> 消费）----
export function listImBindings() {
  return requestJSON('/im/bindings')
}

export function createImBindCode(platform = '') {
  return requestJSON('/im/bindings/code', {
    method: 'POST',
    body: JSON.stringify({ platform })
  })
}

export function deleteImBinding(id) {
  return requestJSON(`/im/bindings/${encodeURIComponent(id)}`, { method: 'DELETE' })
}

// ---- 技能包（B9）：上架目录 / 我的授权 / 自助领取免费包 ----
export function listSkills(kind = '', limit = 100) {
  const q = new URLSearchParams()
  if (kind) q.set('kind', kind)
  q.set('limit', String(limit))
  return requestJSON(`/skills?${q.toString()}`)
}

export function listSkillGrants() {
  return requestJSON('/skills/grants')
}

export function claimSkill(id) {
  return requestJSON(`/skills/${encodeURIComponent(id)}/claim`, {
    method: 'POST',
    body: JSON.stringify({})
  })
}

// ---- 家族传承记录模块（传家365 同族：一生时间轴/家族树/纪念日） ----
export function familySettings() {
  return requestJSON('/family/settings')
}
export function familyMembers() {
  return requestJSON('/family/members').then((d) => d.members || [])
}
export function familyMemberCreate(payload) {
  return requestJSON('/family/members', { method: 'POST', body: JSON.stringify(payload) })
}
export function familyMemberUpdate(id, payload) {
  return requestJSON(`/family/members/${id}`, { method: 'PUT', body: JSON.stringify(payload) })
}
export function familyMemberDelete(id) {
  return requestJSON(`/family/members/${id}`, { method: 'DELETE' })
}
export function familyRelations() {
  return requestJSON('/family/relations').then((d) => d.relations || [])
}
export function familyRelationCreate(payload) {
  return requestJSON('/family/relations', { method: 'POST', body: JSON.stringify(payload) })
}
export function familyRelationDelete(id) {
  return requestJSON(`/family/relations/${id}`, { method: 'DELETE' })
}
export function familyTree() {
  return requestJSON('/family/tree')
}
export function familyTimeline() {
  return requestJSON('/family/timeline').then((d) => d.items || [])
}
export function familyAnniversaries() {
  return requestJSON('/family/anniversaries').then((d) => d.anniversaries || [])
}
export function familyAnniversaryCreate(payload) {
  return requestJSON('/family/anniversaries', { method: 'POST', body: JSON.stringify(payload) })
}
export function familyAnniversaryUpdate(id, payload) {
  return requestJSON(`/family/anniversaries/${id}`, { method: 'PUT', body: JSON.stringify(payload) })
}
export function familyAnniversaryDelete(id) {
  return requestJSON(`/family/anniversaries/${id}`, { method: 'DELETE' })
}

export function familyAdvise(question) {
  return requestJSON('/family/advise', { method: 'POST', body: JSON.stringify({ question }) })
}

// ---- B39 商城（WooCommerce 式） ----
// 商品封面 cover 既可是外链 URL，也可是站内 file id（DDL 约定）。
// 一律当 URL 用会让 file id 形式的封面静默裂图，故统一在此归一。
export function storeCoverUrl(cover) {
  const c = (cover || '').trim()
  if (!c) return ''
  if (/^(https?:)?\/\//i.test(c) || c.startsWith('data:') || c.startsWith('/')) return c
  return `${BASE}/store/media/${encodeURIComponent(c)}`
}
// 公开：商品目录 / 详情
export function storeProducts(q = '') {
  const s = q ? `?q=${encodeURIComponent(q)}` : ''
  return requestJSON(`/store/products${s}`).then((d) => d.items || [])
}
export function storeProductDetail(slug) {
  return requestJSON(`/store/products/${encodeURIComponent(slug)}`)
}
// 公开：购物车（匿名 cookie）
export function storeCartGet() {
  return requestJSON('/store/cart')
}
export function storeCartAdd(productId, qty = 1) {
  return requestJSON('/store/cart/add', { method: 'POST', body: JSON.stringify({ product_id: productId, qty }) })
}
export function storeCartSet(productId, qty) {
  return requestJSON('/store/cart/set', { method: 'POST', body: JSON.stringify({ product_id: productId, qty }) })
}
export function storeCartRemove(productId) {
  return requestJSON('/store/cart/remove', { method: 'POST', body: JSON.stringify({ product_id: productId }) })
}
export function storeCartClear() {
  return requestJSON('/store/cart/clear', { method: 'POST', body: '{}' })
}
// 公开：可用支付通道 + 收银台下单 + 订单查询 + 演示直 confirm
export function storeGateways() {
  return requestJSON('/store/gateways')
}
export function storeCheckout(payload) {
  return requestJSON('/store/checkout', { method: 'POST', body: JSON.stringify(payload) })
}
export function storeOrderLookup(email, orderNo) {
  return requestJSON(`/store/orders/lookup?email=${encodeURIComponent(email)}&order_no=${encodeURIComponent(orderNo)}`)
}
export function storeOrderDetail(no, email = '') {
  // 订单详情含 PII，后端要求「订单号 + 下单邮箱」双因子匹配
  return requestJSON(`/store/order/${encodeURIComponent(no)}?email=${encodeURIComponent(email)}`)
}

// 数字商品交付下载：返回可直接放进 <a href> 的 URL。
// 优先用条目级 download_token（B43，高熵凭据）；无 token 时回退 email 双因子（受次数上限约束）。
export function storeOrderDownloadUrl(no, itemId, email = '', token = '') {
  const q = token ? `token=${encodeURIComponent(token)}` : `email=${encodeURIComponent(email)}`
  return `${BASE}/store/order/${encodeURIComponent(no)}/download/${encodeURIComponent(itemId)}?${q}`
}
export function storeMockPay(no) {
  return requestJSON(`/store/mock-pay/${encodeURIComponent(no)}`, { method: 'POST', body: '{}' })
}
// 管理（isAdmin）：商品 CRUD
export function adminStoreProducts() {
  return requestJSON('/admin/store/products').then((d) => d.items || [])
}
export function adminStoreProductCreate(p) {
  return requestJSON('/admin/store/products', { method: 'POST', body: JSON.stringify(p) })
}
export function adminStoreProductUpdate(id, patch) {
  return requestJSON(`/admin/store/products/${id}`, { method: 'PUT', body: JSON.stringify(patch) })
}
export function adminStoreProductDelete(id) {
  return requestJSON(`/admin/store/products/${id}`, { method: 'DELETE' })
}
// 管理：订单（列表 / 详情 / 发货 / 退款）
export function adminStoreOrders(status = '') {
  const s = status ? `?status=${encodeURIComponent(status)}` : ''
  return requestJSON(`/admin/store/orders${s}`).then((d) => d.items || [])
}
export function adminStoreOrderDetail(no) {
  return requestJSON(`/admin/store/orders/${encodeURIComponent(no)}`)
}
export function adminStoreOrderFulfill(no) {
  return requestJSON(`/admin/store/orders/${encodeURIComponent(no)}/fulfill`, { method: 'POST', body: '{}' })
}
export function adminStoreOrderRefund(no, amountCents = 0, reason = '') {
  // amount_cents<=0 = 全额退完剩余可退余额；网关退款失败时后端明确回绝且**不改订单状态**
  return requestJSON(`/admin/store/orders/${encodeURIComponent(no)}/refund`, {
    method: 'POST',
    body: JSON.stringify({ amount_cents: amountCents, reason })
  })
}
export function adminStoreOrderRefunds(no) {
  return requestJSON(`/admin/store/orders/${encodeURIComponent(no)}/refunds`).then((d) => d.items || [])
}
// 买家发起退货申请（订单号 + 下单邮箱双因子，与订单查询同口径）
export function storeReturnCreate(orderNo, email, reason, amountCents = 0) {
  return requestJSON('/store/orders/return', {
    method: 'POST',
    body: JSON.stringify({ order_no: orderNo, email, reason, amount_cents: amountCents })
  })
}
export function adminStoreReturns(status = '') {
  const s = status ? `?status=${encodeURIComponent(status)}` : ''
  return requestJSON(`/admin/store/returns${s}`).then((d) => d.items || [])
}
export function adminStoreReturnDecide(id, approve, note = '') {
  return requestJSON(`/admin/store/returns/${encodeURIComponent(id)}/decide`, {
    method: 'POST',
    body: JSON.stringify({ approve, note })
  })
}
// 管理：优惠券 CRUD
export function adminStoreCoupons() {
  return requestJSON('/admin/store/coupons').then((d) => d.items || [])
}
export function adminStoreCouponCreate(c) {
  return requestJSON('/admin/store/coupons', { method: 'POST', body: JSON.stringify(c) })
}
export function adminStoreCouponUpdate(id, patch) {
  return requestJSON(`/admin/store/coupons/${id}`, { method: 'PUT', body: JSON.stringify(patch) })
}
export function adminStoreCouponDelete(id) {
  return requestJSON(`/admin/store/coupons/${id}`, { method: 'DELETE' })
}
// 管理：网关配置洞察
export function adminStoreConfig() {
  return requestJSON('/admin/store/config')
}

// ---- B47 应用中心回源统计（官方侧看板） ----
// 装机数 = 去重安装数（匿名 install_id 行数），非回源请求数。
// 内网/离线安装不回源，故此数为**下界**。数据不含任何用户信息。
export function adminMarketBeacon() {
  return requestJSON('/admin/market/beacon')
}
