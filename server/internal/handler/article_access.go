package handler

// article_access.go 文章级访问控制（2.2 密码保护 / 2.3 定时发布）。
//
// 2.2 密码保护：files.access_pwd（sha256 哈希）。公开正文读取（shares/{token}/content）
// 在文件确定后检查：未解锁 → 401 PASSWORD_REQUIRED（前端出密码框）；解锁走
// POST /api/v1/public/unlock 校验密码并签发 HMAC 解锁凭证（会话级 24h，X-Unlock-Token）。
// 2.3 定时发布：files.publish_at（毫秒时间戳，NULL=立即）。公开列表/正文过滤
// publish_at>now 的文章（等同未发布）；后台常驻 ticker 到点置 NULL 自动公开。

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/AiKMAP/AiKmap/server/internal/service"
)

// 复用 handler 包的公共 sentinel 错误（避免重复定义）。
var errPasswordRequired = errors.New("password required")

// ---- 密码哈希 ----

// hashAccessPwd sha256 哈希访问密码（轻量；仅限文章访问口令场景）。
func hashAccessPwd(pwd string) string {
	sum := sha256.Sum256([]byte("aiklog-article-pwd:" + pwd))
	return hex.EncodeToString(sum[:])
}

// ---- 解锁凭证（HMAC 签名，复用配置加密密钥派生） ----

// unlockSecret 解锁凭证签名密钥（由配置加密密钥派生，进程级稳定、仅服务端持有）。
func (a *API) unlockSecret() []byte {
	return append([]byte("aiklog-unlock:"), a.cfg.SecretKey()...)
}

// issueUnlockToken 签发文章解锁凭证（绑定 file_id，24h 会话级；不含密码）。
func (a *API) issueUnlockToken(fileID string) string {
	exp := time.Now().Unix() + 86400
	mac := hmac.New(sha256.New, a.unlockSecret())
	mac.Write([]byte(fileID + "|" + strconv.FormatInt(exp, 10)))
	sig := hex.EncodeToString(mac.Sum(nil))[:32]
	return fileID + "." + strconv.FormatInt(exp, 10) + "." + sig
}

// verifyUnlockToken 校验解锁凭证（返回是否有效且未过期、绑定指定文件）。
func (a *API) verifyUnlockToken(tok, fileID string) bool {
	parts := strings.SplitN(tok, ".", 3)
	if len(parts) != 3 || parts[0] != fileID {
		return false
	}
	exp, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return false
	}
	mac := hmac.New(sha256.New, a.unlockSecret())
	mac.Write([]byte(parts[0] + "|" + parts[1]))
	expect := hex.EncodeToString(mac.Sum(nil))[:32]
	return subtle.ConstantTimeCompare([]byte(expect), []byte(parts[2])) == 1
}

// articleAccessPwd 读文章访问密码（空=无密码）。
func (a *API) articleAccessPwd(r *http.Request, fileID string) (string, error) {
	var pwd string
	err := a.db.QueryRowContext(r.Context(),
		`SELECT COALESCE(access_pwd,'') FROM files WHERE id=?`, fileID).Scan(&pwd)
	return pwd, err
}

// articleUnlocked 检查请求是否已携带有效解锁凭证（X-Unlock-Token 或 query unlock）。
func (a *API) articleUnlocked(r *http.Request, fileID string) bool {
	tok := r.Header.Get("X-Unlock-Token")
	if tok == "" {
		tok = r.URL.Query().Get("unlock")
	}
	return tok != "" && a.verifyUnlockToken(tok, fileID)
}

// requireArticleUnlock 公开正文闸门：有密码且未解锁 → 401 PASSWORD_REQUIRED；已解锁/无密码 → nil 放行。
func (a *API) requireArticleUnlock(w http.ResponseWriter, r *http.Request, fileID string) error {
	pwd, err := a.articleAccessPwd(r, fileID)
	if err != nil {
		return err
	}
	if pwd == "" || a.articleUnlocked(r, fileID) {
		return nil
	}
	writeErr(w, http.StatusUnauthorized, "PASSWORD_REQUIRED", "该文章已加密，请输入访问密码")
	return errPasswordRequired
}

// articleLocked 无副作用的密码闸门探针（不写响应）。C2 修复用：
// SSR 文章页（/blog/{slug}、/{slug}）不能只靠 requireArticleUnlock —— 那是"写 JSON 错误"的形态，
// HTML 页面需要的是"少渲染正文"，故这里给一个纯判定版本供 SSR 轨调用。
func (a *API) articleLocked(r *http.Request, fileID string) bool {
	pwd, err := a.articleAccessPwd(r, fileID)
	if err != nil || pwd == "" {
		return false
	}
	return !a.articleUnlocked(r, fileID)
}

