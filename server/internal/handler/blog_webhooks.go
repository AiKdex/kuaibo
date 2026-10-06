// blog_webhooks.go 事件契约 v1 投递器（WebhookHub）。
//
// 职责：订阅事件总线的内核 topic → 按站长注册的 webhook（blog_webhooks 表）
// 异步投递 HTTP POST（JSON），HMAC-SHA256 签名防伪造；失败重试 1 次。
// 事件发布点在 service 层（file.*）与 handler 层（comment.*），本文件只做订阅与投递。
//
// 投递契约：
//   POST {url}
//   Content-Type: application/json
//   X-AiKlog-Topic: {topic}
//   X-AiKlog-Signature: hex(hmac-sha256(secret, body))   （secret 非空时）
//   body: {"topic","key","data","ts","source":"aiklog"}
package handler

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/AiKMAP/AiKmap/server/internal/engine/bus"
	"github.com/AiKMAP/AiKmap/server/internal/service"
)

// webhookHub 投递器：单 worker 串行消费（低频事件足够；天然限流不打爆目标）。
type webhookHub struct {
	db   *sql.DB
	ch   chan hubJob
	mu   sync.Mutex
	subs []func() // 取消订阅函数（进程生命周期内不退订，保留扩展）
}

type hubJob struct {
	topic string
	key   string
	data  map[string]any
}

// hubHTTPClient 投递客户端；H6 修复：禁止跟随重定向到内网/保留地址（webhook 目标为站长可配 URL，
// 目标机可用 302 把投递引向内网服务/元数据端点，禁用重定向后此类跳板失效）。
var hubHTTPClient = &http.Client{Timeout: 5 * time.Second,
	Transport:     &http.Transport{DialContext: service.SafeDialContext}, // H6 收尾：建连前复验最终 IP
	CheckRedirect: service.SafeCheckRedirect}

// newWebhookHub 创建投递器（chan 容量 256；溢出时丢弃并记日志，不阻塞事件总线）。
func newWebhookHub(db *sql.DB) *webhookHub {
	return &webhookHub{db: db, ch: make(chan hubJob, 256)}
}

// Start 订阅全部内核 topic 并启动 worker（在 handler.New 里调用，进程生命周期常驻）。
func (h *webhookHub) Start(b *bus.Bus) {
	for topic := range bus.KernelTopics {
		t := topic
		unsub := b.Subscribe(t, func(_ context.Context, e bus.Event) error {
			if e.Topic != t {
				return nil
			}
			job := hubJob{topic: e.Topic, key: e.Key, data: e.Data}
			select {
			case h.ch <- job:
			default:
				log.Printf("[webhook-hub] 队列满，丢弃事件 topic=%s key=%s", e.Topic, e.Key)
			}
			return nil
		})
		h.subs = append(h.subs, unsub)
	}
	go h.run()
}

// run worker：串行消费投递。
func (h *webhookHub) run() {
	for job := range h.ch {
		h.deliver(job)
	}
}

// webhookRow 投递目标（按事件即时查库；低频事件，SQLite 开销可忽略，免去缓存失效问题）。
func (h *webhookHub) targets(topic string) []struct {
	URL    string
	Secret string
} {
	var out []struct {
		URL    string
		Secret string
	}
	r, err := h.db.QueryContext(context.Background(),
		`SELECT url, COALESCE(secret,''), topics FROM blog_webhooks WHERE enabled=1`)
	if err != nil {
		return nil
	}
	defer r.Close()
	for r.Next() {
		var url, secret, topicsJSON string
		if r.Scan(&url, &secret, &topicsJSON) != nil {
			continue
		}
		var topics []string
		if json.Unmarshal([]byte(topicsJSON), &topics) != nil {
			continue
		}
		for _, t := range topics {
			if t == topic {
				out = append(out, struct {
					URL    string
					Secret string
				}{url, secret})
				break
			}
		}
	}
	return out
}

// deliver 单次投递：签名 → POST → 失败 2s 后重试 1 次。
func (h *webhookHub) deliver(job hubJob) {
	for _, t := range h.targets(job.topic) {
		payload, _ := json.Marshal(map[string]any{
			"topic": job.topic, "key": job.key, "data": job.data,
			"ts": time.Now().UnixMilli(), "source": "aiklog",
		})
		for attempt := 0; attempt < 2; attempt++ {
			if err := postWebhook(t.URL, t.Secret, job.topic, payload); err == nil {
				break
			} else if attempt == 0 {
				time.Sleep(2 * time.Second)
			} else {
				log.Printf("[webhook-hub] 投递失败 topic=%s url=%s err=%v", job.topic, t.URL, err)
			}
		}
	}
}

// postWebhook 发一次投递（HMAC 签名 + 5s 超时；2xx 视为成功）。
func postWebhook(url, secret, topic string, body []byte) error {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-AiKlog-Topic", topic)
	if secret != "" {
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(body)
		req.Header.Set("X-AiKlog-Signature", hex.EncodeToString(mac.Sum(nil)))
	}
	resp, err := hubHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("http %d", resp.StatusCode)
	}
	return nil
}

// ---- 站长 CRUD（登录；blogAdminOnly） ----

