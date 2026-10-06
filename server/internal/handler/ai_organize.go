// Package handler 的 ai_organize.go 实现 AI 整理/归类/冲突检测（P0-3）。
// 核心：文件入库后可一键"AI 整理"——LLM 给出摘要+建议目录+建议标签；
// 向量检索同主题文件做冲突检测（高相似=疑似重复，中相似=同主题）；
// 用户确认后一键应用（建目录+移动+打标签，标签自动建树）。
// 商业价值点：AI 动作（摘要/归类/冲突）作为付费工具轮数计费的基础能力。
package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/AiKMAP/AiKmap/server/internal/ai"
	"github.com/AiKMAP/AiKmap/server/internal/service"
)

// conflictItem 相似/冲突文件。
type conflictItem struct {
	FileID string  `json:"file_id"`
	Name   string  `json:"name"`
	Path   string  `json:"path"`
	Score  float64 `json:"score"`          // 余弦相似度 0..1
	Reason string  `json:"reason"`         // duplicate=疑似重复（≥0.78）| similar=同主题（≥0.6）
	Note   string  `json:"note,omitempty"` // 一句话说明（AI 给出）
}

// organizeSuggestion 单文件整理建议。
type organizeSuggestion struct {
	FileID    string         `json:"file_id"`
	Name      string         `json:"name"`
	Kind      string         `json:"kind"`
	Textable  bool           `json:"textable"` // 文本类（AI 精读）；非文本只做冲突检测
	Summary   string         `json:"summary,omitempty"`
	DirPath   string         `json:"dir_path,omitempty"` // 建议目录（/分隔层级，如 项目A/文档）
	Tags      []string       `json:"tags,omitempty"`
	Conflicts []conflictItem `json:"conflicts,omitempty"`
	Error     string         `json:"error,omitempty"`
}

