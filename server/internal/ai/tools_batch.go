// tools_batch.go 实现 batch_tag_files / batch_organize / batch_rename 三个批量工具。
// 复用 service 层已有能力（TagStore / FileStore），不直接操作底层存储。
package ai

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/AiKMAP/AiKmap/server/internal/service"
)

// NewBatchTagFilesTool 创建 batch_tag_files 工具：批量给文件打标签。
// tag_path 格式如 "音乐/粤语"，不存在则自动创建层级；幂等（已打过的跳过）；单次限 500。
func NewBatchTagFilesTool(files *service.FileStore, tags *service.TagStore) *Tool {
	return &Tool{
		Name:        "batch_tag_files",
		Description: "批量给一组文件打标签（支持层级路径，如 '技术/分布式/Raft'）。标签不存在时自动创建。幂等：已打过的标签跳过。当用户说'给这些文件加上技术标签'、'把所有 md 文件标为文档'时调用。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file_ids":         map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "要打标的文件 ID 列表（最多 500）"},
				"tag_path":         map[string]any{"type": "string", "description": "标签的层级路径，用 / 分隔，如 '音乐/粤语' 或 '技术/分布式/Raft'"},
				"replace_existing": map[string]any{"type": "boolean", "description": "是否替换已有标签（false=追加，默认追加）"},
			},
			"required": []string{"file_ids", "tag_path"},
		},
		Run: func(ctx context.Context, raw json.RawMessage) (string, error) {
			var args struct {
				FileIDs       []string `json:"file_ids"`
				TagPath       string   `json:"tag_path"`
				ReplaceExists bool     `json:"replace_existing"`
			}
			if err := json.Unmarshal(raw, &args); err != nil {
				return "", err
			}
			if len(args.FileIDs) == 0 {
				return "", errors.New("file_ids 不能为空")
			}
			if len(args.FileIDs) > 500 {
				return "", errors.New("file_ids 超过 500，请分批处理")
			}
			if args.TagPath == "" {
				return "", errors.New("tag_path 不能为空")
			}

			// 取 ownerID：通过第一个文件查（标签与文件同属一个 owner）
			var ownerID string
			firstFile, err := files.Get(ctx, args.FileIDs[0])
			if err != nil {
				return "", fmt.Errorf("获取文件失败: %w", err)
			}
			ownerID = firstFile.OwnerID

			// 解析 tag_path：逐层查找/自动创建（复用 TagStore.EnsurePath）
			tagID, err := tags.EnsurePath(ctx, ownerID, args.TagPath)
			if err != nil {
				return "", fmt.Errorf("解析标签路径失败: %w", err)
			}

			type failEntry struct {
				FileID string `json:"file_id"`
				Reason string `json:"reason"`
			}
			var (
				success int
				fails   []failEntry
			)

			// 分批处理（每批 50）
			for i := 0; i < len(args.FileIDs); i += 50 {
				end := i + 50
				if end > len(args.FileIDs) {
					end = len(args.FileIDs)
				}
				batch := args.FileIDs[i:end]

				for _, fid := range batch {
					var tagIDs []string
					if args.ReplaceExists {
						tagIDs = []string{tagID}
					} else {
						// 追加模式：取已有标签 + 新标签
						existing, err := tags.FileTags(ctx, fid)
						if err != nil {
							fails = append(fails, failEntry{fid, fmt.Sprintf("查已有标签失败: %v", err)})
							continue
						}
						for _, t := range existing {
							tagIDs = append(tagIDs, t.ID)
						}
						// 幂等：已存在则跳过
						dup := false
						for _, t := range existing {
							if t.ID == tagID {
								dup = true
								break
							}
						}
						if dup {
							success++ // 视为成功（已打过）
							continue
						}
						tagIDs = append(tagIDs, tagID)
					}

					if err := tags.SetFileTags(ctx, ownerID, fid, tagIDs); err != nil {
						fails = append(fails, failEntry{fid, err.Error()})
						continue
					}
					success++
				}
			}

			b, _ := json.Marshal(map[string]any{
				"tag_path": args.TagPath,
				"tag_id":   tagID,
				"total":    len(args.FileIDs),
				"success":  success,
				"failed":   len(fails),
				"fails":    fails,
				"mode":     map[bool]string{true: "replace", false: "append"}[args.ReplaceExists],
				"note":     "幂等：已打过的标签跳过。如需清理标签请用 replace_existing=true。",
			})
			return string(b), nil
		},
	}
}

