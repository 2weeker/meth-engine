package routes

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"meth-enginev2/config"
	"meth-enginev2/controller"
)

func get(t *testing.T, h http.Handler, path string) (*http.Response, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
	b, _ := io.ReadAll(rec.Result().Body)
	return rec.Result(), string(b)
}

func TestStaticIconsAndCustomLogo(t *testing.T) {
	logo := filepath.Join(t.TempDir(), "mine.png")
	if err := os.WriteFile(logo, []byte("my logo"), 0o600); err != nil {
		t.Fatal(err)
	}
	var cfg config.Config
	cfg.Site.Banner.Logo = logo
	assets := fstest.MapFS{
		"static/apple-touch-icon.png": {Data: []byte("touch")},
		"static/favicon.ico":          {Data: []byte("ico")},
		"static/css/main.css":         {Data: []byte("@import \"./main/a.css\";\n")},
		"static/css/main/a.css":       {Data: []byte(".a {}\n")},
		"static/assets/logo.gif":      {Data: []byte("gif")},
		"static/fonts/inter.woff2":    {Data: []byte("font")},
	}
	app := Build(&controller.App{Cfg: cfg}, assets)
	for path, want := range map[string]string{
		"/site-logo.png":        "my logo",
		"/apple-touch-icon.png": "touch",
		"/favicon.ico":          "ico",
		"/assets/logo.gif":      "gif",
		"/fonts/inter.woff2":    "font",
	} {
		res, body := get(t, app, path)
		if res.StatusCode != 200 || body != want {
			t.Errorf("%s: %d %q, want %q", path, res.StatusCode, body, want)
		}
		if csp := res.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "script-src 'none'") {
			t.Errorf("%s: Content-Security-Policy %q must forbid scripts", path, csp)
		}
		if len(res.Cookies()) != 0 {
			t.Errorf("%s: a static file sets no cookie", path)
		}
	}
	for _, path := range []string{"/assets/", "/assets/nope.gif", "/js/live.js"} {
		if res, _ := get(t, app, path); res.StatusCode != 404 {
			t.Errorf("%s: want 404, got %d", path, res.StatusCode)
		}
	}
}

func TestStylesheetsCombined(t *testing.T) {
	assets := fstest.MapFS{
		"static/css/main.css":   {Data: []byte("@import \"./main/a.css\";\n@import \"./main/b.css\";\n")},
		"static/css/main/a.css": {Data: []byte(".a {}\n")},
		"static/css/main/b.css": {Data: []byte(".b {}\n")},
		"static/css/theme.css":  {Data: []byte("@import \"./main.css\";\n.t {}\n")},
	}
	app := Build(&controller.App{}, assets)
	res, body := get(t, app, "/css/theme.css")
	if res.StatusCode != 200 || body != ".a {}\n.b {}\n.t {}\n" || !strings.HasPrefix(res.Header.Get("Content-Type"), "text/css") {
		t.Fatalf("theme.css: %d %q %s", res.StatusCode, body, res.Header.Get("Content-Type"))
	}
	etag := res.Header.Get("ETag")
	if etag == "" || res.Header.Get("Cache-Control") != "public, max-age=3600" {
		t.Errorf("caching headers: etag=%q cache=%q", etag, res.Header.Get("Cache-Control"))
	}
	req := httptest.NewRequest("GET", "/css/theme.css", nil)
	req.Header.Set("If-None-Match", etag)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != 304 {
		t.Errorf("revalidation: %d", rec.Code)
	}
	for _, path := range []string{"/css/main/a.css", "/css/nope.css"} {
		if res, _ := get(t, app, path); res.StatusCode != 404 {
			t.Errorf("%s: want 404, got %d", path, res.StatusCode)
		}
	}
}

func TestHealthz(t *testing.T) {
	assets := fstest.MapFS{"static/css/main.css": {Data: []byte(".a {}\n")}}
	healthy := func(context.Context) (time.Duration, time.Duration, error) {
		return 1500 * time.Microsecond, 250 * time.Microsecond, nil
	}
	if res, body := get(t, Build(&controller.App{Ready: healthy}, assets), "/healthz"); res.StatusCode != 200 || body != "200 ok\ndatabase ping: 1.50 ms\ndatabase rtt: 0.25 ms\n" {
		t.Errorf("healthy: %d %q", res.StatusCode, body)
	}
	if res, body := get(t, Build(&controller.App{}, assets), "/healthz"); res.StatusCode != 200 || body != "200 ok\n" {
		t.Errorf("nothing to check: %d %q", res.StatusCode, body)
	}
	down := func(context.Context) (time.Duration, time.Duration, error) {
		return 0, 0, errors.New("pq: password for user meth")
	}
	if res, body := get(t, Build(&controller.App{Ready: down}, assets), "/healthz"); res.StatusCode != 503 || body != "503 unavailable\n" {
		t.Errorf("database down: %d %q", res.StatusCode, body)
	}
}

func TestOldBoardLinks(t *testing.T) {
	app := Build(&controller.App{}, fstest.MapFS{"static/css/main.css": {Data: []byte(".a {}\n")}})
	for in, want := range map[string]string{
		"/b/general":         "/",
		"/b/general/12":      "/12",
		"/b/music?tag=lo-fi": "/?tag=lo-fi",
		"/b/general/nope":    "/",
		"/overboard":         "/",
	} {
		res, _ := get(t, app, in)
		if res.StatusCode != 301 || res.Header.Get("Location") != want {
			t.Errorf("%s: %d to %q, want 301 to %q", in, res.StatusCode, res.Header.Get("Location"), want)
		}
	}
	if res, _ := get(t, app, "/not-a-post"); res.StatusCode != 404 {
		t.Errorf("/not-a-post: want 404, got %d", res.StatusCode)
	}
}
