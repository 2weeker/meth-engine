package controller

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"meth-enginev2/config"
	"meth-enginev2/handler"
	"meth-enginev2/helper/auth"
	"meth-enginev2/helper/captcha/captchav1"
	"meth-enginev2/helper/palette"
)

func captchaApp(a *App) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /captcha/{file}", handler.HTTP(a.CaptchaImage))
	mux.Handle("GET /mod/login", handler.HTTP(a.LoginPage))
	mux.Handle("POST /mod/authenticate", handler.HTTP(a.Authenticate))
	mux.Handle("POST /gate", handler.HTTP(func(w http.ResponseWriter, r *http.Request) error {
		res := a.checkCaptcha(r, postingForm, auth.AddressKey(a.ClientIP(r)), a.PostCaptchaFails)
		switch {
		case res.Lockout > 0:
			io.WriteString(w, "locked "+res.Lockout.Round(time.Second).String())
		case res.Passed:
			io.WriteString(w, "passed")
		default:
			io.WriteString(w, "refused")
		}
		return nil
	}))
	return mux
}

var store *captchav1.Store

func testApp() *App {
	var cfg config.Config
	cfg.TrustedProxy = true
	store = captchav1.New(0)
	return &App{
		Cfg:               cfg,
		Captcha:           store,
		Logins:            auth.NewLoginThrottle(),
		PostCaptchaFails:  auth.NewFailures(3, time.Minute),
		LoginCaptchaFails: auth.NewFailures(2, 10*time.Minute),
	}
}

func do(t *testing.T, app http.Handler, method, path, ip string, form url.Values) (int, string, http.Header) {
	t.Helper()
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req := httptest.NewRequest(method, path, body)
	req.RemoteAddr = "10.0.0.1:4321"
	req.Header.Set("X-Forwarded-For", ip)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String(), rec.Header()
}

const alice, bob = "203.0.113.7", "198.51.100.9"

func post(addr string) captchav1.Binding {
	return captchav1.Binding{Purpose: captchav1.Post, Addr: auth.AddressKey(addr)}
}

func wrong(id string) url.Values {
	return url.Values{"captcha_id": {id}, "captcha_value": {"zzzzzz"}}
}

func TestCaptchaImage(t *testing.T) {
	a := testApp()
	app := captchaApp(a)
	id := store.Issue(post(alice))
	code, body, h := do(t, app, "GET", "/captcha/"+id+".png", alice, nil)
	if code != 200 || !strings.HasPrefix(body, "\x89PNG") || h.Get("Content-Type") != "image/png" {
		t.Errorf("image: %d %q", code, h.Get("Content-Type"))
	}
	if cc := h.Get("Cache-Control"); !strings.Contains(cc, "private") {
		t.Errorf("Cache-Control %q", cc)
	}
	if code, _, _ := do(t, app, "GET", "/captcha/"+id+".png", bob, nil); code != 404 {
		t.Errorf("another address: want 404, got %d", code)
	}
	for _, path := range []string{"/captcha/" + id, "/captcha/" + id + ".gif", "/captcha/0123456789abcdef0123456789abcdef.png"} {
		if code, _, _ := do(t, app, "GET", path, alice, nil); code != 404 {
			t.Errorf("%s: want 404, got %d", path, code)
		}
	}
	if code, _, _ := do(t, captchaApp(&App{}), "GET", "/captcha/"+id+".png", alice, nil); code != 404 {
		t.Errorf("captcha off: want 404, got %d", code)
	}
}

func gate(t *testing.T, app http.Handler, ip string, form url.Values) string {
	t.Helper()
	_, body, _ := do(t, app, "POST", "/gate", ip, form)
	return body
}

