// comments.go 评论核 API（第三轮反馈 2.2c：核只提供"按文章读写评论"API + 权限，展示走插件）。
// 定位方式 = 公开分享 token + path（公开面不暴露内部 file_id；后端解析校验分享可见性）。
//   GET  /api/v1/public/comments?token=..&path=.. （公开读）
//   POST /api/v1/comments { token, path, body, parent_id? } （登录写）
// 展示 UI 由 post_bottom 插件承载（内置示例 blog-comments），核心页不硬编码。
package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AiKMAP/AiKmap/server/internal/engine/bus"
	"github.com/AiKMAP/AiKmap/server/internal/service"
)

// resolveCommentFile 按 token+path 解析文章 file_id（校验分享存在且未撤销、文件在公开目录内）。
// 返回 file_id；错误时返回 (nil, apiErr)。
func (a *API) resolveCommentFile(r *http.Request, token, path string) (string, int, string, string) {
	if strings.TrimSpace(token) == "" {
		return "", http.StatusBadRequest, "COMMENT_TOKEN_REQUIRED", "token 必填"
	}
	row, err := a.loadShare(r.Context(), token)
	if err != nil {
		return "", http.StatusNotFound, "SHARE_NOT_FOUND", "分享不存在或已撤销"
	}
	if row.Scope == "dir" {
		if path == "" {
			return "", http.StatusBadRequest, "COMMENT_PATH_REQUIRED", "目录分享需指定 path"
		}
		files, err := a.collectDirFiles(r.Context(), row.DirID, "")
		if err != nil {
			return "", http.StatusNotFound, "SHARE_FILE_GONE", "公开目录不存在"
		}
		for _, it := range files {
			if it.path == path {
				return it.f.ID, 0, "", ""
			}
		}
		return "", http.StatusNotFound, "SHARE_PATH_NOT_FOUND", "目录中没有该文件"
	}
	return row.FileID, 0, "", ""
}

