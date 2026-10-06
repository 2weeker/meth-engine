package controller

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"

	"meth-enginev2/config"
	"meth-enginev2/handler"
	"meth-enginev2/helper/auth"
	"meth-enginev2/helper/captcha/captchav1"
	"meth-enginev2/model"
	"meth-enginev2/view"
)

type App struct {
	Cfg      config.Config
	Store    *model.Store
	Sessions *auth.Sessions
	Logins   *auth.LoginThrottle

	Captcha                             *captchav1.Store
	PostCaptchaFails, LoginCaptchaFails *auth.Failures
	Bursts                              *auth.Bursts

	Ready func(context.Context) (ping, rtt time.Duration, err error)
}

const sessionCookie = "session"
const sessionTTL = 12 * time.Hour

func New(cfg config.Config, store *model.Store, ready func(context.Context) (ping, rtt time.Duration, err error)) *App {
	pl, ll := cfg.Site.CaptchaLockout.Posting, cfg.Site.CaptchaLockout.Login
	a := &App{
		Cfg: cfg, Store: store,
		Sessions:          auth.NewSessions(cfg.Secret),
		Logins:            auth.NewLoginThrottle(),
		PostCaptchaFails:  auth.NewFailures(pl.Limit(), pl.Duration()),
		LoginCaptchaFails: auth.NewFailures(ll.Limit(), ll.Duration()),
		Bursts:            auth.NewBursts(cfg.PostLimit().Count(), cfg.PostLimit().Window(), cfg.PostLimit().Pause()),
		Ready:             ready,
	}
	_, view.ShowTags = cfg.Tags()
	if cfg.CaptchaEnabled {
		a.Captcha = captchav1.New(captchav1.DefaultCapacity)
	}
	return a
}

func remoteHost(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return strings.Trim(host, "[]")
}

func trustedPeer(host string) bool {
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	addr = addr.Unmap()
	return addr.IsLoopback() || addr.IsPrivate() || addr.IsLinkLocalUnicast() || addr.IsUnspecified()
}

func (a *App) proxied(r *http.Request) bool {
	return a.Cfg.TrustedProxy && trustedPeer(remoteHost(r))
}

func (a *App) ClientIP(r *http.Request) string {
	ip := remoteHost(r)
	if a.proxied(r) {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			ip = strings.TrimSpace(strings.Split(xff, ",")[0])
		}
	}
	if host, _, err := net.SplitHostPort(ip); err == nil {
		ip = host
	}
	return strings.Trim(ip, "[]")
}

func (a *App) Scheme(r *http.Request) string {
	if a.proxied(r) {
		if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
			return strings.ToLower(strings.TrimSpace(strings.Split(proto, ",")[0]))
		}
	}
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

func (a *App) baseURL(r *http.Request) string {
	if a.Cfg.Site.URL != "" {
		return a.Cfg.Site.URL
	}
	return a.Scheme(r) + "://" + r.Host
}

func (a *App) modUser(r *http.Request) (string, bool) {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return "", false
	}
	return a.Sessions.Verify(c.Value)
}

func (a *App) setCookie(w http.ResponseWriter, name, value string, ttl time.Duration) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: value, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode,
		Secure: a.Cfg.Env == "production", Expires: time.Now().Add(ttl)})
}

func (a *App) html(w http.ResponseWriter, r *http.Request, c templ.Component) error {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	return c.Render(r.Context(), w)
}

func redirect(w http.ResponseWriter, r *http.Request, url string) error {
	http.Redirect(w, r, url, http.StatusSeeOther)
	return nil
}

func (a *App) Healthz(w http.ResponseWriter, r *http.Request) error {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if a.Ready == nil {
		_, err := w.Write([]byte("200 ok\n"))
		return err
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	ping, rtt, err := a.Ready(ctx)
	if err != nil {
		slog.Error("health check failed", "err", err)
		w.WriteHeader(http.StatusServiceUnavailable)
		_, err := w.Write([]byte("503 unavailable\n"))
		return err
	}
	_, err = w.Write([]byte("200 ok\n" +
		"database ping: " + millis(ping) + " ms\n" +
		"database rtt: " + millis(rtt) + " ms\n"))
	return err
}

func millis(d time.Duration) string {
	return strconv.FormatFloat(float64(d)/float64(time.Millisecond), 'f', 2, 64)
}

func (a *App) CaptchaImage(w http.ResponseWriter, r *http.Request) error {
	id, isPNG := strings.CutSuffix(r.PathValue("file"), ".png")
	if !isPNG {
		return handler.ErrNotFound
	}
	if a.Captcha == nil {
		return handler.ErrNotFound
	}
	png, ok := a.Captcha.Image(id, auth.AddressKey(a.ClientIP(r)))
	if !ok {
		return handler.ErrNotFound
	}
	w.Header().Set("Cache-Control", "private, max-age=300")
	w.Header().Set("Content-Type", "image/png")
	_, err := w.Write(png)
	return err
}

type captchaResult struct {
	Passed  bool
	Lockout time.Duration
}

func (a *App) checkCaptcha(r *http.Request, f form, addr string, fails *auth.Failures) captchaResult {
	if a.Captcha == nil {
		return captchaResult{Passed: true}
	}
	if wait, locked := fails.Locked(addr); locked {
		return captchaResult{Lockout: wait}
	}
	if a.verifyCaptcha(r, f, addr) {
		fails.Succeed(addr)
		return captchaResult{Passed: true}
	}
	if lock := fails.Fail(addr); lock > 0 {
		slog.Info("address locked out for failing the captcha", "form", map[form]string{postingForm: "post", loginForm: "login"}[f],
			"address", addr, "lockout", lock.String())
		return captchaResult{Lockout: lock}
	}
	return captchaResult{}
}
