// im_binding.go IM 绑定服务（B7）：站内用户 ↔ IM 平台身份映射。
//
// 为什么需要：本壳 IM 网关原为「单用户阶段」——所有 IM 消息一律以系统 owner 身份执行，
// 既无法区分「谁发的」，也没法把日报/通知推给「他自己」的会话。`/bind` 曾是占位
// （映射到 IntentStatus，回复「无需绑定」）。本模块把它做成真实链路。
//
// 设计要点：
//   - **绑定码**由 Web 端（已登录）生成、IM 端回复 `/bind <码>` 消费 → 免在 IM 里输账号密码；
//     一次性、短时效；重复生成会作废同用户同平台未消费的旧码（保证「最新一个才有效」）。
//   - `(platform, platform_user_id)` 唯一：一个 IM 身份同时只对应一个站内用户。
//     拿新码再次绑定＝**改绑**（用户显式操作，允许转移），旧用户失去该身份。
//   - 时间戳一律**毫秒**（与本壳 files.*_at 口径一致）。
//   - 未绑定不报错：调用方（handler）回退到 owner/白名单，**零破坏**既有单用户部署。
package service

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	// IMBindCodeTTL 绑定码默认有效期。
	IMBindCodeTTL = 10 * time.Minute
	// imbCodeAlphabet 绑定码字符集：去掉 I/L/O/0/1 等易混字符，便于人工转述。
	imbCodeAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"
	imbCodeLen      = 6
	// IMBindFailLimit 同一 IM 身份连续失败次数阈值：达到即软锁定（B10 限流）。
	IMBindFailLimit = 5
	// IMBindLockWindow 软锁定时长：锁定期内一律拒绝，且不消耗任何绑定码。
	IMBindLockWindow = 15 * time.Minute
)

// IM 绑定错误（handler 据此给出可读回复，不把内部错误抛给 IM 用户）。
var (
	ErrBindCodeInvalid = errors.New("绑定码无效，请在网页端「IM 绑定」页重新生成")
	ErrBindCodeExpired = errors.New("绑定码已过期，请在网页端重新生成")
	ErrBindCodeUsed    = errors.New("绑定码已失效（已被使用或被新码替换），请在网页端重新生成")
	ErrBindNoIdentity  = errors.New("无法识别你的 IM 身份，请稍后重试")
	ErrBindNotFound    = errors.New("绑定不存在")
)

// BindLockedError 绑定尝试被限流锁定（B10）。带剩余时长，便于给 IM 用户可读回复；
// Error() 文案是**面向用户**的（handler 直接拼进回复），不是内部诊断信息。
type BindLockedError struct{ UntilMS int64 }

func (e *BindLockedError) Error() string {
	mins := int((e.UntilMS - time.Now().UnixMilli()) / 60000)
	if mins < 1 {
		mins = 1
	}
	return fmt.Sprintf("尝试次数过多，已暂时锁定，请在约 %d 分钟后重试", mins)
}

// IMBinding 一条站内用户与 IM 身份的绑定。
type IMBinding struct {
	ID             string `json:"id"`
	UserID         string `json:"user_id"`
	Platform       string `json:"platform"`
	PlatformUserID string `json:"platform_user_id"`
	PlatformName   string `json:"platform_name"`
	ChatID         string `json:"chat_id"`
	BoundAt        int64  `json:"bound_at"`
	LastActiveAt   int64  `json:"last_active_at"`
}

// IMBindingStore IM 绑定数据访问。
type IMBindingStore struct{ db *sql.DB }

// NewIMBindingStore 创建 IM 绑定仓储。
func NewIMBindingStore(db *sql.DB) *IMBindingStore { return &IMBindingStore{db: db} }

// newBindCode 生成一个绑定码（crypto/rand → 字符集映射）。
func newBindCode() (string, error) {
	buf := make([]byte, imbCodeLen)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	out := make([]byte, imbCodeLen)
	for i, v := range buf {
		out[i] = imbCodeAlphabet[int(v)%len(imbCodeAlphabet)]
	}
	return string(out), nil
}

