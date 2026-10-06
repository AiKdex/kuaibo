package bus

// 事件契约 v1 内核 topic 注册表（docs/事件总线契约.md）。
// 插件声明 hooks.subscribe/publish 与 WebhookHub 订阅都必须命中本注册表
// （或带 {plugin_id}. 前缀的自有 topic），防任意 topic 订阅/发布。

// KernelTopics 内核冻结 topic 清单（只增不减；新增视为 minor bump）。
var KernelTopics = map[string]bool{
	"file.created":       true,
	"file.updated":       true,
	"file.moved":         true,
	"file.deleted":       true,
	"file.restored":      true,
	"file.purged":        true,
	"file.status":        true,
	"file.tagged":        true,
	"file.ingested":      true, // 评论收录进正文（B3）：quote 直写 / fuse 采纳 / revert 撤销
	"tag.created":        true,
	"tag.renamed":        true,
	"tag.deleted":        true,
	"comment.created":    true, // 评论产生（登录评论即时；访客评论=进入待审）
	"comment.approved":   true, // 访客评论审核通过
	"config.changed":     true,
	"impex.done":         true,
	"impex.failed":       true,
	"vector.indexed":     true,
	"collector.finished": true,
}

// ValidateTopic 校验 topic 是否可订阅/发布：
//   - 内核冻结清单内，或
//   - 插件自有命名空间 {plugin_id}.{event}（pluginID 非空时）
func ValidateTopic(topic, pluginID string) bool {
	if KernelTopics[topic] {
		return true
	}
	if pluginID != "" && len(topic) > len(pluginID)+1 {
		prefix := topic[:len(pluginID)+1]
		return prefix == pluginID+"."
	}
	return false
}
