package service

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/AiKMAP/AiKmap/server/internal/im"
)

// ---- P1-2 每日知识日报 ----
// 每日 08:00 汇总：今日入库 / 未处理冲突 / 新增评论 / 知识库规模 → 推送到已绑定 IM。

// DigestStore 日报生成与推送。
type DigestStore struct {
	db  *sql.DB
	cfg ConfigGetter
}

// ConfigGetter 读取配置的最小接口（handler API 持有 config.Store）。
type ConfigGetter interface {
	GetString(key string) string
	GetBool(key string) bool
}

// NewDigestStore 创建日报服务。
func NewDigestStore(db *sql.DB, cfg ConfigGetter) *DigestStore { return &DigestStore{db: db, cfg: cfg} }

// DigestReport 日报数据。
type DigestReport struct {
	Date        string   `json:"date"`
	InboxToday  int      `json:"inbox_today"`  // 今日入库
	InboxItems  []string `json:"inbox_items"`  // 今日入库名（前 5）
	Conflicts   int      `json:"conflicts"`    // 未处理冲突通知
	Comments    int      `json:"comments"`     // 今日新增评论
	TotalFiles  int      `json:"total_files"`  // 知识库文件总数
	InactiveDir string   `json:"inactive_dir"` // 活跃度最低目录（提示）
}

// Build 生成日报文本。
func (d *DigestStore) Build(ctx context.Context, userID string) (*DigestReport, error) {
	now := time.Now()
	// 本壳 files.*_at 一律**毫秒**；此处曾用 Unix()（秒），导致 created_at>=dayStart 恒真
	// →「今日入库」实际等于文件总数、「7 天未更新目录」恒为空。已统一为毫秒。
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).UnixMilli()
	rep := &DigestReport{Date: now.Format("2006-01-02")}

	_ = d.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM files WHERE kind='file' AND created_at>=? AND deleted_at IS NULL`, dayStart).
		Scan(&rep.InboxToday)
	rows, err := d.db.QueryContext(ctx,
		`SELECT name FROM files WHERE kind='file' AND created_at>=? AND deleted_at IS NULL ORDER BY created_at DESC LIMIT 5`, dayStart)
	if err == nil {
		for rows.Next() {
			var n string
			if rows.Scan(&n) == nil {
				rep.InboxItems = append(rep.InboxItems, n)
			}
		}
		rows.Close()
	}
	_ = d.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM notifications WHERE user_id=? AND type='conflict' AND read_at IS NULL`, userID).
		Scan(&rep.Conflicts)
	// 本壳评论表是 `comments`（file_id/body/status），上游另起的 `blog_comments` 在本壳不存在。
	// 含 pending（访客待审）：站长日报需要看到"有待审"这件事。
	_ = d.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM comments WHERE created_at>=?`, dayStart).
		Scan(&rep.Comments)
	_ = d.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM files WHERE kind='file' AND deleted_at IS NULL`).
		Scan(&rep.TotalFiles)
	// 沉寂目录：最近 7 天无更新的直接子目录（有文件但 7 天未动）
	weekAgo := now.AddDate(0, 0, -7).UnixMilli()
	_ = d.db.QueryRowContext(ctx,
		`SELECT name FROM files WHERE kind='dir' AND deleted_at IS NULL
		 AND NOT EXISTS (SELECT 1 FROM files f2 WHERE f2.parent_id=files.id AND f2.updated_at>=? )
		 AND EXISTS (SELECT 1 FROM files f2 WHERE f2.parent_id=files.id)
		 ORDER BY updated_at ASC LIMIT 1`, weekAgo).
		Scan(&rep.InactiveDir)
	return rep, nil
}

// Render 渲染为 IM 文本。
func (d *DigestStore) Render(rep *DigestReport) string {
	var b strings.Builder
	b.WriteString("📚 知识库日报 · " + rep.Date + "\n\n")
	b.WriteString(fmt.Sprintf("▸ 今日入库 %d 篇", rep.InboxToday))
	if len(rep.InboxItems) > 0 {
		b.WriteString("：\n")
		for _, n := range rep.InboxItems {
			b.WriteString("  · " + n + "\n")
		}
	} else {
		b.WriteString("\n")
	}
	b.WriteString(fmt.Sprintf("▸ 未处理重复/冲突提醒 %d 条\n", rep.Conflicts))
	b.WriteString(fmt.Sprintf("▸ 今日新增评论 %d 条\n", rep.Comments))
	b.WriteString(fmt.Sprintf("▸ 知识库当前共 %d 个文件\n", rep.TotalFiles))
	if rep.InactiveDir != "" {
		b.WriteString(fmt.Sprintf("▸ 提醒：「%s」目录 7 天未更新，建议整理\n", rep.InactiveDir))
	}
	b.WriteString("\n回复「日报」可手动获取最新汇总。")
	return b.String()
}

// Send 推送到用户已绑定的 IM（B7：按 im_bindings 的 platform 分流）。
//
// 表在 B7 之前**并不存在**（`im_bindings` 只出现在上游抓取件里），
// 所以这条 SELECT 一直必然报错 —— 即「日报推送」是个从未成功过的死路径。B7 建表后打通。
func (d *DigestStore) Send(ctx context.Context, userID string) error {
	rep, err := d.Build(ctx, userID)
	if err != nil {
		return err
	}
	text := d.Render(rep)
	rows, err := d.db.QueryContext(ctx,
		`SELECT platform, platform_user_id, chat_id FROM im_bindings WHERE user_id=?`, userID)
	if err != nil {
		return err
	}
	defer rows.Close()
	var errs []string
	for rows.Next() {
		var platform, platformUserID, chatID string
		if err := rows.Scan(&platform, &platformUserID, &chatID); err != nil {
			continue
		}
		switch platform {
		case "telegram":
			// Telegram 主动推送必须有会话 id（chat_id）
			if chatID == "" {
				continue
			}
			token := d.cfg.GetString("im.telegram.bot_token")
			if token == "" {
				continue
			}
			if err := (&im.TelegramAdapter{}).Send(ctx, token, chatID, text); err != nil {
				errs = append(errs, platform+":"+err.Error())
			}
		case "wecom":
			// 企微：优先应用消息（可定向到个人 = 绑定的 userid），否则退回群机器人 webhook
			corpID, agentID := d.cfg.GetString("im.wecom.corp_id"), d.cfg.GetString("im.wecom.agent_id")
			secret := d.cfg.GetString("im.wecom.secret")
			var serr error
			if corpID != "" && agentID != "" && secret != "" && platformUserID != "" {
				serr = (&im.WeComAdapter{}).ReplyAppMessage(corpID, agentID, secret, platformUserID, text)
			} else if wh := d.cfg.GetString("im.wecom.webhook_url"); wh != "" {
				serr = (&im.WeComAdapter{}).SendText(wh, text)
			} else {
				continue
			}
			if serr != nil {
				errs = append(errs, platform+":"+serr.Error())
			}
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(errs) > 0 {
		return fmt.Errorf("digest send: %s", strings.Join(errs, "; "))
	}
	return nil
}
