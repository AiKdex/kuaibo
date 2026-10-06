// Package ai 提供 AI 网关（embedding / LLM / 工具调用）。
// tokenpool.go 实现模型 API Key 池：多 key 轮替、失败切换、健康状态（实施文档 §12.3 配置域 + §13.3 网关调用）。
//
// 设计要点：
//   - 轮替策略：round-robin；失败 key 临时冷却（cooldown），冷却后自动恢复；
//   - 并发安全：原子计数器 + 互斥锁维护健康表；
//   - key 来源：config.AIProvider（providers: local / hosted），由配置中心注入，敏感项加密存储。
package ai

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// TokenPool 管理一组模型 API Key，按轮替 + 冷却策略分配。
type TokenPool struct {
	keys     []string
	next     atomic.Uint64
	cooldown time.Duration

	mu      sync.Mutex
	failAt  map[string]time.Time // key -> 下次可用时间（冷却中）
	failCnt map[string]int       // key -> 连续失败次数
	healthy map[string]bool      // key -> 最近健康标记（供 dashboard）
}

// NewTokenPool 创建 token 池。cooldown<=0 时默认 30s。
func NewTokenPool(keys []string, cooldown time.Duration) *TokenPool {
	if cooldown <= 0 {
		cooldown = 30 * time.Second
	}
	nonEmpty := make([]string, 0, len(keys))
	for _, k := range keys {
		if k != "" {
			nonEmpty = append(nonEmpty, k)
		}
	}
	return &TokenPool{
		keys:     nonEmpty,
		cooldown: cooldown,
		failAt:   map[string]time.Time{},
		failCnt:  map[string]int{},
		healthy:  map[string]bool{},
	}
}

// ErrNoKey 池为空。
var ErrNoKey = errors.New("tokenpool: no key available")

// Next 返回下一个可用 key（轮替 + 跳过冷却中）。所有 key 均冷却时返回 ErrNoKey。
func (p *TokenPool) Next() (string, error) {
	if len(p.keys) == 0 {
		return "", ErrNoKey
	}
	n := len(p.keys)
	start := int(p.next.Add(1)-1) % n
	for i := 0; i < n; i++ {
		idx := (start + i) % n
		k := p.keys[idx]
		p.mu.Lock()
		until, cooling := p.failAt[k]
		ok := p.healthy[k]
		p.mu.Unlock()
		if cooling && time.Now().Before(until) {
			continue
		}
		_ = ok
		return k, nil
	}
	return "", ErrNoKey
}

// ReportSuccess 标记 key 成功：清冷却、清零失败计数、记健康。
func (p *TokenPool) ReportSuccess(key string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.failAt, key)
	p.failCnt[key] = 0
	p.healthy[key] = true
}

// ReportFailure 标记 key 失败：连续失败计数，超阈值进入冷却。
func (p *TokenPool) ReportFailure(key string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.failCnt[key]++
	p.healthy[key] = false
	// 连续失败 2 次即冷却（可调；避免单 key 拖垮整体延迟）
	if p.failCnt[key] >= 2 {
		p.failAt[key] = time.Now().Add(p.cooldown)
	}
}

// Stats 返回池健康快照（供管理面板展示）。
type KeyStat struct {
	Key     string `json:"key"`
	Healthy bool   `json:"healthy"`
	Cooling bool   `json:"cooling"`
	Fails   int    `json:"fails"`
}

// Stats 输出所有 key 状态（key 脱敏：只留前 6 后 4）。
func (p *TokenPool) Stats() []KeyStat {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]KeyStat, 0, len(p.keys))
	for _, k := range p.keys {
		until, cooling := p.failAt[k]
		out = append(out, KeyStat{
			Key:     maskKey(k),
			Healthy: p.healthy[k],
			Cooling: cooling && time.Now().Before(until),
			Fails:   p.failCnt[k],
		})
	}
	return out
}

// Len 返回 key 数量。
func (p *TokenPool) Len() int { return len(p.keys) }

// maskKey 脱敏展示。
func maskKey(k string) string {
	if len(k) <= 10 {
		return "****"
	}
	return k[:6] + "****" + k[len(k)-4:]
}
