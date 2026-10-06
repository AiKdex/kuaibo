// aikgw —— AiKlog 平台模型中转网关（平台侧成本防线的落点）。
//
// 产品口径（2026-10-03 拍板）：系统支持站长自备模型（实例内 custom，不计量），
// 也支持平台提供模型；平台提供时按天限用量、可购买增量。
//
// 道理：实例内的管控（ai/meter.go，B36）控制面在站长手里，管不了站长自己；
// 平台对站长的防线必须落在平台侧——key 在平台手里 + 中转网关计量限额，
// 实例怎么改设置都绕不过。
//
// 架构：
//   实例(aiklog) --Bearer sk-aikgw-xxx--> aikgw(计量/限额/余额) --平台key池--> 上游(agnes/任意 OpenAI 兼容)
//
// 计量口径：
//   - 非流式 JSON 响应嗅探 usage.prompt/completion_tokens 逐次记账；
//   - 流式（SSE/音频）只计调用次数（token 待增量：SSE 尾块 usage）；
//   - 自然日按北京时间（与实例侧 B36 口径一致）。
//
// 管理面（Bearer -admin-key）：
//   GET  /admin/keys                列表（含今日用量）
//   POST /admin/keys                发 key {name,daily_calls,daily_tokens,balance,balance_enforced}
//   PUT  /admin/keys/{id}           调整 {daily_calls,daily_tokens,balance_delta,enabled,balance_enforced}
//   GET  /admin/usage?days=7        按 key×日 用量报表
//
// 拦截语义：无效 key 401 / 停用 403 / 次数超 429 / token 超 429 / 余额不足 402。
package main

import (
	"bytes"
	"crypto/rand"
	"crypto/tls"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	_ "modernc.org/sqlite"

	"github.com/AiKMAP/AiKmap/server/internal/ai"
)

var (
	db       *sql.DB
	upstream *url.URL
	keyPool  *ai.TokenPool
	adminKey string
	dayZone  = time.FixedZone("CST", 8*3600)
)

func day() string { return time.Now().In(dayZone).Format("2006-01-02") }

func main() {
	addr := flag.String("addr", ":8790", "监听地址")
	dbPath := flag.String("db", "data/gw.db", "SQLite 路径")
	up := flag.String("upstream", "", "上游 OpenAI 兼容基址（含 /v1）")
	keysFile := flag.String("upstream-keys-file", "", "上游 key 池文件（每行一个）")
	admin := flag.String("admin-key", os.Getenv("AIKGW_ADMIN_KEY"), "管理面 Bearer key（必填）")
	flag.Parse()

	if *admin == "" {
		log.Fatal("aikgw: 必须提供 -admin-key 或 AIKGW_ADMIN_KEY")
	}
	if *up == "" {
		log.Fatal("aikgw: 必须提供 -upstream（如 https://apihub.agnes-ai.com/v1）")
	}
	u, err := url.Parse(strings.TrimRight(*up, "/"))
	if err != nil {
		log.Fatalf("aikgw: upstream 解析失败: %v", err)
	}
	upstream = u
	adminKey = *admin

	if err := os.MkdirAll(filepath.Dir(*dbPath), 0o755); err != nil {
		log.Fatalf("aikgw: 建数据目录失败: %v", err)
	}
	db, err = sql.Open("sqlite", *dbPath+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		log.Fatalf("aikgw: 打开 DB 失败: %v", err)
	}
	if err := migrate(); err != nil {
		log.Fatalf("aikgw: 建表失败: %v", err)
	}

	// 上游 key 池（轮替 + 失败冷却，复用 aiklog 的 TokenPool）
	if *keysFile != "" {
		raw, err := os.ReadFile(*keysFile)
		if err != nil {
			log.Fatalf("aikgw: 读上游 key 池失败: %v", err)
		}
		var keys []string
		for _, line := range strings.Split(string(raw), "\n") {
			if k := strings.TrimSpace(line); k != "" && !strings.HasPrefix(k, "#") {
				keys = append(keys, k)
			}
		}
		if len(keys) > 0 {
			keyPool = ai.NewTokenPool(keys, 30*time.Second)
			log.Printf("aikgw: 上游 key 池就绪（%d 把）", len(keys))
		}
	}
	if keyPool == nil {
		log.Fatal("aikgw: 上游 key 池为空（-upstream-keys-file）")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/admin/", handleAdmin)
	mux.HandleFunc("/", handleProxy)

	log.Printf("aikgw: listening %s -> %s", *addr, u)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatal(err)
	}
}