// NewBatchOrganizeTool 创建 batch_organize 工具：按规则批量整理文件到子目录。
// rule: by_type（按扩展名归入 类型/xxx/）| by_tag（按标签归入 标签/xxx/）| custom（自定义映射 JSON）。
// dry_run=true 时只返回预览不落盘。
func NewBatchOrganizeTool(db *sql.DB, files *service.FileStore) *Tool {
	return &Tool{
		Name:        "batch_organize",
		Description: "按规则批量整理文件到子目录。支持：按扩展名分类（如 md → 文档/、pdf → 文档/、jpg → 图片/）、按标签分类、或自定义 JSON 映射。dry_run=true 只返回预览不落盘。当用户说'把 md 文件归到文档文件夹'、'按类型整理一下'时调用。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"dir_id":   map[string]any{"type": "string", "description": "起始目录 ID（整理范围：该目录及其子目录下的文件）"},
				"rule":     map[string]any{"type": "string", "enum": []string{"by_type", "by_tag", "custom"}, "description": "整理规则（默认 by_type）"},
				"mapping":  map[string]any{"type": "object", "description": "custom 规则时的映射 JSON：{'.md': '文档', '.pdf': '文档'}"},
				"dry_run":  map[string]any{"type": "boolean", "description": "是否只预览不落盘（默认 true，安全第一）"},
				"owner_id": map[string]any{"type": "string", "description": "操作发起者用户 ID（必填，用于审计）"},
				"space_id": map[string]any{"type": "string", "description": "空间 ID（用于 EnsurePath 创建目录）"},
			},
			"required": []string{"dir_id", "owner_id", "space_id"},
		},
		Run: func(ctx context.Context, raw json.RawMessage) (string, error) {
			var args struct {
				DirID   string         `json:"dir_id"`
				Rule    string         `json:"rule"`
				Mapping map[string]any `json:"mapping"`
				DryRun  bool           `json:"dry_run"`
				OwnerID string         `json:"owner_id"`
				SpaceID string         `json:"space_id"`
			}
			if err := json.Unmarshal(raw, &args); err != nil {
				return "", err
			}
			// DryRun 默认安全：LLM 未显式传 dry_run 时按预览（true）处理，绝不默认落盘。
			var probe map[string]json.RawMessage
			if err := json.Unmarshal(raw, &probe); err == nil {
				if _, ok := probe["dry_run"]; !ok {
					args.DryRun = true
				}
			} else {
				args.DryRun = true
			}
			if args.DirID == "" || args.OwnerID == "" || args.SpaceID == "" {
				return "", errors.New("dir_id / owner_id / space_id 不能为空")
			}
			if args.Rule == "" {
				args.Rule = "by_type"
			}
			if args.Rule == "custom" && len(args.Mapping) == 0 {
				return "", errors.New("custom 规则需要提供 mapping")
			}

			// 收集 dir_id 下的所有文件（含子目录）
			fileList, err := collectFilesInSubtree(ctx, files, db, args.DirID)
			if err != nil {
				return "", err
			}

			type movePlan struct {
				FileID   string `json:"file_id"`
				FileName string `json:"file_name"`
				FromDir  string `json:"from_dir"`
				ToDir    string `json:"to_dir"`
				Subdir   string `json:"subdir"`
			}

			var plans []movePlan
			typeCount := map[string]int{}

			for _, f := range fileList {
				// 决定目标子目录名
				var subdir string
				switch args.Rule {
				case "by_type":
					ext := strings.ToLower(extOf(f.Name))
					subdir = typeCategory(ext)
				case "by_tag":
					// 取文件第一个标签 path 的第一段作为子目录名（未打标归入"未打标签"）
					var tp string
					if err := db.QueryRowContext(ctx,
						`SELECT t.path FROM file_tags ft JOIN tags t ON ft.tag_id=t.id WHERE ft.file_id=? ORDER BY t.path LIMIT 1`,
						f.ID).Scan(&tp); err == nil && tp != "" {
						subdir = strings.SplitN(tp, "/", 2)[0]
					} else {
						subdir = "未打标签"
					}
				case "custom":
					ext := strings.ToLower(extOf(f.Name))
					if v, ok := args.Mapping[ext]; ok {
						subdir = fmt.Sprintf("%v", v)
					} else if v, ok := args.Mapping["*"]; ok {
						subdir = fmt.Sprintf("%v", v)
					} else {
						continue // 未映射跳过
					}
				}

				typeCount[subdir]++
				plans = append(plans, movePlan{
					FileID:   f.ID,
					FileName: f.Name,
					FromDir:  args.DirID,
					Subdir:   subdir,
					ToDir:    args.DirID + "/" + subdir,
				})
			}

			type failEntry struct {
				FileID string `json:"file_id"`
				Name   string `json:"name"`
				Reason string `json:"reason"`
			}

			var moved, skipped int
			var fails []failEntry

			if args.DryRun {
				for range plans {
					skipped++ // 预览不落盘
				}
			} else {
				// 先确保目标目录存在（EnsurePath 逐级创建）
				for subdir := range typeCount {
					if _, err := files.EnsurePath(ctx, args.OwnerID, args.SpaceID, args.DirID, subdir); err != nil {
						return "", fmt.Errorf("创建目录 %q 失败: %w", subdir, err)
					}
				}
				// 逐个移动
				for _, p := range plans {
					targetParentID, err := files.EnsurePath(ctx, args.OwnerID, args.SpaceID, args.DirID, p.Subdir)
					if err != nil {
						fails = append(fails, failEntry{p.FileID, p.FileName, err.Error()})
						continue
					}
					_, err = files.Move(ctx, args.OwnerID, p.FileID, targetParentID, "")
					if err != nil {
						fails = append(fails, failEntry{p.FileID, p.FileName, err.Error()})
						continue
					}
					moved++
				}
			}

			b, _ := json.Marshal(map[string]any{
				"rule":       args.Rule,
				"total":      len(plans),
				"moved":      moved,
				"skipped":    skipped,
				"dry_run":    args.DryRun,
				"type_count": typeCount,
				"fails":      fails,
				"note":       "dry_run=true 只预览不落盘。确认无误后设 dry_run=false 执行实际移动。同名冲突走现有自动改名逻辑（x-1.md）。",
			})
			return string(b), nil
		},
	}
}

