// im_binding_b10_test.go B10-3 绑定码失败限流（软锁定）：
// 连续失败达到阈值 → 该 IM 身份被锁 IMBindLockWindow；锁定期内拒绝且不消耗任何码；
// 绑定成功清零失败计数；clearBindFailures 解除锁定。
package service

import (
	"context"
	"errors"
	"testing"
	"time"
)

// 错码连击：IM 身份 tg-bad 反复试错码，第 5 次失败触发锁定；第 6 次直接返回 BindLockedError，
// 且锁定期间即便拿着**有效码**也消费不掉（不消耗码）。
func TestIMBindLockAfterFailLimit(t *testing.T) {
	ctx := context.Background()
	s, db := newIMBindStore(t)

	validCode, _, err := s.CreateCode(ctx, "u-alice", "telegram", time.Minute)
	if err != nil {
		t.Fatalf("CreateCode: %v", err)
	}

	const ident = "telegram"
	const pid = "tg-bad"
	// 前 5 次用错码（注意：码不命中 → ErrBindCodeInvalid，且每次计一次失败）
	for i := 1; i <= 5; i++ {
		_, e := s.ConsumeCode(ctx, "ZZZZZZ", ident, pid, "Bad", "c")
		if errors.Is(e, ErrBindCodeInvalid) {
			continue
		}
		t.Fatalf("第 %d 次错码应返 ErrBindCodeInvalid，得到 %v", i, e)
	}
	// 第 6 次：应被锁定（连 BindLockedError 都还没消费码）
	_, e := s.ConsumeCode(ctx, "ZZZZZZ", ident, pid, "Bad", "c")
	if _, ok := e.(*BindLockedError); !ok {
		t.Fatalf("第 6 次应返 BindLockedError，得到 %v", e)
	}
	// 锁定期间拿着有效码也消费不掉
	if _, e := s.ConsumeCode(ctx, validCode, ident, pid, "Bad", "c"); !errors.Is(e, ErrBindCodeInvalid) {
		// 注意：此处期望仍是 BindLockedError；ConsumeCode 在锁检查阶段就返回，不会走到码校验。
		if _, ok := e.(*BindLockedError); !ok {
			t.Fatalf("锁定期间有效码应被锁拒，得到 %v", e)
		}
	}
	// 有效码未被消耗：用另一个身份仍可成功消费
	if uid, e := s.ConsumeCode(ctx, validCode, "telegram", "tg-good", "Alice", "c"); e != nil || uid != "u-alice" {
		t.Fatalf("有效码应仍可用（被锁身份没消耗它）：uid=%q err=%v", uid, e)
	}
	// 锁定状态可见
	if locked, _, e := s.BindLockState(ctx, ident, pid); e != nil || !locked {
		t.Fatalf("BindLockState 应可见锁定：locked=%v err=%v", locked, e)
	}
	_ = db // 保留以备扩展
}

// 失败计数未达阈值前不会被锁；且一次成功绑定会清零历史失败计数。
func TestIMBindSuccessClearsFailures(t *testing.T) {
	ctx := context.Background()
	s, _ := newIMBindStore(t)

	code, _, err := s.CreateCode(ctx, "u-alice", "telegram", time.Minute)
	if err != nil {
		t.Fatalf("CreateCode: %v", err)
	}
	// 先错 3 次（未达阈值 5）
	for i := 0; i < 3; i++ {
		if _, e := s.ConsumeCode(ctx, "ZZZZZZ", "telegram", "tg-cleared", "X", "c"); !errors.Is(e, ErrBindCodeInvalid) {
			t.Fatalf("错码 %d 应返 ErrBindCodeInvalid，得到 %v", i, e)
		}
	}
	// 尚未锁定
	if locked, _, _ := s.BindLockState(ctx, "telegram", "tg-cleared"); locked {
		t.Fatal("3 次失败不应锁定（阈值 5）")
	}
	// 随后用有效码成功绑定
	if uid, e := s.ConsumeCode(ctx, code, "telegram", "tg-cleared", "X", "c"); e != nil || uid != "u-alice" {
		t.Fatalf("成功绑定失败：uid=%q err=%v", uid, e)
	}
	// 成功 → 失败计数清零（无锁定）
	if locked, _, e := s.BindLockState(ctx, "telegram", "tg-cleared"); e != nil || locked {
		t.Fatalf("成功后应无锁定：locked=%v err=%v", locked, e)
	}
	// 已绑定，该身份不可再消费别的码（被占用）
	if _, e := s.ConsumeCode(ctx, "ZZZZZZ", "telegram", "tg-cleared", "X", "c"); !errors.Is(e, ErrBindCodeInvalid) {
		t.Fatalf("已绑定身份再次消费应失败，得到 %v", e)
	}
}

// clearBindFailures 主动解锁：逼近阈值后清零，使下一轮又能从 0 计起。
func TestIMBindClearFailuresUnlocks(t *testing.T) {
	ctx := context.Background()
	s, _ := newIMBindStore(t)

	code, _, err := s.CreateCode(ctx, "u-alice", "telegram", time.Minute)
	if err != nil {
		t.Fatalf("CreateCode: %v", err)
	}
	// 连击 5 次触发锁定
	for i := 0; i < 5; i++ {
		_, _ = s.ConsumeCode(ctx, "ZZZZZZ", "telegram", "tg-clear", "X", "c")
	}
	if locked, _, _ := s.BindLockState(ctx, "telegram", "tg-clear"); !locked {
		t.Fatal("应处于锁定态")
	}
	// 主动清零
	s.clearBindFailures(ctx, "telegram", "tg-clear")
	if locked, _, e := s.BindLockState(ctx, "telegram", "tg-clear"); e != nil || locked {
		t.Fatalf("清零后不应锁定：locked=%v err=%v", locked, e)
	}
	// 清零后有效码可正常消费
	if uid, e := s.ConsumeCode(ctx, code, "telegram", "tg-clear", "X", "c"); e != nil || uid != "u-alice" {
		t.Fatalf("清零后消费失败：uid=%q err=%v", uid, e)
	}
}

// 无身份（空 platform/platform_user_id）不计失败、不限流，且不报错。
func TestIMBindNoIdentityNotCounted(t *testing.T) {
	ctx := context.Background()
	s, _ := newIMBindStore(t)

	if _, _, e := s.BindLockState(ctx, "", "x"); e != nil {
		t.Fatalf("空 platform 不应报错：%v", e)
	}
	s.noteBindFailure(ctx, "", "x", time.Now().UnixMilli()) // 应静默跳过
	if locked, _, e := s.BindLockState(ctx, "", "x"); e != nil || locked {
		t.Fatalf("空身份不应锁定：locked=%v err=%v", locked, e)
	}
}
