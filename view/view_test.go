package view

import (
	"context"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/a-h/templ"

	"meth-enginev2/config"
	"meth-enginev2/model"
)

var (
	spaces  = regexp.MustCompile(`\s+`)
	between = regexp.MustCompile(`>\s+<`)
	closing = regexp.MustCompile(`\s+</`)
)

func norm(s string) string {
	s = spaces.ReplaceAllString(s, " ")
	s = between.ReplaceAllString(s, "><")
	return closing.ReplaceAllString(s, "</")
}

func has(html, want string) bool { return strings.Contains(norm(html), norm(want)) }

func render(t *testing.T, c templ.Component) string {
	t.Helper()
	var b strings.Builder
	if err := c.Render(context.Background(), &b); err != nil {
		t.Fatal(err)
	}
	return norm(b.String())
}

func TestLayoutRenders(t *testing.T) {
	pid := int64(10)
	posts := []model.Post{
		{ID: 10, Message: ">be me\n**bold**", CreatedAt: time.Now()},
		{ID: 11, ParentID: &pid, Message: "reply", Sage: true, CreatedAt: time.Now()},
	}
	page := Page{Threads: Tree(posts, nil, false), Stats: "2 posts made per hour", CSRF: "tok", BaseURL: "https://meth.example",
		Theme: "coffee", Themes: ThemesFor("coffee"), ReturnTo: "/"}
	page.Site.Title, page.Site.Description = "meth", "a wall"
	html := render(t, Layout(page))
	for _, want := range []string{
		`<details class="post selected_post root_post" id="post-root" open="true">`,
		`<details class="post" id="post-10" open="true">`,
		`<span class="greentext">&gt;be me</span><br/><b>bold</b>`,
		`<span class="post_sage">sage</span>`,
		`<span class="post_sage post_sage_mark">sage</span>`,
		`class="backlink backlink_parent">&gt;&gt;10</a>`,
		`class="backlink backlink_reply">&gt;&gt;11</a>`,
		`href="/css/coffee.css"`, `<link rel="icon" type="image/gif" href="/favicon.gif">`, `id="board_top"`, `id="board_bottom"`,
		`<div class="post_header collapsed_board_header">2 posts made per hour</div>`,
		`<img class="banner_logo" src="/assets/logo.gif" alt="meth">`,
		`<meta property="og:image" content="https://meth.example/assets/share.png">`,
		`<meta property="og:image:width" content="1200"><meta property="og:image:height" content="630">`,
		`<meta name="twitter:card" content="summary_large_image">`,
		`<meta name="twitter:image" content="https://meth.example/assets/share.png">`,
		`<meta property="og:description" content="a wall">`,
		`name="_csrf" value="tok"`, `<textarea name="msg" id="msg" aria-label="Message" autofocus="true"></textarea>`,
		`<div class="corner_controls"><a href="/view/flat?return_to=%2F" class="corner_button flatten_button" title="Lay the posts out as a tree">flatten</a><details class="theme_switcher">`,
		`</details><a href="/mod" class="corner_button admin_button">admin</a></div>`,
		`<nav class="mobile_navigation"><a href="/" class="board_jump_button mobile_post_button">Post</a><div class="mobile_board_jump">`,
	} {
		if !has(html, want) {
			t.Errorf("missing %q", want)
		}
	}
	for _, no := range []string{"<script", "board_tab", "verboard", "/b/", "tag_banner", "post_tags", "/mod/move"} {
		if has(html, no) {
			t.Errorf("the page carries %q", no)
		}
	}
	mod := render(t, Layout(Page{Threads: Tree(posts, nil, true), CSRF: "tok"}))
	if !has(mod, `<a href="/mod?id=10&amp;ip=" id="mod-10">Mod</a> <a href="/10#reply-10" id="reply-10">Reply</a>`) {
		t.Error("mod view: each post has its Mod link")
	}
	for _, screen := range []string{"login", "panel", "delete", "filter", "ban", "user", "tags"} {
		render(t, ModLayout(ModPage{Screen: screen, CSRF: "tok"}))
	}
}

