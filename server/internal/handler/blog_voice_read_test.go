// blog_voice_read_test.go 博客文章语音朗读（family.8.3）后端回归。
//
// 覆盖点：
//  1. markdownToPlainText：frontmatter/代码块/HTML/图片清洗，链接保留文本，标记折叠；
//  2. publicBlogTTS 请求层：slug 缺失 400、未知 slug 404、解析到已发布文章并抵达 ai.tts（未配置 provider 时 502 TTS_ERR）。
//
// 真实合成（出网到 MiMo）不在单测范围，留待部署后 curl 验证（见部署清单）。
package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/AiKMAP/AiKmap/server/internal/ai"
	"github.com/AiKMAP/AiKmap/server/internal/config"
	"github.com/AiKMAP/AiKmap/server/internal/engine/bus"
	"github.com/AiKMAP/AiKmap/server/internal/repo"
	"github.com/AiKMAP/AiKmap/server/internal/service"
	"github.com/AiKMAP/AiKmap/server/internal/storage"
)

// newVoiceReadAPI 建「db+cfg+files+ai 都在位」的最小环境，并初始化博客空间。
func newVoiceReadAPI(t *testing.T) *API {
	t.Helper()
	dir := t.TempDir()
	db, err := repo.Open(strings.TrimSuffix(dir, "/") + "/vr.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := repo.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	b := bus.New()
	cfg := config.New(db, b)
	aud := service.New(db)
	ctx := context.Background()
	if err := service.EnsureSystem(ctx, db, aud); err != nil {
		t.Fatalf("ensure system: %v", err)
	}
	if err := service.EnsureBlogSpace(ctx, db); err != nil {
		t.Fatalf("ensure blog space: %v", err)
	}
	st, err := storage.NewLocal(dir + "/files")
	if err != nil {
		t.Fatalf("storage: %v", err)
	}
	return &API{
		db:    db,
		b:     b,
		cfg:   cfg,
		aud:   aud,
		files: service.NewFileStore(db, st, b, aud),
		ai:    ai.NewGateway(cfg, b),
	}
}

// publishPost 在博客目录建一篇已发布文章（默认草稿 → 置 published + node_type=post + 显式 slug）。
func (a *API) publishPost(t *testing.T, ctx context.Context, slug, content string) {
	t.Helper()
	doc, err := a.files.CreateDoc(ctx, service.SystemOwnerID, service.SystemHomeSpaceID, service.BlogDirID, slug+".md", content, service.DefaultSiteID)
	if err != nil {
		t.Fatalf("create doc: %v", err)
	}
	if _, err := a.db.ExecContext(ctx,
		`UPDATE files SET slug=?, content_state=json_set(json_set(content_state,'$.status','published'),'$.node_type','post') WHERE id=?`,
		slug, doc.ID); err != nil {
		t.Fatalf("publish: %v", err)
	}
}

func TestMarkdownToPlainText(t *testing.T) {
	cases := []struct {
		name        string
		in          string
		contains    []string
		notContains []string
	}{
		{"frontmatter stripped", "---\ntitle: x\n---\n# 标题\n正文", []string{"标题", "正文"}, []string{"---", "title:"}},
		{"code fence removed", "前文\n```\ncode();\n```\n后文", []string{"前文", "后文"}, []string{"```", "code"}},
		{"html stripped", "<p>你好</p><div>世界</div>", []string{"你好", "世界"}, []string{"<", ">"}},
		{"image removed", "看图 ![alt](http://x/y.png) 没了", []string{"看图", "没了"}, []string{"![", "http", "alt"}},
		{"link keeps text", "见[官网](https://e.com)了解", []string{"官网"}, []string{"[", "]", "https", "("}},
		{"bold/italic flattened", "这是**重要**和*强调*内容", []string{"重要", "强调"}, []string{"*", "_"}},
		{"heading/list/quote stripped", "# 大标题\n- 项目一\n- 项目二\n> 引用句", []string{"大标题", "项目一", "项目二", "引用句"}, []string{"#", "-", ">"}},
		{"whitespace collapsed", "多   余\n\n\n\n空白", []string{"多", "余", "空白"}, []string{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := markdownToPlainText(c.in)
			for _, w := range c.contains {
				if !strings.Contains(got, w) {
					t.Errorf("got %q 缺期望词 %q", got, w)
				}
			}
			for _, w := range c.notContains {
				if strings.Contains(got, w) {
					t.Errorf("got %q 仍含不应出现的 %q", got, w)
				}
			}
		})
	}
}

func TestPublicBlogTTSBadRequests(t *testing.T) {
	a := newVoiceReadAPI(t)
	ctx := context.Background()
	// 缺失 slug → 400
	rec := httptest.NewRecorder()
	a.publicBlogTTS(rec, httptest.NewRequest(http.MethodGet, "/api/v1/public/blog/tts", nil).WithContext(ctx))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("missing slug: got %d want 400", rec.Code)
	}
	// 未知 slug → 404
	rec = httptest.NewRecorder()
	a.publicBlogTTS(rec, httptest.NewRequest(http.MethodGet, "/api/v1/public/blog/tts?slug=nope", nil).WithContext(ctx))
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown slug: got %d want 404", rec.Code)
	}
}

func TestPublicBlogTTSReachesTTS(t *testing.T) {
	a := newVoiceReadAPI(t)
	ctx := context.Background()
	a.publishPost(t, ctx, "hello-world", "# 欢迎\n\n这是**第一篇**朗读测试文章，包含[链接](https://aiklog.cn)。")
	q := url.Values{}
	q.Set("slug", "hello-world")
	rec := httptest.NewRecorder()
	a.publicBlogTTS(rec, httptest.NewRequest(http.MethodGet, "/api/v1/public/blog/tts?"+q.Encode(), nil).WithContext(ctx))
	// 未配置 ai.tts provider → 抵达合成调用后返回 502 TTS_ERR（证明解析/读取/抽取链路通）。
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("got %d want 502 (TTS_ERR)，响应=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "TTS_ERR") {
		t.Errorf("响应缺 TTS_ERR：%s", rec.Body.String())
	}
}

// TestPublicPathBlogTTS 鉴权白名单：读者侧朗读端点对匿名 GET 必须放行（不读登录身份）。
func TestPublicPathBlogTTS(t *testing.T) {
	cases := []struct {
		method, path string
		want         bool
	}{
		{http.MethodGet, "/api/v1/public/blog/tts", true},
		{http.MethodPost, "/api/v1/public/blog/tts", false}, // 写操作不公开
		{http.MethodGet, "/api/v1/public/blog/tts?slug=x", true},
		// 已安装主题列表（2026-10-09 主题改版：前端懒加载基座依赖，匿名 GET 放行）
		{http.MethodGet, "/api/v1/public/blog/themes", true},
		{http.MethodPost, "/api/v1/public/blog/themes", false}, // 写操作不公开
		{http.MethodGet, "/api/v1/public/blog/editors", true},
		{http.MethodPost, "/api/v1/public/blog/editors", false}, // 写操作不公开
	}
	for _, c := range cases {
		req := httptest.NewRequest(c.method, c.path, nil)
		if got := publicPath(req); got != c.want {
			t.Errorf("%s %s: publicPath=%v want %v", c.method, c.path, got, c.want)
		}
	}
}
