// indexer.go 内容分块索引：对文本类文件提取正文、分块写入 index_chunks。
// 阶段 2 前置能力：让"文件里有什么/总结/找内容"类问答命中正文；向量化在此之上叠加（chunks 的 model 字段记录向量模型）。
// 说明：MVP 采用上传后同步索引（文件小、代价低）；文件量上来后迁移到 jobs 队列异步消费（实施文档 §10.3）。
package service

import (
	"context"
	"io"
	"path"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	chunkSize    = 800             // 每块字符数（rune）
	chunkOverlap = 120             // 相邻块重叠字符数（保证跨块语义不切断）
	MaxIndexSize = 5 * 1024 * 1024 // 导出：索引/解读上限
)

// IndexableMIME 判断是否文本可索引（text/* 与常见代码/数据 MIME）。
// 导出供 ai 包（摘要/解读）与索引共用同一口径。
func IndexableMIME(mime string) bool {
	if mime == "" {
		return false
	}
	m := strings.ToLower(mime)
	if strings.HasPrefix(m, "text/") {
		return true
	}
	switch m {
	case "application/json", "application/xml", "application/javascript", "application/x-javascript",
		"application/x-sh", "application/sql", "application/yaml", "application/toml",
		"application/csv", "application/x-www-form-urlencoded", "application/x-httpd-php",
		"application/octet-stream":
		return true
	}
	return false
}

// IndexFile 对单个文件执行分块索引（幂等：先清后写）。非文本/超限文件直接跳过。
func (s *FileStore) IndexFile(ctx context.Context, f *File) error {
	if f == nil || f.Kind != "file" || !IndexableMIME(f.Mime) {
		return nil
	}
	if f.Size <= 0 || f.Size > MaxIndexSize {
		return nil
	}
	rc, _, err := s.st.Get(ctx, storagePath(f.SpaceID, f.StorageRef))
	if err != nil {
		return err
	}
	defer rc.Close()
	data, err := io.ReadAll(io.LimitReader(rc, MaxIndexSize))
	if err != nil {
		return err
	}
	return s.indexText(ctx, f, data)
}

// indexText 清空旧块并写入新块；同时解析 [[双链]] 写入知识图谱边。
// 分块按文档类型分派模板（A10 模板化分块）：md/txt 走标题语义块、csv/tsv 走行块、其余固定窗口。
func (s *FileStore) indexText(ctx context.Context, f *File, data []byte) error {
	text := string(data)
	if !utf8.Valid(data) {
		text = strings.ToValidUTF8(text, "\uFFFD")
	}
	chunks, tpl := splitChunksByMime(text, f.Name, f.Mime)
	// 先删向量再删块：vectors.chunk_id 引用 index_chunks(id)（无级联），
	// 有向量时直接删块会 FK 失败（A10 实测暴露：embedding 跑过后重索引永远失败）。
	if _, err := s.db.ExecContext(ctx, `DELETE FROM vectors WHERE file_id=?`, f.ID); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM index_chunks WHERE file_id=?`, f.ID); err != nil {
		return err
	}
	ts := time.Now().Unix()
	for i, c := range chunks {
		if _, err := s.db.ExecContext(ctx,
			`INSERT INTO index_chunks (file_id, space_id, chunk_idx, content, meta, status, created_at)
			 VALUES (?,?,?,?,?,'indexed',?)`,
			f.ID, f.SpaceID, i, c, `{"tpl":"`+tpl+`"}`, ts); err != nil {
			return err
		}
	}
	// 双链 → knowledge_edges（幂等：先清本文件的 cites 出链再写）
	s.syncWikiLinks(ctx, f, text)
	return nil
}

// ListWikiNames 返回同空间全部文件名清单（[[ 联想 / 双链解析数据源，O(n) 建映射一次）。
func (s *FileStore) ListWikiNames(ctx context.Context, spaceID string) ([]map[string]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name FROM files WHERE space_id=? AND kind='file' AND deleted_at IS NULL ORDER BY name`, spaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]map[string]string, 0)
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		out = append(out, map[string]string{"id": id, "name": name})
	}
	return out, nil
}

// ResolveWikiTitle 按标题解析双链目标文件（公开封装 matchWikiTitle）。
func (s *FileStore) ResolveWikiTitle(ctx context.Context, spaceID, title string) string {
	return s.matchWikiTitle(ctx, spaceID, title)
}

