package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

// ---- B5 写作增强：AI 写作提示词模板 ----
//
// 上游只给了建表 DDL、没有实现，这里是本壳的接线（增量）。
// 设计要点：
//   - 提示词是「可配置项」而不是硬编码常量 —— 站长/作者可在后台增删改，写作页直接取用。
//   - variables 是 JSON 变量声明 [{name,label,default}]，驱动前端表单；渲染时按名替换 {{name}}。
//   - is_public=1 全站可见；=0 仅作者本人可见（列表按 uid 过滤）。
//   - 渲染是**纯函数**：不改库、不调 AI，只做占位替换 —— 保证可预期、可测试。

// PromptVariable 变量声明（驱动前端表单）。
type PromptVariable struct {
	Name    string `json:"name"`
	Label   string `json:"label,omitempty"`
	Default string `json:"default,omitempty"`
}

// PromptTemplate 提示词模板行模型。
type PromptTemplate struct {
	ID        string           `json:"id"`
	Name      string           `json:"name"`
	Category  string           `json:"category"`
	Content   string           `json:"content"`
	Variables []PromptVariable `json:"variables"`
	IsPublic  bool             `json:"is_public"`
	AuthorID  string           `json:"author_id"`
	Author    string           `json:"author,omitempty"` // 作者显示名（子查询 users；列表展示用）
	CreatedAt int64            `json:"created_at"`
	UpdatedAt int64            `json:"updated_at"`
	Editable  bool             `json:"editable"` // 当前操作者是否可改（前端按此显隐按钮，不靠猜）
}

// PromptActor 操作者：service 层据此判权，避免 handler 重复写 SQL 与规则。
type PromptActor struct {
	UID     string
	IsAdmin bool
}

// PromptStore 提示词模板数据访问。
type PromptStore struct{ db *sql.DB }

// NewPromptStore 创建提示词模板仓储。
func NewPromptStore(db *sql.DB) *PromptStore { return &PromptStore{db: db} }

// 校验上限：提示词是人手写的短文本，超限大概率是误粘贴（比如把整篇文章贴进来）。
const (
	promptMaxContent   = 64 << 10 // 正文 ≤ 64KB
	promptMaxVariables = 32       // 变量 ≤ 32 个
	promptMaxName      = 200
)

var (
	// ErrPromptNotFound 模板不存在或无权访问（不暴露存在性）。
	ErrPromptNotFound = errors.New("service: prompt template not found")
	// ErrPromptInvalid 入参非法（含变量声明不是合法 JSON 等情况）。
	ErrPromptInvalid = errors.New("service: invalid prompt template")
	// ErrPromptForbidden 非作者且非管理员，不可改他人模板。
	ErrPromptForbidden = errors.New("service: prompt template forbidden")
)

// varNameRe 变量名允许字母/数字/下划线/中文，便于 {{标题}} 这类中文字段名也成立。
//
// 注意：Go 的 RE2 **不支持 `\u` 转义**（只认 `\x{...}`），且必须用反引号原始字符串，
// 否则 `\x` 会被 Go 字符串字面量当成字节转义 —— 写成 `"[\u4e00-\u9fa5]"` 会在 init() 直接 panic。
var varNameRe = regexp.MustCompile(`^[\w\x{4e00}-\x{9fa5}]{1,64}$`)

// promptPlaceholderRe 匹配 {{ 名称 }}（允许内部空格）。
var promptPlaceholderRe = regexp.MustCompile(`\{\{\s*[^{}]{1,64}\s*\}\}`)

// normalizeVariables 解析并校验变量声明 JSON（空串按空数组处理）。
func normalizeVariables(raw string) ([]PromptVariable, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []PromptVariable{}, nil
	}
	var vs []PromptVariable
	if err := json.Unmarshal([]byte(raw), &vs); err != nil {
		return nil, fmt.Errorf("%w: variables 不是合法 JSON 数组", ErrPromptInvalid)
	}
	if len(vs) > promptMaxVariables {
		return nil, fmt.Errorf("%w: 变量数量超过 %d", ErrPromptInvalid, promptMaxVariables)
	}
	seen := map[string]bool{}
	for i := range vs {
		vs[i].Name = strings.TrimSpace(vs[i].Name)
		if !varNameRe.MatchString(vs[i].Name) {
			return nil, fmt.Errorf("%w: 变量名 %q 非法（仅字母数字下划线中文，≤64 字）", ErrPromptInvalid, vs[i].Name)
		}
		if seen[vs[i].Name] {
			return nil, fmt.Errorf("%w: 变量名 %q 重复", ErrPromptInvalid, vs[i].Name)
		}
		seen[vs[i].Name] = true
	}
	return vs, nil
}