// blogWebhookList GET /api/v1/blog/webhooks
func (a *API) blogWebhookList(w http.ResponseWriter, r *http.Request) {
	if !a.blogAdminOnly(w, r) {
		return
	}
	rows, err := a.db.QueryContext(r.Context(),
		`SELECT id, url, secret, topics, enabled, created_at, updated_at FROM blog_webhooks ORDER BY created_at ASC`)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "WEBHOOK_LIST_FAILED", err.Error())
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, url, secret, topics string
		var enabled int
		var createdAt, updatedAt int64
		if rows.Scan(&id, &url, &secret, &topics, &enabled, &createdAt, &updatedAt) != nil {
			continue
		}
		var tp []string
		_ = json.Unmarshal([]byte(topics), &tp)
		out = append(out, map[string]any{
			"id": id, "url": url, "secret": secret, "topics": tp,
			"enabled": enabled == 1, "created_at": createdAt, "updated_at": updatedAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": out,
		"kernel_topics": func() []string {
			ks := make([]string, 0, len(bus.KernelTopics))
			for k := range bus.KernelTopics {
				ks = append(ks, k)
			}
			return ks
		}(),
	})
}

// blogWebhookCreate POST /api/v1/blog/webhooks {url, topics[], secret?, enabled?}
func (a *API) blogWebhookCreate(w http.ResponseWriter, r *http.Request) {
	if !a.blogAdminOnly(w, r) {
		return
	}
	var in struct {
		URL     string   `json:"url"`
		Topics  []string `json:"topics"`
		Secret  string   `json:"secret"`
		Enabled *bool    `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "WEBHOOK_BAD_BODY", "请求体解析失败")
		return
	}
	in.URL = strings.TrimSpace(in.URL)
	if !strings.HasPrefix(in.URL, "http://") && !strings.HasPrefix(in.URL, "https://") {
		writeErr(w, http.StatusBadRequest, "WEBHOOK_BAD_URL", "url 必须为 http/https")
		return
	}
	if len(in.Topics) == 0 {
		writeErr(w, http.StatusBadRequest, "WEBHOOK_NO_TOPICS", "topics 必填（内核 topic 白名单内）")
		return
	}
	for _, t := range in.Topics {
		if !bus.ValidateTopic(t, "") {
			writeErr(w, http.StatusUnprocessableEntity, "WEBHOOK_TOPIC_INVALID",
				"topic "+t+" 不在内核冻结清单（见 engine/bus/topics.go）")
			return
		}
	}
	if len(in.Secret) > 128 {
		writeErr(w, http.StatusBadRequest, "WEBHOOK_SECRET_TOO_LONG", "secret 不超过 128 字符")
		return
	}
	id := newID()
	now := time.Now().UnixMilli()
	enabled := 1
	if in.Enabled != nil && !*in.Enabled {
		enabled = 0
	}
	tp, _ := json.Marshal(in.Topics)
	if _, err := a.db.ExecContext(r.Context(),
		`INSERT INTO blog_webhooks (id, url, secret, topics, enabled, created_at, updated_at) VALUES (?,?,?,?,?,?,?)`,
		id, in.URL, in.Secret, string(tp), enabled, now, now); err != nil {
		writeErr(w, http.StatusInternalServerError, "WEBHOOK_CREATE_FAILED", err.Error())
		return
	}
	_, _ = a.aud.Append(r.Context(), a.curUserID(r), "blog.webhook_create", "blog_webhooks", map[string]any{"id": id, "url": in.URL})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id})
}

// blogWebhookDelete DELETE /api/v1/blog/webhooks/{id}
func (a *API) blogWebhookDelete(w http.ResponseWriter, r *http.Request) {
	if !a.blogAdminOnly(w, r) {
		return
	}
	id := r.PathValue("id")
	res, err := a.db.ExecContext(r.Context(), `DELETE FROM blog_webhooks WHERE id=?`, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "WEBHOOK_DELETE_FAILED", err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeErr(w, http.StatusNotFound, "WEBHOOK_NOT_FOUND", "webhook 不存在")
		return
	}
	_, _ = a.aud.Append(r.Context(), a.curUserID(r), "blog.webhook_delete", "blog_webhooks", map[string]any{"id": id})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// blogWebhookTest POST /api/v1/blog/webhooks/{id}/test —— 同步投递测试事件，返回投递结果。
func (a *API) blogWebhookTest(w http.ResponseWriter, r *http.Request) {
	if !a.blogAdminOnly(w, r) {
		return
	}
	id := r.PathValue("id")
	var url, secret, topicsJSON string
	err := a.db.QueryRowContext(r.Context(),
		`SELECT url, COALESCE(secret,''), topics FROM blog_webhooks WHERE id=?`, id).Scan(&url, &secret, &topicsJSON)
	if err != nil {
		writeErr(w, http.StatusNotFound, "WEBHOOK_NOT_FOUND", "webhook 不存在")
		return
	}
	var tp []string
	_ = json.Unmarshal([]byte(topicsJSON), &tp)
	topic := "config.changed"
	if len(tp) > 0 {
		topic = tp[0]
	}
	payload, _ := json.Marshal(map[string]any{
		"topic": topic, "key": "test", "data": map[string]any{"message": "爱库录 webhook 测试事件"},
		"ts": time.Now().UnixMilli(), "source": "aiklog",
	})
	if err := postWebhook(url, secret, topic, payload); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "topic": topic})
}
