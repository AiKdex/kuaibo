package service

import (
	"context"
	"database/sql"
	"regexp"
	"strings"
)

// ---- A08 知识质量 lint（第一版）----
// 六项检查：孤儿文件（无标签无集合）/ 空标签 / 未解读 / sha256 重复 / 断链双链 / 解读失败数。
// 全部只读，不修改数据；前端点击条目可跳转定位。

type LintFileItem struct {
	FileID   string `json:"file_id"`
	FileName string `json:"file_name"`
	Size     int64  `json:"size"`
	Mime     string `json:"mime,omitempty"`
}

type LintTagItem struct {
	TagID string `json:"tag_id"`
	Path  string `json:"path"`
}

type LintDupGroup struct {
	Sha256 string         `json:"sha256"`
	Files  []LintFileItem `json:"files"`
}

type LintBrokenLink struct {
	FileID   string `json:"file_id"`
	FileName string `json:"file_name"`
	Target   string `json:"target"` // 断链目标（[[...]] 内标题）
}

type LintReport struct {
	TotalFiles   int64            `json:"total_files"`
	Orphans      []LintFileItem   `json:"orphans"`
	EmptyTags    []LintTagItem    `json:"empty_tags"`
	NoSummary    []LintFileItem   `json:"no_summary"`
	Duplicates   []LintDupGroup   `json:"duplicates"`
	BrokenLinks  []LintBrokenLink `json:"broken_links"`
	SummaryError int64            `json:"summary_error"`
}

var lintWikiLinkRe = regexp.MustCompile(`\[\[([^\[\]|#]+)(?:#([^\[\]|]+))?(?:\|[^\]]*)?\]\]`)

// noSummaryMimeExclude 未解读检查排除的二进制类型（正常无需 AI 解读）。
var noSummaryMimeExclude = []string{
	"image/", "video/", "audio/",
	"application/zip", "application/x-7z", "application/x-rar",
	"application/gzip", "application/x-tar", "application/x-bzip2",
	"application/x-msdownload", "application/vnd.android.package-archive",
}

func isExcludedMime(mime string) bool {
	m := strings.ToLower(mime)
	for _, p := range noSummaryMimeExclude {
		if strings.HasPrefix(m, p) {
			return true
		}
	}
	return false
}