func TestCaptchaOnTheForms(t *testing.T) {
	page := Page{CSRF: "tok"}
	for name, c := range map[string]templ.Component{"posting form": Layout(page), "login": ModLayout(ModPage{Screen: "login", CSRF: "tok"})} {
		if has(render(t, c), "captcha") {
			t.Errorf("%s: no captcha was issued, so none may be shown", name)
		}
	}
	c := &Captcha{ID: "abc123", Width: 192, Height: 64}
	const block = `<div class="captcha_form"><img src="/captcha/abc123.png" alt="captcha" width="192" height="64" loading="lazy">` +
		`<input type="hidden" name="captcha_id" value="abc123">` +
		`<input type="text" name="captcha_value" placeholder="captcha" autocomplete="off" autocapitalize="off" autocorrect="off" spellcheck="false"></div>`
	page.Captcha = c
	html := render(t, Layout(page))
	if !has(html, `</textarea><br>`+block+`<input type="submit" value="post" id="post">`) {
		t.Errorf("posting form: want the captcha between the message and the button:\n%s", html)
	}
	if has(html, `name="captcha_pick"`) {
		t.Error("the captcha is a text box, not checkboxes")
	}
	page.Composer = Composer{Open: true, Error: "Incorrect or expired CAPTCHA"}
	if html := render(t, Layout(page)); !has(html, `<p class="post_error" role="alert">Incorrect or expired CAPTCHA</p><br>`+block) {
		t.Error("posting form: the refusal belongs directly above the captcha")
	}
	login := render(t, ModLayout(ModPage{Screen: "login", CSRF: "tok", Captcha: c, CaptchaFailed: true}))
	for _, want := range []string{
		`placeholder="password"> ` + block + `<input type="submit" value="login" id="login">`,
		`<p class="login_failed">Incorrect or expired CAPTCHA</p>`,
	} {
		if !has(login, want) {
			t.Errorf("login: missing %q", want)
		}
	}
}

func TestHoneypotOnThePostingForm(t *testing.T) {
	const trap = `<div class="form_trap" aria-hidden="true"><label>Leave this field empty <input type="text" name="website" value="" tabindex="-1" autocomplete="off"></label></div>`
	html := render(t, Layout(Page{CSRF: "tok"}))
	if n := strings.Count(html, trap); n != 1 {
		t.Errorf("want the honeypot in the form once, found it %d times", n)
	}
	if !has(html, `name="parent" id="parent" value="">`+trap) {
		t.Error("the honeypot belongs inside the posting form")
	}
	if has(render(t, Layout(Page{History: "x"})), "form_trap") {
		t.Error("history page: there is no form, so no honeypot")
	}
}

func TestPosterID(t *testing.T) {
	pid := int64(10)
	posts := []model.Post{
		{ID: 10, Message: "thread", PosterID: "FSGtlela", CreatedAt: time.Now()},
		{ID: 11, ParentID: &pid, Message: "saged reply", Sage: true, PosterID: "ab-_cd12", CreatedAt: time.Now()},
		{ID: 12, ParentID: &pid, Message: "old post, no id", CreatedAt: time.Now()},
	}
	html := render(t, Layout(Page{Threads: Tree(posts, nil, false), CSRF: "tok"}))
	for _, want := range []string{
		`id="reply-10">Reply</a> <a href="/id/FSGtlela" class="post_poster_id" title="Every post by this ID">ID:FSGtlela</a> <span class="post_meta">10 `,
		`<div class="post_signature"><a href="/id/FSGtlela" class="post_poster_id" title="Every post by this ID">ID:FSGtlela</a></div>`,
		`<div class="post_signature"><a href="/id/ab-_cd12" class="post_poster_id" title="Every post by this ID">ID:ab-_cd12</a><span class="post_sage post_sage_mark">sage</span></div>`,
		`id="reply-12">Reply</a> <span class="post_meta">12 `,
	} {
		if !has(html, want) {
			t.Errorf("page lacks %s", want)
		}
	}
	if n := strings.Count(html, "post_poster_id"); n != 4 {
		t.Errorf("%d poster ID spans, want 4", n)
	}
	if has(html, `ID:</span>`) {
		t.Error("a post without an ID must not show an empty one")
	}
}

func TestPostLinksFollowWhereThePostIsDrawn(t *testing.T) {
	posts := []model.Post{{ID: 5, Message: ">>3 and >>>/spam/9", Tags: []string{"x"}, CreatedAt: time.Now()}}
	main := string(Tree(posts, nil, false)[0].Message)
	away := string(Tagged(posts, "x", nil, false)[0].Message)
	for _, c := range []struct{ html, want string }{
		{main, `<a href="#post-3" class="quotelink">&gt;&gt;3</a>`},
		{away, `<a href="/#post-3" class="quotelink">&gt;&gt;3</a>`},
		{main, `&gt;&gt;&gt;/spam/9`},
	} {
		if !has(c.html, c.want) {
			t.Errorf("lacks %s in %s", c.want, c.html)
		}
	}
}