// ---- 表 ----

func migrate() error {
	for _, s := range []string{
		`CREATE TABLE IF NOT EXISTS gw_keys (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			key TEXT NOT NULL UNIQUE,
			name TEXT NOT NULL DEFAULT '',
			daily_calls INTEGER NOT NULL DEFAULT 0,
			daily_tokens INTEGER NOT NULL DEFAULT 0,
			balance INTEGER NOT NULL DEFAULT 0,
			balance_enforced INTEGER NOT NULL DEFAULT 0,
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS gw_usage (
			key_id INTEGER NOT NULL,
			day TEXT NOT NULL,
			calls INTEGER NOT NULL DEFAULT 0,
			prompt_tokens INTEGER NOT NULL DEFAULT 0,
			completion_tokens INTEGER NOT NULL DEFAULT 0,
			updated_at INTEGER NOT NULL,
			PRIMARY KEY (key_id, day)
		)`,
		`CREATE TABLE IF NOT EXISTS gw_ledger (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			ts INTEGER NOT NULL,
			key_id INTEGER NOT NULL,
			delta INTEGER NOT NULL,
			balance_after INTEGER NOT NULL,
			reason TEXT NOT NULL DEFAULT ''
		)`,
		// 按 key 指定上游（多模型供给：不同站点可走不同上游服务）
		`ALTER TABLE gw_keys ADD COLUMN upstream_url TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE gw_keys ADD COLUMN upstream_key TEXT NOT NULL DEFAULT ''`,
	} {
		if _, err := db.Exec(s); err != nil {
			// ALTER TABLE 重复加列会报错——已存在即视为完成，其余错误照常返回
			if !strings.Contains(err.Error(), "duplicate column") {
				return err
			}
		}
	}
	return nil
}

// ---- key 视图 ----

type gwKey struct {
	ID              int64  `json:"id"`
	Key             string `json:"key"`
	Name            string `json:"name"`
	DailyCalls      int    `json:"daily_calls"`
	DailyTokens     int    `json:"daily_tokens"`
	Balance         int    `json:"balance"`
	BalanceEnforced bool   `json:"balance_enforced"`
	Enabled         bool   `json:"enabled"`
	CreatedAt       int64  `json:"created_at"`
	UpdatedAt       int64  `json:"updated_at"`
	// 今日用量（列表/详情时填充）
	UsedCalls      int    `json:"used_calls,omitempty"`
	UsedTokens     int    `json:"used_tokens,omitempty"`
	UsedPrompt     int    `json:"used_prompt_tokens,omitempty"`
	UsedCompletion int    `json:"used_completion_tokens,omitempty"`
	// 按 key 上游覆盖（空 = 走网关默认上游 + 平台 key 池）
	UpstreamURL string `json:"upstream_url,omitempty"`
	UpstreamKey string `json:"upstream_key,omitempty"`
}

const keyCols = `id, key, name, daily_calls, daily_tokens, balance, balance_enforced, enabled, created_at, updated_at, upstream_url, upstream_key`

func scanKey(row interface{ Scan(...any) error }) (*gwKey, error) {
	var k gwKey
	var be, en int
	if err := row.Scan(&k.ID, &k.Key, &k.Name, &k.DailyCalls, &k.DailyTokens, &k.Balance, &be, &en, &k.CreatedAt, &k.UpdatedAt, &k.UpstreamURL, &k.UpstreamKey); err != nil {
		return nil, err
	}
	k.BalanceEnforced = be == 1
	k.Enabled = en == 1
	return &k, nil
}