// commentsList GET /api/v1/public/comments?token=&path=（公开读：仅已通过审核的评论，时间正序）
func (a *API) commentsList(w http.ResponseWriter, r *http.Request) {
	fileID, code, codeStr, msg := a.resolveCommentFile(r, r.URL.Query().Get("token"), r.URL.Query().Get("path"))
	if codeStr != "" {
		writeErr(w, code, codeStr, msg)
		return
	}
	rows, err := a.db.QueryContext(r.Context(),
		`SELECT id, file_id, user_id, COALESCE(anchor,''), body, COALESCE(parent_id,''), status, COALESCE(guest_name,''), created_at
		 FROM comments WHERE file_id=? AND status='approved' ORDER BY created_at ASC`, fileID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "COMMENT_LIST_FAILED", err.Error())
		return
	}
	defer rows.Close()
	type c struct {
		ID        string `json:"id"`
		Body      string `json:"body"`
		ParentID  string `json:"parent_id,omitempty"`
		Author    string `json:"author"`
		CreatedAt int64  `json:"created_at"`
	}
	out := []c{}
	for rows.Next() {
		var id, fid, uid, anchor, body, parent, status, guest string
		var createdAt int64
		if rows.Scan(&id, &fid, &uid, &anchor, &body, &parent, &status, &guest, &createdAt) != nil {
			continue
		}
		author := strings.TrimSpace(guest)
		if author == "" {
			author = "读者"
			var uname, urole string
			if err := a.db.QueryRowContext(r.Context(),
				`SELECT COALESCE(display_name,''), COALESCE(role,'') FROM users WHERE id=?`, uid).Scan(&uname, &urole); err == nil {
				if uname != "" {
					author = uname
				} else if urole == "owner" || urole == "admin" {
					author = "管理员"
				}
			}
		}
		out = append(out, c{ID: id, Body: body, ParentID: parent, Author: author, CreatedAt: createdAt})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

// 登录用户评论频率限制（M12 修复）：每用户 60 秒窗口最多 commentRateMax 条，防脚本刷屏灌水。
// 与访客 IP 限流（guestCommentAllowed）互补：访客按 IP，登录用户按 user id。
const (
	commentRateMax = 10
	commentRateWin = 60 * time.Second
)

var commentRate struct {
	mu sync.Mutex
	m  map[string][]int64 // uid → 最近发表时间戳（秒）
}

// commentRateAllow 记录并判定：窗口内超限返回 false。
// 内存防御：条目数超 2000（随机 uid 撞库式攻击）时整体重建，避免 map 无界增长。
func commentRateAllow(uid string) bool {
	now := time.Now().Unix()
	commentRate.mu.Lock()
	defer commentRate.mu.Unlock()
	if commentRate.m == nil {
		commentRate.m = map[string][]int64{}
	}
	if len(commentRate.m) > 2000 {
		commentRate.m = map[string][]int64{}
	}
	cut := now - int64(commentRateWin.Seconds())
	qs := commentRate.m[uid]
	i := 0
	for i < len(qs) && qs[i] < cut {
		i++
	}
	qs = qs[i:]
	if len(qs) >= commentRateMax {
		commentRate.m[uid] = qs
		return false
	}
	commentRate.m[uid] = append(qs, now)
	return true
}

// commentsCreate POST /api/v1/comments（登录写评论）
// 请求：{token, path?, body, parent_id?}；body ≤ 2000 字符；parent_id 必须属于同一文章。
func (a *API) commentsCreate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Token    string `json:"token"`
		Path     string `json:"path"`
		Body     string `json:"body"`
		ParentID string `json:"parent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "COMMENT_BAD_BODY", "请求体解析失败")
		return
	}
	body := strings.TrimSpace(in.Body)
	if body == "" {
		writeErr(w, http.StatusBadRequest, "COMMENT_EMPTY", "评论内容不能为空")
		return
	}
	if len([]rune(body)) > 2000 {
		writeErr(w, http.StatusBadRequest, "COMMENT_TOO_LONG", "评论不超过 2000 字符")
		return
	}
	fileID, code, codeStr, msg := a.resolveCommentFile(r, in.Token, in.Path)
	if codeStr != "" {
		writeErr(w, code, codeStr, msg)
		return
	}
	// parent_id 必须属于同一文章
	if in.ParentID != "" {
		var pid, pfid string
		if err := a.db.QueryRowContext(r.Context(),
			`SELECT id, file_id FROM comments WHERE id=?`, in.ParentID).Scan(&pid, &pfid); err != nil || pfid != fileID {
			writeErr(w, http.StatusBadRequest, "COMMENT_BAD_PARENT", "parent_id 不存在或不属于同一文章")
			return
		}
	}
	uid, _ := r.Context().Value(ctxUserID).(string)
	// M12 修复：登录用户评论频率限制（防刷屏/脚本灌水）
	if !commentRateAllow(uid) {
		writeErr(w, http.StatusTooManyRequests, "COMMENT_RATE_LIMITED", "评论过于频繁，请稍后再试")
		return
	}
	id := newID()
	now := time.Now().UnixMilli()
	// parent_id 空时写 NULL（空串会被当作外键引用导致 FK 失败）
	var parentArg any
	if strings.TrimSpace(in.ParentID) == "" {
		parentArg = nil
	} else {
		parentArg = in.ParentID
	}
	if _, err := a.db.ExecContext(r.Context(),
		`INSERT INTO comments (id, file_id, user_id, body, parent_id, status, created_at)
		 VALUES (?, ?, ?, ?, ?, 'approved', ?)`,
		id, fileID, uid, body, parentArg, now); err != nil {
		writeErr(w, http.StatusInternalServerError, "COMMENT_CREATE_FAILED", err.Error())
		return
	}
	a.b.Publish(r.Context(), bus.Event{Topic: "comment.created", Key: id,
		Data: map[string]any{"file_id": fileID, "guest": false}})
	// B6：@提及落库 + 回复通知父楼作者（增值动作，失败不影响评论落库）
	a.afterCommentCreated(r, id, fileID, uid, body, in.ParentID)
	writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "id": id})
}

// ---- 访客评论（公开写，受站点开关 + IP 限流；默认待审） ----

// 评论 IP 限流（进程内）：每 IP 每 10 分钟 5 条
var (
	gcMu    sync.Mutex
	gcHits  = map[string][]time.Time{}
	gcWin   = 10 * time.Minute
	gcMax   = 5
	gcMaxNm = 40 // 访客昵称长度上限
)

func guestCommentAllowed(ip string) bool {
	gcMu.Lock()
	defer gcMu.Unlock()
	now := time.Now()
	arr := gcHits[ip][:0]
	for _, t := range gcHits[ip] {
		if now.Sub(t) < gcWin {
			arr = append(arr, t)
		}
	}
	if len(arr) >= gcMax {
		gcHits[ip] = arr
		return false
	}
	gcHits[ip] = append(arr, now)
	return true
}

// publicGuestComment POST /api/v1/public/comments {token, path?, body, name?, parent_id?}
// 访客评论：需站点开关 blog.comments_guest=true；写入 status='pending' 待站长审核，
// 审核通过后才公开可见（commentsList 只出 approved）。IP 限流 + 长度限制。
func (a *API) publicGuestComment(w http.ResponseWriter, r *http.Request) {
	// 站点开关（默认关：未开启时引导读者注册后评论）
	if v, ok := a.cfg.Get("blog.comments_guest"); !ok || v != "true" {
		writeErr(w, http.StatusForbidden, "GUEST_COMMENT_DISABLED", "本站未开放访客评论，请登录后评论")
		return
	}
	// M12 修复：限流 ident 走 a.clientIP（默认只用 RemoteAddr；仅 security.trust_proxy=true
	// 时才信任 X-Real-IP/X-Forwarded-For），否则访客伪造请求头即可绕过评论频率限制。
	ip := a.clientIP(r)
	if !guestCommentAllowed(ip) {
		writeErr(w, http.StatusTooManyRequests, "COMMENT_RATE", "评论过于频繁，请稍后再试")
		return
	}
	var in struct {
		Token    string `json:"token"`
		Path     string `json:"path"`
		Body     string `json:"body"`
		Name     string `json:"name"`
		ParentID string `json:"parent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "COMMENT_BAD_BODY", "请求体解析失败")
		return
	}
	body := strings.TrimSpace(in.Body)
	if body == "" {
		writeErr(w, http.StatusBadRequest, "COMMENT_EMPTY", "评论内容不能为空")
		return
	}
	if len([]rune(body)) > 2000 {
		writeErr(w, http.StatusBadRequest, "COMMENT_TOO_LONG", "评论不超过 2000 字符")
		return
	}
	name := strings.TrimSpace(in.Name)
	if len([]rune(name)) > gcMaxNm {
		name = string([]rune(name)[:gcMaxNm])
	}
	if name == "" {
		name = "访客"
	}
	fileID, code, codeStr, msg := a.resolveCommentFile(r, in.Token, in.Path)
	if codeStr != "" {
		writeErr(w, code, codeStr, msg)
		return
	}
	if in.ParentID != "" {
		var pid, pfid string
		if err := a.db.QueryRowContext(r.Context(),
			`SELECT id, file_id FROM comments WHERE id=?`, in.ParentID).Scan(&pid, &pfid); err != nil || pfid != fileID {
			writeErr(w, http.StatusBadRequest, "COMMENT_BAD_PARENT", "parent_id 不存在或不属于同一文章")
			return
		}
	}
	id := newID()
	now := time.Now().UnixMilli()
	var parentArg any
	if strings.TrimSpace(in.ParentID) == "" {
		parentArg = nil
	} else {
		parentArg = in.ParentID
	}
	if _, err := a.db.ExecContext(r.Context(),
		`INSERT INTO comments (id, file_id, user_id, body, parent_id, status, guest_name, created_at)
		 VALUES (?, ?, NULL, ?, ?, 'pending', ?, ?)`,
		id, fileID, body, parentArg, name, now); err != nil {
		writeErr(w, http.StatusInternalServerError, "COMMENT_CREATE_FAILED", err.Error())
		return
	}
	a.b.Publish(r.Context(), bus.Event{Topic: "comment.created", Key: id,
		Data: map[string]any{"file_id": fileID, "guest": true, "status": "pending"}})
	writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "id": id, "status": "pending",
		"message": "评论已提交，待站长审核通过后展示"})
}

