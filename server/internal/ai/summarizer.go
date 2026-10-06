// summarizer.go 入库 AI 解读：对文本文件异步生成要点摘要，结果写入 file_ai_summaries。
//
// 设计（回应"精读慢"处置策略）：
//   - 入库即解读：上传/索引后由事件总线触发入队，后台单 worker 串行分批消费
//     （天然限流，不会一次性打爆 LLM；批量导入时逐文件排队）。
//   - 区别对待：对话时 read_file 优先复用已入库摘要（秒回），未解读文件才临时读正文。
//   - 幂等续跑：重启后扫描 pending 继续；attempt 记录重试次数，超限标记 error。
//   - 大文件只取头部片段（source=head），控制每次 LLM 输入成本与延迟。
package ai

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/AiKMAP/AiKmap/server/internal/engine/bus"
	"github.com/AiKMAP/AiKmap/server/internal/service"
)

const (
	summarizeMaxBytes = 8 * 1024 // 单次解读最多读取的正文字节
	summarizeHeadRune = 2000     // 大文件取头部 rune 数
	maxAttempts       = 3        // 单文件最大尝试次数（之后标记 error 不再抢占）
)

// SummaryRow 一条解读结果（对外结构）。
type SummaryRow struct {
	FileID    string         `json:"file_id"`
	Status    string         `json:"status"`
	Summary   string         `json:"summary"`
	Tags      []string       `json:"tags"`
	Meta      map[string]any `json:"meta,omitempty"`
	Source    string         `json:"source"`
	UpdatedAt int64          `json:"updated_at"`
}

// Summarizer 后台解读队列。
type Summarizer struct {
	db          *sql.DB
	files       *service.FileStore
	tags        *service.TagStore
	gate        *Gateway
	wake        chan struct{}
	mu          sync.RWMutex
	exts        map[string]bool // 解读扩展名白名单（nil=全部文本）；小写含点，如 ".md"
	entityEdges bool            // K20 实体关系自动建边开关
}

// NewSummarizer 创建解读器。extsCfg 为逗号分隔扩展名白名单（如 ".md,.docx,.pdf"），
// 空字符串表示全部文本类型都解读。tags 用于 AI 自动打标（可为 nil 关闭）。
// entityEdges 控制 K20 实体关系自动建边（关闭则只建标签，不写 knowledge_edges 边）。
func NewSummarizer(db *sql.DB, files *service.FileStore, tags *service.TagStore, gate *Gateway, extsCfg string, entityEdges bool) *Summarizer {
	s := &Summarizer{db: db, files: files, tags: tags, gate: gate, wake: make(chan struct{}, 1), entityEdges: entityEdges}
	if extsCfg = strings.TrimSpace(extsCfg); extsCfg != "" {
		s.exts = map[string]bool{}
		for _, e := range strings.Split(extsCfg, ",") {
			e = strings.ToLower(strings.TrimSpace(e))
			if e != "" {
				if !strings.HasPrefix(e, ".") {
					e = "." + e
				}
				s.exts[e] = true
			}
		}
	}
	return s
}

// SetExts 运行时更新解读类型白名单（后台设置即时生效，无需重启）。
func (s *Summarizer) SetExts(extsCfg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.exts = nil
	if extsCfg = strings.TrimSpace(extsCfg); extsCfg != "" {
		s.exts = map[string]bool{}
		for _, e := range strings.Split(extsCfg, ",") {
			e = strings.ToLower(strings.TrimSpace(e))
			if e != "" {
				if !strings.HasPrefix(e, ".") {
					e = "." + e
				}
				s.exts[e] = true
			}
		}
	}
}

// skipFile 判断是否跳过解读：非文本类型跳过；白名单非空且扩展名不在白名单时跳过。
// ixExtMatch 文件名扩展名是否命中 exts（不含点，如 md/doc/pdf；不匹配返回 false）。
func ixExtMatch(name string, exts []string) bool {
	ext := strings.ToLower(strings.TrimPrefix(pathExt(name), "."))
	for _, e := range exts {
		if strings.EqualFold(strings.TrimPrefix(strings.TrimSpace(e), "."), ext) {
			return true
		}
	}
	return false
}

func (s *Summarizer) skipFile(mime, name string) bool {
	if !service.IndexableMIME(mime) {
		return true
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.exts) == 0 {
		return false
	}
	ext := strings.ToLower(pathExt(name))
	return !s.exts[ext]
}

// pathExt 取文件名扩展名（含点，无点返回 ""）。
func pathExt(name string) string {
	i := strings.LastIndexByte(name, '.')
	if i < 0 || i == len(name)-1 {
		return ""
	}
	return name[i:]
}