// CreateCode 为指定用户生成绑定码；platform 为空表示可用于任意平台。
// 会先作废该用户同平台未消费的旧码。
func (s *IMBindingStore) CreateCode(ctx context.Context, userID, platform string, ttl time.Duration) (string, int64, error) {
	if strings.TrimSpace(userID) == "" {
		return "", 0, errors.New("im binding: empty user")
	}
	if ttl <= 0 {
		ttl = IMBindCodeTTL
	}
	now := time.Now().UnixMilli()
	if _, err := s.db.ExecContext(ctx,
		`UPDATE im_binding_codes SET used_at=? WHERE user_id=? AND platform=? AND used_at=0`,
		now, userID, platform); err != nil {
		return "", 0, err
	}
	exp := time.Now().Add(ttl).UnixMilli()
	// 主键冲突（撞码）概率极低，重试几次即可
	for i := 0; i < 5; i++ {
		code, err := newBindCode()
		if err != nil {
			return "", 0, err
		}
		if _, err := s.db.ExecContext(ctx,
			`INSERT INTO im_binding_codes (code, user_id, platform, expires_at, used_at) VALUES (?,?,?,?,0)`,
			code, userID, platform, exp); err == nil {
			return code, exp, nil
		}
	}
	return "", 0, errors.New("im binding: failed to allocate code")
}

// ConsumeCode 在 IM 侧消费绑定码，把 (platform, platformUserID) 绑到码的属主。
// BindLockState 查该 IM 身份是否处于失败锁定态，返回 (是否锁定, 解锁毫秒时间戳)。
// 锁定期已过时顺手清零计数（懒清理，不依赖定时任务）。
func (s *IMBindingStore) BindLockState(ctx context.Context, platform, platformUserID string) (bool, int64, error) {
	platform, platformUserID = strings.TrimSpace(platform), strings.TrimSpace(platformUserID)
	if platform == "" || platformUserID == "" {
		return false, 0, nil
	}
	var lockedUntil, failCount int64
	err := s.db.QueryRowContext(ctx,
		`SELECT locked_until, fail_count FROM im_bind_attempts WHERE platform=? AND platform_user_id=?`,
		platform, platformUserID).Scan(&lockedUntil, &failCount)
	if errors.Is(err, sql.ErrNoRows) {
		return false, 0, nil
	}
	if err != nil {
		return false, 0, err
	}
	now := time.Now().UnixMilli()
	if lockedUntil > now {
		return true, lockedUntil, nil
	}
	// 仅当"曾锁定且已过期"时才清零：locked_until==0 表示从未锁定（只是累积失败计数），
	// 这种情况必须保留计数以继续逼近阈值——否则每次检查都被清零，锁永远触发不了。
	if lockedUntil != 0 && lockedUntil <= now {
		// 锁定已过期：清零，重新给满额机会
		_, _ = s.db.ExecContext(ctx,
			`UPDATE im_bind_attempts SET fail_count=0, locked_until=0, updated_at=?
			 WHERE platform=? AND platform_user_id=?`, now, platform, platformUserID)
	}
	return false, 0, nil
}

// noteBindFailure 记一次失败；达到 IMBindFailLimit 即置软锁定。尽力而为，不影响主流程返回。
// 用 UPSERT 单语句完成「计数 + 判阈值」，避免先查后写的竞态。
func (s *IMBindingStore) noteBindFailure(ctx context.Context, platform, platformUserID string, now int64) {
	if platform == "" || platformUserID == "" {
		return
	}
	lockUntil := now + int64(IMBindLockWindow/time.Millisecond)
	_, _ = s.db.ExecContext(ctx,
		`INSERT INTO im_bind_attempts (platform, platform_user_id, fail_count, locked_until, updated_at)
		 VALUES (?,?,1,0,?)
		 ON CONFLICT(platform, platform_user_id) DO UPDATE SET
		   fail_count   = im_bind_attempts.fail_count + 1,
		   locked_until = CASE WHEN im_bind_attempts.fail_count + 1 >= ? THEN ? ELSE im_bind_attempts.locked_until END,
		   updated_at   = excluded.updated_at`,
		platform, platformUserID, now, IMBindFailLimit, lockUntil)
}