// ---- 站长评论管理（登录；blogAdminOnly） ----

// blogCommentsAdmin GET /api/v1/blog/comments?status=&limit=（全站评论列表；含待审）
func (a *API) blogCommentsAdmin(w http.ResponseWriter, r *http.Request) {
	if !a.blogAdminOnly(w, r) {
		return
	}
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	limit := 100
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 && v <= 500 {
		limit = v
	}
	q := `SELECT c.id, c.file_id, COALESCE(c.guest_name,''), c.body, c.status, c.created_at, COALESCE(f.name,'')
	      FROM comments c LEFT JOIN files f ON f.id=c.file_id`
	args := []any{}
	if status == "pending" || status == "approved" {
		q += ` WHERE c.status=?`
		args = append(args, status)
	}
	q += ` ORDER BY c.created_at DESC LIMIT ?`
	args = append(args, limit)
	rows, err := a.db.QueryContext(r.Context(), q, args...)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "COMMENT_ADMIN_LIST_FAILED", err.Error())
		return
	}
	defer rows.Close()
	type cm struct {
		ID        string `json:"id"`
		FileID    string `json:"file_id"`
		File      string `json:"file_name"`
		Author    string `json:"author"`
		Body      string `json:"body"`
		Status    string `json:"status"`
		CreatedAt int64  `json:"created_at"`
	}
	out := []cm{}
	for rows.Next() {
		var c cm
		var guest string
		if err := rows.Scan(&c.ID, &c.FileID, &guest, &c.Body, &c.Status, &c.CreatedAt, &c.File); err != nil {
			continue
		}
		c.Author = strings.TrimSpace(guest)
		if c.Author == "" {
			c.Author = "注册用户"
		}
		out = append(out, c)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

// blogCommentApprove POST /api/v1/blog/comments/{id}/approve（待审 → 已通过）
func (a *API) blogCommentApprove(w http.ResponseWriter, r *http.Request) {
	if !a.blogAdminOnly(w, r) {
		return
	}
	id := r.PathValue("id")
	res, err := a.db.ExecContext(r.Context(),
		`UPDATE comments SET status='approved' WHERE id=? AND status='pending'`, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "COMMENT_APPROVE_FAILED", err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeErr(w, http.StatusNotFound, "COMMENT_NOT_PENDING", "评论不存在或不在待审状态")
		return
	}
	var fileID string
	_ = a.db.QueryRowContext(r.Context(), `SELECT file_id FROM comments WHERE id=?`, id).Scan(&fileID)
	// B6：待审评论通过时才做 @提及 / 回复通知 —— 未过审的内容不该产生通知
	var cBody, cUid, cParent string
	_ = a.db.QueryRowContext(r.Context(),
		`SELECT body, COALESCE(user_id,''), COALESCE(parent_id,'') FROM comments WHERE id=?`, id).
		Scan(&cBody, &cUid, &cParent)
	a.afterCommentCreated(r, id, fileID, cUid, cBody, cParent)
	a.b.Publish(r.Context(), bus.Event{Topic: "comment.approved", Key: id,
		Data: map[string]any{"file_id": fileID}})
	_, _ = a.aud.Append(r.Context(), a.curUserID(r), "blog.comment_approve", "comments", map[string]any{"id": id})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// blogCommentDelete DELETE /api/v1/blog/comments/{id}（删除评论，含待审与已通过）
func (a *API) blogCommentDelete(w http.ResponseWriter, r *http.Request) {
	if !a.blogAdminOnly(w, r) {
		return
	}
	id := r.PathValue("id")
	// 楼层评论：父评论删除后子评论失去挂载意义，一并删除
	// B6：先收集被删评论 id（含子楼），删除后连带清掉它们的 @提及行
	var delIDs []string
	{
		rows, qerr := a.db.QueryContext(r.Context(), `SELECT id FROM comments WHERE id=? OR parent_id=?`, id, id)
		if qerr == nil {
			for rows.Next() {
				var cid string
				if rows.Scan(&cid) == nil {
					delIDs = append(delIDs, cid)
				}
			}
			rows.Close()
		}
	}
	if _, err := a.db.ExecContext(r.Context(), `DELETE FROM comments WHERE id=? OR parent_id=?`, id, id); err != nil {
		writeErr(w, http.StatusInternalServerError, "COMMENT_DELETE_FAILED", err.Error())
		return
	}
	_ = a.mentions.PurgeSources(r.Context(), service.MentionSourceComment, delIDs)
	_, _ = a.aud.Append(r.Context(), a.curUserID(r), "blog.comment_delete", "comments", map[string]any{"id": id})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