func TestSageIsForReplies(t *testing.T) {
	tid := int64(10)
	posts := []model.Post{
		{ID: 10, Message: "a thread saged before the change", Sage: true, CreatedAt: time.Now()},
		{ID: 11, ParentID: &tid, Message: "a saged reply", Sage: true, CreatedAt: time.Now()},
	}
	page := Page{Threads: Tree(posts, nil, false), CSRF: "tok", Tripcodes: true}
	html := render(t, Layout(page))
	if has(html, `name="sage"`) {
		t.Error("the new-thread form must not offer sage")
	}
	if has(html, `Message:`) || !has(html, `<br><input type="text" name="name" id="name" class="post_name_input"`) ||
		!has(html, `spellcheck="false"><textarea name="msg" id="msg" aria-label="Message"`) {
		t.Error("the message box has no visible label, and follows the line break and the name field")
	}
	if has(html, `id="post-10" open="true"><summary class="post_header"><span class="post_sage">`) {
		t.Error("a thread must not show a sage label")
	}
	if !has(html, `id="post-11" open="true"><summary class="post_header"><span class="post_sage">sage</span>`) {
		t.Error("a saged reply keeps its label")
	}
	page.ReplyTo = "10"
	if html := render(t, Layout(page)); !has(html, `<label class="sage_option"><input type="checkbox" name="sage" value="true"> Sage</label><br>`) {
		t.Error("the reply form must offer sage")
	}
	page.Composer.Sage = true
	if html := render(t, Layout(page)); !has(html, `name="sage" value="true" checked>`) {
		t.Error("a handed-back draft keeps sage ticked")
	}
}

func TestHistoryPage(t *testing.T) {
	pid := int64(3)
	posts := []model.Post{
		{ID: 9, Message: "a thread", PosterID: "HR3oJ8FX", CreatedAt: time.Now()},
		{ID: 5, ParentID: &pid, Message: "a reply, see >>4", PosterID: "HR3oJ8FX", CreatedAt: time.Now()},
	}
	page := Page{CSRF: "tok", History: "ID:HR3oJ8FX", Threads: History(posts, false), Stats: "2 posts by ID:HR3oJ8FX"}
	html := render(t, Layout(page))
	for _, want := range []string{
		`<div class="post root_post selected_post overboard history" id="post-root">`,
		`<div class="form overboard_prompt history_prompt">Posts by ID:HR3oJ8FX</div>`,
		`<a href="/9#reply-9" id="reply-9">Reply</a>`,
		`<a href="/#post-3" class="backlink backlink_parent">&gt;&gt;3</a></span>`,
		`<a href="/#post-4" class="quotelink">&gt;&gt;4</a>`,
		`<span class="post_poster_id">ID:HR3oJ8FX</span>`,
		`<div class="post_header collapsed_board_header">2 posts by ID:HR3oJ8FX</div>`,
	} {
		if !has(html, want) {
			t.Errorf("history page lacks %s", want)
		}
	}
	if strings.Index(html, `id="post-9"`) > strings.Index(html, `id="post-5"`) {
		t.Error("newest first")
	}
	for _, no := range []string{`href="/id/`, `name="msg"`} {
		if has(html, no) {
			t.Errorf("history page should not have %s", no)
		}
	}
	if strings.Count(html, `<details class="post" id="post-`) != 2 {
		t.Error("two posts, flat")
	}
	page.Threads = nil
	if !has(render(t, Layout(page)), `<p class="overboard_empty">No posts by ID:HR3oJ8FX.</p>`) {
		t.Error("an ID with no posts says so")
	}

	page.Threads, page.Flat = History(posts, false), true
	html = render(t, Layout(page))
	for _, want := range []string{
		`<div class="post root_post selected_post overboard history" id="post-root"><span id="board_top" class="board_anchor"></span><details class="post_tree" open><summary class="post_header tree_root"><a href="/">ID:HR3oJ8FX</a></summary><div class="tree_children"><div class="tree_node"><details class="post" id="post-9"`,
		`<span class="post_poster_id">ID:HR3oJ8FX</span>`,
	} {
		if !has(html, want) {
			t.Errorf("flat history page lacks %s", want)
		}
	}
	if strings.Count(html, `<div class="tree_node">`) != 2 || has(html, `href="/id/`) {
		t.Error("two posts as tree nodes, without links back to this page")
	}
	page.Threads = nil
	if !has(render(t, Layout(page)), `tree_root"><a href="/">ID:HR3oJ8FX</a></summary><p class="overboard_empty">No posts by ID:HR3oJ8FX.</p></details>`) {
		t.Error("a flat history page with no posts says so inside the tree")
	}
}