func keyByKey(key string) (*gwKey, error) {
	k, err := scanKey(db.QueryRow(`SELECT `+keyCols+` FROM gw_keys WHERE key=?`, key))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return k, err
}

func todayUsage(keyID int64) (calls, pt, ct int) {
	_ = db.QueryRow(`SELECT COALESCE(calls,0), COALESCE(prompt_tokens,0), COALESCE(completion_tokens,0)
		FROM gw_usage WHERE key_id=? AND day=?`, keyID, day()).Scan(&calls, &pt, &ct)
	return
}

func bumpUsage(keyID int64, pt, ct int) {
	_, _ = db.Exec(`INSERT INTO gw_usage(key_id, day, calls, prompt_tokens, completion_tokens, updated_at)
		VALUES(?,?,1,?,?,?)
		ON CONFLICT(key_id, day) DO UPDATE SET calls=calls+1,
			prompt_tokens=prompt_tokens+excluded.prompt_tokens,
			completion_tokens=completion_tokens+excluded.completion_tokens,
			updated_at=excluded.updated_at`, keyID, day(), pt, ct, time.Now().Unix())
}

// ---- 代理面 ----

var callCounter atomic.Int64

func handleProxy(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.URL.Path, "/v1/") {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": map[string]any{"code": "GW_NOT_FOUND", "message": "仅代理 /v1/* 路径"}})
		return
	}
	bearer := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	k, err := keyByKey(bearer)
	if err != nil || k == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": map[string]any{"code": "GW_KEY_INVALID", "message": "平台模型 key 无效"}})
		return
	}
	if !k.Enabled {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": map[string]any{"code": "GW_KEY_DISABLED", "message": "该站点平台模型服务已停用，请联系平台"}})
		return
	}
	calls, pt, ct := todayUsage(k.ID)
	if k.DailyCalls > 0 && calls >= k.DailyCalls {
		writeJSON(w, http.StatusTooManyRequests, map[string]any{"error": map[string]any{"code": "GW_DAILY_CALLS", "message": fmt.Sprintf("该站点今日平台模型调用次数已达上限（%d）", k.DailyCalls)}})
		return
	}
	if k.DailyTokens > 0 && pt+ct >= k.DailyTokens {
		writeJSON(w, http.StatusTooManyRequests, map[string]any{"error": map[string]any{"code": "GW_DAILY_TOKENS", "message": fmt.Sprintf("该站点今日平台模型 token 已达上限（%d）", k.DailyTokens)}})
		return
	}
	if k.BalanceEnforced && k.Balance <= 0 {
		writeJSON(w, http.StatusPaymentRequired, map[string]any{"error": map[string]any{"code": "GW_NO_BALANCE", "message": "平台模型 token 余额不足，请购买增量包"}})
		return
	}

	// 计一次调用（含上游失败——真实成本口径）
	bumpUsage(k.ID, 0, 0)
	callCounter.Add(1)

	forward(w, r, k)
}

// outTransport 与实例侧 ai.newHTTPClient 同款：仅 HTTP/1.1、禁 keep-alive——
// 默认 Transport（HTTP/2 + Go 指纹）会被上游 Cloudflare 挑战回 HTML 页。
var outTransport = &http.Transport{
	TLSClientConfig:       &tls.Config{NextProtos: []string{"http/1.1"}},
	ForceAttemptHTTP2:     false,
	DisableKeepAlives:     true,
	ResponseHeaderTimeout: 120 * time.Second,
}

var outClient = &http.Client{Transport: outTransport}

// forward 手写转发：出站请求与实测可通过上游 CF WAF 的最小形态同构——
// 仅 Content-Type/Authorization（+Go 默认 Host/UA），显式 Content-Length，
// 非 chunked。不用 ReverseProxy：它透传入站请求形态（curl 特征头/编码），
// 实测必被上游 CF「Attention Required」挑战（同机 Go 极简请求 200 可证）。
// 上游选择：key 绑定了 upstream_url 则用之（Authorization=upstream_key），
// 否则走网关默认上游 + 平台 key 池轮替。
func forward(w http.ResponseWriter, r *http.Request, k *gwKey) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 64<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("GW_BAD_BODY", "请求体读取失败"))
		return
	}
	target, authKey := upstream, ""
	if k.UpstreamURL != "" {
		u2, perr := url.Parse(strings.TrimRight(k.UpstreamURL, "/"))
		if perr != nil || u2.Host == "" {
			writeJSON(w, http.StatusInternalServerError, errBody("GW_UPSTREAM_CFG", "该 key 的上游配置无效"))
			return
		}
		target, authKey = u2, k.UpstreamKey
	} else if pk, perr := keyPool.Next(); perr == nil {
		authKey = pk
	}
	path := strings.TrimPrefix(r.URL.Path, "/v1") // 上游基址约定已含 /v1
	if r.URL.RawQuery != "" {
		path += "?" + r.URL.RawQuery
	}
	req, err := http.NewRequest(r.Method, target.String()+path, bytes.NewReader(body))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody("GW_REQ", err.Error()))
		return
	}
	req.ContentLength = int64(len(body))
	ctype := r.Header.Get("Content-Type")
	if ctype == "" {
		ctype = "application/json"
	}
	req.Header.Set("Content-Type", ctype)
	if authKey != "" {
		req.Header.Set("Authorization", "Bearer "+authKey)
	}
	resp, err := outClient.Do(req)
	if err != nil {
		log.Printf("aikgw: upstream error: %v", err)
		writeJSON(w, http.StatusBadGateway, errBody("GW_UPSTREAM", "平台模型上游暂不可用"))
		return
	}
	defer resp.Body.Close()

	ctt := resp.Header.Get("Content-Type")
	// JSON 且非流式：整读嗅探 usage 记账后回写；其余（SSE/音频）逐块直通只计次。
	if resp.StatusCode < 400 && strings.Contains(ctt, "application/json") && !strings.Contains(ctt, "text/event-stream") {
		rb, rerr := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
		if rerr == nil {
			recordTokens(k.ID, rb)
			h := w.Header()
			for _, hk := range []string{"Content-Type", "Content-Length"} {
				if v := resp.Header.Get(hk); v != "" {
					h.Set(hk, v)
				}
			}
			w.WriteHeader(resp.StatusCode)
			_, _ = w.Write(rb)
			return
		}
	}
	h := w.Header()
	for _, hk := range []string{"Content-Type", "Content-Disposition", "Cache-Control"} {
		if v := resp.Header.Get(hk); v != "" {
			h.Set(hk, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	flusher, _ := w.(http.Flusher)
	buf := make([]byte, 32<<10)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if rerr != nil {
			return
		}
	}
}

func recordTokens(keyID int64, body []byte) {
	var out struct {
		Usage *struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal(body, &out) == nil && out.Usage != nil {
		if _, err := db.Exec(`UPDATE gw_usage SET prompt_tokens=prompt_tokens+?, completion_tokens=completion_tokens+?, updated_at=?
			WHERE key_id=? AND day=?`, out.Usage.PromptTokens, out.Usage.CompletionTokens, time.Now().Unix(), keyID, day()); err != nil {
			log.Printf("aikgw: usage 更新失败: %v", err)
		}
	}
}

func errBody(code, msg string) map[string]any {
	return map[string]any{"error": map[string]any{"code": code, "message": msg}}
}

// ---- 管理面 ----

func handleAdmin(w http.ResponseWriter, r *http.Request) {
	if strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ") != adminKey {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "admin key 无效"})
		return
	}
	switch {
	case r.URL.Path == "/admin/keys" && r.Method == http.MethodGet:
		adminListKeys(w, r)
	case r.URL.Path == "/admin/keys" && r.Method == http.MethodPost:
		adminCreateKey(w, r)
	case strings.HasPrefix(r.URL.Path, "/admin/keys/") && r.Method == http.MethodPut:
		adminUpdateKey(w, r)
	case r.URL.Path == "/admin/usage" && r.Method == http.MethodGet:
		adminUsage(w, r)
	default:
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "未知管理端点"})
	}
}

func adminListKeys(w http.ResponseWriter, _ *http.Request) {
	rows, err := db.Query(`SELECT ` + keyCols + ` FROM gw_keys ORDER BY id`)
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	defer rows.Close()
	out := []gwKey{}
	for rows.Next() {
		k, err := scanKey(rows)
		if err != nil {
			continue
		}
		k.UsedCalls, k.UsedPrompt, k.UsedCompletion = todayUsage3(k.ID)
		k.UsedTokens = k.UsedPrompt + k.UsedCompletion
		out = append(out, *k)
	}
	writeJSON(w, 200, map[string]any{"keys": out})
}

func todayUsage3(id int64) (calls, pt, ct int) {
	_ = db.QueryRow(`SELECT COALESCE(calls,0),COALESCE(prompt_tokens,0),COALESCE(completion_tokens,0)
		FROM gw_usage WHERE key_id=? AND day=?`, id, day()).Scan(&calls, &pt, &ct)
	return
}

func adminCreateKey(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name            string `json:"name"`
		DailyCalls      int    `json:"daily_calls"`
		DailyTokens     int    `json:"daily_tokens"`
		Balance         int    `json:"balance"`
		BalanceEnforced bool   `json:"balance_enforced"`
		UpstreamURL     string `json:"upstream_url"`
		UpstreamKey     string `json:"upstream_key"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<15)).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]any{"error": "bad json"})
		return
	}
	key, err := genKey()
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	now := time.Now().Unix()
	be, en := boolInt(req.BalanceEnforced), 1
	res, err := db.Exec(`INSERT INTO gw_keys(key,name,daily_calls,daily_tokens,balance,balance_enforced,enabled,created_at,updated_at,upstream_url,upstream_key)
		VALUES(?,?,?,?,?,?,?, ?, ?, ?, ?)`, key, req.Name, req.DailyCalls, req.DailyTokens, req.Balance, be, en, now, now, req.UpstreamURL, req.UpstreamKey)
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	id, _ := res.LastInsertId()
	if req.Balance > 0 {
		_, _ = db.Exec(`INSERT INTO gw_ledger(ts,key_id,delta,balance_after,reason) VALUES(?,?,?,?,?)`,
			now, id, req.Balance, req.Balance, "初始额度")
	}
	log.Printf("aikgw: key 已签发 id=%d name=%s", id, req.Name)
	writeJSON(w, 200, map[string]any{"id": id, "key": key, "name": req.Name})
}

func adminUpdateKey(w http.ResponseWriter, r *http.Request) {
	var id int64
	if _, err := fmt.Sscanf(strings.TrimPrefix(r.URL.Path, "/admin/keys/"), "%d", &id); err != nil {
		writeJSON(w, 400, map[string]any{"error": "bad key id"})
		return
	}
	var req struct {
		DailyCalls      *int   `json:"daily_calls"`
		DailyTokens     *int   `json:"daily_tokens"`
		BalanceDelta    *int   `json:"balance_delta"`
		Enabled         *bool  `json:"enabled"`
		BalanceEnforced *bool  `json:"balance_enforced"`
		UpstreamURL     *string `json:"upstream_url"`
		UpstreamKey     *string `json:"upstream_key"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<15)).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]any{"error": "bad json"})
		return
	}
	if req.DailyCalls != nil && *req.DailyCalls >= 0 {
		_, _ = db.Exec(`UPDATE gw_keys SET daily_calls=?, updated_at=? WHERE id=?`, *req.DailyCalls, time.Now().Unix(), id)
	}
	if req.DailyTokens != nil && *req.DailyTokens >= 0 {
		_, _ = db.Exec(`UPDATE gw_keys SET daily_tokens=?, updated_at=? WHERE id=?`, *req.DailyTokens, time.Now().Unix(), id)
	}
	if req.Enabled != nil {
		v := 0
		if *req.Enabled {
			v = 1
		}
		_, _ = db.Exec(`UPDATE gw_keys SET enabled=?, updated_at=? WHERE id=?`, v, time.Now().Unix(), id)
	}
	if req.BalanceEnforced != nil {
		v := 0
		if *req.BalanceEnforced {
			v = 1
		}
		_, _ = db.Exec(`UPDATE gw_keys SET balance_enforced=?, updated_at=? WHERE id=?`, v, time.Now().Unix(), id)
	}
	if req.UpstreamURL != nil {
		_, _ = db.Exec(`UPDATE gw_keys SET upstream_url=?, updated_at=? WHERE id=?`, *req.UpstreamURL, time.Now().Unix(), id)
	}
	if req.UpstreamKey != nil && *req.UpstreamKey != "" {
		_, _ = db.Exec(`UPDATE gw_keys SET upstream_key=?, updated_at=? WHERE id=?`, *req.UpstreamKey, time.Now().Unix(), id)
	}
	if req.BalanceDelta != nil && *req.BalanceDelta != 0 {
		now := time.Now().Unix()
		if _, err := db.Exec(`UPDATE gw_keys SET balance=balance+?, updated_at=? WHERE id=?`, *req.BalanceDelta, now, id); err == nil {
			var bal int
			_ = db.QueryRow(`SELECT balance FROM gw_keys WHERE id=?`, id).Scan(&bal)
			_, _ = db.Exec(`INSERT INTO gw_ledger(ts,key_id,delta,balance_after,reason) VALUES(?,?,?,?,?)`,
				now, id, *req.BalanceDelta, bal, r.Header.Get("X-Reason"))
		}
	}
	k, err := scanKey(db.QueryRow(`SELECT `+keyCols+` FROM gw_keys WHERE id=?`, id))
	if err != nil {
		writeJSON(w, 404, map[string]any{"error": "key 不存在"})
		return
	}
	writeJSON(w, 200, k)
}

func adminUsage(w http.ResponseWriter, r *http.Request) {
	days := 7
	fmt.Sscanf(r.URL.Query().Get("days"), "%d", &days)
	if days <= 0 || days > 90 {
		days = 7
	}
	rows, err := db.Query(`SELECT u.key_id, COALESCE(k.name,''), u.day, u.calls, u.prompt_tokens, u.completion_tokens
		FROM gw_usage u LEFT JOIN gw_keys k ON k.id=u.key_id
		WHERE u.day >= ? ORDER BY u.day DESC, u.calls DESC`,
		time.Now().In(dayZone).AddDate(0, 0, -(days-1)).Format("2006-01-02"))
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	defer rows.Close()
	type row struct {
		KeyID            int64  `json:"key_id"`
		Name             string `json:"name"`
		Day              string `json:"day"`
		Calls            int    `json:"calls"`
		PromptTokens     int    `json:"prompt_tokens"`
		CompletionTokens int    `json:"completion_tokens"`
	}
	out := []row{}
	for rows.Next() {
		var x row
		_ = rows.Scan(&x.KeyID, &x.Name, &x.Day, &x.Calls, &x.PromptTokens, &x.CompletionTokens)
		out = append(out, x)
	}
	writeJSON(w, 200, map[string]any{"days": days, "usage": out})
}

func genKey() (string, error) {
	b := make([]byte, 18)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "sk-aikgw-" + hex.EncodeToString(b), nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
