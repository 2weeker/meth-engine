package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"meth-enginev2/handler"
)

func TestSetThemeSetsCookieAndReturns(t *testing.T) {
	a := &App{}
	a.Cfg.Site.Theme = "coffee"
	mux := http.NewServeMux()
	mux.Handle("GET /style/{name}", handler.HTTP(a.SetTheme))
	do := func(path string) (string, string) {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		res := w.Result()
		for _, c := range res.Cookies() {
			if c.Name == themeCookie {
				return c.Value, res.Header.Get("Location")
			}
		}
		return "<none>", res.Header.Get("Location")
	}
	for path, want := range map[string][2]string{
		"/style/cyb?return_to=%2F7%3Ftag%3Dgo":         {"cyb", "/7?tag=go"},
		"/style/bm?return_to=/id/abcdefgh":             {"meth", "/id/abcdefgh"},
		"/style/nope?return_to=/mod/filter":            {"coffee", "/mod/filter"},
		"/style/macos":                                 {"macos", "/"},
		"/style/macos?return_to=https://evil.example/": {"macos", "/"},
		"/style/macos?return_to=//evil.example/":       {"macos", "/"},
		"/style/macos?return_to=/style/cyb":            {"macos", "/"},
		"/style/macos?return_to=/captcha/x.png":        {"macos", "/"},
	} {
		if cookie, loc := do(path); cookie != want[0] || loc != want[1] {
			t.Errorf("%s: cookie %q location %q, want %q %q", path, cookie, loc, want[0], want[1])
		}
	}
}

func TestThemeFromCookie(t *testing.T) {
	a := &App{}
	a.Cfg.Site.Theme = "ratwires"
	r := httptest.NewRequest("GET", "/", nil)
	if got := a.theme(r); got != "ratwires" {
		t.Errorf("no cookie: %q, want the site's theme", got)
	}
	r.AddCookie(&http.Cookie{Name: themeCookie, Value: "main"})
	if got := a.theme(r); got != "ratwires" {
		t.Errorf("renamed cookie: %q", got)
	}
	r = httptest.NewRequest("GET", "/", nil)
	r.AddCookie(&http.Cookie{Name: themeCookie, Value: "../x"})
	if got := a.theme(r); got != "ratwires" {
		t.Errorf("bad cookie falls back to the site's theme: %q", got)
	}
}