// blogPostsAccess POST /api/v1/blog/posts/access {"id":"文章id","password":"...或空串清除"}
// 2.2 文章密码设置（站长/作者；密码 sha256 哈希入库，不存明文）。
func (a *API) blogPostsAccess(w http.ResponseWriter, r *http.Request) {
	if !a.blogAdminOnly(w, r) {
		return
	}
	var req struct {
		ID       string `json:"id"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", "请求体解析失败")
		return
	}
	if req.ID == "" {
		writeErr(w, http.StatusBadRequest, "POST_ID_REQUIRED", "缺少文章 id")
		return
	}
	dirID := service.BlogDirID
	var cnt int
	if err := a.db.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM files WHERE id=? AND parent_id IN
		   (SELECT id FROM files WHERE parent_id=? AND kind='dir' AND deleted_at IS NULL)
		 AND kind='file' AND deleted_at IS NULL`,
		req.ID, dirID).Scan(&cnt); err != nil || cnt == 0 {
		writeErr(w, http.StatusNotFound, "POST_NOT_FOUND", "文章不存在或不属于博客："+req.ID)
		return
	}
	var pwd any
	desc := "clear"
	if req.Password != "" {
		pwd = hashAccessPwd(req.Password)
		desc = "set"
	}
	if _, err := a.db.ExecContext(r.Context(),
		`UPDATE files SET access_pwd=?, updated_at=? WHERE id=?`, pwd, time.Now().UnixMilli(), req.ID); err != nil {
		writeErr(w, http.StatusInternalServerError, "POST_ACCESS_FAILED", err.Error())
		return
	}
	_, _ = a.aud.Append(r.Context(), a.curUserID(r), "blog.posts_access_"+desc, "files",
		map[string]any{"id": req.ID})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// publicUnlock POST /api/v1/public/unlock {"file_id":"...","password":"..."}
// 公开解锁：校验文章密码 → 签发解锁凭证（24h 会话级）。
//
// 限流（安全加固）：本端点在 publicPath 白名单内 = 完全匿名，若不限流则可被无限次
// 爆破文章访问密码（密码空间通常很小）。复用 loginThrottle 的滑窗计数，按
// 「客户端 IP + file_id」聚合计数——既防单篇爆破，也防同一 IP 换文章批量试。
func (a *API) publicUnlock(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FileID   string `json:"file_id"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.FileID == "" || req.Password == "" {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", "缺少 file_id 或 password")
		return
	}
	// 限流键：IP + 文章 id（不同文章独立计数，避免误伤正常读者）
	tkey := "unlock:" + a.clientIP(r) + ":" + req.FileID
	if ok, remain := loginThrottler.allow(tkey); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(remain))
		writeErr(w, http.StatusTooManyRequests, "RATE_LIMITED",
			"尝试过于频繁，请 "+strconv.Itoa(remain)+" 秒后再试")
		return
	}
	pwd, err := a.articleAccessPwd(r, req.FileID)
	if err != nil || pwd == "" {
		writeErr(w, http.StatusNotFound, "NOT_PROTECTED", "该文章未设置密码")
		return
	}
	if subtle.ConstantTimeCompare([]byte(hashAccessPwd(req.Password)), []byte(pwd)) != 1 {
		loginThrottler.hit(tkey) // 仅失败计数
		writeErr(w, http.StatusForbidden, "WRONG_PASSWORD", "密码错误")
		return
	}
	loginThrottler.reset(tkey) // 成功清零
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "unlock_token": a.issueUnlockToken(req.FileID), "expires_in": 86400})
}

// blogPostsSchedule POST /api/v1/blog/posts/schedule {"id":"文章id","publish_at":毫秒时间戳|null}
// 2.3 定时发布：设置发布时间（null=立即发布/清除定时）。未到点文章不出现在公开列表，
// 由常驻调度（main.go ticker）到点自动公开。
func (a *API) blogPostsSchedule(w http.ResponseWriter, r *http.Request) {
	if !a.blogAdminOnly(w, r) {
		return
	}
	var req struct {
		ID        string `json:"id"`
		PublishAt *int64 `json:"publish_at"` // null=立即发布（清除定时）
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ID == "" {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", "缺少文章 id 或请求体错误")
		return
	}
	dirID := service.BlogDirID
	var cnt int
	if err := a.db.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM files WHERE id=? AND parent_id IN
		   (SELECT id FROM files WHERE parent_id=? AND kind='dir' AND deleted_at IS NULL)
		 AND kind='file' AND deleted_at IS NULL`,
		req.ID, dirID).Scan(&cnt); err != nil || cnt == 0 {
		writeErr(w, http.StatusNotFound, "POST_NOT_FOUND", "文章不存在或不属于博客："+req.ID)
		return
	}
	if req.PublishAt != nil && *req.PublishAt < time.Now().UnixMilli() {
		writeErr(w, http.StatusBadRequest, "PAST_TIME", "publish_at 必须晚于当前时间（立即发布请传 null）")
		return
	}
	var pa any
	if req.PublishAt != nil {
		pa = *req.PublishAt
	}
	if _, err := a.db.ExecContext(r.Context(),
		`UPDATE files SET publish_at=?, updated_at=? WHERE id=?`, pa, time.Now().UnixMilli(), req.ID); err != nil {
		writeErr(w, http.StatusInternalServerError, "POST_SCHEDULE_FAILED", err.Error())
		return
	}
	_, _ = a.aud.Append(r.Context(), a.curUserID(r), "blog.posts_schedule", "files",
		map[string]any{"id": req.ID, "publish_at": req.PublishAt})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
