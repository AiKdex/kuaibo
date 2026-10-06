// types.go IM 网关统一消息结构（平台无关）。
package im

// Msg 内部统一消息（平台适配器解析后输出）。
type Msg struct {
	Platform string // telegram | wecom | ...
	MsgID    string
	MsgType  string // text | voice | image | file
	ChatID   string
	UserID   string
	UserName string
	Text     string // 文本内容（可能为空）
	FileURL  string // 平台文件标识（Telegram file_id）
	FileName string
	Caption  string // 图片/文件的说明文字
	LinkURL  string // 消息内 URL（文本消息中识别）
}

// Reply IM 回复。
type Reply struct {
	Text   string
	OK     bool   // 业务是否成功（webhook 监控用）
	FileID any    // 入库动作产生的文件 id（幂等记录用；非入库动作为 nil）
}

// Gateway 业务分发接口：由 handler 实现，避免包循环依赖。
// Dispatch 收到已路由的意图结果，返回 IM 回复。
type Dispatcher interface {
	Dispatch(msg *Msg, intent IntentResult) *Reply
}