func TestComposer(t *testing.T) {
	page := Page{CSRF: "tok", MaxChars: 512, Placeholder: "512 character limit"}
	html := render(t, Layout(page))
	if !has(html, `<textarea name="msg" id="msg" aria-label="Message" autofocus="true" maxlength="512" placeholder="512 character limit"></textarea>`) {
		t.Error("the message box carries the limit and the hint")
	}
	if !has(html, `<details class="form"><summary class="form_heading">Posting <br></summary>`) {
		t.Error("a fresh form is sent closed")
	}
	page.Composer = Composer{Open: true, Message: "draft <b>", Error: "Too long"}
	html = render(t, Layout(page))
	if !has(html, `<details class="form form_kept_open" open>`) || !has(html, `>draft &lt;b&gt;</textarea>`) {
		t.Error("a handed-back draft is sent open and escaped")
	}
	page.ReplyTo = "7"
	if html := render(t, Layout(page)); !has(html, `Replying to post 7 <br>`) || !has(html, `action="/post/create/7"`) || !has(html, `name="parent" id="parent" value="7">`) {
		t.Error("the reply form names its parent")
	}
	if html := render(t, Layout(page)); !has(html, `<a href="/" class="board_jump_button mobile_post_button">Post</a>`) {
		t.Error("the mobile bar's Post button leads back to the board")
	}
}

func TestTags(t *testing.T) {
	ShowTags = true
	defer func() { ShowTags = false }()
	p10 := int64(10)
	posts := []model.Post{
		{ID: 10, Message: "a", Tags: []string{"lo-fi", "vinyl"}, CreatedAt: time.Now()},
		{ID: 12, ParentID: &p10, Message: "reply", Tags: []string{"stray"}, CreatedAt: time.Now()},
		{ID: 11, Message: "b", Tags: []string{"lo-fi"}, CreatedAt: time.Now()},
	}
	threads := Tree(posts, nil, false)
	if got := threads[0].Tags; len(got) != 2 || got[0] != (TagLink{Name: "lo-fi", Href: "/?tag=lo-fi"}) {
		t.Errorf("thread 10's tags: %+v", got)
	}
	if got := threads[0].Children[0].Tags; len(got) != 1 || got[0].Name != "stray" {
		t.Errorf("a reply shows its own tags: %+v", got)
	}
	if got := TagHref("c++ & go"); got != "/?tag=c%2B%2B+%26+go" {
		t.Errorf("TagHref escapes the tag: %q", got)
	}
	page := Page{CSRF: "tok", Threads: Tree(posts, nil, false), TagsHint: "Up to 3 tags, separated by spaces", Composer: Composer{Tags: "lo-fi vinyl"}}
	html := render(t, Layout(page))
	for _, want := range []string{
		`<a href="#post-10" class="backlink backlink_parent">&gt;&gt;10</a> <a href="/?tag=stray" class="thread_board thread_tag">stray</a></span>`,
		` <a href="/?tag=lo-fi" class="thread_board thread_tag">lo-fi</a> <a href="/?tag=vinyl" class="thread_board thread_tag">vinyl</a></span>`,
		`class="tags_input" aria-label="Tags" placeholder="Up to 3 tags, separated by spaces" value="lo-fi vinyl" autocomplete="off" autocapitalize="off" spellcheck="false"> <textarea name="msg"`,
	} {
		if !has(html, want) {
			t.Errorf("page lacks %s", want)
		}
	}
	page.Threads, page.TagFilter = Tagged(posts, "lo-fi", nil, false), "lo-fi"
	html = render(t, Layout(page))
	for _, want := range []string{
		`<a href="/" class="thread_board thread_tag current_tag" aria-current="page">lo-fi</a>`,
		`<a href="/?tag=vinyl" class="thread_board thread_tag">vinyl</a>`,
	} {
		if !has(html, want) {
			t.Errorf("filtered page lacks %s", want)
		}
	}
	if has(html, "tag_filter") {
		t.Error("a tag's page carries no filter note")
	}
	ShowTags = false
	html = render(t, Layout(Page{CSRF: "tok", Threads: Tree(posts, nil, false)}))
	for _, no := range []string{"thread_tag", `name="tags"`, ">lo-fi<"} {
		if has(html, no) {
			t.Errorf("with tags off the page shows %q", no)
		}
	}
}

