// sourcepack.go 采集源包（Source Pack，《采集源包格式规范 v1》SPEC-SP-001）。
//
// 定位：源包 = 可导入的 sources 模板集合配置包，**纯配置、无代码、无任意执行**。
// 三层归属（上游 v3.0 回执 §1.1）：引擎=内核 kind=capacity；源包=本文件（L3 配置包）；AI 面=ai.Registry 工具。
//
// 落地图式（规范 §6）：表=索引、目录=载荷。安装产物写 sources 表（每源一行），
// zip 仅作为可分发/可审计的交付物，运行期 Collector 只读表不读 zip。
package service

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"strings"
	"time"
)

// SourcePackSchemaVersion 宿主当前支持的源包 schema 版本（规范 §3.2：当前恒 1）。
const SourcePackSchemaVersion = 1

// SourcePackKind 源包 manifest.kind 的合法取值（恒为 source-pack，与插件 kind=capacity 区分）。
const SourcePackKind = "source-pack"

// sourcePackBannedExt 源包内禁止出现的可执行物扩展名（规范 §2：违规包上架审核直接拒）。
var sourcePackBannedExt = map[string]bool{
	".so": true, ".dll": true, ".exe": true, ".dylib": true,
	".sh": true, ".bash": true, ".bat": true, ".cmd": true, ".ps1": true,
	".py": true, ".rb": true, ".pl": true, ".js": true, ".mjs": true, ".ts": true,
}

// SourcePackCompliance 源包合规声明（规范 §5；必填——上架审核硬门槛）。
type SourcePackCompliance struct {
	TargetSite     string `json:"target_site"`
	Usage          string `json:"usage"`
	RobotsAllowed  bool   `json:"robots_allowed"`
	FetchFrequency string `json:"fetch_frequency,omitempty"`
	CopyrightNote  string `json:"copyright_note,omitempty"`
}

// SourcePackEntry 源模板条目（规范 §4）：一条对应 sources 表一行。
type SourcePackEntry struct {
	ID          string          `json:"id,omitempty"`
	City        string          `json:"city,omitempty"`
	Name        string          `json:"name"`
	Kind        string          `json:"kind,omitempty"`
	ChannelType string          `json:"channel_type,omitempty"`
	FetchMode   string          `json:"fetch_mode,omitempty"`
	TargetDir   string          `json:"target_dir,omitempty"`
	URL         string          `json:"url"`
	Enabled     *bool           `json:"enabled,omitempty"`
	Template    json.RawMessage `json:"template"`
}

// SourcePackManifest 源包声明（规范 §3）。
type SourcePackManifest struct {
	ID             string               `json:"id"`
	Name           string               `json:"name"`
	Version        string               `json:"version"`
	Description    string               `json:"description,omitempty"`
	Author         string               `json:"author,omitempty"`
	License        string               `json:"license,omitempty"`
	MinCoreVersion string               `json:"min_core_version,omitempty"`
	MinSchema      *int                 `json:"min_schema,omitempty"`
	MaxSchema      *int                 `json:"max_schema,omitempty"`
	Kind           string               `json:"kind"`
	Target         []string             `json:"target,omitempty"`
	Compliance     *SourcePackCompliance `json:"compliance"`
	Sources        []SourcePackEntry    `json:"sources"`
}

// SourcePack 解析后的源包（manifest + 包内文件清单，供审计留痕）。
type SourcePack struct {
	Manifest *SourcePackManifest `json:"manifest"`
	Files    []string           `json:"files,omitempty"`
}

// SourcePackErr 校验失败（携带协议错误码，供 handler 直出 422）。
type SourcePackErr struct {
	Code string
	Msg  string
}

func (e *SourcePackErr) Error() string { return e.Code + ": " + e.Msg }