func marshalVariables(vs []PromptVariable) string {
	if vs == nil {
		vs = []PromptVariable{}
	}
	b, err := json.Marshal(vs)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// PromptInput 新建/更新入参。
type PromptInput struct {
	Name      string
	Category  string
	Content   string
	Variables string // 原始 JSON（handler 从请求体透传）；service 负责校验
	IsPublic  bool
}

// validatePrompt 通用入参校验。
func validatePrompt(in *PromptInput) ([]PromptVariable, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Category = strings.TrimSpace(in.Category)
	if in.Name == "" {
		return nil, fmt.Errorf("%w: 名称必填", ErrPromptInvalid)
	}
	if len(in.Name) > promptMaxName {
		return nil, fmt.Errorf("%w: 名称过长", ErrPromptInvalid)
	}
	if strings.TrimSpace(in.Content) == "" {
		return nil, fmt.Errorf("%w: 提示词正文必填", ErrPromptInvalid)
	}
	if len(in.Content) > promptMaxContent {
		return nil, fmt.Errorf("%w: 提示词正文超过 %d 字节", ErrPromptInvalid, promptMaxContent)
	}
	return normalizeVariables(in.Variables)
}

const promptCols = `p.id, p.name, p.category, p.content, p.variables, p.is_public, p.author_id,
	(SELECT COALESCE(NULLIF(u.display_name,''), u.username, '') FROM users u WHERE u.id = p.author_id) AS author,
	p.created_at, p.updated_at`

// scanPrompt 统一行扫描。
func scanPrompt(sc interface{ Scan(...any) error }) (*PromptTemplate, error) {
	var t PromptTemplate
	var vars, author sql.NullString
	var pub int
	if err := sc.Scan(&t.ID, &t.Name, &t.Category, &t.Content, &vars, &pub, &t.AuthorID, &author, &t.CreatedAt, &t.UpdatedAt); err != nil {
		return nil, err
	}
	t.IsPublic = pub != 0
	t.Author = author.String
	if vs, err := normalizeVariables(vars.String); err == nil {
		t.Variables = vs
	} else {
		t.Variables = []PromptVariable{} // 历史脏数据不阻断列表
	}
	return &t, nil
}

// markEditable 标注当前操作者是否可改。
func markEditable(t *PromptTemplate, act PromptActor) {
	t.Editable = act.IsAdmin || t.AuthorID == act.UID
}

// List 列出可见模板：公开的 + 自己私有的（管理员看全部）。category 非空则按分类过滤。
func (s *PromptStore) List(ctx context.Context, act PromptActor, category string) ([]PromptTemplate, error) {
	q := `SELECT ` + promptCols + ` FROM prompt_templates p WHERE 1=1`
	args := []any{}
	if !act.IsAdmin {
		q += ` AND (p.is_public = 1 OR p.author_id = ?)`
		args = append(args, act.UID)
	}
	if strings.TrimSpace(category) != "" {
		q += ` AND p.category = ?`
		args = append(args, strings.TrimSpace(category))
	}
	q += ` ORDER BY p.updated_at DESC`
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PromptTemplate{}
	for rows.Next() {
		t, err := scanPrompt(rows)
		if err != nil {
			return nil, err
		}
		markEditable(t, act)
		out = append(out, *t)
	}
	return out, rows.Err()
}

// Get 取单个模板（非公开且非本人且非管理员 → ErrPromptNotFound，不泄露存在性）。
func (s *PromptStore) Get(ctx context.Context, act PromptActor, id string) (*PromptTemplate, error) {
	var t *PromptTemplate
	var err error
	if act.IsAdmin {
		t, err = scanPrompt(s.db.QueryRowContext(ctx,
			`SELECT `+promptCols+` FROM prompt_templates p WHERE p.id = ?`, id))
	} else {
		t, err = scanPrompt(s.db.QueryRowContext(ctx,
			`SELECT `+promptCols+` FROM prompt_templates p WHERE p.id = ? AND (p.is_public = 1 OR p.author_id = ?)`, id, act.UID))
	}
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrPromptNotFound
	}
	if err != nil {
		return nil, err
	}
	markEditable(t, act)
	return t, nil
}