// syncWikiLinks 解析 Obsidian 兼容双链 [[标题#锚点|显示名]]，匹配同空间文件（文件名去扩展名或全名），
// 写入 knowledge_edges（relation='cites'，source='auto'，anchor=锚点）。先清旧边再写，保证幂等。
func (s *FileStore) syncWikiLinks(ctx context.Context, f *File, text string) {
	re := wikiLinkRe
	targets := map[string]struct {
		id     string
		anchor string
	}{} // 规范化标题+#锚点 → {文件id, 锚点}
	seen := map[string]bool{}
	for _, m := range re.FindAllStringSubmatch(text, -1) {
		title := strings.TrimSpace(m[1])
		anchor := cleanAnchor(m[2])
		key := title + "\x00" + anchor
		if title == "" || seen[key] {
			continue
		}
		seen[key] = true
		if id := s.matchWikiTitle(ctx, f.SpaceID, title); id != "" && id != f.ID {
			targets[key] = struct {
				id     string
				anchor string
			}{id, anchor}
		}
	}
	// 清旧 cites 出链（本文件）
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM knowledge_edges WHERE source_type='file' AND source_id=? AND relation='cites' AND source='auto'`,
		f.ID); err != nil {
		return
	}
	ts := time.Now().Unix()
	for _, t := range targets {
		// 存在相同边（含锚点）则跳过
		var cnt int
		if err := s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM knowledge_edges WHERE source_type='file' AND source_id=? AND target_type='file' AND target_id=? AND relation='cites' AND IFNULL(anchor,'')=?`,
			f.ID, t.id, t.anchor).Scan(&cnt); err != nil || cnt > 0 {
			continue
		}
		s.db.ExecContext(ctx,
			`INSERT INTO knowledge_edges (space_id, source_type, source_id, target_type, target_id, relation, weight, source, anchor, created_at)
			 VALUES (?, 'file', ?, 'file', ?, 'cites', 1, 'auto', ?, ?)`,
			f.SpaceID, f.ID, t.id, nullStr(t.anchor), ts)
	}
}

// markWikiTitleDirty 任何改变文件集合/名称的写操作后调用（Upload/CreateDoc/Move/Copy/Delete/Restore/Purge/EmptyTrash）。
func (s *FileStore) markWikiTitleDirty() {
	s.wikiMu.Lock()
	s.wikiDirty = true
	s.wikiMu.Unlock()
}

// ensureWikiTitleIdx 懒构建 name→id 内存映射（dirty 或未构建时全量加载一次，之后 O(1) 查）。
func (s *FileStore) ensureWikiTitleIdx(ctx context.Context, spaceID string) {
	s.wikiMu.Lock()
	defer s.wikiMu.Unlock()
	if !s.wikiDirty && s.wikiIdx != nil {
		return
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name FROM files WHERE space_id=? AND kind='file' AND deleted_at IS NULL`, spaceID)
	if err != nil {
		return // 构建失败保持旧索引/dirty，下次再试
	}
	defer rows.Close()
	idx := make(map[string]string)
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return
		}
		idx[name] = id
		if i := strings.LastIndex(name, "."); i > 0 {
			idx[name[:i]] = id
		}
	}
	if s.wikiIdx == nil {
		s.wikiIdx = make(map[string]map[string]string)
	}
	s.wikiIdx[spaceID] = idx
	s.wikiDirty = false
}

// matchWikiTitle 按标题匹配同空间文件：O(1) 内存映射优先，映射缺失时全表扫描兜底。
// 匹配语义与原实现一致：优先全名，其次去掉最后一个扩展名（支持 AIWebs.cc-完整方案.md 这类多后缀名）。
func (s *FileStore) matchWikiTitle(ctx context.Context, spaceID, title string) string {
	s.ensureWikiTitleIdx(ctx, spaceID)
	s.wikiMu.Lock()
	idx := s.wikiIdx[spaceID]
	s.wikiMu.Unlock()
	if idx == nil {
		return s.matchWikiTitleScan(ctx, spaceID, title)
	}
	if id, ok := idx[title]; ok {
		return id
	}
	return ""
}

// matchWikiTitleScan 全表扫描兜底（映射构建失败/缺失时回退，语义与原实现一致）。
func (s *FileStore) matchWikiTitleScan(ctx context.Context, spaceID, title string) string {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name FROM files WHERE space_id=? AND kind='file' AND deleted_at IS NULL`, spaceID)
	if err != nil {
		return ""
	}
	defer rows.Close()
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return ""
		}
		if name == title {
			return id
		}
		if i := strings.LastIndex(name, "."); i > 0 {
			if name[:i] == title {
				return id
			}
		}
	}
	return ""
}

// wikiLinkRe 匹配 Obsidian 兼容双链：[[标题]]、[[标题|显示名]]、[[标题#锚点]]、[[标题#锚点|显示名]]、
// [[标题#^块ID]]（块引用）。锚点经锚点清洗后存 knowledge_edges.anchor。
var wikiLinkRe = mustCompileWiki()

func mustCompileWiki() *regexp.Regexp {
	return regexp.MustCompile(`\[\[([^\[\]|#]+)(?:#([^\[\]|]+))?(?:\|[^\]]*)?\]\]`)
}

// cleanAnchor 锚点清洗：保留标题或块ID语义（去除首尾空白；块ID保留 ^ 前缀）。
func cleanAnchor(a string) string {
	return strings.TrimSpace(a)
}

