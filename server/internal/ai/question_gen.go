// question_gen.go 预生成问题索引（WeKnora 借鉴 A4，B27 自上游 f1555e0 移植并适配本仓队列模式）：
// 文档入库自动生成 Q&A 变体作为检索单元。
//
// 设计（复用 A2 FAQ 结构 + Summarizer 范式）：
//   - 入库即生成：订阅 file.created → 内部串行队列（本仓无 engine/jobs，改用 Summarizer 同款
//     wake 通道 + 内存去重；上游 jobs 幂等由「同文件先删旧再插」兜底）；
//   - AI 从文档内容提取 3-5 组高频疑问（问题+答案片段），写入 kb_faq（source=import:{file_id}）；
//   - 效果：检索"问题"即可命中文档知识，FAQ 检索通道覆盖文档内容；
//   - 开关：ai.question_gen.enabled=false 关闭（缺省开启）；扩展名白名单沿用 ai.summarize.exts。
package ai

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/AiKMAP/AiKmap/server/internal/engine/bus"
	"github.com/AiKMAP/AiKmap/server/internal/service"
	"github.com/google/uuid"
)

const (
	qgenMaxBytes = 8 * 1024
	qgenHeadRune = 1500
)

// QGenItem 一条生成的问题索引（LLM 输出 JSON 结构）。
type QGenItem struct {
	Question string   `json:"question"`
	Answer   string   `json:"answer"`
	Tags     []string `json:"tags,omitempty"`
}

// QuestionGen 预生成问题索引器（file.created → 串行后台生成）。
type QuestionGen struct {
	db       *sql.DB
	files    *service.FileStore
	kb       *service.KBStore
	gate     *Gateway
	exts     map[string]bool // nil=全部文本
	enabled  bool
	wake     chan struct{}
	mu       sync.Mutex
	inflight map[string]bool // 处理中去重（替代上游 jobs 幂等）
}

// NewQuestionGen 创建问题索引器。
// enabled 缺省开（false 显式关闭）；extsCfg 同 Summarizer 白名单格式。
func NewQuestionGen(db *sql.DB, files *service.FileStore, kb *service.KBStore, gate *Gateway, extsCfg string, enabled bool) *QuestionGen {
	q := &QuestionGen{db: db, files: files, kb: kb, gate: gate, enabled: enabled,
		wake: make(chan struct{}, 1), inflight: map[string]bool{}}
	if c := strings.TrimSpace(extsCfg); c != "" {
		q.exts = map[string]bool{}
		for _, e := range strings.Split(c, ",") {
			e = strings.ToLower(strings.TrimSpace(e))
			if e != "" {
				if !strings.HasPrefix(e, ".") {
					e = "." + e
				}
				q.exts[e] = true
			}
		}
	}
	if enabled {
		go q.worker()
	}
	return q
}

// Enqueue 入队（内存去重；enabled 关闭时空转）。
func (q *QuestionGen) Enqueue(fileID string) {
	if fileID == "" || !q.enabled {
		return
	}
	q.mu.Lock()
	if q.inflight[fileID] {
		q.mu.Unlock()
		return
	}
	q.inflight[fileID] = true
	q.mu.Unlock()
	select {
	case q.wake <- struct{}{}:
	default:
	}
}

// Subscribe 订阅 file.created：文本文件入队预生成问题索引。
func (q *QuestionGen) Subscribe(b *bus.Bus) func() {
	return b.Subscribe("file.created", func(_ context.Context, e bus.Event) error {
		if !q.enabled || e.Key == "" {
			return nil
		}
		mime, _ := e.Data["mime"].(string)
		name, _ := e.Data["name"].(string)
		if q.skipFile(mime, name) {
			return nil
		}
		q.Enqueue(e.Key)
		return nil
	})
}

// worker 串行消费队列（Summarizer 同款 wake 模式）。
func (q *QuestionGen) worker() {
	for range q.wake {
		for {
			q.mu.Lock()
			var fid string
			for id := range q.inflight {
				fid = id
				break
			}
			if fid == "" {
				q.mu.Unlock()
				break
			}
			delete(q.inflight, fid)
			q.mu.Unlock()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			_ = q.process(ctx, fid)
			cancel()
		}
	}
}

