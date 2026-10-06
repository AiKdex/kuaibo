// media_skill_test.go B8 媒体元信息 + B9 技能包 测试。
package service

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
	"time"
)

// pngBytes 生成一张 w×h 的 PNG（用于探测断言）。
func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

// 图片 header 探测：只解 header 也能拿到真实宽高。
func TestProbeImage(t *testing.T) {
	data := pngBytes(t, 120, 80)
	w, h, format, ok := ProbeImage(bytes.NewReader(data))
	if !ok {
		t.Fatal("ProbeImage should succeed on a valid PNG")
	}
	if w != 120 || h != 80 {
		t.Fatalf("size = %dx%d, want 120x80", w, h)
	}
	if format != "png" {
		t.Fatalf("format = %q, want png", format)
	}
	// 非图片流必须明确失败（不能把任意二进制当图片）
	if _, _, _, ok := ProbeImage(strings.NewReader("这只是一段普通文本，不是图片")); ok {
		t.Fatal("ProbeImage should fail on non-image input")
	}
	// 截断的 PNG 也应失败而非 panic
	if len(data) > 8 {
		if _, _, _, ok := ProbeImage(bytes.NewReader(data[:8])); ok {
			t.Fatal("truncated png should not report ok")
		}
	}
}

// 元信息写读一致 + 幂等更新。
func TestMediaUpsertGet(t *testing.T) {
	ctx := context.Background()
	db := newB6DB(t)
	m := NewMediaStore(db)

	if _, ok := m.Get(ctx, "f-none"); ok {
		t.Fatal("Get on empty table should report not found")
	}
	info := &MediaInfo{FileID: "f-1", Width: 640, Height: 480, Codec: "png", ProbeStatus: MediaOK}
	if err := m.Upsert(ctx, info); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if info.UpdatedAt <= 0 {
		t.Fatal("Upsert should fill UpdatedAt")
	}
	got, ok := m.Get(ctx, "f-1")
	if !ok || got.Width != 640 || got.Height != 480 || got.ProbeStatus != MediaOK {
		t.Fatalf("Get = %+v ok=%v", got, ok)
	}
	// 再写一次不产生重复行
	if err := m.Upsert(ctx, &MediaInfo{FileID: "f-1", ProbeStatus: MediaUnsupported}); err != nil {
		t.Fatalf("Upsert#2: %v", err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM file_media WHERE file_id='f-1'`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Fatalf("rows = %d, want 1（Upsert 必须幂等）", n)
	}
	if got, _ := m.Get(ctx, "f-1"); got.ProbeStatus != MediaUnsupported {
		t.Fatalf("status after update = %q, want unsupported", got.ProbeStatus)
	}
	// PurgeFile
	if err := m.PurgeFile(ctx, "f-1"); err != nil {
		t.Fatalf("PurgeFile: %v", err)
	}
	if _, ok := m.Get(ctx, "f-1"); ok {
		t.Fatal("PurgeFile should remove the row")
	}
}

// 上架后进入目录；未上架（draft）不出现；kind 过滤生效。
func TestSkillPublishAndList(t *testing.T) {
	ctx := context.Background()
	db := newB6DB(t)
	seedUser(t, db, "u-alice", "alice", "爱丽丝")
	st := NewSkillStore(db)

	pub := &SkillPackage{Name: "collector-pack", Kind: "tool", Title: "采集包",
		Status: "published", AuthorID: "u-alice", PriceCents: 0}
	if err := st.UpsertPackage(ctx, pub); err != nil {
		t.Fatalf("UpsertPackage: %v", err)
	}
	if pub.ID == "" || pub.CreatedAt == 0 {
		t.Fatal("UpsertPackage should allocate id/created_at")
	}
	draft := &SkillPackage{Name: "draft-pack", Kind: "skill", Status: "draft", AuthorID: "u-alice"}
	if err := st.UpsertPackage(ctx, draft); err != nil {
		t.Fatalf("UpsertPackage(draft): %v", err)
	}
	wf := &SkillPackage{Name: "wf-pack", Kind: "workflow", Status: "published", AuthorID: "u-alice"}
	if err := st.UpsertPackage(ctx, wf); err != nil {
		t.Fatalf("UpsertPackage(wf): %v", err)
	}

	all, err := st.ListPackages(ctx, "", 100)
	if err != nil {
		t.Fatalf("ListPackages: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("published count = %d, want 2（draft 不得出现）", len(all))
	}
	onlyTool, err := st.ListPackages(ctx, "tool", 100)
	if err != nil || len(onlyTool) != 1 || onlyTool[0].Name != "collector-pack" {
		t.Fatalf("kind filter = %+v, err=%v", onlyTool, err)
	}

	// 更新：状态改 archived 后不再出现在目录
	pub.Status = "archived"
	if err := st.UpsertPackage(ctx, pub); err != nil {
		t.Fatalf("update: %v", err)
	}
	if all, _ := st.ListPackages(ctx, "", 100); len(all) != 1 {
		t.Fatalf("after archive count = %d, want 1", len(all))
	}
	// 更新不存在的 id 明确报错
	if err := st.UpsertPackage(ctx, &SkillPackage{ID: "nope", Name: "x", Kind: "tool"}); !errors.Is(err, ErrSkillNotFound) {
		t.Fatalf("update missing err = %v, want ErrSkillNotFound", err)
	}
}

// 自助领取：仅免费且已上架；付费包不给、draft 不给。
func TestSkillClaim(t *testing.T) {
	ctx := context.Background()
	db := newB6DB(t)
	seedUser(t, db, "u-alice", "alice", "爱丽丝")
	st := NewSkillStore(db)

	free := &SkillPackage{Name: "free-pack", Kind: "tool", Status: "published", AuthorID: "u-alice", PriceCents: 0}
	if err := st.UpsertPackage(ctx, free); err != nil {
		t.Fatalf("seed free: %v", err)
	}
	paid := &SkillPackage{Name: "paid-pack", Kind: "skill", Status: "published", AuthorID: "u-alice", PriceCents: 9900}
	if err := st.UpsertPackage(ctx, paid); err != nil {
		t.Fatalf("seed paid: %v", err)
	}
	hidden := &SkillPackage{Name: "hidden", Kind: "skill", Status: "draft", AuthorID: "u-alice"}
	if err := st.UpsertPackage(ctx, hidden); err != nil {
		t.Fatalf("seed draft: %v", err)
	}

	if err := st.Claim(ctx, free.ID, "user", "u-alice"); err != nil {
		t.Fatalf("claim free: %v", err)
	}
	if !st.HasGrant(ctx, free.ID, "user", "u-alice") {
		t.Fatal("HasGrant should be true after claim")
	}
	// 重复领取幂等，不报错、不新增行
	if err := st.Claim(ctx, free.ID, "user", "u-alice"); err != nil {
		t.Fatalf("re-claim: %v", err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM skill_grants WHERE package_id=?`, free.ID).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Fatalf("grant rows = %d, want 1（领取必须幂等）", n)
	}

	if err := st.Claim(ctx, paid.ID, "user", "u-alice"); !errors.Is(err, ErrSkillNotFree) {
		t.Fatalf("claim paid err = %v, want ErrSkillNotFree", err)
	}
	if err := st.Claim(ctx, hidden.ID, "user", "u-alice"); !errors.Is(err, ErrSkillNotPub) {
		t.Fatalf("claim draft err = %v, want ErrSkillNotPub", err)
	}
	if err := st.Claim(ctx, "no-such", "user", "u-alice"); !errors.Is(err, ErrSkillNotFound) {
		t.Fatalf("claim missing err = %v, want ErrSkillNotFound", err)
	}
}

