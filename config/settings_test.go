package config

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func write(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSiteFile(t *testing.T) {
	s, err := LoadSite(write(t, `
title: ratwires
description: a wall
banner:
  text: "Send tips:"
  addresses:
    - {label: Bitcoin, address: 1Lse}
    - {label: Monero, address: 44AB}
  links:
    - {label: TOR, url: "http://x.onion/"}
about: "<b>Ours.</b> Purges at 254."
`))
	if err != nil {
		t.Fatal(err)
	}
	if s.Title != "ratwires" || s.Banner.Text != "Send tips:" || len(s.Banner.Addresses) != 2 || s.Banner.Addresses[1].Label != "Monero" || s.Banner.Links[0].URL != "http://x.onion/" || !strings.HasPrefix(string(s.AboutHTML), "<b>") {
		t.Errorf("parsed wrong: %+v", s)
	}
}

func TestSiteDefaultsAndErrors(t *testing.T) {
	if _, err := LoadSite(filepath.Join(t.TempDir(), "nope.yaml")); err == nil {
		t.Error("an explicit missing file must fail")
	}
	t.Setenv("METH_CONFIG", "")
	t.Chdir(t.TempDir())
	s, err := LoadSite("")
	if err != nil || s.Title != "meth" || s.AboutHTML == "" {
		t.Errorf("defaults: %v %+v", err, s)
	}
	for _, body := range []string{"title: ''", "banner: {addresses: [{label: BTC}]}", "banner: {links: [{url: x}]}", "banner: [nope"} {
		if _, err := LoadSite(write(t, body)); err == nil {
			t.Errorf("%q should fail", body)
		}
	}
}

func TestEnvironment(t *testing.T) {
	t.Setenv("METH_CONFIG", "")
	t.Chdir(t.TempDir())
	t.Setenv("DATABASE_URL", "postgres://a@b/c")
	t.Setenv("PORT", "4000")
	t.Setenv("METH_POST_COOLDOWN", "")
	c, err := Load("")
	if err != nil || c.DatabaseURL != "postgres://a@b/c" || c.Addr != ":4000" || c.Site.Title != "meth" {
		t.Errorf("got %v %+v", err, c)
	}
}

func TestSiteLogoAndURL(t *testing.T) {
	s, err := LoadSite(write(t, "title: x"))
	if err != nil || s.LogoURL() != "/assets/logo.gif" || s.ShareImagePath() != "/assets/share.png" {
		t.Errorf("built-in logo: %v %q %q", err, s.LogoURL(), s.ShareImagePath())
	}

	p := write(t, "title: x\nurl: https://meth.example/\nbanner: {logo: art/Mine.PNG}")
	dir := filepath.Dir(p)
	if err := os.MkdirAll(filepath.Join(dir, "art"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "art", "Mine.PNG"), []byte("png"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err = LoadSite(p)
	if err != nil {
		t.Fatal(err)
	}
	if s.Banner.Logo != filepath.Join(dir, "art", "Mine.PNG") || s.LogoURL() != "/site-logo.png" || s.ShareImagePath() != "/site-logo.png" || s.URL != "https://meth.example" {
		t.Errorf("custom logo: %+v %q", s, s.LogoURL())
	}

	for _, body := range []string{
		"banner: {logo: missing.png}",
		"banner: {logo: notes.txt}",
		"banner: {logo: .}",
		"url: meth.example",
		"url: ftp://meth.example",
	} {
		if _, err := LoadSite(write(t, body)); err == nil {
			t.Errorf("%q should fail", body)
		}
	}
}

func TestLogOptions(t *testing.T) {
	for _, k := range []string{"METH_LOG_LEVEL", "METH_LOG_FORMAT", "METH_SLOW_REQUEST", "METH_SLOW_QUERY", "METH_STATS_INTERVAL"} {
		t.Setenv(k, "")
	}
	o, err := LogOptions()
	if err != nil || o.Level != slog.LevelInfo || o.JSON || o.SlowRequest != 500*time.Millisecond || o.SlowQuery != 100*time.Millisecond || o.StatsInterval != time.Minute {
		t.Errorf("defaults: %v %+v", err, o)
	}
	t.Setenv("METH_LOG_LEVEL", "debug")
	t.Setenv("METH_LOG_FORMAT", "JSON")
	t.Setenv("METH_SLOW_REQUEST", "2s")
	t.Setenv("METH_STATS_INTERVAL", "0")
	if o, err = LogOptions(); err != nil || o.Level != slog.LevelDebug || !o.JSON || o.SlowRequest != 2*time.Second || o.StatsInterval != 0 {
		t.Errorf("set: %v %+v", err, o)
	}
	for k, v := range map[string]string{"METH_LOG_LEVEL": "loud", "METH_LOG_FORMAT": "xml", "METH_SLOW_QUERY": "fast", "METH_STATS_INTERVAL": "-1m"} {
		t.Setenv(k, v)
		if _, err := LogOptions(); err == nil {
			t.Errorf("%s=%s should fail", k, v)
		}
		t.Setenv(k, "")
	}
}

func TestExampleConfig(t *testing.T) {
	t.Setenv("METH_POST_COOLDOWN", "")
	t.Setenv("METH_CAPTCHA_ENABLED", "")
	c, err := Load(filepath.Join("..", "config.example.yaml"))
	if err != nil {
		t.Fatalf("config.example.yaml does not load: %v", err)
	}
	s := c.Site
	if s.Title == "" || len(s.Banner.Addresses) == 0 || len(s.Banner.Links) == 0 || s.AboutHTML == "" {
		t.Errorf("example lost a section: %+v", s)
	}
	if s.Captcha == nil || !*s.Captcha {
		t.Errorf("the example should set captcha: true")
	}
	if l, on := c.Tags(); !on || l.Max != 3 || l.Length != 16 {
		t.Errorf("the example should turn tags on, 3 of 16 characters: %+v %v", l, on)
	}
	if c.MaxChars() != 2048 {
		t.Errorf("the example's max_chars: %d", c.MaxChars())
	}
	if p, l := s.CaptchaLockout.Posting, s.CaptchaLockout.Login; p.Limit() != 5 || p.Duration() != time.Minute || l.Limit() != 3 || l.Duration() != 15*time.Minute {
		t.Errorf("example captcha lockouts: posting %d/%v login %d/%v", p.Limit(), p.Duration(), l.Limit(), l.Duration())
	}
}

func TestCaptchaSetting(t *testing.T) {
	t.Setenv("METH_CAPTCHA_ENABLED", "")
	for _, c := range []struct {
		body       string
		want, fail bool
	}{

		{"title: x", true, false},
		{"title: x\ncaptcha: true", true, false},
		{"title: x\ncaptcha: false", false, false},

		{"title: x\ncaptcha: banana", false, true},
		{"title: x\ncaptcha: {enabled: true}", false, true},
	} {

		for _, env := range []string{"production", "development"} {
			t.Setenv("METH_ENV", env)
			cfg, err := Load(write(t, c.body))
			if (err != nil) != c.fail || (err == nil && cfg.CaptchaEnabled != c.want) {
				t.Errorf("%q in %s: enabled=%v err=%v", c.body, env, cfg.CaptchaEnabled, err)
			}
		}
	}

	t.Setenv("METH_CONFIG", "")
	t.Chdir(t.TempDir())
	if cfg, err := Load(""); err != nil || !cfg.CaptchaEnabled {
		t.Errorf("no config.yaml: enabled=%v err=%v", cfg.CaptchaEnabled, err)
	}

	for _, v := range []string{"false", "true", "0"} {
		t.Setenv("METH_CAPTCHA_ENABLED", v)
		if _, err := Load(""); err == nil || !strings.Contains(err.Error(), "config.yaml") {
			t.Errorf("METH_CAPTCHA_ENABLED=%s must be refused and point at config.yaml: %v", v, err)
		}
	}
}

func TestRetiredKeysRefused(t *testing.T) {
	t.Setenv("METH_POST_COOLDOWN", "")
	t.Setenv("METH_CAPTCHA_ENABLED", "")
	for body, names := range map[string]string{
		"boards: [{name: General}]": "one stream",
		"boards: []":                "one stream",
		"boards: [{name: A, captcha: true, mode: kareha}]": "one stream",
		"theme: coffee":                 "coffee",
		"theme: ratwires":               "coffee",
		"overboard: {enabled: false}":   "overboard",
		"rate_limit: {free_strikes: 3}": "captcha_post_limit",
		"rate_limit: {}":                "captcha_post_limit",
	} {
		_, err := Load(write(t, body))
		if err == nil || !strings.Contains(err.Error(), "no longer a setting") || !strings.Contains(err.Error(), names) {
			t.Errorf("%q must be refused as a removed setting, naming %q: %v", body, names, err)
		}
	}
}

func TestPostCooldownEnvRefused(t *testing.T) {
	p := write(t, "title: x")
	t.Setenv("METH_POST_COOLDOWN", "")
	if _, err := Load(p); err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{"0", "10s", "soon"} {
		t.Setenv("METH_POST_COOLDOWN", v)
		if _, err := Load(p); err == nil || !strings.Contains(err.Error(), "rate limit") || !strings.Contains(err.Error(), "captcha_post_limit") {
			t.Errorf("METH_POST_COOLDOWN=%s must be refused and point at the captcha: %v", v, err)
		}
	}
}

func TestCaptchaLockoutSettings(t *testing.T) {
	t.Setenv("METH_POST_COOLDOWN", "")
	t.Setenv("METH_CAPTCHA_ENABLED", "")

	s, err := LoadSite(write(t, "title: x"))
	if err != nil {
		t.Fatal(err)
	}
	if p, l := s.CaptchaLockout.Posting, s.CaptchaLockout.Login; p.Limit() != 5 || p.Duration() != time.Minute || l.Limit() != 3 || l.Duration() != 15*time.Minute {
		t.Errorf("defaults: posting %d/%v, login %d/%v", p.Limit(), p.Duration(), l.Limit(), l.Duration())
	}

	s, err = LoadSite(write(t, `
captcha_lockout:
  posting: {failures: 8, lockout: 30s}
  login: {failures: 2, lockout: 1h}
`))
	if err != nil {
		t.Fatal(err)
	}
	if p, l := s.CaptchaLockout.Posting, s.CaptchaLockout.Login; p.Limit() != 8 || p.Duration() != 30*time.Second || l.Limit() != 2 || l.Duration() != time.Hour {
		t.Errorf("set: posting %d/%v, login %d/%v", p.Limit(), p.Duration(), l.Limit(), l.Duration())
	}

	s, _ = LoadSite(write(t, "captcha_lockout: {login: {lockout: 2h}}"))
	if l := s.CaptchaLockout.Login; l.Limit() != 3 || l.Duration() != 2*time.Hour || s.CaptchaLockout.Posting.Limit() != 5 {
		t.Errorf("partial: login %d/%v posting %d", l.Limit(), l.Duration(), s.CaptchaLockout.Posting.Limit())
	}

	if s, err = LoadSite(write(t, "captcha_lockout: {posting: {lockout: 0}}")); err != nil || s.CaptchaLockout.Posting.Duration() != 0 {
		t.Errorf("off: %v %v", err, s.CaptchaLockout.Posting.Duration())
	}

	for name, body := range map[string]string{
		"no failures allowed":  "captcha_lockout: {posting: {failures: 0}}",
		"negative failures":    "captcha_lockout: {login: {failures: -1}}",
		"absurd failures":      "captcha_lockout: {login: {failures: 5000}}",
		"lockout with no unit": "captcha_lockout: {login: {lockout: 60}}",
		"negative lockout":     "captcha_lockout: {posting: {lockout: -1m}}",
		"lockout over the cap": "captcha_lockout: {login: {lockout: 1000h}}",
		"a side that isn't":    "captcha_lockout: {replies: {failures: 3}}",
		"a setting that isn't": "captcha_lockout: {login: {tries: 3}}",
	} {
		if _, err := LoadSite(write(t, body)); err == nil {
			t.Errorf("%s: %q should fail", name, body)
		}
	}
}

func TestCaptchaPostLimit(t *testing.T) {
	t.Setenv("METH_POST_COOLDOWN", "")
	t.Setenv("METH_CAPTCHA_ENABLED", "")
	c, err := Load(write(t, "title: x"))
	if err != nil {
		t.Fatal(err)
	}
	if p := c.Site.CaptchaPostLimit; p.Count() != 5 || p.Window() != time.Minute || p.Pause() != 30*time.Second {
		t.Errorf("defaults: %d posts in %v, wait %v; want 5, 1m, 30s", p.Count(), p.Window(), p.Pause())
	}
	c, err = Load(write(t, "captcha_post_limit: {posts: 3, within: 20s, wait: 0}"))
	if err != nil {
		t.Fatal(err)
	}
	if p := c.Site.CaptchaPostLimit; p.Count() != 3 || p.Window() != 20*time.Second || p.Pause() != 0 {
		t.Errorf("set: %d, %v, %v", p.Count(), p.Window(), p.Pause())
	}
	for _, body := range []string{
		"captcha_post_limit: {posts: 0}",
		"captcha_post_limit: {within: 0s}",
		"captcha_post_limit: {wait: -1s}",
		"captcha_post_limit: {wait: 25h}",
		"captcha_post_limit: {post: 5}",
	} {
		if _, err := Load(write(t, body)); err == nil {
			t.Errorf("%q must be refused", body)
		}
	}
}

func TestMaxChars(t *testing.T) {
	t.Setenv("METH_POST_COOLDOWN", "")
	t.Setenv("METH_CAPTCHA_ENABLED", "")
	if c, err := Load(write(t, "max_chars: 2000")); err != nil || c.MaxChars() != 2000 {
		t.Errorf("max_chars: 2000: %v %d", err, c.MaxChars())
	}
	if d, _ := Load(write(t, "title: x")); d.MaxChars() != DefaultMaxChars {
		t.Error("the default is 1024")
	}
	for _, body := range []string{"max_chars: 0", "max_chars: 100001"} {
		if _, err := Load(write(t, body)); err == nil {
			t.Errorf("%q must be refused", body)
		}
	}
}

func TestReport(t *testing.T) {
	t.Setenv("METH_POST_COOLDOWN", "")
	t.Setenv("METH_CAPTCHA_ENABLED", "")
	c, err := Load(write(t, `max_chars: 2000
captcha_post_limit: {posts: 2, within: 1m, wait: 10s}
tags: {max: 2, length: 12}`))
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	c.Report(&b)
	out := b.String()
	for _, want := range []string{
		"configuration ok: ",
		"captcha on\n",
		"  post limit: 2 posts within 1m0s, then a 10s wait\n",
		"  post length: 2000 characters\n",
		"  tags: up to 2 of 12 characters\n",
		"  posts kept: 254, then the oldest is purged\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("report lacks %q:\n%s", want, out)
		}
	}
	if got := c.Protection(); got != "captcha on, 2 posts within 1m0s then 10s" {
		t.Errorf("Protection() = %q", got)
	}
}

func TestMaxPostsAndAboutPlaceholders(t *testing.T) {
	t.Setenv("METH_POST_COOLDOWN", "")
	t.Setenv("METH_CAPTCHA_ENABLED", "")
	c, err := Load(write(t, `max_posts: 300
max_chars: 500
about: "Keeps {max_posts} posts of up to {max_chars} characters; {other} stays."`))
	if err != nil {
		t.Fatal(err)
	}
	if c.Site.PostCap() != 300 {
		t.Errorf("PostCap = %d, want 300", c.Site.PostCap())
	}
	if got, want := string(c.Site.AboutHTML), "Keeps 300 posts of up to 500 characters; {other} stays."; got != want {
		t.Errorf("about = %q, want %q", got, want)
	}

	c, err = Load(write(t, `about: "{max_posts} and {max_chars}"`))
	if err != nil {
		t.Fatal(err)
	}
	if c.Site.PostCap() != DefaultMaxPosts || string(c.Site.AboutHTML) != "254 and 1024" {
		t.Errorf("defaults: cap %d, about %q", c.Site.PostCap(), c.Site.AboutHTML)
	}

	for _, n := range []string{"0", "-1", "10001"} {
		if _, err := Load(write(t, "max_posts: "+n)); err == nil {
			t.Errorf("max_posts: %s must be refused", n)
		}
	}
}

func TestTags(t *testing.T) {
	t.Setenv("METH_POST_COOLDOWN", "")
	t.Setenv("METH_CAPTCHA_ENABLED", "")
	for body, want := range map[string]struct {
		on bool
		l  TagLimits
	}{
		"title: x":                   {false, TagLimits{}},
		"tags: false":                {false, TagLimits{}},
		"tags: true":                 {true, TagLimits{3, 24}},
		"tags: {max: 5}":             {true, TagLimits{5, 24}},
		"tags: {max: 2, length: 12}": {true, TagLimits{2, 12}},
	} {
		c, err := Load(write(t, body))
		if err != nil {
			t.Fatalf("%q: %v", body, err)
		}
		if l, on := c.Tags(); on != want.on || l != want.l {
			t.Errorf("%q: Tags() = %+v, %v; want %+v, %v", body, l, on, want.l, want.on)
		}
	}
	for _, body := range []string{
		"tags: maybe",
		"tags: {max: 0}",
		"tags: {max: 11}",
		"tags: {length: 0}",
		"tags: {length: 49}",
		"tags: {maxx: 3}",
	} {
		if _, err := Load(write(t, body)); err == nil {
			t.Errorf("%q must be refused", body)
		}
	}
}

func TestWebring(t *testing.T) {
	t.Setenv("METH_POST_COOLDOWN", "")
	t.Setenv("METH_CAPTCHA_ENABLED", "")
	if s, err := LoadSite(write(t, "title: x")); err != nil || len(s.Webring) != 0 {
		t.Errorf("default: %v %+v", err, s.Webring)
	}
	s, err := LoadSite(write(t, `webring:
  - {label: ratwires, url: "https://ratwires.example/"}
  - {label: TOR, url: "http://abc.onion/"}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Webring) != 2 || s.Webring[0] != (Link{Label: "ratwires", URL: "https://ratwires.example/"}) || s.Webring[1].Label != "TOR" {
		t.Errorf("parsed: %+v", s.Webring)
	}
	for _, body := range []string{
		"webring: [{url: 'https://a.example/'}]",
		"webring: [{label: '  ', url: 'https://a.example/'}]",
		"webring: [{label: A}]",
		"webring: [{label: A, url: /local}]",
		"webring: [{label: A, url: 'javascript:alert(1)'}]",
		"webring: [{label: A, url: a.example}]",
		"webring: [{label: A, url: 'ftp://a.example/'}]",
	} {
		if _, err := LoadSite(write(t, body)); err == nil || !strings.Contains(err.Error(), "webring[0]") {
			t.Errorf("%q must be refused, naming the link: %v", body, err)
		}
	}
}
