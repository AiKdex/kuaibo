// Package service 业务编排层。
// tagrules.go 规则打标引擎（L0 层，零 AI 成本）：
//   - 扩展名 → 类型标签（.mp3/.flac → 类型/音频，.pdf/.docx → 类型/文档 …）
//   - 文件名关键词 → 语义标签（"合同/协议" → 合同，"教程/指南/手册" → 教程 …）
//
// 上传/入库时即时打标；AI 解读打标（L1）与之合并，不互相覆盖。
package service

import (
	"context"
	"strings"

	"github.com/AiKMAP/AiKmap/server/internal/engine/bus"
)

// extTypeTags 扩展名 → 类型标签（小写无点）。
var extTypeTags = map[string]string{
	// 音频
	"mp3": "类型/音频", "wav": "类型/音频", "flac": "类型/音频", "m4a": "类型/音频",
	"aac": "类型/音频", "ogg": "类型/音频", "wma": "类型/音频", "opus": "类型/音频",
	// 视频
	"mp4": "类型/视频", "mkv": "类型/视频", "avi": "类型/视频", "mov": "类型/视频",
	"flv": "类型/视频", "wmv": "类型/视频", "webm": "类型/视频",
	// 图片
	"jpg": "类型/图片", "jpeg": "类型/图片", "png": "类型/图片", "gif": "类型/图片",
	"bmp": "类型/图片", "webp": "类型/图片", "svg": "类型/图片", "tiff": "类型/图片",
	"heic": "类型/图片", "ico": "类型/图片",
	// 文档
	"pdf": "类型/文档", "doc": "类型/文档", "docx": "类型/文档", "xls": "类型/文档",
	"xlsx": "类型/文档", "ppt": "类型/文档", "pptx": "类型/文档", "txt": "类型/文档",
	"md": "类型/文档", "markdown": "类型/文档", "rtf": "类型/文档", "odt": "类型/文档",
	// 代码
	"go": "类型/代码", "py": "类型/代码", "js": "类型/代码", "ts": "类型/代码",
	"vue": "类型/代码", "java": "类型/代码", "c": "类型/代码", "cpp": "类型/代码",
	"h": "类型/代码", "rb": "类型/代码", "rs": "类型/代码", "php": "类型/代码",
	"sh": "类型/代码", "sql": "类型/代码", "html": "类型/代码", "css": "类型/代码",
	"json": "类型/代码", "yaml": "类型/代码", "yml": "类型/代码", "toml": "类型/代码",
	"xml": "类型/代码", "ini": "类型/代码", "conf": "类型/代码",
	// 压缩包
	"zip": "类型/压缩包", "rar": "类型/压缩包", "7z": "类型/压缩包", "tar": "类型/压缩包",
	"gz": "类型/压缩包", "bz2": "类型/压缩包", "xz": "类型/压缩包",
	// 电子书
	"epub": "类型/电子书", "mobi": "类型/电子书", "azw3": "类型/电子书", "djvu": "类型/电子书",
}

// nameKeywordTags 文件名关键词 → 语义标签（按顺序匹配，命中即加）。
var nameKeywordTags = []struct {
	Keywords []string
	Tag      string
}{
	{[]string{"合同", "协议", "契约"}, "合同"},
	{[]string{"简历", "cv", "resume"}, "简历"},
	{[]string{"发票", "报销", "账单", "invoice"}, "财务"},
	{[]string{"会议", "纪要", "meeting"}, "会议纪要"},
	{[]string{"教程", "指南", "手册", "说明", "使用", "tutorial", "guide", "manual"}, "教程"},
	{[]string{"笔记", "备忘", "note"}, "笔记"},
	{[]string{"报告", "总结", "汇报", "report"}, "报告"},
	{[]string{"方案", "计划", "规划", "plan", "proposal"}, "方案"},
	{[]string{"测试", "test", "demo"}, "测试"},
	{[]string{"备份", "backup"}, "备份"},
	{[]string{"设计", "原型", "design", "ui", "ux"}, "设计"},
	{[]string{"清单", "列表", "checklist"}, "清单"},
}

// RuleTags 计算规则标签路径（扩展名 + 文件名关键词），去重保序。空返回 nil。
func RuleTags(name string) []string {
	seen := map[string]bool{}
	out := []string{}
	add := func(p string) {
		if p != "" && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	lower := strings.ToLower(name)
	// 扩展名（最后一个点之后；无点跳过）
	if i := strings.LastIndexByte(lower, '.'); i >= 0 && i < len(lower)-1 {
		if t, ok := extTypeTags[lower[i+1:]]; ok {
			add(t)
		}
	}
	// 文件名关键词（不含扩展名的部分，避免把 "合同.txt" 命中 "txt" 相关词——词表无此问题，但统一用全名匹配更稳）
	for _, rule := range nameKeywordTags {
		for _, kw := range rule.Keywords {
			if strings.Contains(lower, kw) {
				add(rule.Tag)
				break
			}
		}
	}
	return out
}

// EnsurePath 按层级路径建标签（如 类型/音频 不存在则逐级创建），返回末端标签 ID。
func (s *TagStore) EnsurePath(ctx context.Context, ownerID, path string) (string, error) {
	segs := strings.Split(strings.Trim(path, "/ "), "/")
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
		id := s.findByPath(ctx, ownerID, cur)
		if id == "" {
			t, err := s.Create(ctx, ownerID, seg, parentID)
			if err != nil {
				if strings.Contains(err.Error(), "conflict") {
					// 并发竞争：重查一次
					if id2 := s.findByPath(ctx, ownerID, cur); id2 != "" {
						id = id2
						parentID = id
						continue
					}
				}
				return "", err
			}
			id = t.ID
		}
		parentID = id
	}
	return parentID, nil
}

// findByPath 按完整 path 查标签 ID（不存在返回空串）。
func (s *TagStore) findByPath(ctx context.Context, ownerID, path string) string {
	var id string
	_ = s.db.QueryRowContext(ctx, `SELECT id FROM tags WHERE owner_id=? AND path=?`, ownerID, path).Scan(&id)
	return id
}

// ApplyRuleTags 对一个文件应用规则打标：规则标签 ∪ 现有标签 → 全量写回。
// ownerID 为空时使用 SystemOwnerID。返回挂载的标签路径（新增或已存在的）。
func (s *TagStore) ApplyRuleTags(ctx context.Context, ownerID, fileID, name string) []string {
	if ownerID == "" {
		ownerID = SystemOwnerID
	}
	paths := RuleTags(name)
	if len(paths) == 0 {
		return nil
	}
	ids := make([]string, 0, len(paths))
	for _, p := range paths {
		id, err := s.EnsurePath(ctx, ownerID, p)
		if err != nil || id == "" {
			continue
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil
	}
	// 合并现有标签（不覆盖手工/AI 已打的）
	cur, err := s.FileTags(ctx, fileID)
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
	_ = s.SetFileTags(ctx, ownerID, fileID, ids)
	return paths
}

// SubscribeRuleTags 订阅事件总线：file.created → 规则打标（零成本，先于 AI 解读）。
func (s *TagStore) SubscribeRuleTags(b *bus.Bus) func() {
	return b.Subscribe("file.created", func(_ context.Context, e bus.Event) error {
		if e.Key == "" {
			return nil
		}
		name, _ := e.Data["name"].(string)
		if name == "" {
			return nil
		}
		s.ApplyRuleTags(context.Background(), SystemOwnerID, e.Key, name)
		return nil
	})
}