func TestPostingCaptchaLockout(t *testing.T) {
	a := testApp()
	app := captchaApp(a)
	if got := gate(t, app, bob, wrong(store.Issue(post(alice)))); got != "refused" {
		t.Errorf("answered from elsewhere: %s", got)
	}
	for i := 1; i <= 2; i++ {
		if got := gate(t, app, alice, wrong(store.Issue(post(alice)))); got != "refused" {
			t.Fatalf("failure %d is refused, not yet locked: %s", i, got)
		}
	}
	if got := gate(t, app, alice, wrong(store.Issue(post(alice)))); got != "locked 1m0s" {
		t.Fatalf("the third failure locks for 1m: %s", got)
	}
	ch := store.Issue(post(alice))
	if got := gate(t, app, alice, wrong(ch)); !strings.HasPrefix(got, "locked") {
		t.Errorf("while locked out: %s", got)
	}
	if _, ok := store.Image(ch, auth.AddressKey(alice)); !ok {
		t.Error("a locked-out attempt is refused unchecked, so its captcha is not spent")
	}
	if got := gate(t, app, bob, wrong(store.Issue(post(bob)))); got != "refused" {
		t.Errorf("another address is unaffected: %s", got)
	}
	for i := 1; i <= 2; i++ {
		gate(t, app, "192.0.2.1", url.Values{})
	}
	if got := gate(t, app, "192.0.2.1", url.Values{}); !strings.HasPrefix(got, "locked") {
		t.Errorf("sending no captcha at all, three times: %s", got)
	}
	if _, locked := a.LoginCaptchaFails.Locked(auth.AddressKey(alice)); locked {
		t.Error("a posting lockout must not lock the login")
	}
}

func login(t *testing.T, app http.Handler, ip string, form url.Values) string {
	t.Helper()
	form.Set("username", "mod")
	form.Set("password", "guess")
	code, _, h := do(t, app, "POST", "/mod/authenticate", ip, form)
	if code != 303 {
		t.Fatalf("login: want a redirect, got %d", code)
	}
	return h.Get("Location")
}

func TestLoginCaptchaLockout(t *testing.T) {
	a := testApp()
	app := captchaApp(a)
	issue := func(ip string) string {
		return store.Issue(captchav1.Binding{Purpose: captchav1.Login, Addr: auth.AddressKey(ip)})
	}
	ch := issue(alice)
	if loc := login(t, app, alice, wrong(ch)); loc != "/mod/login?failed=captcha" {
		t.Fatalf("first wrong captcha: %q", loc)
	}
	if _, ok := store.Image(ch, auth.AddressKey(alice)); ok {
		t.Error("the login uses its captcha up, whatever the password was")
	}
	if loc := login(t, app, alice, wrong(issue(alice))); loc != "/mod/login?failed=captchalock" {
		t.Fatalf("second wrong captcha locks the login: %q", loc)
	}
	_, page, _ := do(t, app, "GET", "/mod/login?failed=captchalock", alice, nil)
	if !strings.Contains(page, "Too many wrong captchas: the login is locked for 10 minutes") || strings.Contains(page, `name="captcha_id"`) {
		t.Errorf("login page while locked out:\n%s", page)
	}
	if loc := login(t, app, bob, wrong(store.Issue(post(bob)))); loc != "/mod/login?failed=captcha" {
		t.Errorf("another address, posting captcha at the login: %q", loc)
	}
	if got := gate(t, app, alice, wrong(store.Issue(post(alice)))); got != "refused" {
		t.Errorf("a login lockout must not lock posting: %s", got)
	}
}

func TestCaptchaOff(t *testing.T) {
	a := &App{PostCaptchaFails: auth.NewFailures(1, time.Hour), Logins: auth.NewLoginThrottle()}
	app := captchaApp(a)
	for i := 0; i < 3; i++ {
		if got := gate(t, app, alice, url.Values{}); got != "passed" {
			t.Fatalf("captcha off: %s", got)
		}
	}
	if _, page, _ := do(t, app, "GET", "/mod/login", alice, nil); strings.Contains(page, "captcha") {
		t.Error("with the captcha off the login shows none")
	}
}

