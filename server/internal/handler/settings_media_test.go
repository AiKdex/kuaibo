// settings_media_test.go 媒体派生资产参数（B16）回归。
//
// 背景：B15 把缩略图的长边上限 / 抽帧上限 / JPEG 质量写死为代码常量（480 / 1000 / 5），
// 站长无法按站点图片尺寸或共享主机的 CPU 余量调整 —— 典型的「魔法参数」。
// B16 提取为可在线修改的配置项，本文件守住四件事：
//
//  1. **可写性**：三项必须在 settableKeys 白名单内，否则面板保存直接 403，站长根本改不了；
//  2. **区间校验**：越界 / 非数字 400，空串放行（= 清除显式配置、回退内置默认）；
//  3. **即时生效**：写完成立刻能从读取层看到新值（Store.Set 更新内存快照，无需重启）。
//     这是 B16 对站长的核心承诺，也最容易被「其实改了但读的是启动快照」悄悄破坏；
//  4. **运维端点**：未装配存储时给可读的 503（而不是空 200 显示全 0），装配后下发面板
//     自解释所需的上下文（为什么没缩略图：能力关了？没装 ffmpeg？参数配错？）。
package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AiKMAP/AiKmap/server/internal/service"
	"github.com/AiKMAP/AiKmap/server/internal/storage"
)

// 1) 三项在白名单内，且区间校验按 service 层同一组常量生效。
func TestMediaThumbSettingsWhitelist(t *testing.T) {
	a := newGateAPI(t)
	cases := []struct {
		key string
		ok  []string
		bad []string
		why string
	}{
		{
			service.KeyThumbMaxEdge,
			[]string{"480", "64", "4096", "320"},
			[]string{"0", "63", "4097", "99999", "abc", "12.5", "480px"},
			"长边上限 64-4096",
		},
		{
			service.KeyThumbSeekMS,
			[]string{"0", "1000", "60000"}, // 0 合法：恒抽第 0 帧
			[]string{"-1", "60001", "1s", "x"},
			"抽帧上限 0-60000",
		},
		{
			service.KeyThumbQuality,
			[]string{"2", "5", "31"},
			[]string{"0", "1", "32", "-3", "high"},
			"JPEG 质量 2-31",
		},
	}
	for _, c := range cases {
		if _, ok := settableKeys[c.key]; !ok {
			t.Fatalf("%s 未进白名单（%s）→ 面板保存会 403，站长改不了", c.key, c.why)
		}
		for _, v := range c.ok {
			if code, body := putSetting(t, a, c.key, v); code != http.StatusOK {
				t.Fatalf("%s=%q 应被接受，实得 %d %s", c.key, v, code, body)
			}
		}
		for _, v := range c.bad {
			if code, _ := putSetting(t, a, c.key, v); code != http.StatusBadRequest {
				t.Fatalf("%s=%q 应被拒（期望 400，%s），实得 %d", c.key, v, c.why, code)
			}
		}
		// 空串 = 清除显式配置（回退默认），必须放行 —— 否则站长无法「恢复默认」
		if code, body := putSetting(t, a, c.key, ""); code != http.StatusOK {
			t.Fatalf("%s 空串应放行（清除配置），实得 %d %s", c.key, code, body)
		}
	}
}

// 2) 改完即时生效：不经重启、不重读 DB，读取层立刻看到新值；清空后回退默认。
//
// 为什么这条必须测：若读取层改成「启动时快照」，写接口仍会返回 200，
// 面板显示「已保存」而实际生成仍用旧参数 —— 判定与实现漂移，正是 B14/B15 反复踩的那类坑。
func TestMediaThumbSettingsTakeEffectImmediately(t *testing.T) {
	a := newGateAPI(t)

	if got := service.ThumbOptionsFrom(a.cfg); got != service.DefaultThumbOptions() {
		t.Fatalf("初始应为内置默认，实得 %+v", got)
	}

	for k, v := range map[string]string{
		service.KeyThumbMaxEdge: "320",
		service.KeyThumbSeekMS:  "0",
		service.KeyThumbQuality: "8",
	} {
		if code, body := putSetting(t, a, k, v); code != http.StatusOK {
			t.Fatalf("写 %s=%s 失败：%d %s", k, v, code, body)
		}
	}

	got := service.ThumbOptionsFrom(a.cfg)
	if got.MaxEdge != 320 || got.SeekMS != 0 || got.Quality != 8 {
		t.Fatalf("改完应立即生效（无需重启），实得 %+v", got)
	}

	// 清空 → 回退默认（三键同时验，证明空串语义在读取层一致）
	for _, k := range []string{service.KeyThumbMaxEdge, service.KeyThumbSeekMS, service.KeyThumbQuality} {
		if code, body := putSetting(t, a, k, ""); code != http.StatusOK {
			t.Fatalf("清空 %s 失败：%d %s", k, code, body)
		}
	}
	if got := service.ThumbOptionsFrom(a.cfg); got != service.DefaultThumbOptions() {
		t.Fatalf("清空后应回退内置默认，实得 %+v", got)
	}
}

