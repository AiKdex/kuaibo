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
	// Desc 应用中心展示用的一句话介绍（与各主题 manifest.js 的 desc 同源维护）。
	Desc string
}

// BuiltinSpaThemes 常驻内置（aiklog/default）之外的全部源码主题。
// 顺序即应用中心「主题」类别展示顺序。
var BuiltinSpaThemes = []BuiltinSpaTheme{
	{"minimal", "极简阅读", "单栏沉浸列表，衬线标题，少装饰"},
	{"docs", "技术文档", "侧栏目录 + 正文栏，适合文档站与长文"},
	{"paper", "暖纸情报", "暖纸底 + 青绿顶栏 + 珊瑚强调（对齐求智情报站视觉）"},
	{"elevated", "高端暗色", "暗色高端博客主题：玉色点缀、衬线正文、AI 侧栏"},
	{"parchment", "暖纸杂志", "暖纸杂志风格：衬线排版、琥珀色调、双列网格"},
	{"emforum", "论坛社区", "深蓝鎏金论坛风：版块页签切换 + 帖子流列表 + 归档/搜索内页"},
	{"brutal", "新粗野", "新粗野风：硬边框硬阴影 + 高饱和撞色 + 几何图案卡片，含分类/作者/归档/搜索内页"},
	{"aurora", "极光 Aurora", "Bento Grid 卡片式 · Apple 风格：模块化栅格 + 玻璃页头，含分类/作者/归档/搜索内页"},
	{"verdant", "青野 Verdant", "明亮清爽：暖白底 + 森林绿 + 有机圆角，适合生活/自然/产品类内容，含分类/作者/归档/搜索内页"},
	{"zircon", "青璃 Zircon", "青绿胶囊 · 社区资讯式双栏布局：渐变头图 + 卡片流 + 信息侧栏，含分类/作者/归档/搜索内页"},
	{"butterfly", "蝶语 Butterfly", "卡片式双栏：半透明毛玻璃顶栏 + 文章卡片 + 侧栏卡片组 + 右下悬浮按钮 + 夜间模式，含分类/作者/归档/搜索内页"},
	{"chenxi", "晨曦笔记 Chenxi", "温暖渐变博客主题 — 玫粉主调、彩虹渐变 Hero、左图右文卡片、毛玻璃顶栏"},
	{"jaded", "翡翠 Jaded", "暗色翡翠绿主题 — 毛玻璃顶栏、渐变文字、左图右文卡片、文章衬线体"},
	{"zhicang", "知藏 Zhicang", "资源课程分享主题 — 靛蓝主调、学习路径追踪、课程进度条、资源卡片网格"},
	{"aiknav", "好站导航 AikNav", "导航站风格 — 蓝色主调卡片网格，左侧分类树与热门榜单"},
	{"clawblog", "ClawBlog", "现代渐变风格博客主题：玻璃拟态导航、混合布局卡片、极致阅读体验"},
	{"huajian", "花笺 Huajian", "暖纸杂志风博客主题：衬线标题、琥珀强调、杂志式双栏版式"},
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