// NewBatchRenameTool 创建 batch_rename 工具：按规则批量重命名文件。
// rule: prefix（加前缀）| suffix（加后缀）| replace（替换 old→new）| case（大小写）。
func NewBatchRenameTool(files *service.FileStore) *Tool {
	return &Tool{
		Name:        "batch_rename",
		Description: "按规则批量重命名文件：加前缀/后缀、替换字符、转大小写。保留扩展名。同名冲突自动改名。当用户说'给这些文件加上 Q1 前缀'、'把下划线换成横杠'、'全部转小写'时调用。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file_ids":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "要重命名的文件 ID 列表（最多 500）"},
				"rule":        map[string]any{"type": "string", "enum": []string{"prefix", "suffix", "replace", "case"}, "description": "重命名规则"},
				"value":       map[string]any{"type": "string", "description": "prefix/suffix 的值，或 replace 的 old 值，或 case 的目标（lower/upper/title）"},
				"replacement": map[string]any{"type": "string", "description": "replace 规则的 new 值"},
				"owner_id":    map[string]any{"type": "string", "description": "操作发起者用户 ID（必填，用于审计）"},
			},
			"required": []string{"file_ids", "rule", "owner_id"},
		},
		Run: func(ctx context.Context, raw json.RawMessage) (string, error) {
			var args struct {
				FileIDs     []string `json:"file_ids"`
				Rule        string   `json:"rule"`
				Value       string   `json:"value"`
				Replacement string   `json:"replacement"`
				OwnerID     string   `json:"owner_id"`
			}
			if err := json.Unmarshal(raw, &args); err != nil {
				return "", err
			}
			if len(args.FileIDs) == 0 {
				return "", errors.New("file_ids 不能为空")
			}
			if len(args.FileIDs) > 500 {
				return "", errors.New("file_ids 超过 500，请分批处理")
			}
			if args.OwnerID == "" {
				return "", errors.New("owner_id 不能为空")
			}

			type failEntry struct {
				FileID  string `json:"file_id"`
				OldName string `json:"old_name"`
				Reason  string `json:"reason"`
			}
			type renameEntry struct {
				FileID  string `json:"file_id"`
				OldName string `json:"old_name"`
				NewName string `json:"new_name"`
			}

			var (
				success int
				fails   []failEntry
				results []renameEntry
			)

			for _, fid := range args.FileIDs {
				f, err := files.Get(ctx, fid)
				if err != nil {
					fails = append(fails, failEntry{fid, "", err.Error()})
					continue
				}
				if f.Kind != "file" {
					fails = append(fails, failEntry{fid, f.Name, "目标不是文件"})
					continue
				}

				newName := applyRenameRule(f.Name, args.Rule, args.Value, args.Replacement)
				if newName == f.Name {
					continue // 无变化跳过
				}

				// FileStore.Move 支持 newName 参数：同父目录 + 新名字 = 改名
				// 同名冲突自动改名（x.md → x-1.md），并触发 file.moved 事件 + 审计
				_, err = files.Move(ctx, args.OwnerID, fid, f.ParentID, newName)
				if err != nil {
					fails = append(fails, failEntry{fid, f.Name, fmt.Sprintf("改名失败: %v", err)})
					continue
				}
				results = append(results, renameEntry{fid, f.Name, newName})
				success++
			}

			b, _ := json.Marshal(map[string]any{
				"rule":    args.Rule,
				"total":   len(args.FileIDs),
				"success": success,
				"failed":  len(fails),
				"fails":   fails,
				"results": results,
				"note":    "已通过 FileStore.Move(newName) 执行真实改名；同名冲突自动改名（x.md → x-1.md）。",
			})
			return string(b), nil
		},
	}
}