// Lint 知识质量检查（只读）。
func (s *KBStore) Lint(ctx context.Context, ownerID, spaceID string) (*LintReport, error) {
	out := &LintReport{
		Orphans:     []LintFileItem{},
		EmptyTags:   []LintTagItem{},
		NoSummary:   []LintFileItem{},
		Duplicates:  []LintDupGroup{},
		BrokenLinks: []LintBrokenLink{},
	}
	_ = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM files WHERE space_id=? AND kind='file' AND deleted_at IS NULL`, spaceID).Scan(&out.TotalFiles)

	// 1) 孤儿文件：无任何标签、无任何集合归属（含智能集合；collection_files 仅 manual，smart 不落表——以标签为准兜底）
	rows, err := s.db.QueryContext(ctx, `
		SELECT f.id, f.name, f.size, IFNULL(f.mime,'') FROM files f
		LEFT JOIN file_tags ft ON ft.file_id = f.id
		LEFT JOIN collection_files cf ON cf.file_id = f.id
		WHERE f.space_id=? AND f.kind='file' AND f.deleted_at IS NULL
		  AND ft.file_id IS NULL AND cf.file_id IS NULL
		ORDER BY f.updated_at DESC LIMIT 30`, spaceID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var it LintFileItem
		if err := rows.Scan(&it.FileID, &it.FileName, &it.Size, &it.Mime); err != nil {
			rows.Close()
			return nil, err
		}
		out.Orphans = append(out.Orphans, it)
	}
	rows.Close()

	// 2) 空标签：无文件关联
	trows, err := s.db.QueryContext(ctx, `
		SELECT t.id, t.path FROM tags t
		LEFT JOIN file_tags ft ON ft.tag_id = t.id
		WHERE t.owner_id=? AND ft.tag_id IS NULL
		ORDER BY t.path LIMIT 50`, ownerID)
	if err != nil {
		return nil, err
	}
	for trows.Next() {
		var it LintTagItem
		if err := trows.Scan(&it.TagID, &it.Path); err != nil {
			trows.Close()
			return nil, err
		}
		out.EmptyTags = append(out.EmptyTags, it)
	}
	trows.Close()

	// 3) 未解读：无 done 摘要的文本/文档类文件（排除二进制，limit 30）
	// 条件：mime 为空（未知类型视为文本）或不以任何排除前缀开头（AND 连接，保证 zip 等被排除）
	mimeCond := "(f.mime IS NULL OR f.mime=''"
	for _, p := range noSummaryMimeExclude {
		mimeCond += " OR f.mime LIKE '" + strings.ReplaceAll(p, "'", "''") + "%'"
	}
	mimeCond += ")"
	mimeCond = "AND NOT " + mimeCond
	srows, err := s.db.QueryContext(ctx, `
		SELECT f.id, f.name, f.size, IFNULL(f.mime,'') FROM files f
		LEFT JOIN file_ai_summaries sa ON sa.file_id = f.id AND sa.status='done'
		WHERE f.space_id=? AND f.kind='file' AND f.deleted_at IS NULL
		  AND sa.file_id IS NULL AND f.size < 2097152 `+mimeCond+`
		ORDER BY f.updated_at DESC LIMIT 30`, spaceID)
	if err != nil {
		return nil, err
	}
	for srows.Next() {
		var it LintFileItem
		if err := srows.Scan(&it.FileID, &it.FileName, &it.Size, &it.Mime); err != nil {
			srows.Close()
			return nil, err
		}
		out.NoSummary = append(out.NoSummary, it)
	}
	srows.Close()

	// 4) sha256 重复组（同空间内；sha 为空跳过）
	drows, err := s.db.QueryContext(ctx, `
		SELECT sha256 FROM files
		WHERE space_id=? AND kind='file' AND deleted_at IS NULL
		  AND sha256 IS NOT NULL AND sha256<>''
		GROUP BY sha256 HAVING COUNT(*)>1
		ORDER BY MAX(updated_at) DESC LIMIT 10`, spaceID)
	if err != nil {
		return nil, err
	}
	var shas []string
	for drows.Next() {
		var sh string
		if err := drows.Scan(&sh); err != nil {
			drows.Close()
			return nil, err
		}
		shas = append(shas, sh)
	}
	drows.Close()
	for _, sh := range shas {
		frows, err := s.db.QueryContext(ctx,
			`SELECT `+fileCols+` FROM files WHERE space_id=? AND kind='file' AND deleted_at IS NULL AND sha256=? ORDER BY updated_at ASC LIMIT 12`, spaceID, sh)
		if err != nil {
			return nil, err
		}
		g := LintDupGroup{Sha256: sh, Files: []LintFileItem{}}
		for frows.Next() {
			f, err := scanFile(frows)
			if err != nil {
				frows.Close()
				return nil, err
			}
			g.Files = append(g.Files, LintFileItem{FileID: f.ID, FileName: f.Name, Size: f.Size, Mime: f.Mime})
		}
		frows.Close()
		out.Duplicates = append(out.Duplicates, g)
	}

	// 5) 断链双链：扫 index_chunks 中的 [[标题]]，与同空间文件名映射比对（全名/去扩展名，语义与双链解析一致）
	titleMap := map[string]bool{}
	nrows, err := s.db.QueryContext(ctx,
		`SELECT name FROM files WHERE space_id=? AND kind='file' AND deleted_at IS NULL`, spaceID)
	if err != nil {
		return nil, err
	}
	for nrows.Next() {
		var n string
		if err := nrows.Scan(&n); err != nil {
			nrows.Close()
			return nil, err
		}
		titleMap[n] = true
		if i := strings.LastIndex(n, "."); i > 0 {
			titleMap[n[:i]] = true
		}
	}
	nrows.Close()

	crows, err := s.db.QueryContext(ctx, `
		SELECT f.id, f.name, c.content FROM index_chunks c
		JOIN files f ON f.id = c.file_id
		WHERE f.space_id=? AND f.deleted_at IS NULL AND f.kind='file'
		LIMIT 8000`, spaceID)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for crows.Next() {
		var fid, fname, content string
		if err := crows.Scan(&fid, &fname, &content); err != nil {
			crows.Close()
			return nil, err
		}
		for _, m := range lintWikiLinkRe.FindAllStringSubmatch(content, -1) {
			target := strings.TrimSpace(m[1])
			if target == "" || titleMap[target] {
				continue
			}
			key := fid + "\x00" + target
			if seen[key] {
				continue
			}
			seen[key] = true
			out.BrokenLinks = append(out.BrokenLinks, LintBrokenLink{FileID: fid, FileName: fname, Target: target})
			if len(out.BrokenLinks) >= 20 {
				break
			}
		}
		if len(out.BrokenLinks) >= 20 {
			break
		}
	}
	crows.Close()

	// 6) 解读失败数
	_ = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM file_ai_summaries s JOIN files f ON s.file_id=f.id
		 WHERE s.status='error' AND f.space_id=?`, spaceID).Scan(&out.SummaryError)

	_ = sql.ErrNoRows // keep sql import when future checks are added
	return out, nil
}