func TestTaggedFilter(t *testing.T) {
	p10, p11, p12 := int64(10), int64(11), int64(12)
	posts := []model.Post{
		{ID: 10, Message: "thread, untagged", CreatedAt: time.Now()},
		{ID: 11, ParentID: &p10, Message: "reply, tagged", Tags: []string{"synth"}, CreatedAt: time.Now()},
		{ID: 13, ParentID: &p11, Message: "reply to it, tagged", Tags: []string{"synth"}, CreatedAt: time.Now()},
		{ID: 14, ParentID: &p11, Message: "reply to it, untagged", CreatedAt: time.Now()},
		{ID: 12, Message: "thread, tagged", Tags: []string{"synth", "vinyl"}, CreatedAt: time.Now()},
		{ID: 15, ParentID: &p12, Message: "reply, other tag", Tags: []string{"vinyl"}, CreatedAt: time.Now()},
	}
	got := Tagged(posts, "synth", nil, false)
	var ids []int64
	for _, v := range got {
		ids = append(ids, v.ID)
	}
	if !slices.Equal(ids, []int64{11, 12}) {
		t.Fatalf("top-level posts %v, want [11 12]", ids)
	}
	if len(got[0].Children) != 1 || got[0].Children[0].ID != 13 {
		t.Errorf("the tagged reply keeps its tagged reply nested, and drops the untagged one: %+v", got[0].Children)
	}
	if got[0].ParentHref != "/#post-10" {
		t.Errorf("a standalone reply's parent link opens the main page at the parent: %q", got[0].ParentHref)
	}
	if got[0].Children[0].ParentHref != "#post-11" {
		t.Errorf("a nested reply's parent is on the page: %q", got[0].Children[0].ParentHref)
	}
	if len(got[1].Children) != 0 {
		t.Error("a reply without the tag is dropped, even under a thread that has it")
	}
	if len(Tagged(posts, "nothing", nil, false)) != 0 {
		t.Error("a tag nothing carries shows nothing")
	}
}

func TestModScreens(t *testing.T) {
	hit := time.Date(2026, 9, 29, 8, 30, 0, 0, time.UTC)
	filters := render(t, ModLayout(ModPage{Screen: "filter", CSRF: "tok", Filters: []model.Filter{
		{ID: 1, Regex: `(?i)darn`, Action: model.FilterReplace, Replacement: "d***", Note: "language", Hits: 12, LastHitAt: &hit},
		{ID: 2, Regex: `casino\.example`, Action: model.FilterBan, BanDays: 7, Note: "spam <wave>"},
		{ID: 3, Regex: `(unclosed`, Action: model.FilterReject, Broken: true},
	}}))
	for _, want := range []string{
		`<td class="filter_table_data">replace with “d***”</td>`,
		`<td class="filter_table_data filter_note">language</td><td class="filter_table_data">12</td><td class="filter_table_data">2026-09-29 08:30</td>`,
		`<td class="filter_table_data">ban for 7 days</td><td class="filter_table_data filter_note">spam &lt;wave&gt;</td><td class="filter_table_data">0</td><td class="filter_table_data">never</td>`,
		`<tr class="filter_table_row filter_broken">`, `(unclosed <strong>(broken: skipped)</strong>`,
		`<option value="reject" selected>`,
	} {
		if !has(filters, want) {
			t.Errorf("filter screen: missing %q", want)
		}
	}
	refused := render(t, ModLayout(ModPage{Screen: "filter", CSRF: "tok", FilterForm: FilterForm{Regex: `a"b<c`, Action: "ban", BanDays: "soon", Note: "why"}}))
	for _, want := range []string{`value="a&#34;b&lt;c"`, `<option value="ban" selected>`, `<option value="reject">`, `value="soon"`, `value="why"`} {
		if !has(refused, want) {
			t.Errorf("refused filter form: missing %q", want)
		}
	}
	tags := render(t, ModLayout(ModPage{Screen: "tags", CSRF: "tok", TagsOn: true, Tags: []model.TagCount{{Tag: "lo-fi", Posts: 4}}}))
	if !has(tags, `<td class="filter_table_data">lo-fi</td><td class="filter_table_data">4</td>`) ||
		!has(tags, `<form action="/tags/delete" method="post"><input type="hidden" name="_csrf" value="tok"> <input type="hidden" name="tag" value="lo-fi"> <input type="submit" value="delete"></form>`) {
		t.Errorf("tags screen:\n%s", tags)
	}
	if !has(render(t, ModLayout(ModPage{Screen: "tags"})), "Tags are turned off.") {
		t.Error("with tags off the screen says so")
	}
	exp := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	bans := render(t, ModLayout(ModPage{Screen: "ban", CSRF: "tok", Bans: []model.Ban{{ID: 1, IPAddress: "1.2.3.4", Reason: "spam", ExpiresAt: &exp}, {ID: 2, IPAddress: "5.6.7.8"}}}))
	if !has(bans, `<td class="ban_table_data">spam</td><td class="ban_table_data">2026-10-09 12:00</td>`) || !has(bans, `<td class="ban_table_data">5.6.7.8</td><td class="ban_table_data"></td><td class="ban_table_data">never</td>`) {
		t.Errorf("ban screen:\n%s", bans)
	}
	panel := render(t, ModLayout(ModPage{Screen: "panel", CSRF: "tok"}))
	if !has(panel, `<a href="/mod/tags?id=&amp;ip=">Manage tags</a>`) || has(panel, "/mod/board") || has(panel, "/mod/move") {
		t.Error("the panel links to the tags screen, and to no board or move screen")
	}
}

