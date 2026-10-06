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
