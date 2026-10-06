// capabilities_test.go 能力门控判定（B14）回归。
//
// 重点守住历史上踩过的坑（🔴 门控铁律）：**命中应用中心记录时不得再退回「内置默认 true」**——
// 否则表现为「在应用中心停用了能力，却熄不灭」。四条优先级必须逐条可判定、且能报出判定来源。
package service

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/AiKMAP/AiKmap/server/internal/repo"
)

// stubCfg 最小配置桩（只实现 GetString，避免 service 依赖 config 包）。
type stubCfg map[string]string

func (m stubCfg) GetString(k string) string { return m[k] }

func newCapDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := repo.Open(filepath.Join(t.TempDir(), "cap.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := repo.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// addPlugin 插一条应用中心记录（capability_mode='capacity'）。
func addPlugin(t *testing.T, db *sql.DB, id, name, capabilities string, enabled int) {
	t.Helper()
	if _, err := db.Exec(
		`INSERT INTO blog_plugins(id, name, capabilities, capability_mode, enabled, created_at, updated_at)
		 VALUES(?, ?, ?, 'capacity', ?, 0, 0)`, id, name, capabilities, enabled); err != nil {
		t.Fatalf("insert plugin %s: %v", id, err)
	}
}

// 1) 无配置、无记录 → 内置默认开启。
func TestCapabilityDefaultOn(t *testing.T) {
	db := newCapDB(t)
	// 用核心能力（采集）验证「空库默认开启」；org/family 是应用中心可安装能力，
	// capabilityDefault 对它们返回 false（未安装=不装配），另由 TestCapabilityDefaultOffInstalled 覆盖。
	on, src := CapabilityEnabled(db, stubCfg{}, CapCollector)
	if !on || src != CapSourceDefault {
		t.Fatalf("空库应为内置默认开启，得 on=%v src=%s", on, src)
	}
}

// 1b) org/family 是应用中心可安装的垂直能力：空库默认「关」（未安装=不装配、不亮侧栏）。
// 这条与 TestCapabilityDefaultOn 互为对照，防止有人把默认语义改反。
func TestCapabilityDefaultOffInstalled(t *testing.T) {
	db := newCapDB(t)
	for _, name := range []string{CapOrg, CapFamily, CapCSInbox, CapCSContacts, CapCSChannels, CapCSAIDraft} {
		on, src := CapabilityEnabled(db, stubCfg{}, name)
		if on || src != CapSourceDefault {
			t.Fatalf("%s 空库应为默认关闭（可安装能力），得 on=%v src=%s", name, on, src)
		}
	}
}

// 2) 站长配置优先于应用中心记录（配置 true 压过停用记录）。
func TestCapabilityConfigWinsOverPlugin(t *testing.T) {
	db := newCapDB(t)
	addPlugin(t, db, "org", "组织架构", "[]", 0)
	on, src := CapabilityEnabled(db, stubCfg{"capability.org": "true"}, CapOrg)
	if !on || src != CapSourceConfig {
		t.Fatalf("站长配置应优先，得 on=%v src=%s", on, src)
	}
}

// 3) 🔴 铁律：停用的 capacity 记录必须「熄得灭」，不得退回内置默认 true。
func TestCapabilityDisabledPluginDoesNotFallBackToDefault(t *testing.T) {
	db := newCapDB(t)
	addPlugin(t, db, "org", "组织架构", "[]", 0)
	on, src := CapabilityEnabled(db, stubCfg{}, CapOrg)
	if on {
		t.Fatal("停用的 capacity 记录不得退回内置默认 true（否则停用无效）")
	}
	if src != CapSourcePlugin {
		t.Fatalf("来源应为 plugin，得 %s", src)
	}
}

// 3b) 同一铁律在 capabilities 声明路径上同样成立。
func TestCapabilityDisabledDeclarationDoesNotFallBack(t *testing.T) {
	db := newCapDB(t)
	addPlugin(t, db, "com.x.pack", "第三方能力包", `["org"]`, 0)
	on, src := CapabilityEnabled(db, stubCfg{}, CapOrg)
	if on || src != CapSourceDeclaration {
		t.Fatalf("声明命中且停用应为 false/plugin-capability，得 on=%v src=%s", on, src)
	}
}

// 4) 启用的 capacity 记录 → 开，来源 plugin。
func TestCapabilityEnabledPlugin(t *testing.T) {
	db := newCapDB(t)
	addPlugin(t, db, "org", "组织架构", "[]", 1)
	on, src := CapabilityEnabled(db, stubCfg{}, CapOrg)
	if !on || src != CapSourcePlugin {
		t.Fatalf("启用的记录应为 true/plugin，得 on=%v src=%s", on, src)
	}
}

// 5) 包名≠能力名：capabilities 声明命中。
func TestCapabilityDeclarationHit(t *testing.T) {
	db := newCapDB(t)
	addPlugin(t, db, "com.aiklog.capacity-pack", "能力包", `["digest","org"]`, 1)
	on, src := CapabilityEnabled(db, stubCfg{}, CapOrg)
	if !on || src != CapSourceDeclaration {
		t.Fatalf("声明命中应为 true/plugin-capability，得 on=%v src=%s", on, src)
	}
}

// 6) 多条记录取「任一启用即启用」（ORDER BY enabled DESC）。
func TestCapabilityMultiRecordAnyEnabled(t *testing.T) {
	db := newCapDB(t)
	addPlugin(t, db, "pack-a", "能力包 A", `["org"]`, 0)
	addPlugin(t, db, "pack-b", "能力包 B", `["org"]`, 1)
	on, src := CapabilityEnabled(db, stubCfg{}, CapOrg)
	if !on || src != CapSourceDeclaration {
		t.Fatalf("任一启用即启用，得 on=%v src=%s", on, src)
	}
}

// 7) 空串/空白视为未配置（「跟随默认」语义），继续走应用中心 → 内置默认。
func TestCapabilityBlankConfigFallsThrough(t *testing.T) {
	db := newCapDB(t)
	addPlugin(t, db, "org", "组织架构", "[]", 0)
	for _, v := range []string{"", "   "} {
		on, src := CapabilityEnabled(db, stubCfg{"capability.org": v}, CapOrg)
		if on || src != CapSourcePlugin {
			t.Fatalf("配置=%q 应视作未配置并落应用中心，得 on=%v src=%s", v, on, src)
		}
	}
}

// 8) db=nil（纯配置/纯默认模式）不 panic：配置优先，否则内置默认。
func TestCapabilityNilDBSafe(t *testing.T) {
	if on, src := CapabilityEnabled(nil, nil, CapOrg); !on || src != CapSourceDefault {
		t.Fatalf("nil db + nil cfg 应为内置默认，得 on=%v src=%s", on, src)
	}
	if on, src := CapabilityEnabled(nil, stubCfg{"capability.org": "false"}, CapOrg); on || src != CapSourceConfig {
		t.Fatalf("nil db 下配置仍应生效，得 on=%v src=%s", on, src)
	}
	// "1" 也认（与 cmd 侧历史口径一致）
	if on, _ := CapabilityEnabled(nil, stubCfg{"capability.org": "1"}, CapOrg); !on {
		t.Fatal(`"1" 应被认作开启`)
	}
}

// 9) 能力名清单与装配一一对应（少一个就会出现「面板上没有、但实际被装配」的模块）。
func TestCapabilityNamesCoverage(t *testing.T) {
	want := []string{"collector", "webdav", "digest", "inbox", "review", "org", "family", "thumb", "transcode",
		"cs.inbox", "cs.contacts", "cs.channels", "cs.ai_draft", "store"}
	if len(CapabilityNames) != len(want) {
		t.Fatalf("能力数=%d，期望 %d", len(CapabilityNames), len(want))
	}
	got := map[string]bool{}
	for _, n := range CapabilityNames {
		got[n] = true
	}
	for _, n := range want {
		if !got[n] {
			t.Fatalf("缺少能力名 %s", n)
		}
	}
}

// 10) 生效时机声明：装配类须重启，运行期项须即时（防止面板把两者混为一谈）。
func TestCapabilityApplySemantics(t *testing.T) {
	for _, n := range CapabilityNames {
		want := CapApplyRestart
		if n == CapThumb || n == CapTranscode {
			want = CapApplyLive
		}
		if got := CapabilityApply(n); got != want {
			t.Fatalf("%s 生效时机应为 %s，得 %s", n, want, got)
		}
	}
	if got := CapabilityApply("unknown-cap"); got != CapApplyRestart {
		t.Fatalf("未登记能力应保守回落到 %s，得 %s", CapApplyRestart, got)
	}
}