func TestClientIPTrustsOnlyTheProxy(t *testing.T) {
	ask := func(trustProxy bool, remote string) string {
		a := &App{}
		a.Cfg.TrustedProxy = trustProxy
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = remote
		req.Header.Set("X-Forwarded-For", "203.0.113.7, 10.0.0.1")
		return a.ClientIP(req)
	}
	if got := ask(true, "127.0.0.1:5000"); got != "203.0.113.7" {
		t.Errorf("from the trusted proxy: %q", got)
	}
	if got := ask(true, "172.16.0.2:5000"); got != "203.0.113.7" {
		t.Errorf("from the docker network: %q", got)
	}
	if got := ask(true, "8.8.8.8:5000"); got != "8.8.8.8" {
		t.Errorf("from an untrusted address the header must be ignored, got %q", got)
	}
	if got := ask(false, "127.0.0.1:5000"); got != "127.0.0.1" {
		t.Errorf("with METH_TRUSTED_PROXY off the header must be ignored, got %q", got)
	}
	a := &App{}
	a.Cfg.TrustedProxy = true
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "127.0.0.1:5000"
	req.Header.Set("X-Forwarded-Proto", "https")
	if a.Scheme(req) != "https" {
		t.Error("the proxy's scheme is believed")
	}
	req.RemoteAddr = "8.8.8.8:5000"
	if a.Scheme(req) != "http" {
		t.Error("a stranger's scheme is not")
	}
}