func TestWebring(t *testing.T) {
	page := Page{CSRF: "tok", Stats: "1 post made per hour with 1 identity"}
	page.Site.Webring = []config.Link{{Label: "ratwires", URL: "https://ratwires.example/"}, {Label: "TOR & co", URL: "http://abc.onion/?a=1&b=2"}}
	html := render(t, Layout(page))
	want := `<div class="post_header collapsed_board_header">1 post made per hour with 1 identity</div>` +
		`<nav class="webring" aria-label="Webring"><span class="webring_title">Webring:</span> ` +
		`<a href="https://ratwires.example/" class="webring_link">ratwires</a> ` +
		`<a href="http://abc.onion/?a=1&amp;b=2" class="webring_link">TOR &amp; co</a></nav></div>`
	if !has(html, want) {
		t.Errorf("page lacks the webring:\n%s\n%s", want, html)
	}
	page.Flat = true
	html = render(t, Layout(page))
	if strings.Count(html, `class="webring"`) != 1 || !has(html, `<div class="tree_summary">1 post made per hour with 1 identity</div><nav class="webring"`) {
		t.Errorf("the tree should carry the webring once, under its tally:\n%s", html)
	}
	page.Flat = false
	page.Site.Webring = nil
	if has(render(t, Layout(page)), "webring") {
		t.Error("with no links there is no webring")
	}
}

func TestThemeSwitcher(t *testing.T) {
	html := render(t, Layout(Page{Theme: "cyb", Themes: ThemesFor("cyb"), ReturnTo: "/7?tag=go"}))
	if !has(html, `href="/css/cyb.css"`) {
		t.Error("the page links the chosen theme's stylesheet")
	}
	want := `<details class="theme_switcher"><summary class="theme_handle" aria-label="Themes" title="Themes"></summary><div class="theme_drawer"><div class="theme_drawer_title">Appearance</div>` +
		`<a href="/style/angelic?return_to=%2F7%3Ftag%3Dgo" class="theme_choice theme_choice_angelic">Angelic</a>` +
		`<a href="/style/blame?return_to=%2F7%3Ftag%3Dgo" class="theme_choice theme_choice_blame">BLAME!</a>` +
		`<a href="/style/coffee?return_to=%2F7%3Ftag%3Dgo" class="theme_choice theme_choice_coffee">Coffee</a>` +
		`<a href="/style/cyb?return_to=%2F7%3Ftag%3Dgo" class="theme_choice theme_choice_cyb current_theme">Cyb</a>` +
		`<a href="/style/macos?return_to=%2F7%3Ftag%3Dgo" class="theme_choice theme_choice_macos">MacOS</a>` +
		`<a href="/style/meth?return_to=%2F7%3Ftag%3Dgo" class="theme_choice theme_choice_meth">Meth</a>` +
		`<a href="/style/ratwires?return_to=%2F7%3Ftag%3Dgo" class="theme_choice theme_choice_ratwires">Ratwires</a>` +
		`<a href="/theme/custom?return_to=%2F7%3Ftag%3Dgo" class="theme_choice theme_choice_custom">Custom…</a>` +
		`</div></details>`
	if !has(html, want) {
		t.Errorf("switcher markup:\n%s", html)
	}
	if !has(html, `</details><a href="/mod" class="corner_button admin_button">admin</a></div>`) {
		t.Error("on a regular screen the themes button sits left of admin")
	}
	if !has(render(t, Layout(Page{})), `href="/css/coffee.css"`) {
		t.Error("a page with no theme falls back to coffee")
	}
	if !has(render(t, ModLayout(ModPage{Theme: "macos", Screen: "login"})), `href="/css/macos.css"`) {
		t.Error("the mod layout follows the theme too")
	}
}

