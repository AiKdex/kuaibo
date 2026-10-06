// tools_dedup.go 实现 dedup_files 工具：查重（按文件名 / 文件名+大小 / SHA256 内容哈希）。
// 只出清单不做删除（删除交用户确认或主代理执行）。
package ai

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

// NewDedupFilesTool 创建 dedup_files 工具。
// db 用于直接查 files 表（content_hash 查重需要跨文件对比，走 service 层会逐个拉取，直接 SQL 更高效）。
// homeSpaceID 为默认空间 ID：AI 未提供 space_id 时兜底（对话上下文通常没有空间 ID 概念）。
func NewDedupFilesTool(db *sql.DB, homeSpaceID string) *Tool {
	return &Tool{
		Name:        "dedup_files",
		Description: "在指定目录（或全空间）内找出重复文件，按不同维度分组：仅文件名、文件名+大小、SHA256 内容哈希。输出重复分组清单，不自动删除。当用户想清理重复文档、网盘多副本时调用。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"space_id":   map[string]any{"type": "string", "description": "空间 ID（必填）"},
				"dir_id":     map[string]any{"type": "string", "description": "起始目录 ID（可选，空=全空间根）"},
				"basis":      map[string]any{"type": "string", "enum": []string{"name", "name_size", "content_hash"}, "description": "查重维度：仅文件名 / 文件名+大小 / SHA256 内容哈希（默认 content_hash）"},
				"min_group":  map[string]any{"type": "integer", "description": "最小重复组大小（默认 2，至少 2 个才算重复）"},
				"max_groups": map[string]any{"type": "integer", "description": "最多返回的分组数（默认 30）"},
			},
			"required": []string{"space_id"},
		},
		Run: func(ctx context.Context, raw json.RawMessage) (string, error) {
			var args struct {
				SpaceID   string `json:"space_id"`
				DirID     string `json:"dir_id"`
				Basis     string `json:"basis"`
				MinGroup  int    `json:"min_group"`
				MaxGroups int    `json:"max_groups"`
			}
			if err := json.Unmarshal(raw, &args); err != nil {
				return "", err
			}
			if args.SpaceID == "" {
				// 兜底默认空间（AI 上下文没有空间 ID；单用户阶段固定 home 空间）
				args.SpaceID = homeSpaceID
			}
			if args.Basis == "" {
				args.Basis = "content_hash"
			}
			if args.MinGroup < 2 {
				args.MinGroup = 2
			}
			if args.MaxGroups <= 0 {
				args.MaxGroups = 30
			}

			// 收集目录子树 id（含自身）
			dirIDs := []string{}
			if args.DirID != "" {
				ids, err := collectSubtreeIDs(ctx, db, args.DirID)
				if err != nil {
					return "", fmt.Errorf("查目录子树失败: %w", err)
				}
				dirIDs = append([]string{args.DirID}, ids...)
			}

			groups, err := runDedup(ctx, db, args.SpaceID, dirIDs, args.Basis, args.MinGroup, args.MaxGroups)
			if err != nil {
				return "", err
			}

			b, err := json.Marshal(map[string]any{
				"basis":       args.Basis,
				"dir_id":      args.DirID,
				"group_count": len(groups),
				"groups":      groups,
				"note":        "仅输出重复分组清单，不自动删除。删除需用户确认后调用主代理。",
			})
			if err != nil {
				return "", err
			}
			return string(b), nil
		},
	}
}

// collectSubtreeIDs 递归收集 dirID 下所有子目录 id（不含自身）。
func collectSubtreeIDs(ctx context.Context, db *sql.DB, dirID string) ([]string, error) {
	rows, err := db.QueryContext(ctx, `
		WITH RECURSIVE sub(id) AS (
			SELECT id FROM files WHERE parent_id=? AND kind='dir' AND deleted_at IS NULL
			UNION ALL
			SELECT f.id FROM files f JOIN sub ON f.parent_id = sub.id WHERE f.kind='dir' AND f.deleted_at IS NULL
		) SELECT id FROM sub`, dirID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// runDedup 按指定维度执行查重，返回分组列表。
func runDedup(ctx context.Context, db *sql.DB, spaceID string, dirIDs []string, basis string, minGroup, maxGroups int) ([]map[string]any, error) {
	var query string
	var qargs []any
	qargs = append(qargs, spaceID)

	// 先按目录范围过滤文件
	dirFilter := ""
	if len(dirIDs) > 0 {
		ph := strings.TrimRight(strings.Repeat("?,", len(dirIDs)), ",")
		dirFilter = fmt.Sprintf(" AND parent_id IN (%s)", ph)
		for _, id := range dirIDs {
			qargs = append(qargs, id)
		}
	}

	switch basis {
	case "name":
		query = fmt.Sprintf(`
			SELECT id, name, size
			FROM files
			WHERE space_id=? AND kind='file' AND deleted_at IS NULL %s
			ORDER BY name, updated_at DESC`, dirFilter)
	case "name_size":
		query = fmt.Sprintf(`
			SELECT id, name, size
			FROM files
			WHERE space_id=? AND kind='file' AND deleted_at IS NULL AND size > 0 %s
			ORDER BY name, size, updated_at DESC`, dirFilter)
	case "content_hash":
		query = fmt.Sprintf(`
			SELECT id, name, size, sha256
			FROM files
			WHERE space_id=? AND kind='file' AND deleted_at IS NULL AND sha256 IS NOT NULL AND sha256 <> '' %s
			ORDER BY sha256, updated_at DESC`, dirFilter)
	default:
		return nil, fmt.Errorf("未知 basis: %s", basis)
	}

	rows, err := db.QueryContext(ctx, query, qargs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type fileRow struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Size int64  `json:"size"`
	}

	groups := map[string][]fileRow{}
	for rows.Next() {
		var (
			id, name string
			size     int64
			sha      sql.NullString
		)
		switch basis {
		case "content_hash":
			if err := rows.Scan(&id, &name, &size, &sha); err != nil {
				return nil, err
			}
			if !sha.Valid || sha.String == "" {
				continue
			}
			groups[sha.String] = append(groups[sha.String], fileRow{id, name, size})
		case "name_size":
			if err := rows.Scan(&id, &name, &size); err != nil {
				return nil, err
			}
			key := fmt.Sprintf("%s|%d", name, size)
			groups[key] = append(groups[key], fileRow{id, name, size})
		default: // name
			if err := rows.Scan(&id, &name, &size); err != nil {
				return nil, err
			}
			groups[name] = append(groups[name], fileRow{id, name, size})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var result []map[string]any
	for key, files := range groups {
		if len(files) < minGroup {
			continue
		}
		var keepID, keepName string
		var dupIDs []string
		for i, f := range files {
			if i == 0 {
				keepID, keepName = f.ID, f.Name
			} else {
				dupIDs = append(dupIDs, f.ID)
			}
		}
		g := map[string]any{
			"group_key": key,
			"basis":     basis,
			"count":     len(files),
			"keep": map[string]any{
				"file_id": keepID,
				"name":    keepName,
			},
			"dup_ids": dupIDs,
			"all":     files,
		}
		result = append(result, g)
		if len(result) >= maxGroups {
			break
		}
	}

	// count 多的在前
	for i := 1; i < len(result); i++ {
		for j := i; j > 0 && result[j]["count"].(int) > result[j-1]["count"].(int); j-- {
			result[j], result[j-1] = result[j-1], result[j]
		}
	}

	return result, nil
}
