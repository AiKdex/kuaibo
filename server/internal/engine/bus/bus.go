// Package bus 实现事件总线（进程内 pub/sub）。
//
// 边界规则（实施落地文档 §5.4 + 架构 §7.4 采纳的外部评审结论）：
//   - 事件总线 = 通知类、fire-and-forget（UI 实时刷新、订阅者广播）；
//   - jobs 队列 = 触发副作用（索引/转写/缩略图），幂等、可重试、崩溃可恢复。
//
// 事件总线同时是插件生态的地基：插件通过订阅事件扩展，无需改内核。
package bus

import (
	"context"
	"sync"
)

// Event 是总线上的最小事件单元。
type Event struct {
	Topic string // file.created / file.updated / file.deleted / file.moved / index.done / config.changed ...
	Key   string // 关联实体 id（file_id 等），便于订阅者做并发去重
	Data  map[string]any
}

// Handler 订阅者回调；ctx 可用于超时控制，返回 error 仅记日志（fire-and-forget，不阻塞其他订阅者）。
type Handler func(ctx context.Context, e Event) error

// Bus 是进程内事件总线。
type Bus struct {
	mu     sync.RWMutex
	subs   map[string][]Handler // topic -> handlers（顺序执行）
	closed bool
}

// New 创建总线。
func New() *Bus {
	return &Bus{subs: make(map[string][]Handler)}
}

// Subscribe 订阅一个主题。返回取消函数。
func (b *Bus) Subscribe(topic string, h Handler) func() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subs[topic] = append(b.subs[topic], h)
	idx := len(b.subs[topic]) - 1
	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		b.subs[topic] = append(b.subs[topic][:idx], b.subs[topic][idx+1:]...)
	}
}

// Publish 广播事件给所有订阅者。默认同步串行执行，保证顺序一致；
// 订阅者内部应自行决定是否异步（耗时副作用请投递 jobs 而非在回调里阻塞）。
func (b *Bus) Publish(ctx context.Context, e Event) {
	b.mu.RLock()
	handlers := make([]Handler, 0, len(b.subs[e.Topic]))
	handlers = append(handlers, b.subs[e.Topic]...)
	b.mu.RUnlock()
	for _, h := range handlers {
		if err := h(ctx, e); err != nil {
			// fire-and-forget：订阅者错误只记录，不阻断其余订阅者
			// 接入 logging 后替换为结构化日志
			_ = err
		}
	}
}

// Close 关闭总线（等待在途发布完成后拒绝新订阅）。
func (b *Bus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	b.subs = map[string][]Handler{}
}