// 授权生命周期：发放幂等 / 列表只看生效 / 吊销后失效 / 到期自动不生效。
func TestSkillGrantLifecycle(t *testing.T) {
	ctx := context.Background()
	db := newB6DB(t)
	seedUser(t, db, "u-alice", "alice", "爱丽丝")
	seedUser(t, db, "u-bob", "bob", "鲍勃")
	st := NewSkillStore(db)

	p := &SkillPackage{Name: "pack", Kind: "extension", Status: "published", AuthorID: "u-alice", PriceCents: 100}
	if err := st.UpsertPackage(ctx, p); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// 手工发放（付费包也走这条，模拟购买后发放）
	if err := st.Grant(ctx, p.ID, "user", "u-alice", "purchase", 0); err != nil {
		t.Fatalf("Grant: %v", err)
	}
	if err := st.Grant(ctx, p.ID, "user", "u-alice", "purchase", 0); err != nil {
		t.Fatalf("re-Grant should be idempotent: %v", err)
	}
	grants, err := st.ListGrants(ctx, "user", "u-alice")
	if err != nil || len(grants) != 1 {
		t.Fatalf("ListGrants = %d, err=%v, want 1", len(grants), err)
	}
	if grants[0].Source != "purchase" {
		t.Fatalf("source = %q, want purchase", grants[0].Source)
	}
	// 别人没有该授权
	if st.HasGrant(ctx, p.ID, "user", "u-bob") {
		t.Fatal("bob should not hold the grant")
	}
	// 吊销
	if err := st.Revoke(ctx, p.ID, "user", "u-alice"); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if st.HasGrant(ctx, p.ID, "user", "u-alice") {
		t.Fatal("HasGrant should be false after revoke")
	}
	if items, _ := st.ListGrants(ctx, "user", "u-alice"); len(items) != 0 {
		t.Fatalf("ListGrants after revoke = %d, want 0", len(items))
	}
	if err := st.Revoke(ctx, p.ID, "user", "u-nobody"); !errors.Is(err, ErrSkillNotFound) {
		t.Fatalf("revoke missing err = %v, want ErrSkillNotFound", err)
	}

	// 到期授权视为不生效
	past := time.Now().UnixMilli() - 60_000
	if err := st.Grant(ctx, p.ID, "space", "sp-1", "trial", past); err != nil {
		t.Fatalf("Grant(expired): %v", err)
	}
	if st.HasGrant(ctx, p.ID, "space", "sp-1") {
		t.Fatal("expired grant should not count as active")
	}
	if items, _ := st.ListGrants(ctx, "space", "sp-1"); len(items) != 0 {
		t.Fatalf("expired grant in list = %d, want 0", len(items))
	}
}