// ParseSourcePackZip 解析源包 zip：读 manifest.json，合并 sources/*.json（外置覆盖同名 id），
// 拦截可执行物。files 返回包内文件清单（审计用）。
func ParseSourcePackZip(raw []byte) (*SourcePack, error) {
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return nil, &SourcePackErr{"SOURCE_PACK_BAD_ZIP", "无法解析 zip：" + err.Error()}
	}
	var manifestRaw []byte
	external := map[string]SourcePackEntry{} // id → 条目（外置优先）
	var extOrder []string
	files := make([]string, 0, len(zr.File))
	for _, zf := range zr.File {
		name := strings.TrimPrefix(strings.ReplaceAll(zf.Name, "\\", "/"), "./")
		files = append(files, name)
		if zf.FileInfo().IsDir() {
			continue
		}
		if ext := strings.ToLower(path.Ext(name)); sourcePackBannedExt[ext] {
			return nil, &SourcePackErr{"SOURCE_PACK_EXECUTABLE_DENIED",
				"源包禁止包含可执行物，发现 " + name + "（源包为纯配置包，协议 §3 不允许任意代码执行）"}
		}
		base := path.Base(name)
		if base == "manifest.json" && manifestRaw == nil {
			rc, err := zf.Open()
			if err != nil {
				continue
			}
			manifestRaw, _ = io.ReadAll(io.LimitReader(rc, 1<<20))
			rc.Close()
			continue
		}
		if strings.HasPrefix(name, "sources/") && strings.EqualFold(path.Ext(name), ".json") {
			rc, err := zf.Open()
			if err != nil {
				continue
			}
			raw, _ := io.ReadAll(io.LimitReader(rc, 1<<20))
			rc.Close()
			var e SourcePackEntry
			if err := json.Unmarshal(raw, &e); err != nil {
				return nil, &SourcePackErr{"SOURCE_PACK_BAD_ENTRY", "源模板文件 " + name + " 不是合法 JSON：" + err.Error()}
			}
			if e.ID == "" {
				e.ID = strings.TrimSuffix(base, path.Ext(base))
			}
			external[e.ID] = e
			extOrder = append(extOrder, e.ID)
		}
	}
	if len(manifestRaw) == 0 {
		return nil, &SourcePackErr{"SOURCE_PACK_NO_MANIFEST", "zip 内未找到 manifest.json"}
	}
	var m SourcePackManifest
	if err := json.Unmarshal(manifestRaw, &m); err != nil {
		return nil, &SourcePackErr{"SOURCE_PACK_BAD_MANIFEST", "manifest.json 非法：" + err.Error()}
	}
	// 外置条目合并：同名 id 覆盖内嵌（规范 §2：并存时外置优先）
	for i := range m.Sources {
		if m.Sources[i].ID == "" {
			m.Sources[i].ID = fmt.Sprintf("%s-%d", m.ID, i+1)
			continue
		}
		if e, ok := external[m.Sources[i].ID]; ok {
			m.Sources[i] = e
			delete(external, m.Sources[i].ID)
		}
	}
	for _, id := range extOrder {
		if e, ok := external[id]; ok {
			m.Sources = append(m.Sources, e)
		}
	}
	return &SourcePack{Manifest: &m, Files: files}, nil
}

// ValidateSourcePackOptions 校验选项（SSRF 校验在离线测试时可跳过）。
type ValidateSourcePackOptions struct {
	ShellID     string // 本壳标识（target 匹配对象）
	HostVersion string // 宿主核心版本（min_core_version 下限比较基准）
	SkipSSRF    bool   // 跳过 SSRF 白名单校验（仅测试用；线上必须 false）
}

// VersionAtLeast 语义化版本下限比较（a >= b），空 b 视为无要求。
// 与插件体系同口径（规范 §3.2：源包复用 v2 比较，不另立体系）。
func VersionAtLeast(a, b string) bool {
	if b == "" || b == "0" {
		return true
	}
	pa := strings.Split(strings.TrimPrefix(a, "v"), ".")
	pb := strings.Split(strings.TrimPrefix(b, "v"), ".")
	for i := 0; i < len(pb); i++ {
		va, vb := 0, 0
		if i < len(pa) {
			fmt.Sscanf(pa[i], "%d", &va)
		}
		fmt.Sscanf(pb[i], "%d", &vb)
		if va != vb {
			return va > vb
		}
	}
	return true
}

