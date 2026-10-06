// capabilities.go 能力模块装配（CoreModules 注入 + 能力门控）。
//
// 背景：上游《AiKlog需求清单 v3.0 回执》§二 R2 已对齐统一口径——
//
//	「安装 capacity 应用写一条 blog_plugins 记录（kind=capacity, capabilities=[...], enabled=1）；
//	  停用/卸载置 enabled=0（或删除记录）；main 装配阶段查已启用 capacity 记录决定是否构造
//	  对应 Store 注入 CoreModules；模块为 nil → 路由不注册（404 = 能力未启用）。」
//
// 本文件即该口径的装配实现。门控判定的优先级（高 → 低）：
//  1. 站长显式配置 settings `capability.<name>`（"true"/"false"）——后台可自助开关，便于先验证；
//  2. 应用中心记录 blog_plugins（kind='capacity' 且 id/name 命中能力名）的 enabled；
//  3. 内置默认：true（本 fork 精简发行仍内置这些能力，装/卸载走应用中心）。
//     如需改为"必须装应用才可用"（上游 strict 默认），只需把 buildCoreModules 的兜底改为 false。
//
// 能力名（R2 口语里的 capabilities）与本壳模块的映射：
//
//	collector → Sources + Collector（采集引擎：一源多城、kind 分流、去重入库）
//	webdav    → WebDAV（外部 WebDAV 挂载导入 + 对外 DAV 服务端）
//	digest    → Digest（每日知识日报）
//	inbox     → Inbox（收件箱：采集聚合/未读/归档）
//	review    → Review（SM-2 复习队列）
//	org       → Org（企业知识库组织模块 M0/M1/M2）
//
// 例外（B15）：service.CapThumb「媒体缩略图」**不是装配点** —— 它不控制任何 Store 的构造，
// 只在请求路径上就地判定（关闭则不调 ffmpeg、缩略图端点 404），因此不出现在 buildCoreModules 里。
// 生效时机由 service.CapabilityApply 声明（restart / live），并经 /admin/capabilities 逐项下发，
// 避免设置页把运行期开关也标成「需重启」。
package main

import (
	"database/sql"
	"log"

	"github.com/AiKMAP/AiKmap/server/internal/config"
	"github.com/AiKMAP/AiKmap/server/internal/engine/bus"
	"github.com/AiKMAP/AiKmap/server/internal/handler"
	"github.com/AiKMAP/AiKmap/server/internal/service"
)

// capabilityEnabled 能力门控判定（装配阶段调用）。判定逻辑与「设置页运维面板」
// 共用 service.CapabilityEnabled（单一事实来源），避免两处口径漂移；本函数只补日志。
func capabilityEnabled(db *sql.DB, cfg *config.Store, name string) bool {
	on, src := service.CapabilityEnabled(db, cfg, name)
	log.Printf("capability %s: 来源=%s → %v", name, src, on)
	return on
}

// buildCoreModules 按门控结果装配 CoreModules（未启用的字段留 nil → 对应路由 404）。
func buildCoreModules(db *sql.DB, cfg *config.Store, b *bus.Bus, aud *service.AuditStore,
	files *service.FileStore, tags *service.TagStore, notify *service.NotifyStore) handler.CoreModules {
	var mods handler.CoreModules

	// 采集引擎（Sources + Collector 成对注入：Collector 依赖 SourceStore/FileStore/TagStore）
	if capabilityEnabled(db, cfg, "collector") {
		mods.Sources = service.NewSourceStore(db)
		mods.Collector = service.NewCollector(mods.Sources, files, tags)
	}
	if capabilityEnabled(db, cfg, "digest") {
		mods.Digest = service.NewDigestStore(db, cfg)
	}
	if capabilityEnabled(db, cfg, "inbox") {
		mods.Inbox = service.NewInboxStore(db)
	}
	if capabilityEnabled(db, cfg, "review") {
		mods.Review = service.NewReviewStore(db)
	}

	// WebDAV：外部挂载导入 + 对外 DAV 服务端（服务端另受 dav.enabled 开关控制，默认开）。
	// 接线前的核心层分歧已补齐（2026-09-20）：File.ViewCount、FileStore.ReplaceContentBinary、
	// NotifyStore.AddUser、handler spaceRole/quotaUse/quotaUploadMB/curHomeSpaceID，
	// 另补建 space_members 表（fork 侧缺失，Org 部门空间查询依赖）。
	if capabilityEnabled(db, cfg, "webdav") {
		mods.WebDAV = service.NewWebDAVStore(db)
	}
	// 组织架构（Org）：组织树/任职/部门空间/移交流（依赖 bus 事件与审计）
	if capabilityEnabled(db, cfg, "org") {
		mods.Org = service.NewOrgStore(db, b, aud, cfg)
	}
	// 家族传承记录（Family）：[family] REQ-010/011 自上游移植；一生时间轴/家族树/纪念日提醒（notify 通知中心）。
	// 细粒度开关 settings family.enabled（默认关）+ 路由层 familyPaidGate（scope=feature:family）。
	if capabilityEnabled(db, cfg, "family") {
		mods.Family = service.NewFamilyStore(db, cfg, notify)
	}
	// 商城（B39：WooCommerce 式）。作为标准产品能力默认开启；关闭后路由不注册（404=能力未启用）。
	if capabilityEnabled(db, cfg, "store") {
		mods.Store = service.NewStoreStore(db, cfg.GetString)
	}
	return mods
}