// organizeSuggestBatch POST /api/v1/ai/organize/suggest {"file_ids":["..."]}
// 批量整理建议：逐文件处理，单个失败不阻塞整体（error 字段标记）。
func (a *API) organizeSuggestBatch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FileIDs []string `json:"file_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "ORGANIZE_BAD_REQ", err.Error())
		return
	}
	if len(req.FileIDs) == 0 {
		writeErr(w, http.StatusBadRequest, "ORGANIZE_NO_FILES", "缺少 file_ids")
		return
	}
	if len(req.FileIDs) > 20 {
		writeErr(w, http.StatusBadRequest, "ORGANIZE_TOO_MANY", "单次最多 20 个文件")
		return
	}
	ctx := r.Context()
	out := make([]organizeSuggestion, 0, len(req.FileIDs))
	for _, id := range req.FileIDs {
		sug, err := a.organizeSuggestOne(ctx, id)
		if err != nil {
			sug = organizeSuggestion{FileID: id, Error: err.Error()}
		}
		out = append(out, sug)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

// organizeSuggestOne 单文件整理建议：摘要 + 建议目录/标签（LLM）+ 相似冲突（向量）。
func (a *API) organizeSuggestOne(ctx context.Context, fileID string) (organizeSuggestion, error) {
	f, err := a.files.Get(ctx, fileID)
	if err != nil || f == nil {
		return organizeSuggestion{}, fmt.Errorf("文件不存在")
	}
	sug := organizeSuggestion{FileID: f.ID, Name: f.Name, Kind: f.Kind}
	if f.Kind != "file" {
		return sug, nil // 目录不做整理
	}
	textable := isTextLike(f.Mime, f.Name)
	sug.Textable = textable

	// 1) 内容（文本类截断喂 LLM；向量冲突检测用前 2000 字）
	text := ""
	if textable {
		if rc, _, cerr := a.files.Content(ctx, f.ID); cerr == nil {
			if b, _ := io.ReadAll(io.LimitReader(rc, 32<<10)); len(b) > 0 {
				text = string(b)
			}
			rc.Close()
		}
	}
	vecText := text
	if len([]rune(vecText)) > 2000 {
		r := []rune(vecText)
		vecText = string(r[:2000])
	}

	// 2) 冲突检测：向量检索相似文件（排除自身）
	sug.Conflicts = a.organizeConflicts(ctx, f, vecText)

	// 3) LLM 整理：摘要 + 建议目录/标签（文本类且内容非空才调，省 token）
	if textable && strings.TrimSpace(text) != "" {
		if m := a.ai.EmbeddingModel(); m != "" || a.ai.CurrentModel("llm") != "" {
			out, err := a.organizeLLM(ctx, text)
			if err == nil {
				sug.Summary = out.Summary
				sug.DirPath = out.DirPath
				sug.Tags = out.Tags
			}
		}
	}
	return sug, nil
}

// organizeConflicts 向量检索相似文件，按相似度分档。
func (a *API) organizeConflicts(ctx context.Context, f *service.File, text string) []conflictItem {
	if text == "" {
		return nil
	}
	model := a.ai.EmbeddingModel()
	if model == "" {
		return nil
	}
	hits, err := ai.VectorSearch(ctx, a.db, a.ai, model, f.SpaceID, text, 6)
	if err != nil {
		return nil
	}
	out := make([]conflictItem, 0, len(hits))
	for _, h := range hits {
		if h.FileID == f.ID || h.Score < 0.6 {
			continue
		}
		reason := "similar"
		note := "同主题"
		if h.Score >= 0.78 {
			reason = "duplicate"
			note = "内容高度相似，疑似重复"
		}
		name := h.FileID
		if t, err := a.files.Get(ctx, h.FileID); err == nil && t != nil {
			name = t.Name
		}
		out = append(out, conflictItem{
			FileID: h.FileID, Name: name, Path: a.filePath(ctx, h.FileID, ""),
			Score: round4(h.Score), Reason: reason, Note: note,
		})
	}
	return out
}

// organizeLLM 调 LLM 输出整理建议（JSON）。
func (a *API) organizeLLM(ctx context.Context, text string) (*organizeAIOut, error) {
	system := `你是一个知识库整理助手。阅读文件内容，输出且只输出一个 JSON 对象，不要输出任何其他文字：
{"summary":"2-3 句要点摘要，覆盖主要内容与结论，不添加原文没有的信息","dir_path":"建议存放目录，用 / 分隔层级（最多 3 层，如 项目A/文档；不确定就返回空字符串）","tags":["2-4 个简短分类标签"]}
约束：dir_path 不含文件名、不以 / 开头结尾；tags 每个 2-6 字，可含层级（用 / 分隔），不含文件扩展名；宁缺毋滥。`
	res, err := a.ai.ChatJSON(ctx, []ai.Msg{
		{Role: "system", Content: system},
		{Role: "user", Content: text},
	}, nil)
	if err != nil || res == nil {
		return nil, fmt.Errorf("AI 整理服务暂不可用")
	}
	return parseOrganizeJSON(res.Content)
}

// organizeAIOut LLM 输出结构。
type organizeAIOut struct {
	Summary string   `json:"summary"`
	DirPath string   `json:"dir_path"`
	Tags    []string `json:"tags"`
}

// parseOrganizeJSON 容忍围栏与前后缀地提取 JSON。
func parseOrganizeJSON(out string) (*organizeAIOut, error) {
	s := strings.TrimSpace(out)
	if i := strings.Index(s, "```"); i >= 0 {
		j := strings.Index(s[i+3:], "```")
		if j >= 0 {
			block := s[i+3 : i+3+j]
			if k := strings.IndexByte(block, '\n'); k >= 0 {
				block = block[k+1:]
			}
			s = strings.TrimSpace(block)
		}
	}
	start, end := strings.IndexByte(s, '{'), strings.LastIndexByte(s, '}')
	if start < 0 || end <= start {
		return nil, fmt.Errorf("organize: no json object")
	}
	var r organizeAIOut
	if err := json.Unmarshal([]byte(s[start:end+1]), &r); err != nil {
		return nil, err
	}
	r.Summary = strings.TrimSpace(r.Summary)
	r.DirPath = strings.Trim(strings.TrimSpace(r.DirPath), "/")
	tags := r.Tags[:0]
	for _, t := range r.Tags {
		t = strings.TrimSpace(t)
		if t != "" {
			tags = append(tags, t)
		}
	}
	r.Tags = tags
	return &r, nil
}

// organizeApplyReq 应用整理建议。
type organizeApplyReq struct {
	FileID   string   `json:"file_id"`
	DirPath  string   `json:"dir_path,omitempty"` // 建议目录（/分隔），空=不移动
	Tags     []string `json:"tags,omitempty"`     // 建议标签（层级路径自动建树）
	SkipMove bool     `json:"skip_move,omitempty"`
}