// ValidateSourcePack 源包安装前校验（规范 §3.2 + §4.2）。返回 *SourcePackErr 供 handler 直出 422。
// 校验全过才允许导入——不合规条目会导致整包失败（原子回滚，不写半包）。
func ValidateSourcePack(p *SourcePack, opt ValidateSourcePackOptions) error {
	if p == nil || p.Manifest == nil {
		return &SourcePackErr{"SOURCE_PACK_BAD_MANIFEST", "源包 manifest 缺失"}
	}
	m := p.Manifest
	if m.ID == "" {
		return &SourcePackErr{"SOURCE_PACK_BAD_MANIFEST", "manifest.id 必填（建议 source-pack- 前缀）"}
	}
	if m.Kind != SourcePackKind {
		return &SourcePackErr{"SOURCE_PACK_BAD_KIND",
			"manifest.kind 必须为 " + SourcePackKind + "，实际 " + m.Kind}
	}
	if m.Name == "" || m.Version == "" {
		return &SourcePackErr{"SOURCE_PACK_BAD_MANIFEST", "manifest.name / version 必填"}
	}
	// 宿主兼容：复用 v2 下限比较（规范 §3.2 问题 3 答复）
	if m.MinCoreVersion != "" && !VersionAtLeast(opt.HostVersion, m.MinCoreVersion) {
		return &SourcePackErr{"PLUGIN_CORE_TOO_OLD",
			"宿主版本 " + opt.HostVersion + " 低于源包要求的 " + m.MinCoreVersion}
	}
	// schema 区间（当前恒 1）
	if m.MinSchema != nil && *m.MinSchema > SourcePackSchemaVersion {
		return &SourcePackErr{"MARKET_SCHEMA_MISMATCH",
			fmt.Sprintf("源包要求 schema >= %d，宿主当前 schema=%d", *m.MinSchema, SourcePackSchemaVersion)}
	}
	if m.MaxSchema != nil && *m.MaxSchema < SourcePackSchemaVersion {
		return &SourcePackErr{"MARKET_SCHEMA_MISMATCH",
			fmt.Sprintf("源包仅支持 schema <= %d，宿主当前 schema=%d", *m.MaxSchema, SourcePackSchemaVersion)}
	}
	// 目标壳（沿用 B3 口径）
	if len(m.Target) > 0 {
		ok := false
		for _, t := range m.Target {
			if strings.EqualFold(strings.TrimSpace(t), opt.ShellID) || t == "*" {
				ok = true
				break
			}
		}
		if !ok {
			return &SourcePackErr{"MARKET_TARGET_MISMATCH",
				"该源包不适用于本壳（target=" + strings.Join(m.Target, ",") + "，本壳=" + opt.ShellID + "）"}
		}
	}
	// 合规声明（上架审核硬门槛）
	if m.Compliance == nil || m.Compliance.TargetSite == "" || m.Compliance.Usage == "" {
		return &SourcePackErr{"SOURCE_PACK_COMPLIANCE_REQUIRED",
			"compliance.target_site 与 compliance.usage 必填（上架审核硬门槛）"}
	}
	if len(m.Sources) == 0 {
		return &SourcePackErr{"SOURCE_PACK_EMPTY", "源包不含任何源模板条目（sources 为空且无 sources/*.json）"}
	}
	seen := map[string]bool{}
	for i := range m.Sources {
		e := &m.Sources[i]
		if e.ID == "" {
			e.ID = fmt.Sprintf("%s-%d", m.ID, i+1)
		}
		if seen[e.ID] {
			return &SourcePackErr{"SOURCE_PACK_DUPLICATE_ID", "源模板 id 重复：" + e.ID}
		}
		seen[e.ID] = true
		if e.Name == "" {
			return &SourcePackErr{"SOURCE_PACK_BAD_ENTRY", "源模板 " + e.ID + " 缺 name"}
		}
		if e.URL == "" {
			return &SourcePackErr{"SOURCE_PACK_BAD_ENTRY", "源模板 " + e.ID + " 缺 url"}
		}
		if e.Kind == "" {
			e.Kind = "job"
		}
		// kind=job 且 city 空 → 拒（规范 §4.1：job 源必须声明城市）
		if e.Kind == "job" && e.City == "" {
			return &SourcePackErr{"SOURCE_PACK_BAD_ENTRY", "源模板 " + e.ID + "：kind=job 必须声明 city"}
		}
		// SSRF：列表页 URL 必须指向公网（与采集器同源防护）
		if !opt.SkipSSRF {
			if err := checkPublicURL(e.URL); err != nil {
				return &SourcePackErr{"SOURCE_PACK_SSRF_BLOCKED",
					"源模板 " + e.ID + " 的 url 未通过 SSRF 白名单校验：" + err.Error()}
			}
		}
		if err := validateSourceTemplate(e.Template); err != nil {
			return &SourcePackErr{err.Code, "源模板 " + e.ID + "：" + err.Msg}
		}
		// 归一化（写库前定形，避免脏值入库）
		e.ChannelType = NormalizeChannelType(e.ChannelType)
		e.FetchMode = NormalizeFetchMode(e.FetchMode)
	}
	return nil
}