// clearBindFailures 绑定成功后清零该身份的失败计数（尽力而为）。
// 改绑场景同样清零：历史失败不应拖累这次成功的新绑定。
func (s *IMBindingStore) clearBindFailures(ctx context.Context, platform, platformUserID string) {
	if platform == "" || platformUserID == "" {
		return
	}
	_, _ = s.db.ExecContext(ctx,
		`DELETE FROM im_bind_attempts WHERE platform=? AND platform_user_id=?`, platform, platformUserID)
}

// 返回绑定的站内 user_id。
func (s *IMBindingStore) ConsumeCode(ctx context.Context, code, platform, platformUserID, platformName, chatID string) (string, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	platform = strings.TrimSpace(platform)
	platformUserID = strings.TrimSpace(platformUserID)
	if code == "" {
		return "", ErrBindCodeInvalid
	}
	if platform == "" || platformUserID == "" {
		return "", ErrBindNoIdentity
	}
	now := time.Now().UnixMilli()

	// B10 失败限流：先判该 IM 身份是否处于软锁定态（锁定期内直接拒绝，不消耗任何码）。
	// 按 (platform, platform_user_id) 计数而非 IP/全局：绑定码是 6 位短码，真正的暴力面是
	// 「某个 IM 身份反复试码」；按身份锁既挡住暴力尝试，也不会因共用出口把无关用户一起锁住。
	if locked, until, err := s.BindLockState(ctx, platform, platformUserID); err != nil {
		return "", err
	} else if locked {
		return "", &BindLockedError{UntilMS: until}
	}
	// fail 统一「记一次失败 + 返回该错误」。只对**码本身的问题**计数；
	// 无身份（ErrBindNoIdentity）属于调用方取不到身份，计了也没意义（下次换身份即绕过），故不计。
	fail := func(err error) (string, error) {
		s.noteBindFailure(ctx, platform, platformUserID, now)
		return "", err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()

	var uid, codePlat string
	var exp, used int64
	err = tx.QueryRowContext(ctx,
		`SELECT user_id, platform, expires_at, used_at FROM im_binding_codes WHERE code=?`, code).
		Scan(&uid, &codePlat, &exp, &used)
	if errors.Is(err, sql.ErrNoRows) {
		return fail(ErrBindCodeInvalid)
	}
	if err != nil {
		return "", err
	}
	if used != 0 {
		return fail(ErrBindCodeUsed)
	}
	if exp <= now {
		return fail(ErrBindCodeExpired)
	}
	// 码可限定平台；码未限定（platform=''）时任意平台可用
	if codePlat != "" && !strings.EqualFold(codePlat, platform) {
		return fail(ErrBindCodeInvalid)
	}
	// 防并发双用：只有把 used_at 从 0 改成 now 的那一次才算消费成功
	res, err := tx.ExecContext(ctx,
		`UPDATE im_binding_codes SET used_at=? WHERE code=? AND used_at=0`, now, code)
	if err != nil {
		return "", err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return fail(ErrBindCodeUsed)
	}
	// 落绑定；同 IM 身份已存在则**改绑**到新属主（用户显式操作）
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO im_bindings
		   (id, user_id, platform, platform_user_id, platform_name, chat_id, bound_at, last_active_at)
		 VALUES (?,?,?,?,?,?,?,?)
		 ON CONFLICT(platform, platform_user_id) DO UPDATE SET
		   user_id          = excluded.user_id,
		   platform_name    = excluded.platform_name,
		   chat_id          = CASE WHEN excluded.chat_id<>'' THEN excluded.chat_id ELSE im_bindings.chat_id END,
		   bound_at         = excluded.bound_at,
		   last_active_at   = excluded.last_active_at`,
		uuid.NewString(), uid, platform, platformUserID, platformName, chatID, now, now); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	s.clearBindFailures(ctx, platform, platformUserID)
	return uid, nil
}

// ResolveUser 按 IM 身份反查站内用户；未绑定返回 ok=false（调用方回退，不视为错误）。
func (s *IMBindingStore) ResolveUser(ctx context.Context, platform, platformUserID string) (string, bool) {
	platform, platformUserID = strings.TrimSpace(platform), strings.TrimSpace(platformUserID)
	if platform == "" || platformUserID == "" {
		return "", false
	}
	var uid string
	if err := s.db.QueryRowContext(ctx,
		`SELECT user_id FROM im_bindings WHERE platform=? AND platform_user_id=?`, platform, platformUserID).
		Scan(&uid); err != nil || uid == "" {
		return "", false
	}
	return uid, true
}

// ListByUser 列出某用户的全部绑定（新→旧）。
func (s *IMBindingStore) ListByUser(ctx context.Context, userID string) ([]IMBinding, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_id, platform, platform_user_id, platform_name, chat_id, bound_at, last_active_at
		 FROM im_bindings WHERE user_id=? ORDER BY bound_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []IMBinding{}
	for rows.Next() {
		var b IMBinding
		if err := rows.Scan(&b.ID, &b.UserID, &b.Platform, &b.PlatformUserID, &b.PlatformName,
			&b.ChatID, &b.BoundAt, &b.LastActiveAt); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// Unbind 解绑：只能删自己的绑定（按 id + user_id 双条件，防越权）。
func (s *IMBindingStore) Unbind(ctx context.Context, userID, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM im_bindings WHERE id=? AND user_id=?`, id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrBindNotFound
	}
	return nil
}