// ---- 辅助函数 ----

// collectFilesInSubtree 收集 dirID 下所有文件（含子目录），通过 FileStore.Get 装配。
func collectFilesInSubtree(ctx context.Context, files *service.FileStore, db *sql.DB, dirID string) ([]*service.File, error) {
	if db == nil {
		return nil, errors.New("db 未注入")
	}
	// 递归取所有后代 id（目录 + 文件）；deleted_at 过滤放 CTE 内两处 SELECT（外层只有 id 列）
	rows, err := db.QueryContext(ctx, `
		WITH RECURSIVE sub(id) AS (
			SELECT id FROM files WHERE id=? AND deleted_at IS NULL
			UNION ALL
			SELECT f.id FROM files f JOIN sub ON f.parent_id = sub.id WHERE f.deleted_at IS NULL
		) SELECT id FROM sub`, dirID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// 逐个装配（复用已导出 Get，避免手写列扫描与 service 列序耦合）
	var out []*service.File
	for _, id := range ids {
		f, err := files.Get(ctx, id)
		if err != nil {
			continue
		}
		if f.Kind == "file" {
			out = append(out, f)
		}
	}
	return out, nil
}

// extOf 提取文件扩展名（含点），如 ".md"。
func extOf(name string) string {
	i := strings.LastIndex(name, ".")
	if i <= 0 {
		return ""
	}
	return strings.ToLower(name[i:])
}

// typeCategory 按扩展名返回"类型/xxx"子目录名。
func typeCategory(ext string) string {
	switch ext {
	case ".md", ".txt", ".doc", ".docx", ".pdf", ".rtf":
		return "文档"
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp", ".svg":
		return "图片"
	case ".mp3", ".wav", ".flac", ".aac", ".ogg":
		return "音频"
	case ".mp4", ".mkv", ".avi", ".mov", ".webm":
		return "视频"
	case ".xls", ".xlsx", ".csv":
		return "表格"
	case ".ppt", ".pptx", ".key":
		return "演示"
	case ".zip", ".rar", ".7z", ".tar", ".gz":
		return "压缩包"
	case ".go", ".py", ".js", ".ts", ".java", ".c", ".cpp", ".rs":
		return "代码"
	default:
		if ext == "" {
			return "无扩展名"
		}
		return "其他"
	}
}

// applyRenameRule 按规则生成新文件名（保留扩展名）。
func applyRenameRule(name, rule, value, replacement string) string {
	base := name
	ext := ""
	if i := strings.LastIndex(base, "."); i > 0 {
		ext = base[i:]
		base = base[:i]
	}
	switch rule {
	case "prefix":
		return value + base + ext
	case "suffix":
		return base + value + ext
	case "replace":
		return strings.ReplaceAll(base, value, replacement) + ext
	case "case":
		switch strings.ToLower(value) {
		case "lower":
			return strings.ToLower(base) + ext
		case "upper":
			return strings.ToUpper(base) + ext
		case "title":
			return strings.Title(base) + ext
		default:
			return name
		}
	default:
		return name
	}
}