// validateSourceTemplate 模板结构校验（规范 §4.2：list.urls 非空数组；fields 至少含 title/link）。
func validateSourceTemplate(raw json.RawMessage) *SourcePackErr {
	if len(raw) == 0 {
		return &SourcePackErr{"SOURCE_PACK_BAD_TEMPLATE", "template 必填（站点模板 JSON）"}
	}
	var t struct {
		List struct {
			URLs         []string       `json:"urls"`
			ItemSelector string         `json:"item_selector"`
			Fields       map[string]any `json:"fields"`
		} `json:"list"`
	}
	if err := json.Unmarshal(raw, &t); err != nil {
		return &SourcePackErr{"SOURCE_PACK_BAD_TEMPLATE", "template 非法 JSON：" + err.Error()}
	}
	if len(t.List.URLs) == 0 {
		return &SourcePackErr{"SOURCE_PACK_BAD_TEMPLATE", "template.list.urls 必须为非空数组"}
	}
	if t.List.ItemSelector == "" {
		return &SourcePackErr{"SOURCE_PACK_BAD_TEMPLATE", "template.list.item_selector 必填"}
	}
	if _, ok := t.List.Fields["title"]; !ok {
		return &SourcePackErr{"SOURCE_PACK_BAD_TEMPLATE", "template.list.fields 必须含 title"}
	}
	if _, ok := t.List.Fields["link"]; !ok {
		return &SourcePackErr{"SOURCE_PACK_BAD_TEMPLATE", "template.list.fields 必须含 link"}
	}
	return nil
}

// SourcePackToSources 把校验通过的源包条目转成待入库的 Source（pack_id 记录来源包，供覆盖更新与卸载）。
func SourcePackToSources(m *SourcePackManifest) []*Source {
	out := make([]*Source, 0, len(m.Sources))
	for i := range m.Sources {
		e := &m.Sources[i]
		tpl := e.Template
		if len(tpl) == 0 {
			tpl = json.RawMessage("{}")
		}
		enabled := true
		if e.Enabled != nil {
			enabled = *e.Enabled
		}
		out = append(out, &Source{
			ID:          e.ID,
			City:        e.City,
			Name:        e.Name,
			Kind:        e.Kind,
			ChannelType: NormalizeChannelType(e.ChannelType),
			FetchMode:   NormalizeFetchMode(e.FetchMode),
			TargetDir:   e.TargetDir,
			URL:         e.URL,
			Template:    tpl,
			Enabled:     enabled,
			PackID:      m.ID,
		})
	}
	return out
}

// ImportSources 事务导入源包条目（规范 §6：同 id 重装=覆盖式更新，enabled 保留原状态）。
// 任一条目失败即整体回滚——不写半包。返回新增/更新条数。
func (s *SourceStore) ImportSources(ctx context.Context, items []*Source) (created, updated int, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	now := time.Now().Unix()
	for _, src := range items {
		var oldEnabled int
		qErr := tx.QueryRowContext(ctx, `SELECT enabled FROM sources WHERE id=?`, src.ID).Scan(&oldEnabled)
		enabled := 1
		if !src.Enabled {
			enabled = 0
		}
		if qErr == nil {
			// 已存在：覆盖配置但保留用户手动改过的启停状态
			enabled = oldEnabled
			updated++
			if _, err = tx.ExecContext(ctx,
				`UPDATE sources SET city=?, name=?, kind=?, channel_type=?, fetch_mode=?, target_dir=?, url=?, template=?, pack_id=?, updated_at=?
				 WHERE id=?`,
				src.City, src.Name, src.Kind, src.ChannelType, src.FetchMode, src.TargetDir, src.URL,
				string(src.Template), src.PackID, now, src.ID); err != nil {
				return 0, 0, fmt.Errorf("service: 更新源 %s 失败: %w", src.ID, err)
			}
			continue
		}
		created++
		if _, err = tx.ExecContext(ctx,
			`INSERT INTO sources (id, city, name, kind, channel_type, fetch_mode, target_dir, url, template, enabled, pack_id, created_at, updated_at)
			 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			src.ID, src.City, src.Name, src.Kind, src.ChannelType, src.FetchMode, src.TargetDir, src.URL,
			string(src.Template), enabled, src.PackID, now, now); err != nil {
			return 0, 0, fmt.Errorf("service: 写入源 %s 失败: %w", src.ID, err)
		}
		src.Enabled = enabled == 1
	}
	if err = tx.Commit(); err != nil {
		return 0, 0, err
	}
	return created, updated, nil
}

// DeleteSourcesByPack 卸载源包：删除该包导入的全部源（pack_id 精确匹配，不影响用户自建源）。
func (s *SourceStore) DeleteSourcesByPack(ctx context.Context, packID string) (int, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM sources WHERE pack_id=?`, packID)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}