// UnbindIdentity 按 IM 身份解绑（IM 端 `/unbind` 用）。
func (s *IMBindingStore) UnbindIdentity(ctx context.Context, platform, platformUserID string) (string, error) {
	uid, ok := s.ResolveUser(ctx, platform, platformUserID)
	if !ok {
		return "", ErrBindNotFound
	}
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM im_bindings WHERE platform=? AND platform_user_id=?`, platform, platformUserID); err != nil {
		return "", err
	}
	return uid, nil
}

// Touch 更新最近活跃时间（尽力而为，失败不影响主流程）。
func (s *IMBindingStore) Touch(ctx context.Context, platform, platformUserID string) {
	if platform == "" || platformUserID == "" {
		return
	}
	_, _ = s.db.ExecContext(ctx,
		`UPDATE im_bindings SET last_active_at=? WHERE platform=? AND platform_user_id=?`,
		time.Now().UnixMilli(), platform, platformUserID)
}

// ListAll 列出全部绑定（管理员视图用；含 user_id 便于排查）。
func (s *IMBindingStore) ListAll(ctx context.Context, limit int) ([]IMBinding, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_id, platform, platform_user_id, platform_name, chat_id, bound_at, last_active_at
		 FROM im_bindings ORDER BY bound_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []IMBinding{}
	for rows.Next() {
		var b IMBinding
		if err := rows.Scan(&b.ID, &b.UserID, &b.Platform, &b.PlatformUserID, &b.PlatformName,
			&b.ChatID, &b.BoundAt, &b.LastActiveAt); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// PurgeUser 清理某用户的绑定与绑定码（用户注销时调用，预留）。
func (s *IMBindingStore) PurgeUser(ctx context.Context, userID string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM im_bindings WHERE user_id=?`, userID); err != nil {
		return fmt.Errorf("purge im_bindings: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM im_binding_codes WHERE user_id=?`, userID); err != nil {
		return fmt.Errorf("purge im_binding_codes: %w", err)
	}
	return nil
}

// PurgeStaleAttempts 清理长期未活动的失败计数（B10）。
// 正常路径下绑定成功即删行、不会堆积；这里兜底清理「试了几次就再也不来」的残留。
func (s *IMBindingStore) PurgeStaleAttempts(ctx context.Context, olderThan time.Duration) (int64, error) {
	if olderThan <= 0 {
		olderThan = 30 * 24 * time.Hour
	}
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM im_bind_attempts WHERE updated_at<?`, time.Now().Add(-olderThan).UnixMilli())
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// PurgeExpiredCodes 清理过期且未使用的绑定码（可由定时任务调用）。
func (s *IMBindingStore) PurgeExpiredCodes(ctx context.Context) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM im_binding_codes WHERE expires_at<? OR used_at>0`, time.Now().UnixMilli()-24*3600*1000)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}