func TestNew(t *testing.T) {
	t.Setenv("METH_POST_COOLDOWN", "")
	t.Setenv("METH_CAPTCHA_ENABLED", "")
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("tags: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	a := New(cfg, nil, nil)
	if a.Sessions == nil || a.Logins == nil || a.PostCaptchaFails == nil || a.LoginCaptchaFails == nil || a.Bursts == nil || a.Captcha == nil {
		t.Fatal("New left a guard unset")
	}
	cfg.CaptchaEnabled = false
	if b := New(cfg, nil, nil); b.Captcha != nil {
		t.Error("captcha off: want no captcha")
	}
}

func TestPlaceholder(t *testing.T) {
	t.Setenv("METH_POST_COOLDOWN", "")
	t.Setenv("METH_CAPTCHA_ENABLED", "")
	for body, want := range map[string]string{
		"max_chars: 2048\ncaptcha_post_limit: {posts: 1, within: 1m, wait: 60s}": "2048 character limit",
		"captcha_post_limit: {posts: 10, within: 1m, wait: 15s}":                 "1024 character limit",
		"title: x": "1024 character limit",
	} {
		path := filepath.Join(t.TempDir(), "config.yaml")
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		cfg, err := config.Load(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := (&App{Cfg: cfg}).placeholder(); got != want {
			t.Errorf("%q: got %q, want %q", body, got, want)
		}
	}
	if got := tagsHint(config.TagLimits{Max: 3, Length: 24}); got != "Up to 3 tags, separated by spaces" {
		t.Errorf("tagsHint(3) = %q", got)
	}
	if got := tagsHint(config.TagLimits{Max: 1, Length: 24}); got != "1 tag" {
		t.Errorf("tagsHint(1) = %q", got)
	}
}

func TestRefusedPostKeepsNameNotSecret(t *testing.T) {
	on := true
	a := &App{}
	a.Cfg.Site.Tripcodes = &on
	loc, u := postForm(t, a, url.Values{"msg": {"hello"}, "name": {"Anon#hunter2"}, HoneypotField: {"spam"}})
	if got := u.Query().Get("name"); got != "Anon" {
		t.Errorf("the form gets back name %q, want %q (redirect %s)", got, "Anon", loc)
	}
	if strings.Contains(loc, "hunter2") {
		t.Errorf("the redirect carries the secret: %s", loc)
	}
}

func postForm(t *testing.T, a *App, form url.Values) (string, *url.URL) {
	t.Helper()
	r := httptest.NewRequest("POST", "/post/create", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	if err := a.CreatePost(w, r); err != nil {
		t.Fatal(err)
	}
	loc := w.Result().Header.Get("Location")
	u, err := url.Parse(loc)
	if err != nil {
		t.Fatal(err)
	}
	return loc, u
}

func TestNameNeedsTripcode(t *testing.T) {
	on := true
	a := &App{}
	a.Cfg.Site.Tripcodes = &on
	for _, name := range []string{"Anon", "Anon#"} {
		_, u := postForm(t, a, url.Values{"msg": {"hello"}, "name": {name}})
		if q := u.Query(); q.Get("error") != "tripcode" || q.Get("msg") != "hello" || q.Get("name") != "Anon" {
			t.Errorf("name %q: redirected to %s, want a tripcode error with the draft and name", name, u)
		}
	}

	off := &App{}
	_, u := postForm(t, off, url.Values{"msg": {"hello"}, "name": {"Anon#hunter2"}, HoneypotField: {"spam"}})
	if u.Query().Has("name") {
		t.Errorf("with tripcodes off the name field is still read: %s", u)
	}
}

func TestNameCookie(t *testing.T) {
	a := &App{}
	for _, field := range []string{"Anon#tea", "名無し#ﾃｽﾄ", "a b#c,d;e\"f"} {
		w := httptest.NewRecorder()
		a.rememberName(w, field)
		r := httptest.NewRequest("GET", "/", nil)
		for _, c := range w.Result().Cookies() {
			if c.Name != nameCookie || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode {
				t.Errorf("cookie %+v is not an HttpOnly, Lax %s cookie", c, nameCookie)
			}
			r.AddCookie(c)
		}
		if got := rememberedName(r); got != field {
			t.Errorf("remembered %q, want %q", got, field)
		}
	}
	w := httptest.NewRecorder()
	a.rememberName(w, strings.Repeat("n", 200)+"#x")
	r := httptest.NewRequest("GET", "/", nil)
	for _, c := range w.Result().Cookies() {
		r.AddCookie(c)
	}
	if got := rememberedName(r); len([]rune(got)) != maxNameField {
		t.Errorf("an oversized field is kept to %d runes, not %d", maxNameField, len([]rune(got)))
	}

	r = httptest.NewRequest("GET", "/", nil)
	r.AddCookie(&http.Cookie{Name: nameCookie, Value: "not base64!"})
	if got := rememberedName(r); got != "" {
		t.Errorf("a bad cookie remembers %q", got)
	}
	if rememberedName(httptest.NewRequest("GET", "/", nil)) != "" {
		t.Error("no cookie remembers something")
	}

	w = httptest.NewRecorder()
	forgetName(w)
	if cs := w.Result().Cookies(); len(cs) != 1 || cs[0].Name != nameCookie || cs[0].MaxAge != -1 {
		t.Errorf("forgetting should expire the cookie: %+v", cs)
	}

	for _, c := range []struct {
		remembered, typed string
		kept              bool
		want              string
	}{
		{"Anon#tea", "Anon", true, "Anon#tea"},
		{"Anon#tea", "Anon", false, "Anon"},
		{"Anon#tea", "Other", true, "Other"},
		{"Anon#tea", "", true, ""},
		{"", "Anon", true, "Anon"},
	} {
		if got := composerName(c.remembered, c.typed, c.kept); got != c.want {
			t.Errorf("composerName(%q, %q, %v) = %q, want %q", c.remembered, c.typed, c.kept, got, c.want)
		}
	}
}

func TestPalette(t *testing.T) {
	a := &App{}
	a.Cfg.Site.Theme = "coffee"
	a.Palettes = map[string]palette.Palette{"coffee": {Colors: map[string]string{"accent": "#d99a5b", "text": "#f2dfc6"}, Tile: "coffee"}}
	mux := http.NewServeMux()
	mux.Handle("GET /theme/custom", handler.HTTP(a.PaletteEditor))
	mux.Handle("POST /theme/custom", handler.HTTP(a.SetPalette))
	mux.Handle("POST /theme/reset", handler.HTTP(a.ResetPalette))
	post := func(path string, form url.Values, cookies ...*http.Cookie) *http.Response {
		r := httptest.NewRequest("POST", path, strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		for _, c := range cookies {
			r.AddCookie(c)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w.Result()
	}

	res := post("/theme/custom", url.Values{"accent": {"#112233"}, "tile": {"dark"}, "image": {"https://pics.example/t.png"}, "fit": {"cover"}, "return_to": {"/7?tag=go"}})
	var set *http.Cookie
	for _, c := range res.Cookies() {
		if c.Name == paletteCookie {
			set = c
		}
	}
	if set == nil || !set.HttpOnly || set.SameSite != http.SameSiteLaxMode {
		t.Fatalf("no HttpOnly Lax palette cookie: %+v", res.Cookies())
	}
	if loc := res.Header.Get("Location"); loc != "/7?tag=go" {
		t.Errorf("after applying, sent to %q", loc)
	}
	r := httptest.NewRequest("GET", "/", nil)
	r.AddCookie(set)
	if css := a.customPalette(r).CSS(); css != `:root{--theme-accent:#112233;--theme-accent-rgb:17,34,51;--theme-background-image:url("https://pics.example/t.png");}body{background-size:cover;background-repeat:no-repeat;background-position:center;background-attachment:fixed;}` {
		t.Errorf("cookie gives %s", css)
	}
	if p := a.page(r); p.Palette == "" || !p.Custom {
		t.Error("the page does not carry the custom palette")
	}
	if p := a.page(httptest.NewRequest("GET", "/", nil)); p.Palette != "" || p.Custom {
		t.Error("without a cookie the page carries a palette")
	}

	res = post("/theme/custom", url.Values{"accent": {"red; } body { display:none"}, "return_to": {"/7"}})
	if len(res.Cookies()) != 0 || res.Header.Get("Location") != "/theme/custom?error=1&return_to=%2F7" {
		t.Errorf("bad colours: cookies %v, sent to %q", res.Cookies(), res.Header.Get("Location"))
	}
	res = post("/theme/custom", url.Values{"return_to": {"https://evil.example/"}})
	if res.Header.Get("Location") != "/" {
		t.Errorf("an off-site return_to is followed: %q", res.Header.Get("Location"))
	}

	res = post("/theme/reset", url.Values{"return_to": {"/id/abcdefgh"}}, set)
	if cs := res.Cookies(); len(cs) != 1 || cs[0].Name != paletteCookie || cs[0].MaxAge != -1 || res.Header.Get("Location") != "/id/abcdefgh" {
		t.Errorf("reset: cookies %+v, sent to %q", cs, res.Header.Get("Location"))
	}

	r = httptest.NewRequest("GET", "/theme/custom?return_to=/7", nil)
	e := a.editor(r)
	if e.Active || e.ReturnTo != "/7" || e.Tile != "coffee" || len(e.Fields) != len(palette.Fields) {
		t.Errorf("editor from the theme alone: %+v", e)
	}
	for _, f := range e.Fields {
		switch f.Key {
		case "accent":
			if f.Value != "#d99a5b" {
				t.Errorf("accent starts from the theme's: %s", f.Value)
			}
		case "greentext":
			if f.Value != "#000000" {
				t.Errorf("a colour the theme does not name falls back: %s", f.Value)
			}
		}
	}
	r.AddCookie(set)
	e = a.editor(r)
	if !e.Active || e.Tile != "dark" || e.Image != "https://pics.example/t.png" || e.Fit != "cover" || e.Fields[8].Key != "accent" || e.Fields[8].Value != "#112233" || e.Fields[0].Value != "#f2dfc6" {
		t.Errorf("editor with a palette set: %+v", e)
	}
}

func TestRefusedPostKeepsRememberedSecretOnlyIfUsed(t *testing.T) {
	on := true
	a := &App{}
	a.Cfg.Site.Tripcodes = &on
	w := httptest.NewRecorder()
	a.rememberName(w, "Anon#old")
	cookie := w.Result().Cookies()[0]
	for _, c := range []struct {
		field string
		kept  bool
	}{{"Anon#old", true}, {"Anon#new", false}, {"Anon", false}} {
		form := url.Values{"msg": {"hello"}, "name": {c.field}, HoneypotField: {"spam"}}
		r := httptest.NewRequest("POST", "/post/create", strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.AddCookie(cookie)
		w := httptest.NewRecorder()
		if err := a.CreatePost(w, r); err != nil {
			t.Fatal(err)
		}
		u, _ := url.Parse(w.Result().Header.Get("Location"))
		if got := u.Query().Get("kept") == "1"; got != c.kept || u.Query().Get("name") != "Anon" || strings.Contains(u.String(), "old") || strings.Contains(u.String(), "new") {
			t.Errorf("%q refused: redirect %s, want kept=%v and no secret", c.field, u, c.kept)
		}
	}
}

func TestPaletteStoresOnlyChanges(t *testing.T) {
	a := &App{}
	a.Cfg.Site.Theme = "coffee"
	a.Palettes = map[string]palette.Palette{"coffee": {Colors: map[string]string{"accent": "#d99a5b", "text": "#f2dfc6"}, Tile: "coffee"}}
	post := func(form url.Values) *http.Response {
		r := httptest.NewRequest("POST", "/theme/custom", strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		if err := a.SetPalette(w, r); err != nil {
			t.Fatal(err)
		}
		return w.Result()
	}
	res := post(url.Values{"accent": {"#d99a5b"}, "text": {"#f2dfc6"}, "tile": {"coffee"}})
	if cs := res.Cookies(); len(cs) != 1 || cs[0].MaxAge != -1 {
		t.Errorf("applying the theme's own colours should set no palette: %+v", cs)
	}
	res = post(url.Values{"accent": {"#112233"}, "text": {"#f2dfc6"}, "tile": {"coffee"}})
	var got palette.Palette
	for _, c := range res.Cookies() {
		if c.Name == paletteCookie {
			got = palette.Decode(c.Value)
		}
	}
	if len(got.Colors) != 1 || got.Colors["accent"] != "#112233" || got.Tile != "" {
		t.Errorf("only the changed colour should be kept: %+v", got)
	}
}

func TestBanTarget(t *testing.T) {
	for _, c := range []struct {
		raw    string
		wide   bool
		self   string
		entry  string
		status string
	}{
		{"203.0.113.7", false, "198.51.100.1", "203.0.113.7", ""},
		{"203.0.113.7/24", false, "198.51.100.1", "203.0.113.0/24", ""},
		{"2001:db8:1:2::9", true, "198.51.100.1", "2001:db8:1:2::/64", ""},
		{"2001:db8:1:2::9", false, "198.51.100.1", "2001:db8:1:2::9", ""},
		{"203.0.113.7", true, "198.51.100.1", "203.0.113.7", ""},
		{"", false, "198.51.100.1", "", "Enter an address"},
		{"banana", false, "198.51.100.1", "", "Not banned: not an address or a range like 2001:db8::/64"},
		{"10.0.0.0/8", false, "198.51.100.1", "", "Not banned: an IPv4 range wider than /16 is too broad to ban"},
		{"198.51.100.1", false, "198.51.100.1", "", "You cannot ban yourself"},
		{"198.51.100.0/24", false, "198.51.100.1", "", "You cannot ban yourself"},
		{"2001:db8:1:2::9", true, "2001:db8:1:2::abcd", "", "You cannot ban yourself"},
	} {
		entry, status := banTarget(c.raw, c.wide, c.self)
		if entry != c.entry || status != c.status {
			t.Errorf("banTarget(%q, %v, %q) = %q, %q; want %q, %q", c.raw, c.wide, c.self, entry, status, c.entry, c.status)
		}
	}
}