func TestSiteTags(t *testing.T) {
	tags := []SiteTag{{Name: "go", Href: TagHref("go"), Posts: 3}, {Name: "lo-fi", Href: TagHref("lo-fi"), Posts: 1}}
	html := render(t, Layout(Page{SiteTags: tags}))
	list := `<p class="tag_list"><a href="/?tag=go" class="tag_list_link">go<span class="tag_list_count"> (3)</span></a> <a href="/?tag=lo-fi" class="tag_list_link">lo-fi<span class="tag_list_count"> (1)</span></a></p>`
	if !has(html, `<details id="tags_opener"><summary id="tags_button">Tags</summary>`+list+`</details>`) {
		t.Error("the banner lists every tag under About")
	}
	if has(html, "mobile_tags") {
		t.Error("the mobile nav has no Tags button")
	}
	if html := render(t, Layout(Page{})); has(html, "tags_opener") || has(html, "mobile_tags") {
		t.Error("no tags, no Tags button")
	}
}

func TestPagesAreStandardsMode(t *testing.T) {
	if html := render(t, Layout(Page{})); !strings.HasPrefix(html, "<!doctype html>") {
		t.Error("the board page must start with a doctype")
	}
	if html := render(t, ModLayout(ModPage{Screen: "login"})); !strings.HasPrefix(html, "<!doctype html>") {
		t.Error("the mod pages must start with a doctype")
	}
}

func TestTreeBoard(t *testing.T) {
	p := func(n int64) *int64 { return &n }
	now := time.Now()
	posts := []model.Post{
		{ID: 20, Message: "second thread", CreatedAt: now},
		{ID: 10, Message: "first thread", CreatedAt: now},
		{ID: 13, ParentID: p(11), Message: "deep", CreatedAt: now},
		{ID: 12, ParentID: p(10), Message: "direct", CreatedAt: now},
		{ID: 11, ParentID: p(10), Message: "reply", CreatedAt: now},
	}
	threads := Tree(posts, nil, false)
	if n := Descendants(threads[1]); n != 3 {
		t.Errorf("thread 10 hides %d posts when folded, want 3", n)
	}

	page := Page{Threads: threads, CSRF: "tok", ReturnTo: "/7", Flat: true, Stats: "5 posts made per hour with 1 identity"}
	page.Site.Title = "meth"
	html := render(t, Layout(page))
	for _, want := range []string{
		`<body class="flat_view">`,
		`<details class="post_tree" open><summary class="post_header tree_root"><a href="/">Post</a></summary><div class="tree_children">`,
		`</details><div class="tree_summary">5 posts made per hour with 1 identity</div>`,
		`<span class="tree_folded" title="Replies hidden while this post is folded">+3</span>`,
		`<div class="backlinks"><span class="backlinks_label">Replies:</span><a href="#post-11" class="backlink backlink_reply">&gt;&gt;11</a><a href="#post-12" class="backlink backlink_reply">&gt;&gt;12</a>`,
		`href="/view/nested?return_to=%2F7" class="corner_button flatten_button flatten_on"`,
	} {
		if !has(html, want) {
			t.Errorf("tree board: missing %q", want)
		}
	}
	order := regexp.MustCompile(`id="post-(\d+)"|</details><div class="tree_children">`).FindAllStringSubmatch(html, -1)
	var got []string
	for _, m := range order {
		if m[1] == "" {
			got = append(got, "{")
		} else {
			got = append(got, m[1])
		}
	}
	if want := []string{"20", "10", "{", "11", "{", "13", "12"}; !slices.Equal(got, want) {
		t.Errorf("tree order %v, want %v", got, want)
	}

	nested := render(t, Layout(Page{Threads: Tree(posts, nil, false), CSRF: "tok", ReturnTo: "/"}))
	for _, gone := range []string{"flat_view", "post_tree", "tree_folded"} {
		if strings.Contains(nested, gone) {
			t.Errorf("the nested board carries %s", gone)
		}
	}
	if !has(nested, `href="/view/flat?return_to=%2F" class="corner_button flatten_button"`) {
		t.Error("the nested board has no flatten button")
	}
	body := nested[strings.Index(nested, `id="post-10"`):strings.Index(nested, `id="post-12"`)]
	if strings.Contains(body, "</details>") {
		t.Error("the nested board no longer nests post 12 in post 10")
	}
}