// organizeApply POST /api/v1/ai/organize/apply：一键应用（建目录+移动+打标签）。
func (a *API) organizeApply(w http.ResponseWriter, r *http.Request) {
	var req organizeApplyReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "ORGANIZE_BAD_REQ", err.Error())
		return
	}
	if req.FileID == "" {
		writeErr(w, http.StatusBadRequest, "ORGANIZE_NO_FILE", "缺少 file_id")
		return
	}
	ctx := r.Context()
	owner := a.homeOwnerID()
	applied := map[string]any{"file_id": req.FileID}

	// 1) 标签：层级路径自动建树（EnsureTagPath），合并保留原标签
	if len(req.Tags) > 0 {
		ids := make([]string, 0, len(req.Tags))
		for _, t := range req.Tags {
			tag, err := a.tags.EnsureTagPath(ctx, owner, t)
			if err != nil {
				writeErr(w, http.StatusInternalServerError, "ORGANIZE_TAG_FAILED", err.Error())
				return
			}
			ids = append(ids, tag.ID)
		}
		if err := a.tags.MergeFileTags(ctx, owner, req.FileID, ids); err != nil {
			writeErr(w, http.StatusInternalServerError, "ORGANIZE_TAG_FAILED", err.Error())
			return
		}
		applied["tags"] = req.Tags
	}

	// 2) 目录：逐级建目录（已存在则复用），再移动
	if !req.SkipMove && strings.Trim(req.DirPath, "/") != "" {
		dirID, err := a.ensureDirPath(ctx, owner, strings.Trim(req.DirPath, "/"))
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "ORGANIZE_DIR_FAILED", err.Error())
			return
		}
		if dirID != "" {
			if _, err := a.files.Move(ctx, owner, req.FileID, dirID, ""); err != nil {
				writeErr(w, http.StatusBadRequest, "ORGANIZE_MOVE_FAILED", err.Error())
				return
			}
			applied["dir_path"] = req.DirPath
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "applied": applied})
}

// ensureDirPath 在 home 空间逐级建目录（已存在复用），返回末端目录 ID。
func (a *API) ensureDirPath(ctx context.Context, ownerID, path string) (string, error) {
	parts := strings.Split(path, "/")
	var parentID string
	var curID string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		id, err := a.findOrCreateDir(ctx, ownerID, parentID, p)
		if err != nil {
			return "", err
		}
		curID = id
		parentID = id
	}
	return curID, nil
}

// findOrCreateDir 查同父级同名目录；不存在则创建。
func (a *API) findOrCreateDir(ctx context.Context, ownerID, parentID, name string) (string, error) {
	var id string
	var pid any
	if parentID != "" {
		pid = parentID
	}
	err := a.db.QueryRowContext(ctx,
		`SELECT id FROM files WHERE owner_id=? AND kind='dir' AND parent_id IS ? AND name=? AND deleted_at IS NULL`,
		ownerID, pid, name).Scan(&id)
	if err == nil {
		return id, nil
	}
	d, err := a.files.CreateDir(ctx, ownerID, a.homeSpaceID(), parentID, name, service.DefaultSiteID)
	if err != nil {
		return "", err
	}
	return d.ID, nil
}

// isTextLike 判断文本类（可 AI 精读/索引）。
func isTextLike(mime, name string) bool {
	m := strings.ToLower(mime)
	if strings.HasPrefix(m, "text/") || strings.Contains(m, "json") || strings.Contains(m, "xml") ||
		strings.Contains(m, "markdown") || strings.Contains(m, "yaml") || strings.Contains(m, "csv") ||
		strings.Contains(m, "javascript") || strings.Contains(m, "typescript") || strings.Contains(m, "sql") {
		return true
	}
	n := strings.ToLower(name)
	for _, ext := range []string{".md", ".txt", ".markdown", ".log", ".json", ".yaml", ".yml", ".csv", ".go", ".py", ".js", ".ts", ".html", ".css", ".sql", ".xml"} {
		if strings.HasSuffix(n, ext) {
			return true
		}
	}
	return false
}

func round4(v float64) float64 {
	return float64(int(v*10000+0.5)) / 10000
}
