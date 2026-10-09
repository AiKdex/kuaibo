// blog.go 实现"目录即博客"（klog 地基）：固定博客目录 + 自动发布分享。
// 设计：home 空间根下固定 `博客/` 目录（BlogDirID），种子时自动创建并挂永久 dir 分享
// （token=blog，scope=dir，不过期不撤销）。用户把文件/子目录放进博客目录 = 发布；
// 子目录即博客分类（public/posts 展开带 path，前端按 path 首段分组）。
// 移除/删除文件即下线；在共享管理页可撤销分享整体关闭博客（revoke 后可重建）。
package service

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// BlogDirID 固定博客目录 ID（幂等种子用）。
const BlogDirID = "00000000-0000-0000-0000-000000000010"

// BlogToken 博客目录分享的固定 token（记忆友好 + 幂等查询）。
const BlogToken = "blog"

// EnsureFileSlug 为博客目录下的文件生成公开稳定链接 slug（幂等）：
// slug = 文件名去扩展名；同名冲突自动加 -2/-3 后缀。文件名/分类改名后 slug 不变 → 外链不碎。
// 仅对"无 slug"的文件生成一次，之后固定（第三轮反馈 2.2a）。
func EnsureFileSlug(ctx context.Context, db *sql.DB, fileID, name string) (string, error) {
	base := strings.TrimSpace(name)
	if i := strings.LastIndex(base, "."); i > 0 {
		base = base[:i]
	}
	base = strings.TrimSpace(base)
	if base == "" {
		base = "post"
	}
	// 查重：已存在的同名 slug 追加 -N
	slug := base
	for n := 2; ; n++ {
		var cnt int
		if err := db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM files WHERE slug=? AND id<>?`, slug, fileID).Scan(&cnt); err != nil {
			return "", err
		}
		if cnt == 0 {
			break
		}
		slug = fmt.Sprintf("%s-%d", base, n)
	}
	if _, err := db.ExecContext(ctx,
		`UPDATE files SET slug=?, updated_at=? WHERE id=? AND (slug IS NULL OR slug='')`, slug, time.Now().UnixMilli(), fileID); err != nil {
		return "", err
	}
	return slug, nil
}

// EnsureDirSlugs 为目录（含全部子目录）下无 slug 文件补齐 slug（blogManage 加载时调用）。
// 递归 BFS 各层子目录（modernc 驱动对递归 CTE 支持不稳，不用 CTE）。
func EnsureDirSlugs(ctx context.Context, db *sql.DB, dirID string) error {
	levels := []string{dirID}
	for len(levels) > 0 {
		ph := strings.TrimSuffix(strings.Repeat("?,", len(levels)), ",")
		args := make([]any, len(levels))
		for i, d := range levels {
			args[i] = d
		}
		rows, err := db.QueryContext(ctx,
			`SELECT id, name, kind FROM files WHERE parent_id IN (`+ph+`) AND deleted_at IS NULL`, args...)
		if err != nil {
			return err
		}
		var pending []struct{ id, name string }
		var next []string
		for rows.Next() {
			var id, name, kind string
			if rows.Scan(&id, &name, &kind) != nil {
				continue
			}
			if kind == "dir" {
				next = append(next, id)
			} else {
				pending = append(pending, struct{ id, name string }{id, name})
			}
		}
		rows.Close()
		for _, p := range pending {
			if _, err := EnsureFileSlug(ctx, db, p.id, p.name); err != nil {
				return err
			}
		}
		levels = next
	}
	return nil
}

// EnsureBlogSpace 幂等确保博客目录与公开分享存在（启动种子；可重复调用）。
// 目录：home 空间根下 name='博客'（parent_id NULL）；分享：token='blog' scope=dir 永久。
func EnsureBlogSpace(ctx context.Context, db *sql.DB) error {
	now := time.Now().UnixMilli()

	// 1) 博客目录（固定 ID，已存在则跳过）
	var cnt int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM files WHERE id=? AND kind='dir' AND deleted_at IS NULL`, BlogDirID).Scan(&cnt); err != nil {
		return fmt.Errorf("check blog dir: %w", err)
	}
	if cnt == 0 {
		if _, err := db.ExecContext(ctx,
			`INSERT INTO files (id, space_id, owner_id, parent_id, name, kind, storage_ref, version, content_state, created_at, updated_at)
			 VALUES (?, ?, ?, NULL, '博客', 'dir', '博客', 1, '{"visibility":"private","status":"draft"}', ?, ?)`,
			BlogDirID, SystemHomeSpaceID, SystemOwnerID, now, now); err != nil {
			return fmt.Errorf("seed blog dir: %w", err)
		}
	}

	// 2) 博客公开分享（固定 token）：无记录则新建；已有记录（含已撤销）一律不自动复活——
	//    博客对外状态由 settings.blog.open 开关控制（共享管理页撤销 blog 分享被 403 锁定），
	//    避免"管理员以为撤销=关博客，重启被静默打开"的语义冲突（Aikdex 第三轮反馈 1.1）。
	var revoked int64
	var has int
	err := db.QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(MAX(revoked_at),0) FROM shares WHERE owner_id=? AND token=?`,
		SystemOwnerID, BlogToken).Scan(&has, &revoked)
	if err != nil {
		return fmt.Errorf("check blog share: %w", err)
	}
	if has == 0 {
		if _, err := db.ExecContext(ctx,
			`INSERT INTO shares (id, file_id, dir_id, owner_id, token, permission, scope, expires_at, created_at)
			 VALUES (?, ?, ?, ?, ?, 'read', 'dir', 0, ?)`,
			uuid.NewString(), BlogDirID, BlogDirID, SystemOwnerID, BlogToken, now); err != nil {
			return fmt.Errorf("seed blog share: %w", err)
		}
	}

	// 3) 内置博客插件种子注册（协议示例：文章元信息/列表徽章/信息卡/SEO meta；第三方插件由 API 注册）
	plugins := []struct {
		id, name, desc, mount, frontend string
	}{
		{"blog-meta-footer", "文章元信息", "文章页正文下显示分类/更新时间/大小（内置示例插件，演示博客插件协议）", `["post_bottom"]`, "blog-meta-footer"},
		{"blog-tag-badges", "列表徽章", "公开首页文章列表项显示分类徽章（内置示例插件，挂载点 list_item）", `["list_item"]`, "blog-tag-badges"},
		{"blog-meta-card", "文章信息卡", "文章页侧栏信息卡：分类/更新时间/大小（内置示例插件，挂载点 sidebar）", `["sidebar"]`, "blog-meta-card"},
		{"blog-seo-meta", "SEO 元信息", "公开页注入 description/OG 标签（内置示例插件，挂载点 head）", `["head"]`, "blog-seo-meta"},
		{"blog-comments", "文章评论", "文章页评论列表与发表（评论核 API 展示层；读公开写需登录，挂载点 post_bottom）", `["post_bottom"]`, "blog-comments"},
		{"blog-related-posts", "相关推荐", "文章页相关推荐：同分类优先，按更新时间（挂载点 post_bottom）", `["post_bottom"]`, "blog-related-posts"},
		{"blog-archives", "归档", "侧栏按月归档文章列表（挂载点 sidebar）", `["sidebar"]`, "blog-archives"},
		{"blog-tag-cloud", "分类云", "侧栏分类计数云（挂载点 sidebar）", `["sidebar"]`, "blog-tag-cloud"},
		{"blog-cover", "封面图", "列表项封面：从正文预览提取首图（挂载点 list_item）", `["list_item"]`, "blog-cover"},
		{"blog-reader-ai", "读者问答", "公开页读者 AI 问答（限公开文章上下文，挂载点 post_bottom）", `["post_bottom"]`, "blog-reader-ai"},
		{"blog-stats", "访问统计", "文章 PV 本地计数（挂载点 post_bottom）", `["post_bottom"]`, "blog-stats"},
		{"blog-voice-read", "语音朗读", "文章页标题下元信息行内联朗读按钮（复用 ai.tts 语音合成，挂载点 post_meta）", `["post_meta"]`, "blog-voice-read"},
	}
	for _, p := range plugins {
		if _, err := db.ExecContext(ctx,
			`INSERT OR IGNORE INTO blog_plugins
			   (id, name, version, description, author, mount_points, api_permissions, frontend_entry, enabled, created_at, updated_at)
			 VALUES (?, ?, '1.0.0', ?, '爱库录', ?, '["blog.read"]', ?, 1, ?, ?)`,
			p.id, p.name, p.desc, p.mount, p.frontend, now, now); err != nil {
			return fmt.Errorf("seed blog plugin %s: %w", p.id, err)
		}
	}
	// 内置展示名回填：上表是 INSERT OR IGNORE，改常量只影响全新库，已有库拿不到新名字。
	// 仅当库里的当前值仍等于旧名时才改（站长若已自行改名则不覆盖）。
	for _, r := range builtinNameBackfill {
		if _, err := db.ExecContext(ctx,
			`UPDATE blog_plugins SET name=?, updated_at=? WHERE id=? AND name=?`,
			r.newName, now, r.id, r.oldName); err != nil {
			return fmt.Errorf("backfill blog plugin name %s: %w", r.id, err)
		}
	}
	// 内置插件挂载点回填：blog-voice-read 从文章底部（post_bottom）上移到标题下元信息行（post_meta）。
	// 同样只改「仍等于旧值」的库，站长若已自行调整过挂载点则不覆盖。
	if _, err := db.ExecContext(ctx,
		`UPDATE blog_plugins SET mount_points=?, updated_at=? WHERE id=? AND mount_points=?`,
		`["post_meta"]`, now, "blog-voice-read", `["post_bottom"]`); err != nil {
		return fmt.Errorf("backfill blog plugin mount %s: %w", "blog-voice-read", err)
	}
	return nil
}

// builtinNameBackfill 内置插件展示名回填表（仅内置 id，不碰第三方包）。
// 背景：应用中心「已装应用」列表原先把插件 id 当主标题、name 当副标题，
// 导致站长先看到 blog-meta-footer 这类英文机器名。改为 name 主、id 次之后，
// 少数早期种子里用英文名的条目（如 SEO Meta）就露了出来，这里一次性改成中文。
var builtinNameBackfill = []struct{ id, oldName, newName string }{
	{"blog-seo-meta", "SEO Meta", "SEO 元信息"},
}

// BlogDirIDOf 返回博客目录 ID（供 handler 查询博客源）。
func BlogDirIDOf() string { return BlogDirID }