// Create 新建模板（author = act.UID）。
func (s *PromptStore) Create(ctx context.Context, act PromptActor, in PromptInput) (*PromptTemplate, error) {
	vs, err := validatePrompt(&in)
	if err != nil {
		return nil, err
	}
	id := uuid.NewString()
	ts := now()
	pub := 0
	if in.IsPublic {
		pub = 1
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO prompt_templates (id, name, category, content, variables, is_public, author_id, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?)`,
		id, in.Name, in.Category, in.Content, marshalVariables(vs), pub, act.UID, ts, ts); err != nil {
		return nil, err
	}
	return s.Get(ctx, act, id)
}

// Update 更新模板：作者本人或管理员。语义为**整体替换**（PUT）。
func (s *PromptStore) Update(ctx context.Context, act PromptActor, id string, in PromptInput) (*PromptTemplate, error) {
	vs, err := validatePrompt(&in)
	if err != nil {
		return nil, err
	}
	var authorID string
	err = s.db.QueryRowContext(ctx, `SELECT author_id FROM prompt_templates WHERE id = ?`, id).Scan(&authorID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrPromptNotFound
	}
	if err != nil {
		return nil, err
	}
	if !act.IsAdmin && authorID != act.UID {
		return nil, ErrPromptForbidden
	}
	pub := 0
	if in.IsPublic {
		pub = 1
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE prompt_templates SET name=?, category=?, content=?, variables=?, is_public=?, updated_at=? WHERE id=?`,
		in.Name, in.Category, in.Content, marshalVariables(vs), pub, now(), id); err != nil {
		return nil, err
	}
	return s.Get(ctx, act, id)
}

// Delete 删除模板：作者本人或管理员。
func (s *PromptStore) Delete(ctx context.Context, act PromptActor, id string) error {
	var authorID string
	err := s.db.QueryRowContext(ctx, `SELECT author_id FROM prompt_templates WHERE id = ?`, id).Scan(&authorID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrPromptNotFound
	}
	if err != nil {
		return err
	}
	if !act.IsAdmin && authorID != act.UID {
		return ErrPromptForbidden
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM prompt_templates WHERE id = ?`, id)
	return err
}

// RenderResult 渲染结果。
type RenderResult struct {
	Text       string   `json:"text"`
	Missing    []string `json:"missing"`    // 有占位符但未提供值、且无默认值的变量（前端提示补全）
	Undeclared []string `json:"undeclared"` // 正文用到但变量声明里没有的占位符（提示作者补声明）
}

// renderPromptText 占位替换的**纯函数**实现（无 IO，便于单测）。
//
// 规则（按优先级）：
//  1. values 里有非空值 → 用它；
//  2. 变量声明里有 default → 用它（可能是空串，属作者明确意图，不算缺失）；
//  3. 都没有 → 原样保留 {{name}}（可见即可自查），并同时记入 missing / undeclared。
func renderPromptText(content string, declared []PromptVariable, values map[string]string) *RenderResult {
	if values == nil {
		values = map[string]string{}
	}
	defs := map[string]string{}
	for _, v := range declared {
		defs[v.Name] = v.Default
	}
	missing := []string{}
	undeclared := []string{}
	seenMissing := map[string]bool{}
	seenUndecl := map[string]bool{}

	text := promptPlaceholderRe.ReplaceAllStringFunc(content, func(m string) string {
		name := strings.TrimSpace(m[2 : len(m)-2])
		if val, ok := values[name]; ok && val != "" {
			return val
		}
		if def, ok := defs[name]; ok {
			return def
		}
		_, isDeclared := defs[name]
		if !isDeclared && !seenUndecl[name] {
			seenUndecl[name] = true
			undeclared = append(undeclared, name)
		}
		if !seenMissing[name] {
			seenMissing[name] = true
			missing = append(missing, name)
		}
		return m // 未解析 → 原样保留（作者一眼能看出还缺什么）
	})
	return &RenderResult{Text: text, Missing: missing, Undeclared: undeclared}
}

// Render 取模板并渲染（纯函数语义：不改库、不调 AI）。
func (s *PromptStore) Render(ctx context.Context, act PromptActor, id string, values map[string]string) (*RenderResult, error) {
	t, err := s.Get(ctx, act, id)
	if err != nil {
		return nil, err
	}
	return renderPromptText(t.Content, t.Variables, values), nil
}
