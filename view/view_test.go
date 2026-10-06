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
	page := Page{Threads: Tree(posts, nil, false), Stats: "2 posts made per hour", CSRF: "tok", BaseURL: "https://meth.example"}
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
		`<div class="corner_controls"><a href="/mod" class="corner_button admin_button">admin</a></div>`,
	} {
		if !has(html, want) {
			t.Errorf("missing %q", want)
		}
	}
	for _, no := range []string{"<script", "board_tab", "theme_", "verboard", "/b/", "tag_banner", "post_tags", "/mod/move"} {
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
	page := Page{Threads: Tree(posts, nil, false), CSRF: "tok"}
	html := render(t, Layout(page))
	if has(html, `name="sage"`) {
		t.Error("the new-thread form must not offer sage")
	}
	if has(html, `Message:`) || !has(html, `<br><textarea name="msg" id="msg" aria-label="Message"`) {
		t.Error("the message box has no visible label, and follows the line break")
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
	page := Page{CSRF: "tok", History: "HR3oJ8FX", Threads: History(posts, false), Stats: "2 posts by ID:HR3oJ8FX"}
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
	if !has(render(t, Layout(page)), `<p class="overboard_empty">No posts by this ID.</p>`) {
		t.Error("an ID with no posts says so")
	}
}

func TestComposer(t *testing.T) {
	page := Page{CSRF: "tok", MaxChars: 512, Placeholder: "512 character limit"}
	html := render(t, Layout(page))
	if !has(html, `<textarea name="msg" id="msg" aria-label="Message" autofocus="true" maxlength="512" placeholder="512 character limit"></textarea>`) {
		t.Error("the message box carries the limit and the hint")
	}
	if !has(html, `<details class="form"><summary class="form_heading">New thread <br></summary>`) {
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
}

func TestLoginLogo(t *testing.T) {
	html := render(t, ModLayout(ModPage{Site: config.DefaultSite(), Screen: "login", CSRF: "tok"}))
	logo := `<a href="/" class="mod_logo_link"><img class="mod_logo" src="/assets/logo.gif" alt="meth"></a>`
	form, logoAt, user := strings.Index(html, `<form action="/mod/authenticate"`), strings.Index(html, logo), strings.Index(html, `name="username"`)
	if form < 0 || logoAt < form || user < logoAt {
		t.Errorf("want the logo inside the form, above the username:\n%s", html)
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
	page.Site.Webring = nil
	if has(render(t, Layout(page)), "webring") {
		t.Error("with no links there is no webring")
	}
}
