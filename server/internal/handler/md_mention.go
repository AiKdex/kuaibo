// md_mention.go 正文 @提及 渲染（B12）。
//
// 为什么需要这个扩展：提及能力（B6）此前只做「服务端落库 + 投递通知」，
// **正文里的 @ 不渲染**，于是有两处问题：
//
//  1. `@[显示名](user_id)` 是合法 Markdown 链接语法 → 渲染成 `<a href="user_id">`，
//     即一条**指向 user_id 的死链**（相对路径，点开 404）。写法本身没错，错在没有渲染层接管。
//  2. `@用户名` 纯写法完全没有视觉标记 —— "谁被提到了"读者看不出来。
//
// 本文件把两种写法统一渲染为 `<span class="mention">`（显式写法额外带 data-mention-id）。
//
// 两条硬约束（都是踩过才知道要写下来的）：
//
//  1. **规则必须与 service/mention.go 的 mentionExplicitRe / mentionPlainRe 逐字一致** ——
//     "会发通知的 @" 与 "被高亮的 @" 必须是同一批；两边各写一套迟早错配。
//  2. **纯写法必须做前界检查** —— 否则 `someone@example.com` 里的域名会被当成提及高亮。
//     用 text.Reader.PrecendingCharacter() 看前一个字符即可（不依赖正则 lookbehind，RE2 没有）。
//
// 不做跳转：本壳没有公开用户主页，硬造一个 href 就是造死链；显式写法把 user_id 落到
// data-mention-id，将来接入用户主页只需把 span 换成 a，不必改语法、不必动历史正文。
package handler

import (
	"regexp"
	"unicode"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

var (
	// mdMentionExplicitRe @[显示名](user_id)。与 service/mention.go 的 mentionExplicitRe 同源；
	// Go 的 RE2 不支持 \u 转义，非 ASCII 范围用 \p{...} 表达。
	mdMentionExplicitRe = regexp.MustCompile(`^@\[([^\]\n]{1,64})\]\(([0-9a-zA-Z_-]{8,64})\)`)
	// mdMentionPlainRe @名字（\p{L} 已含中文）。与 service/mention.go 的 mentionPlainRe 同源。
	mdMentionPlainRe = regexp.MustCompile(`^@([\p{L}\p{N}_.\-]{1,32})`)
)

// KindMention 提及节点的 NodeKind（渲染器据此注册）。
var KindMention = ast.NewNodeKind("Mention")

// mdMention goldmark 扩展：注册 @提及 的行内解析器与渲染器。
type mdMention struct{}

// Extend 实现 goldmark.Extender。
func (mdMention) Extend(m goldmark.Markdown) {
	// 优先级 500：高于默认链接解析器，保证 `@[名](id)` 由本扩展先接手
	// （否则会被解析成「字面 @ + 一条指向 user_id 的链接」，即上面说的死链）。
	m.Parser().AddOptions(parser.WithInlineParsers(util.Prioritized(&mdMentionParser{}, 500)))
	m.Renderer().AddOptions(renderer.WithNodeRenderers(util.Prioritized(&mdMentionRenderer{}, 500)))
}

// mdMentionParser 由 '@' 触发的行内解析器。
type mdMentionParser struct{}

// Trigger 触发字符。
func (p *mdMentionParser) Trigger() []byte { return []byte{'@'} }

// Parse 解析一个提及。
func (p *mdMentionParser) Parse(parent ast.Node, block text.Reader, pc parser.Context) ast.Node {
	// 1) 显式写法：无歧义，优先，不看前界。
	if m := block.FindSubMatch(mdMentionExplicitRe); m != nil {
		block.Advance(len(m[0]))
		return &mdMentionNode{Name: string(m[1]), UserID: string(m[2])}
	}
	// 2) 纯写法：前界检查。前一个字符属于「词内字符」时不算提及
	//    （否则邮箱 someone@example.com、代码里的 @media 之类会误高亮）。
	//    注意代码块/行内代码不经过行内解析器，天然不会中招。
	if ch := block.PrecendingCharacter(); ch != 0 {
		if unicode.IsLetter(ch) || unicode.IsDigit(ch) ||
			ch == '_' || ch == '.' || ch == '-' || ch == '+' {
			return nil
		}
	}
	if m := block.FindSubMatch(mdMentionPlainRe); m != nil {
		block.Advance(len(m[0]))
		return &mdMentionNode{Name: string(m[1])}
	}
	return nil
}

// mdMentionNode 一个提及节点。Name 不含前导 '@'；UserID 仅显式写法有值。
type mdMentionNode struct {
	ast.BaseInline
	Name   string
	UserID string
}

// Kind 实现 ast.Node。
func (n *mdMentionNode) Kind() ast.NodeKind { return KindMention }

// Dump 实现 ast.Node（调试用）。
func (n *mdMentionNode) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, map[string]string{"Name": n.Name, "UserID": n.UserID}, nil)
}

// mdMentionRenderer 把提及节点渲染为 span.mention。
type mdMentionRenderer struct{}

// RegisterFuncs 注册渲染函数。
func (r *mdMentionRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(KindMention, r.render)
}

func (r *mdMentionRenderer) render(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	mn, _ := n.(*mdMentionNode)
	_, _ = w.WriteString(`<span class="mention"`)
	if mn != nil && mn.UserID != "" {
		// UserID 由 `[0-9a-zA-Z_-]{8,64}` 限定，不含引号，属性位置安全。
		_, _ = w.WriteString(` data-mention-id="`)
		_, _ = w.Write(util.EscapeHTML([]byte(mn.UserID)))
		_, _ = w.WriteString(`"`)
	}
	_, _ = w.WriteString(`>@`)
	if mn != nil {
		_, _ = w.Write(util.EscapeHTML([]byte(mn.Name)))
	}
	_, _ = w.WriteString(`</span>`)
	return ast.WalkContinue, nil
}
