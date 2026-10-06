// concept_summarizer.go 概念页 AI 摘要（A09+A5，B27 自上游 2d12a42/b0fc988 移植）：
// 聚合标签下文件摘要 → 概念级概述，结果写入 kb_concept_summaries 缓存
// （避免每次打开概念页调模型；文件解读变化后可重新生成覆盖）；
// sources 记录摘要依据的来源文档引用链（A5，前端可回链）。
package ai

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ConceptSummaryItem 概念下单个文件的摘要素材（file_id=回链定位，name=文件名，summary=已生成的文件解读）。
type ConceptSummaryItem struct {
	FileID  string `json:"file_id"`
	Name    string `json:"name"`
	Summary string `json:"summary"`
}

const (
	conceptMaxItems      = 20   // 最多聚合的文件数（保成本）
	conceptMaxInputRune  = 6000 // 输入材料总长上限（rune），超长截断
)

// conceptSummarizeSystem 概念页概述指令：综合子文档标题与摘要，产出一段概念级概述。
const conceptSummarizeSystem = `你是知识库的概念页编辑。下面是一个概念标签下的若干文档标题与要点摘要。
请综合这些材料输出一段概念级概述（中文，150-300 字）：
1. 概括这个概念的核心内涵与整体知识范围；
2. 覆盖主要子主题，以及文档之间的关联（如有明显分工、演进或互补关系请点出）；
3. 只使用材料中出现的信息，不编造、不评价材料质量、不重复罗列文件名；
4. 只输出概述正文本身，不要标题、编号、引号或任何其他文字。`

// ConceptSummarizer 概念页 AI 摘要生成器（缓存读写在 kb_concept_summaries）。
type ConceptSummarizer struct {
	db   *sql.DB
	gate *Gateway
}

// NewConceptSummarizer 创建概念摘要器。gate 为 nil 时所有调用返回明确错误（模块未配置）。
func NewConceptSummarizer(db *sql.DB, gate *Gateway) *ConceptSummarizer {
	return &ConceptSummarizer{db: db, gate: gate}
}

// Get 读缓存摘要（无记录返回空串与 0；概念页回退路径用）。
func (s *ConceptSummarizer) Get(ctx context.Context, tagID string) (string, int64) {
	var sum string
	var upd int64
	if s.db == nil {
		return "", 0
	}
	_ = s.db.QueryRowContext(ctx,
		`SELECT summary, updated_at FROM kb_concept_summaries WHERE tag_id=?`, tagID).Scan(&sum, &upd)
	return sum, upd
}

// SummarizeConcept 综合概念下文件摘要生成概念概述并写缓存。返回 (摘要, updated_at_ms, error)。
// items 为空或全部无摘要时返回明确错误（前端提示先为文件生成解读）。
func (s *ConceptSummarizer) SummarizeConcept(ctx context.Context, tagID, tagName string, items []ConceptSummaryItem) (string, int64, error) {
	if s.gate == nil {
		return "", 0, errors.New("AI 服务未配置")
	}
	if len(items) == 0 {
		return "", 0, errors.New("该概念下没有文件，无法生成概念摘要")
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "概念：%s\n\n文档材料：\n", tagName)
	srcs := []map[string]string{} // A5 来源引用链：实际聚合进输入的文件
	n := 0
	for _, it := range items {
		it.Summary = strings.TrimSpace(it.Summary)
		it.Name = strings.TrimSpace(it.Name)
		if it.Summary == "" || it.Name == "" {
			continue
		}
		if n >= conceptMaxItems {
			break
		}
		fmt.Fprintf(&sb, "%d. %s：%s\n", n+1, it.Name, it.Summary)
		if it.FileID != "" {
			srcs = append(srcs, map[string]string{"file_id": it.FileID, "name": it.Name})
		}
		n++
	}
	if n == 0 {
		return "", 0, errors.New("该概念下文件均无可用解读，请先为文件生成解读")
	}
	text := sb.String()
	if r := []rune(text); len(r) > conceptMaxInputRune {
		text = string(r[:conceptMaxInputRune])
	}
	res, err := s.gate.ChatJSON(ctx, []Msg{
		{Role: "system", Content: conceptSummarizeSystem},
		{Role: "user", Content: text},
	}, nil)
	if err != nil {
		return "", 0, err
	}
	if res.Error != "" || strings.TrimSpace(res.Content) == "" {
		return "", 0, errors.New("AI 服务不可用：" + res.Error)
	}
	out := strings.TrimSpace(res.Content)
	now := time.Now().UnixMilli()
	srcsJSON, _ := json.Marshal(srcs)
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO kb_concept_summaries (tag_id, summary, sources, updated_at) VALUES (?,?,?,?)
		 ON CONFLICT(tag_id) DO UPDATE SET summary=excluded.summary, sources=excluded.sources, updated_at=excluded.updated_at`,
		tagID, out, string(srcsJSON), now); err != nil {
		return "", 0, err
	}
	return out, now, nil
}
