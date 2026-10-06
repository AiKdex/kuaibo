// Package ai 测试。
// tokenpool_test.go 覆盖：轮替、失败冷却、恢复、空池、脱敏。
package ai

import (
	"testing"
	"time"
)

func TestTokenPoolRoundRobin(t *testing.T) {
	p := NewTokenPool([]string{"k1", "k2", "k3"}, 0)
	got := map[string]int{}
	for i := 0; i < 6; i++ {
		k, err := p.Next()
		if err != nil {
			t.Fatalf("next: %v", err)
		}
		got[k]++
	}
	for _, k := range []string{"k1", "k2", "k3"} {
		if got[k] != 2 {
			t.Fatalf("key %s count = %d, want 2", k, got[k])
		}
	}
}

func TestTokenPoolCooldownAndRecovery(t *testing.T) {
	p := NewTokenPool([]string{"k1", "k2"}, 50*time.Millisecond)
	// k1 连续失败 2 次 → 冷却
	k1, _ := p.Next()
	p.ReportFailure(k1)
	k2, _ := p.Next()
	if k2 != "k2" && k2 != "k1" {
		t.Fatalf("unexpected key %s", k2)
	}
	// 主动失败 k1 第二次（模拟刚才那次也是 k1）
	p.ReportFailure(k1)
	// 冷却期内：应只能拿到 k2
	for i := 0; i < 3; i++ {
		k, err := p.Next()
		if err != nil {
			t.Fatalf("next during cooldown: %v", err)
		}
		if k != "k2" {
			t.Fatalf("during cooldown got %s, want k2", k)
		}
	}
	// k2 成功后保持健康；等 k1 冷却结束应恢复轮替
	p.ReportSuccess(k2)
	time.Sleep(80 * time.Millisecond)
	seen := map[string]bool{}
	for i := 0; i < 4; i++ {
		k, _ := p.Next()
		seen[k] = true
	}
	if !seen["k1"] {
		t.Fatal("k1 should recover after cooldown")
	}
	stats := p.Stats()
	for _, s := range stats {
		if s.Healthy && s.Fails != 0 {
			t.Fatalf("healthy key should have 0 fails: %+v", s)
		}
	}
}

func TestTokenPoolEmptyAndMask(t *testing.T) {
	p := NewTokenPool(nil, 0)
	if _, err := p.Next(); err != ErrNoKey {
		t.Fatalf("want ErrNoKey, got %v", err)
	}
	p2 := NewTokenPool([]string{"sk-abcdef1234567890XYZ"}, 0)
	st := p2.Stats()
	if len(st) != 1 {
		t.Fatalf("stats len = %d", len(st))
	}
	if st[0].Key == "sk-abcdef1234567890XYZ" || len(st[0].Key) < 8 {
		t.Fatalf("key not masked: %q", st[0].Key)
	}
}