// 白名单校验：非法 kind / grantee_type 必须被拒（而不是静默写入）。
func TestSkillValidation(t *testing.T) {
	ctx := context.Background()
	db := newB6DB(t)
	seedUser(t, db, "u-alice", "alice", "爱丽丝")
	st := NewSkillStore(db)

	if err := st.UpsertPackage(ctx, &SkillPackage{Name: "x", Kind: "bogus", Status: "published"}); !errors.Is(err, ErrSkillBadKind) {
		t.Fatalf("bad kind err = %v, want ErrSkillBadKind", err)
	}
	p := &SkillPackage{Name: "ok", Kind: "tool", Status: "published", AuthorID: "u-alice"}
	if err := st.UpsertPackage(ctx, p); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := st.Grant(ctx, p.ID, "team", "t-1", "manual", 0); !errors.Is(err, ErrSkillBadGrantee) {
		t.Fatalf("bad grantee err = %v, want ErrSkillBadGrantee", err)
	}
	if _, err := st.ListGrants(ctx, "team", "t-1"); !errors.Is(err, ErrSkillBadGrantee) {
		t.Fatalf("ListGrants bad grantee err = %v", err)
	}
	if _, ok := st.GetPackage(ctx, "no-such"); ok {
		t.Fatal("GetPackage on missing id should be false")
	}
}