// withFiles 给测试 API 装上本地文件存储（派生资产统计/清理需要它）。
func withFiles(t *testing.T, a *API) {
	t.Helper()
	loc, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatalf("storage: %v", err)
	}
	a.files = service.NewFileStore(a.db, loc, a.b, a.aud)
}

// 3) 统计端点：未装配存储给可读 503；装配后 200 且下发面板自解释所需的四个字段。
func TestDerivedStatsEndpoint(t *testing.T) {
	a := newGateAPI(t)
	get := func() *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		a.derivedStats(rec, httptest.NewRequest(http.MethodGet, "/api/v1/admin/derived/stats", nil))
		return rec
	}
	// 未装配 FileStore → 503 + 可读原因（不能返回空 200 让面板显示「占用 0」而误导站长）
	rec := get()
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "FILE_STORE_UNAVAILABLE") {
		t.Fatalf("未装配存储应 503 FILE_STORE_UNAVAILABLE，实得 %d %s", rec.Code, rec.Body.String())
	}

	withFiles(t, a)
	rec = get()
	if rec.Code != http.StatusOK {
		t.Fatalf("应 200，实得 %d %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, k := range []string{"stats", "ffmpeg", "options", "capability_enabled"} {
		if !strings.Contains(body, `"`+k+`"`) {
			t.Fatalf("响应缺少 %q（面板需要它自解释状态）：%s", k, body)
		}
	}
	// 空 store：0 对象、可列举（本地后端）—— 面板据此显示「尚无派生对象」
	for _, want := range []string{`"objects":0`, `"listable":true`} {
		if !strings.Contains(body, want) {
			t.Fatalf("空存储应含 %s，实得 %s", want, body)
		}
	}
}

// 4) 清理端点：空 body 视为全清；清理后统计归零；响应提示「下次访问会按当前参数重生成」。
func TestDerivedPurgeEndpoint(t *testing.T) {
	a := newGateAPI(t)
	withFiles(t, a)
	ctx := context.Background()
	for _, fid := range []string{"F1", "F2"} {
		key := service.DerivedKey(service.SystemHomeSpaceID, fid, "thumb.jpg")
		if err := a.files.PutDerived(ctx, key, strings.NewReader("xx"), 2); err != nil {
			t.Fatalf("PutDerived: %v", err)
		}
	}
	if st := a.files.DerivedStats(ctx); st.Objects != 2 {
		t.Fatalf("前置条件：应有 2 个派生对象，实得 %+v", st)
	}

	rec := httptest.NewRecorder()
	a.derivedPurge(rec, httptest.NewRequest(http.MethodPost, "/api/v1/admin/derived/purge", strings.NewReader("{}")))
	if rec.Code != http.StatusOK {
		t.Fatalf("purge 应 200，实得 %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"removed":2`) {
		t.Fatalf("应删 2 个：%s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "重新生成") {
		t.Fatalf("响应应提示懒生成语义：%s", rec.Body.String())
	}
	if st := a.files.DerivedStats(ctx); st.Objects != 0 {
		t.Fatalf("清理后应为空，实得 %+v", st)
	}

	// 幂等：再清一次仍 200 且 removed=0（清理路径随时可能被重复触发）
	rec = httptest.NewRecorder()
	a.derivedPurge(rec, httptest.NewRequest(http.MethodPost, "/api/v1/admin/derived/purge", strings.NewReader("{}")))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"removed":0`) {
		t.Fatalf("重复清理应 200 removed=0，实得 %d %s", rec.Code, rec.Body.String())
	}
}

// 5) 清理孤儿端点：库里不存在 fid 的派生对象 → 全部清掉；幂等。
func TestDerivedSweepEndpoint(t *testing.T) {
	a := newGateAPI(t)
	withFiles(t, a)
	ctx := context.Background()
	for _, fid := range []string{"ORPHAN1", "ORPHAN2"} {
		key := service.DerivedKey(service.SystemHomeSpaceID, fid, "thumb.jpg")
		if err := a.files.PutDerived(ctx, key, strings.NewReader("xx"), 2); err != nil {
			t.Fatalf("PutDerived: %v", err)
		}
	}
	if st := a.files.DerivedStats(ctx); st.Objects != 2 {
		t.Fatalf("前置条件：应有 2 个派生对象，实得 %+v", st)
	}

	rec := httptest.NewRecorder()
	a.derivedSweep(rec, httptest.NewRequest(http.MethodPost, "/api/v1/admin/derived/sweep", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("sweep 应 200，实得 %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"removed":2`) {
		t.Fatalf("应清 2 个孤儿：%s", rec.Body.String())
	}
	if st := a.files.DerivedStats(ctx); st.Objects != 0 {
		t.Fatalf("清理后应为空，实得 %+v", st)
	}

	// 幂等：再 sweep 一次仍 200 且 removed=0
	rec = httptest.NewRecorder()
	a.derivedSweep(rec, httptest.NewRequest(http.MethodPost, "/api/v1/admin/derived/sweep", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"removed":0`) {
		t.Fatalf("重复清理应 200 removed=0，实得 %d %s", rec.Code, rec.Body.String())
	}
}
