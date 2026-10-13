// AiKlog（爱库录）API 客户端：对接后端 /api/v1 文件 API
// 阶段 1 覆盖：文件列表/树/上传/下载/目录/移动/删除/预览内容

const BASE = '/api/v1'

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
// 鉴权判定：旧 token（localStorage，兼容旧会话/API 客户端）或登录标记（新会话 HttpOnly cookie 模式）
export function isAuthed() {
  return !!getToken() || sessionStorage.getItem(AUTH_FLAG) === '1'
}
function authHeaders(headers = {}) {
  const tok = getToken()
  return tok ? { ...headers, Authorization: `Bearer ${tok}` } : headers
}
// 401 统一处理：清 token 回控制台（登录接口自身除外，避免循环）
function handle401(url) {
  // 免登录/登录页自身接口不触发跳转（避免循环）：登录、注册、邮箱验证码、能力探测
  if (url.includes('/auth/login') || url.includes('/auth/register') || url.includes('/auth/email-code') || url.includes('/auth/config')) return
  clearToken()
  clearAuthFlag()
  if (!location.hash.startsWith('#/desk') && !location.hash.startsWith('#/login')) {
    const cur = location.hash.slice(1) || '/files'
    location.hash = `#/desk?next=${encodeURIComponent(cur)}`
  }
}
export function authLogin(username, password) {
  return requestJSON('/auth/login', { method: 'POST', body: JSON.stringify({ username, password }) })
}
export function authLogout() {
  return requestJSON('/auth/logout', { method: 'POST', body: JSON.stringify({}) })
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
  return requestJSON('/auth/register', {
    method: 'POST',
    body: JSON.stringify({ username, password, display_name: displayName || '', email: email || '', code: code || '' })
  })
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
  const url = URL.createObjectURL(blob)
  _mediaCache.set(id, url)
  return url
}
export function fetchMediaUrlSync(id) {
  return _mediaCache.get(id) || ''
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

export function publicBlogPVGet(key) {
  return requestJSON(`/public/blog/pv?key=${encodeURIComponent(key)}`)
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

// 应用中心：一键安装（按市场 id，或直接 url+sha256，或第三方 listing_id）
export async function appsMarketInstall({ id, url, sha256, listing_id } = {}) {
  return requestJSON('/admin/apps/market/install', {
    method: 'POST',
    body: JSON.stringify({ id: id || '', url: url || '', sha256: sha256 || '', listing_id: listing_id || '' }),
  })
}

// ---- 第三方开发者体系（devmarket） ----

// 申请成为开发者（pending）
export function developerApply({ display_name, bio }) {
  return requestJSON('/developer/apply', { method: 'POST', body: JSON.stringify({ display_name: display_name || '', bio: bio || '' }) })
}

// 当前用户开发者档案（exists=false 表示尚未申请）
export function developerMe() {
  return requestJSON('/developer/me')
}

// 我的发布列表（全部状态）
export function developerListings() {
  return requestJSON('/developer/listings')
}

// 新建条目（草稿）
export function developerListingCreate(payload) {
  return requestJSON('/developer/listings', { method: 'POST', body: JSON.stringify(payload || {}) })
}

// 更新条目
export function developerListingUpdate(id, payload) {
  return requestJSON(`/developer/listings/${id}`, { method: 'PUT', body: JSON.stringify(payload || {}) })
}

// 删除条目
export function developerListingDelete(id) {
  return requestJSON(`/developer/listings/${id}`, { method: 'DELETE' })
}

// 提交审核（草稿/驳回 → pending）
export function developerListingSubmit(id) {
  return requestJSON(`/developer/listings/${id}/submit`, { method: 'POST' })
}

// 上传资源（kind=icon|package|screenshot），返回 { url, sha256?, screenshots? }
export async function developerListingUpload(id, kind, file) {
  const fd = new FormData()
  fd.append('file', file)
  const res = await fetch(BASE + `/developer/listings/${id}/upload?kind=${encodeURIComponent(kind)}`, {
    method: 'POST',
    headers: authHeaders(),
    body: fd,
  })
  if (res.status === 401) { handle401(`/developer/listings/${id}/upload`); throw new Error('需要登录') }
  const data = await res.json().catch(() => ({}))
  if (!res.ok) throw new Error(data?.error?.message || data.message || '上传失败')
  return data
}

// 站长：开发者申请列表（status=pending|all）
export function adminDevelopers(status = 'pending') {
  return requestJSON(`/admin/market/developers?status=${encodeURIComponent(status)}`)
}

// 站长：通过 / 驳回开发者
export function adminDeveloperApprove(id) {
  return requestJSON(`/admin/market/developers/${id}/approve`, { method: 'POST' })
}
export function adminDeveloperReject(id) {
  return requestJSON(`/admin/market/developers/${id}/reject`, { method: 'POST' })
}

// 站长：第三方条目审核列表（status=pending|all）
export function adminMarketListings(status = 'pending') {
  return requestJSON(`/admin/market/listings?status=${encodeURIComponent(status)}`)
}

// 站长：通过 / 驳回条目
export function adminMarketApprove(id) {
  return requestJSON(`/admin/market/listings/${id}/approve`, { method: 'POST' })
}
export function adminMarketReject(id, review_note = '') {
  return requestJSON(`/admin/market/listings/${id}/reject`, { method: 'POST', body: JSON.stringify({ review_note }) })
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

