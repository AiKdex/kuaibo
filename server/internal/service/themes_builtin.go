// themes_builtin.go 内置源码主题目录 + 主题改版升级迁移（2026-10-09）。
//
// 「只内置 aiklog，其余主题进应用中心按需安装」改版的单一定义点：
//  - BuiltinSpaThemes：改版前随主系统编译的全部 SPA 主题（17 个），现改为按需安装
//    （安装 = blog_plugins kind=theme 登记；SPA 组件仍随主系统构建分发）；
//  - MigrateLegacySpaThemes：升级兼容 —— 启动时把「站点/全局配置正在指向的主题」
//    自动登记为已安装，避免升级后线上博客掉皮（主题从未安装 → 前端回落默认皮）。
package service

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// BuiltinSpaTheme 内置源码主题元数据（与 web/src/themes/catalog.js 一一对应）。
type BuiltinSpaTheme struct {
	ID    string
	Title string
}

// BuiltinSpaThemes 常驻内置（aiklog/default）之外的全部源码主题。
// 顺序即应用中心「主题」类别展示顺序。
var BuiltinSpaThemes = []BuiltinSpaTheme{
	{"minimal", "极简阅读"},
	{"docs", "技术文档"},
	{"paper", "暖纸情报"},
	{"elevated", "高端暗色"},
	{"parchment", "暖纸杂志"},
	{"emforum", "论坛社区"},
	{"brutal", "新粗野"},
	{"aurora", "极光 Aurora"},
	{"verdant", "青野 Verdant"},
	{"zircon", "青璃 Zircon"},
	{"butterfly", "蝶语 Butterfly"},
	{"chenxi", "晨曦笔记 Chenxi"},
	{"jaded", "翡翠 Jaded"},
	{"zhicang", "知藏 Zhicang"},
	{"aiknav", "好站导航 AikNav"},
	{"clawblog", "ClawBlog"},
	{"huajian", "花笺 Huajian"},
}

// builtinSpaThemeTitles id → 标题（登记 blog_plugins 时写 name）。
var builtinSpaThemeTitles = func() map[string]string {
	m := make(map[string]string, len(BuiltinSpaThemes))
	for _, t := range BuiltinSpaThemes {
		m[t.ID] = t.Title
	}
	return m
}()

// IsBuiltinSpaTheme 判定 id 是否为源码轨内置主题（应用中心可一键安装）。
func IsBuiltinSpaTheme(id string) bool {
	_, ok := builtinSpaThemeTitles[id]
	return ok
}

// MigrateLegacySpaThemes 主题改版升级迁移（幂等，启动时调用）。
//
// 改版前 18 个 SPA 主题全部内置常驻，站点可以把 blog.theme / site_settings.default_theme
// 指向其中任一；改版后只常驻 aiklog/default，其余主题需登记 blog_plugins(kind=theme, enabled=1)
// 才算「已安装」。为避免升级后线上博客掉皮：启动时扫描两处主题指向，命中源码轨内置主题
// 且尚未登记的，自动登记（保留既有 enabled 状态，不覆盖用户手动卸载以外的语义）。
//
// 注意：这里只补「正在使用」的主题 —— 其余主题等站长在应用中心按需安装，不做全量回填。
func MigrateLegacySpaThemes(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return nil
	}
	need := map[string]bool{}
	// 1) per-site 站点默认主题
	rows, err := db.QueryContext(ctx,
		`SELECT DISTINCT default_theme FROM site_settings WHERE default_theme IS NOT NULL AND default_theme != ''`)
	if err == nil {
		for rows.Next() {
			var id string
			if rows.Scan(&id) == nil && IsBuiltinSpaTheme(id) {
				need[id] = true
			}
		}
		rows.Close()
	} else {
		// site_settings 未建表等场景不阻塞启动（首次安装无迁移需求）
		fmt.Printf("[themes] 迁移跳过 site_settings 扫描: %v\n", err)
	}
	// 2) 全局配置 blog.theme（settings 表 scope='instance'；未显式设置时用默认 aiklog，无需登记）
	var g string
	if err := db.QueryRowContext(ctx,
		`SELECT value FROM settings WHERE key='blog.theme' AND scope='instance'`).Scan(&g); err == nil && IsBuiltinSpaTheme(g) {
		need[g] = true
	}
	if len(need) == 0 {
		return nil
	}
	now := time.Now().UnixMilli()
	for id := range need {
		title := builtinSpaThemeTitles[id]
		if _, err := db.ExecContext(ctx,
			`INSERT INTO blog_plugins (id, name, version, description, author, kind, enabled, created_at, updated_at)
			 VALUES (?, ?, '1.0.0', ?, '爱库录', 'theme', 1, ?, ?)
			 ON CONFLICT(id) DO UPDATE SET enabled=1, updated_at=excluded.updated_at`,
			id, title, "内置源码主题（应用中心按需安装；随主系统构建分发）", now, now); err != nil {
			return fmt.Errorf("migrate legacy spa theme %s: %w", id, err)
		}
		fmt.Printf("[themes] 升级兼容：站点正在使用的主题 %s 已自动登记为已安装\n", id)
	}
	return nil
}
