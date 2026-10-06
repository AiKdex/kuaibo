// b10_content_filter_test.go B10-1 内容类型过滤：公开轨只列文章，附件不进列表/热门/RSS/sitemap。
// 这里聚焦纯函数决策逻辑（articleVisibleInPublic / nodeTypeOf）；collectBlogArticles 只是
// collectDirFilesOpt(...,articlesOnly=true) 的薄封装，过滤动作完全由 articleVisibleInPublic 决定。
package handler

import "testing"

// nodeTypeOf 解析 content_state 的 node_type；空/非法 JSON 返回空串。
func TestNodeTypeOf(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{`{"node_type":"post","title":"x"}`, "post"},
		{`{"node_type":"image"}`, "image"},
		{"", ""},
		{"not-json", ""},
		{`{"title":"no type"}`, ""},
		{`{"node_type": 5}`, ""}, // 类型不符，解析失败
	}
	for _, c := range cases {
		if got := nodeTypeOf(c.in); got != c.want {
			t.Fatalf("nodeTypeOf(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// articleVisibleInPublic 决策矩阵：
//   - listAttachments=true：恢复旧行为，一律放行
//   - node_type 显式声明优先（post/article/page 视为文章；attachment/asset/media/image 视为附件）
//   - 未声明/非法：按 mime 兜底（isTextContent）
func TestArticleVisibleInPublic(t *testing.T) {
	post := `{"node_type":"post"}`
	image := `{"node_type":"image"}`
	asset := `{"node_type":"asset"}`

	cases := []struct {
		name  string
		mime  string
		state string
		list  bool
		want  bool
	}{
		{"listAttachments 强制放行", "image/png", image, true, true},
		{"listAttachments 强制放行(无类型)", "image/png", "", true, true},

		{"显式 article", "text/markdown", post, false, true},
		{"显式 post 即便 mime 是图片", "image/png", post, false, true},
		{"显式 page", "text/html", `{"node_type":"page"}`, false, true},

		{"显式 image 即便 mime 是文本", "text/markdown", image, false, false},
		{"显式 asset", "application/zip", asset, false, false},
		{"显式 media", "video/mp4", `{"node_type":"media"}`, false, false},

		// 未声明 → 按 mime 兜底
		{"无类型+文本 mime", "text/markdown", "", false, true},
		{"无类型+图片 mime", "image/png", "", false, false},
		{"无类型+空 mime(收录侧放行)", "", "", false, true},
		{"非法 JSON 按 mime 兜底", "application/pdf", "garbage{", false, false},
	}
	for _, c := range cases {
		if got := articleVisibleInPublic(c.mime, c.state, c.list); got != c.want {
			t.Fatalf("%s: articleVisibleInPublic(%q,%q,%v) = %v, want %v",
				c.name, c.mime, c.state, c.list, got, c.want)
		}
	}
}