// nullStr 空串转 SQL NULL（anchor 无值时存 NULL，IFNULL 查询可归一）。
func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// splitChunksByMime 按文档类型分派分块模板（A10）。返回 (块列表, 模板名)。
// 以扩展名优先、MIME 辅助（MIME 可能被上传端识别为 octet-stream，扩展名兜底）。
//   - md/markdown/txt/log：mdHeadingChunks 标题语义块（标题行起新块，块内超长按段落切，段落超长回退固定窗口）
//   - csv/tsv：lineChunks 行块（保持表格行完整）
//   - 其余：splitChunks 固定窗口（代码/JSON 等结构化文本）
func splitChunksByMime(text, name, mime string) ([]string, string) {
	ext := strings.ToLower(strings.TrimPrefix(path.Ext(name), "."))
	m := strings.ToLower(strings.SplitN(mime, ";", 2)[0])
	switch {
	case ext == "md" || ext == "markdown" || ext == "txt" || ext == "log" || m == "text/markdown" || m == "text/plain":
		return mdHeadingChunks(text), "md-heading"
	case ext == "csv" || ext == "tsv" || m == "text/csv" || m == "text/tab-separated-values" || m == "application/csv":
		return lineChunks(text), "line"
	default:
		return splitChunks(text), "window"
	}
}

var headingLineRe = regexp.MustCompile(`^#{1,6}\s+\S`)

// mdHeadingChunks Markdown 标题语义分块：以标题行作为块的语义边界（块首即标题，检索命中自带上下文）；
// 块内按空行分隔的段落聚合，累计超 chunkSize 即切；单段落超长回退固定窗口（带 overlap）。
func mdHeadingChunks(text string) []string {
	// 先按空行切成段落块
	raw := strings.Split(text, "\n")
	var paras [][]string
	var cur []string
	flush := func() {
		if len(cur) > 0 {
			paras = append(paras, cur)
			cur = nil
		}
	}
	for _, ln := range raw {
		if strings.TrimSpace(ln) == "" {
			flush()
			continue
		}
		cur = append(cur, ln)
	}
	flush()
	if len(paras) == 0 {
		return nil
	}

	var out []string
	var buf []string
	bufLen := 0
	for _, p := range paras {
		blockText := strings.Join(p, "\n")
		isHeading := headingLineRe.MatchString(p[0])
		// 标题段落：作为新块起点（若 buf 已有内容先收口）
		if isHeading && len(buf) > 0 {
			out = append(out, strings.Join(buf, "\n\n"))
			buf, bufLen = nil, 0
		}
		// 段落本身超长：先收口 buf，段落内部回退固定窗口
		if utf8.RuneCountInString(blockText) > chunkSize {
			if len(buf) > 0 {
				out = append(out, strings.Join(buf, "\n\n"))
				buf, bufLen = nil, 0
			}
			for _, c := range splitChunks(blockText) {
				out = append(out, c)
			}
			continue
		}
		buf = append(buf, blockText)
		bufLen += utf8.RuneCountInString(blockText) + 2 // "\n\n"
		if bufLen >= chunkSize {
			out = append(out, strings.Join(buf, "\n\n"))
			buf, bufLen = nil, 0
		}
	}
	if len(buf) > 0 {
		out = append(out, strings.Join(buf, "\n\n"))
	}
	return out
}

// lineChunks CSV/TSV 行块：按行聚合（每块约 200 行），表格行不被截断。
func lineChunks(text string) []string {
	lines := strings.Split(text, "\n")
	if len(lines) == 0 {
		return nil
	}
	per := 200
	var out []string
	for start := 0; start < len(lines); start += per {
		end := start + per
		if end > len(lines) {
			end = len(lines)
		}
		out = append(out, strings.Join(lines[start:end], "\n"))
	}
	return out
}

// splitChunks 按 rune 滑动窗口分块（重叠 overlap，窗口在字符边界）。
func splitChunks(text string) []string {
	runes := []rune(text)
	if len(runes) == 0 {
		return nil
	}
	step := chunkSize - chunkOverlap
	var out []string
	for start := 0; start < len(runes); start += step {
		end := start + chunkSize
		if end > len(runes) {
			end = len(runes)
		}
		out = append(out, string(runes[start:end]))
		if end == len(runes) {
			break
		}
	}
	return out
}

// ReindexSpace 对空间内缺失索引的文本文件补索引（幂等；启动时调用）。
// 返回本次新增索引的文件数。
func (s *FileStore) ReindexSpace(ctx context.Context, spaceID string) (int, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+fileCols+` FROM files WHERE space_id=? AND kind='file' AND deleted_at IS NULL`, spaceID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		f, err := scanFile(rows)
		if err != nil {
			return n, err
		}
		var cnt int
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM index_chunks WHERE file_id=?`, f.ID).Scan(&cnt); err != nil {
			return n, err
		}
		if cnt > 0 {
			continue
		}
		if err := s.IndexFile(ctx, f); err == nil {
			n++
		}
	}
	return n, rows.Err()
}