func TestTripcode(t *testing.T) {
	now := time.Now()
	posts := []model.Post{
		{ID: 1, Message: "named", PosterID: "abcdEFGH", Name: "Anon", Tripcode: "WokonZwxw2", CreatedAt: now},
		{ID: 2, Message: "trip only", PosterID: "abcdEFGH", Tripcode: "WokonZwxw2", CreatedAt: now},
		{ID: 3, Message: "plain", PosterID: "ijklMNOP", CreatedAt: now},
	}
	html := render(t, Layout(Page{Threads: Tree(posts, nil, false), CSRF: "tok"}))
	named := `<span class="post_author">Anon</span><a href="/trip/WokonZwxw2" class="post_tripcode" title="Every post with this tripcode">◆WokonZwxw2</a>`
	if strings.Count(norm(html), norm(named)) != 2 {
		t.Error("a named post should show its name and tripcode link in the header and the signature")
	}
	if !has(html, `Reply</a><a href="/trip/WokonZwxw2" class="post_tripcode"`) {
		t.Error("a tripcode without a name should stand alone")
	}
	if got := TripHref("Wok/nZ.xw2"); got != "/trip/Wok%2FnZ.xw2" {
		t.Errorf("a tripcode's '/' must travel escaped: %s", got)
	}
	history := render(t, Layout(Page{CSRF: "tok", History: "◆WokonZwxw2", Threads: History(posts[:1], false)}))
	if has(history, `href="/trip/`) || !has(history, `<span class="post_tripcode" title="Tripcode">◆WokonZwxw2</span>`) {
		t.Error("on its own history page a tripcode is plain text")
	}
	if strings.Contains(html, `/id/abcdEFGH`) {
		t.Error("a post with a tripcode still shows its poster ID")
	}
	if !has(html, `<a href="/id/ijklMNOP" class="post_poster_id" title="Every post by this ID">ID:ijklMNOP</a>`) {
		t.Error("a post without a tripcode lost its poster ID")
	}
	page := Page{CSRF: "tok", Tripcodes: true, Composer: Composer{Open: true, Name: "Anon"}}
	if !has(render(t, Layout(page)), `placeholder="Name#tripcode (optional)" value="Anon"`) {
		t.Error("a refused post's name does not come back to the form")
	}
	page.Tripcodes = false
	if has(render(t, Layout(page)), `name="name"`) {
		t.Error("with tripcodes off the form still asks for a name")
	}
}

func TestCustomPalette(t *testing.T) {
	css := ":root{--theme-accent:#112233;--theme-accent-rgb:17,34,51;}"
	page := Page{CSRF: "tok", Theme: "coffee", Themes: ThemesFor("coffee"), ReturnTo: "/7", Palette: css, Custom: true}
	html := render(t, Layout(page))
	if !has(html, `href="/css/coffee.css"><style>`+css+`</style><link rel="icon"`) {
		t.Error("the custom palette is not applied right after the theme's stylesheet")
	}
	if !has(html, `<a href="/theme/custom?return_to=%2F7" class="theme_choice theme_choice_custom current_theme">Custom…</a></div></details>`) {
		t.Error("the drawer does not mark Custom as active")
	}
	if plain := render(t, Layout(Page{CSRF: "tok"})); strings.Contains(plain, "<style>") {
		t.Error("without a palette a <style> is emitted")
	}
	mod := render(t, ModLayout(ModPage{Theme: "coffee", Palette: css, Screen: "panel"}))
	if !has(mod, `<style>`+css+`</style>`) {
		t.Error("the mod pages do not carry the custom palette")
	}

	e := Editor{ReturnTo: "/7", Active: true, Tile: "dark", Tiles: []string{"bg", "dark", "none"},
		Image: "https://pics.example/t.png", Fit: "cover",
		Fields: []EditorField{{Key: "accent", Label: "Accent", Value: "#112233"}}}
	ed := render(t, ThemeEditor(page, e))
	for _, want := range []string{
		`<form action="/theme/custom" method="post"><input type="hidden" name="_csrf" value="tok"><input type="hidden" name="return_to" value="/7">`,
		`<label class="theme_editor_row"><span>Accent</span><input type="color" name="accent" value="#112233"></label>`,
		`<option value="dark" selected>dark</option>`,
		`<form action="/theme/reset" method="post" class="theme_editor_reset">`,
		`<input type="url" name="image" value="https://pics.example/t.png"`,
		`<input type="radio" name="fit" value="cover" checked>`,
		`<a href="/7" class="back_link">`,
		`<p class="theme_editor_intro">Custom theme editor</p>`,
	} {
		if !has(ed, want) {
			t.Errorf("editor lacks %s", want)
		}
	}
	e.Active = false
	if has(render(t, ThemeEditor(page, e)), "/theme/reset") {
		t.Error("with no palette set there is nothing to reset")
	}
}