// process 生成问题索引（失败仅记日志，不阻塞主流程）。
func (q *QuestionGen) process(ctx context.Context, fileID string) error {
	if !q.enabled {
		return nil
	}
	f, err := q.files.Get(ctx, fileID)
	if err != nil || f == nil || f.Kind != "file" {
		return nil // 文件不可用：放弃
	}
	if q.skipFile(f.Mime, f.Name) {
		return nil
	}
	rc, _, err := q.files.Content(ctx, fileID)
	if err != nil {
		return errors.New("read content: " + err.Error())
	}
	defer rc.Close()
	data, err := io.ReadAll(io.LimitReader(rc, qgenMaxBytes))
	if err != nil {
		return errors.New("read body: " + err.Error())
	}
	text := string(data)
	if r := []rune(text); len(r) > qgenHeadRune {
		text = string(r[:qgenHeadRune])
	}
	if strings.TrimSpace(text) == "" {
		return nil
	}
	items, err := q.generate(ctx, f.Name, text)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return nil
	}
	// 同文件先删旧再插（source=import:{file_id} 幂等，防陈旧问题索引残留）
	if _, err := q.db.ExecContext(ctx,
		`DELETE FROM kb_faq WHERE space_id=? AND source=?`, f.SpaceID, "import:"+fileID); err != nil {
		return err
	}
	now := time.Now().UnixMilli()
	for _, it := range items {
		if it.Question == "" {
			continue
		}
		if it.Answer == "" {
			it.Answer = it.Question // 兜底：无答案时问题作答案，避免空条目
		}
		tagsJSON := "[]"
		if len(it.Tags) > 0 {
			if b, e := json.Marshal(it.Tags); e == nil {
				tagsJSON = string(b)
			}
		}
		if _, err := q.db.ExecContext(ctx,
			`INSERT INTO kb_faq(id, space_id, standard_q, similar_qs, counter_qs, answer, tags, source, created_by, created_at, updated_at)
			 VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
			uuid.NewString(), f.SpaceID, it.Question, "[]", "[]", it.Answer, tagsJSON,
			"import:"+fileID, f.OwnerID, now, now); err != nil {
			return err
		}
	}
	return nil
}

// generate 调 LLM 生成 Q&A 变体（输出 JSON 数组；失败返回错误）。
func (q *QuestionGen) generate(ctx context.Context, name, text string) ([]QGenItem, error) {
	prompt := `你是知识库问题索引器。根据下面的文档内容，提取读者最可能提出的 3-5 个问题并给出简短准确的答案。
要求：
- 每个问题必须能从文档内容直接回答，不编造文档外信息；
- 问题要覆盖文档关键信息点（定义、操作步骤、配置项、注意事项等）；
- 答案控制在 80 字以内，保留关键术语；
- 输出严格为 JSON 数组：[{"question":"...","answer":"...","tags":["..."]}]
只输出 JSON，不要任何其他文字。

文档名：` + name + `

文档内容：
` + text

	res, err := q.gate.ChatJSON(ctx, []Msg{
		{Role: "system", Content: "你只输出合法 JSON。"},
		{Role: "user", Content: prompt},
	}, nil)
	if err != nil {
		return nil, err
	}
	if res.Error != "" {
		return nil, errors.New(res.Error)
	}
	content := strings.TrimSpace(res.Content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)
	// 提取第一个 [ ... ] 区间（防模型输出前后缀）
	if i := strings.IndexByte(content, '['); i >= 0 {
		if j := strings.LastIndexByte(content, ']'); j > i {
			content = content[i : j+1]
		}
	}
	var items []QGenItem
	if err := json.Unmarshal([]byte(content), &items); err != nil {
		return nil, nil // 非 JSON：视为无可用输出，不重试（避免死循环烧 token）
	}
	out := make([]QGenItem, 0, len(items))
	for _, it := range items {
		if strings.TrimSpace(it.Question) != "" {
			out = append(out, it)
		}
	}
	return out, nil
}

// skipFile 扩展名白名单过滤（nil=全部文本）。
func (q *QuestionGen) skipFile(mime, name string) bool {
	if q.exts == nil {
		return !strings.HasPrefix(mime, "text/")
	}
	return !q.exts[strings.ToLower(nameExt(name))]
}

// nameExt 取小写扩展名（含点；无扩展名返回空）。
func nameExt(name string) string {
	i := strings.LastIndexByte(name, '.')
	if i < 0 {
		return ""
	}
	return name[i:]
}