// Enqueue 入队（幂等：已存在则忽略；并发安全由主键保证）。非文本由 IndexFile 同口径过滤。
func (s *Summarizer) Enqueue(fileID string) {
	if fileID == "" {
		return
	}
	if _, err := s.db.Exec(
		`INSERT INTO file_ai_summaries (file_id, status, summary, tags, source, attempt, updated_at)
		 VALUES (?, 'pending', '', '[]', '', 0, ?)
		 ON CONFLICT(file_id) DO NOTHING`,
		fileID, time.Now().Unix()); err != nil {
		return // 记录日志后忽略（fire-and-forget）
	}
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// RetryErrors 清理孤儿解读记录并把真实 error 重置为 pending（attempt 归零）并唤醒 worker。
// 返回 (cleaned, retried)；用于知识库页"重试失败"——失败多为历史模型故障/文件已删，
// 孤儿记录（文件已不存在）直接清理，真实错误按当前配置重跑。
func (s *Summarizer) RetryErrors(ctx context.Context) (int64, int64, error) {
	// 1. 清理孤儿（文件已删除的残留解读记录）
	cleaned, err := s.db.ExecContext(ctx,
		`DELETE FROM file_ai_summaries WHERE file_id NOT IN (SELECT id FROM files)`)
	if err != nil {
		return 0, 0, err
	}
	nc, _ := cleaned.RowsAffected()
	// 2. 重置真实 error → pending
	res, err := s.db.ExecContext(ctx,
		`UPDATE file_ai_summaries SET status='pending', attempt=0, updated_at=? WHERE status='error'`,
		time.Now().Unix())
	if err != nil {
		return nc, 0, err
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		select {
		case s.wake <- struct{}{}:
		default:
		}
	}
	return nc, n, nil
}

// Run 启动后台 worker（阻塞；ctx 取消时退出）。串行消费 = 分批处理，天然限流。
func (s *Summarizer) Run(ctx context.Context) {
	for {
		if !s.drainOne(ctx) {
			select {
			case <-ctx.Done():
				return
			case <-s.wake:
			}
		}
	}
}

// drainOne 处理一条 pending；无任务返回 false（进入等待）。
func (s *Summarizer) drainOne(ctx context.Context) bool {
	row := s.db.QueryRowContext(ctx,
		`SELECT file_id, attempt FROM file_ai_summaries
		 WHERE status='pending' ORDER BY updated_at LIMIT 1`)
	var fileID string
	var attempt int
	if err := row.Scan(&fileID, &attempt); err != nil {
		return false
	}
	ctx2, cancel := context.WithTimeout(ctx, 300*time.Second)
	defer cancel()
	s.process(ctx2, fileID, attempt)
	return true
}

// process 处理单个文件解读。
func (s *Summarizer) process(ctx context.Context, fileID string, attempt int) {
	mark := func(status, summary, src, errMsg string, tags []string, meta map[string]any) {
		now := time.Now().Unix()
		tagsJSON := "[]"
		if len(tags) > 0 {
			if b, e := json.Marshal(tags); e == nil {
				tagsJSON = string(b)
			}
		}
		metaJSON := "{}"
		if len(meta) > 0 {
			if b, e := json.Marshal(meta); e == nil {
				metaJSON = string(b)
			}
		}
		if _, err := s.db.ExecContext(ctx,
			`UPDATE file_ai_summaries SET status=?, summary=?, source=?, tags=?, meta=?, last_error=?, updated_at=?
			 WHERE file_id=?`,
			status, summary, src, tagsJSON, metaJSON, errMsg, now, fileID); err != nil {
			return
		}
	}
	f, err := s.files.Get(ctx, fileID)
	if err != nil {
		mark("error", "", "", "文件不可用: "+err.Error(), nil, nil)
		return
	}
	// 兜底过滤（入队侧已过滤；防止手动/历史入队的漏网记录占队）
	if s.skipFile(f.Mime, f.Name) {
		_, _ = s.db.ExecContext(ctx,
			`DELETE FROM file_ai_summaries WHERE file_id=? AND status='pending'`, fileID)
		return
	}
	rc, _, err := s.files.Content(ctx, fileID)
	if err != nil {
		mark("error", "", "", err.Error(), nil, nil)
		return
	}
	defer rc.Close()
	data, err := io.ReadAll(io.LimitReader(rc, summarizeMaxBytes))
	if err != nil {
		mark("error", "", "", err.Error(), nil, nil)
		return
	}
	text := string(data)
	src := "full"
	if r := []rune(text); len(r) > summarizeHeadRune {
		text = string(r[:summarizeHeadRune])
		src = "head"
	}
	out, err := s.gate.SummarizeMeta(ctx, "文件名："+f.Name+"\n\n"+text)
	if err != nil {
		na := attempt + 1
		if na >= maxAttempts {
			mark("error", "", src, err.Error(), nil, nil)
			return
		}
		// 保留 pending 并累加 attempt，下一轮重试
		if _, e := s.db.ExecContext(ctx,
			`UPDATE file_ai_summaries SET attempt=?, last_error=?, updated_at=? WHERE file_id=?`,
			na, err.Error(), time.Now().Unix(), fileID); e == nil {
			s.wake <- struct{}{}
		}
		return
	}
	if out == nil || out.Summary == "" {
		out = &aiKnowledgeExtractEmpty
	}
	mark("done", out.Summary, src, "", out.Tags, metaToMap(out.Meta))
	// 自动打标（非阻断：失败不影响摘要入库）
	if s.tags != nil {
		// K20：AI 标签 + 抽取实体（实体/前缀归类）一并建树挂载
		tagPaths := append([]string{}, out.Tags...)
		for _, e := range out.Entities {
			if e.Name != "" {
				tagPaths = append(tagPaths, "实体/"+e.Name)
			}
		}
		if len(tagPaths) > 0 {
			s.applyAutoTags(ctx, fileID, tagPaths)
		}
		// K20：实体关系自动建边（AI 生成 tag→tag 边，按 file_id 幂等清理重建）
		if s.entityEdgesEnabled() && len(out.Relations) > 0 {
			s.applyEntityRelations(ctx, fileID, out.Relations)
		}
	}
}

// aiKnowledgeExtractEmpty 空抽取兜底（避免 nil 解引用）。
var aiKnowledgeExtractEmpty = KnowledgeExtract{Summary: "（文件内容为空或无可摘要内容）"}

// entityEdgesEnabled K20 实体关系建边开关（构造时注入，默认开启）。
func (s *Summarizer) entityEdgesEnabled() bool {
	return s.entityEdges
}

// metaToMap 把 MetaInfo 结构转为 map（空字段剔除，避免前端展示空串占位）。
func metaToMap(m MetaInfo) map[string]any {
	out := map[string]any{}
	if m.Title != "" {
		out["title"] = m.Title
	}
	if m.Author != "" {
		out["author"] = m.Author
	}
	if m.Source != "" {
		out["source"] = m.Source
	}
	if len(m.Keywords) > 0 {
		out["keywords"] = m.Keywords
	}
	if m.DocType != "" {
		out["doc_type"] = m.DocType
	}
	if m.Language != "" {
		out["language"] = m.Language
	}
	return out
}

// applyAutoTags 把 AI 提取的标签建树（支持 a/b 层级）并挂载到文件。失败静默（仅日志）。
func (s *Summarizer) applyAutoTags(ctx context.Context, fileID string, tagPaths []string) {
	owner := service.SystemOwnerID
	var ids []string
	for _, p := range tagPaths {
		p = strings.Trim(p, "/ ")
		if p == "" {
			continue
		}
		if id := s.ensureTagPath(ctx, owner, p); id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return
	}
	// 保留已手工打的标签，合并自动标签（读现有 → 并集 → 全量写）
	cur, err := s.tags.FileTags(ctx, fileID)
	if err == nil {
		seen := map[string]bool{}
		for _, id := range ids {
			seen[id] = true
		}
		for _, t := range cur {
			if !seen[t.ID] {
				ids = append(ids, t.ID)
			}
		}
	}
	_ = s.tags.SetFileTags(ctx, owner, fileID, ids)
}

// applyEntityRelations K20：实体关系自动建边（AI 生成 tag→tag 边）。
// 幂等：先按 (source='ai', file_id=本文件) 清旧边再写入；实体标签已由 applyAutoTags 建立，
// 此处只写 knowledge_edges（relation=谓词原文，weight=置信度，file_id=来源文件）。
func (s *Summarizer) applyEntityRelations(ctx context.Context, fileID string, rels []RelationInfo) {
	owner := service.SystemOwnerID
	_, _ = s.db.ExecContext(ctx,
		`DELETE FROM knowledge_edges WHERE source='ai' AND file_id=? AND relation NOT IN ('cites','related','tagged_as')`,
		fileID)
	for _, rl := range rels {
		subID := s.findTagPath(ctx, owner, "实体/"+rl.Subject)
		objID := s.findTagPath(ctx, owner, "实体/"+rl.Object)
		if subID == "" || objID == "" {
			continue // 实体标签未建成（超出 8 个上限等），跳过该边
		}
		_, _ = s.db.ExecContext(ctx,
			`INSERT OR IGNORE INTO knowledge_edges
			 (space_id, source_type, source_id, target_type, target_id, relation, weight, source, file_id, created_at)
			 VALUES (?, 'tag', ?, 'tag', ?, ?, ?, 'ai', ?, ?)`,
			service.SystemHomeSpaceID, subID, objID, rl.Predicate, rl.Weight, fileID, time.Now().Unix())
	}
}

// ensureTagPath 按路径逐级创建标签（如 音乐/粤语 先建 音乐 再建 粤语），返回末端标签 ID。
func (s *Summarizer) ensureTagPath(ctx context.Context, owner, path string) string {
	segs := strings.Split(path, "/")
	var parentID string
	cur := ""
	for _, seg := range segs {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		if cur == "" {
			cur = seg
		} else {
			cur = cur + "/" + seg
		}
		id := s.findTagPath(ctx, owner, cur)
		if id == "" {
			t, err := s.tags.Create(ctx, owner, seg, parentID)
			if err != nil {
				return "" // 冲突等：放弃本次路径
			}
			id = t.ID
		}
		parentID = id
	}
	return parentID
}

// findTagPath 按完整 path 查标签 ID（不存在返回空）。
func (s *Summarizer) findTagPath(ctx context.Context, owner, path string) string {
	var id string
	_ = s.db.QueryRowContext(ctx, `SELECT id FROM tags WHERE owner_id=? AND path=?`, owner, path).Scan(&id)
	return id
}

// Get 查询文件解读结果（无记录返回 nil）。
func (s *Summarizer) Get(ctx context.Context, fileID string) (*SummaryRow, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT file_id, status, summary, tags, meta, source, updated_at FROM file_ai_summaries WHERE file_id=?`, fileID)
	var r SummaryRow
	var tags, meta string
	if err := row.Scan(&r.FileID, &r.Status, &r.Summary, &tags, &meta, &r.Source, &r.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	_ = json.Unmarshal([]byte(tags), &r.Tags)
	if strings.TrimSpace(meta) != "" && meta != "{}" {
		_ = json.Unmarshal([]byte(meta), &r.Meta)
	}
	return &r, nil
}

// RequeueSpace 对空间内存量文本文件补解读入队（启动时调用）。
// 幂等：无记录则新增；已 error 的置回 pending 重试；done/pending 保持不动。非文本/白名单外跳过。
func (s *Summarizer) RequeueSpace(ctx context.Context, spaceID string) (int, error) {
	return s.RequeueSpaceByExts(ctx, spaceID, nil)
}

// RequeueSpaceByExts 把空间内未解读/解读失败的文件重排进解读队列（精读策略：批量分批后台 + 按扩展名筛选；
// 空 exts = 全部类型）。已解读(done)的文件不重跑（不浪费额度），error 状态重置重试。
func (s *Summarizer) RequeueSpaceByExts(ctx context.Context, spaceID string, exts []string) (int, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, mime, name FROM files
		 WHERE space_id=? AND kind='file' AND deleted_at IS NULL AND size>0 AND size<=?`,
		spaceID, int64(service.MaxIndexSize))
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id, mime, name string
		if err := rows.Scan(&id, &mime, &name); err != nil {
			return len(ids), err
		}
		if s.skipFile(mime, name) {
			continue
		}
		if len(exts) > 0 && !ixExtMatch(name, exts) {
			continue
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return len(ids), err
	}
	now := time.Now().Unix()
	n := 0
	for _, id := range ids {
		// 先确保行存在（文本类由 process 内再次校验）
		if _, err := s.db.ExecContext(ctx,
			`INSERT INTO file_ai_summaries (file_id, status, summary, tags, source, attempt, updated_at)
			 VALUES (?, 'pending', '', '[]', '', 0, ?)
			 ON CONFLICT(file_id) DO NOTHING`, id, now); err != nil {
			continue
		}
		// error 状态重置重试
		if _, err := s.db.ExecContext(ctx,
			`UPDATE file_ai_summaries SET status='pending', attempt=0, last_error=NULL, updated_at=?
			 WHERE file_id=? AND status='error'`, now, id); err == nil {
			// 命中则计数
		}
		n++
	}
	select {
	case s.wake <- struct{}{}:
	default:
	}
	return n, nil
}

// Subscribe 订阅事件总线：file.created → 入队。非文本/不在类型白名单不入队。
func (s *Summarizer) Subscribe(b *bus.Bus) func() {
	return b.Subscribe("file.created", func(_ context.Context, e bus.Event) error {
		if e.Key == "" {
			return nil
		}
		mime, _ := e.Data["mime"].(string)
		name, _ := e.Data["name"].(string)
		if s.skipFile(mime, name) {
			return nil // 非文本或类型不在解读白名单
		}
		s.Enqueue(e.Key)
		return nil
	})
}
