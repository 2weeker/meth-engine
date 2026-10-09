package controller

import (
	"errors"
	"math"
	"net/http"
	"strconv"
	"time"

	"meth-enginev2/core/posterror"
	"meth-enginev2/handler"
	"meth-enginev2/helper/auth"
	"meth-enginev2/view"
)

var errRedirected = errors.New("redirected")

func (a *App) modPage(r *http.Request, screen string) view.ModPage {
	q := r.URL.Query()
	id := q.Get("id")
	if id == "" {
		id = q.Get("post_id")
	}
	return view.ModPage{Site: a.Cfg.Site, Theme: a.theme(r), Screen: screen, CSRF: handler.CSRFToken(r), ID: id, IP: q.Get("ip")}
}

func (a *App) guard(w http.ResponseWriter, r *http.Request) (string, error) {
	user, ok := a.modUser(r)
	if !ok {
		http.Redirect(w, r, "/mod/login", http.StatusSeeOther)
		return "", errRedirected
	}
	return user, nil
}

func guarded(err error) error {
	if errors.Is(err, errRedirected) {
		return nil
	}
	return err
}

func (a *App) LoginPage(w http.ResponseWriter, r *http.Request) error {
	p := a.modPage(r, "login")
	failed := r.URL.Query().Get("failed")
	p.Failed = failed == "true"
	p.CaptchaFailed = failed == "captcha"
	addr := auth.AddressKey(a.ClientIP(r))
	if a.Captcha != nil {
		if wait, locked := a.LoginCaptchaFails.Locked(addr); locked {
			p.CaptchaLocked = posterror.CaptchaLockoutMessage(strconv.Itoa(int(math.Ceil(wait.Seconds()))), true)
			p.CaptchaFailed = false
		} else {
			p.Captcha = a.issueCaptcha(loginForm, addr)
		}
	}
	if wait, ok := a.Logins.Allow(a.ClientIP(r)); !ok {
		p.Wait = wait.Round(time.Second).String()
	}
	return a.html(w, r, view.ModLayout(p))
}

func (a *App) Authenticate(w http.ResponseWriter, r *http.Request) error {
	ip := a.ClientIP(r)
	if _, ok := a.Logins.Allow(ip); !ok {
		return redirect(w, r, "/mod/login?failed=true")
	}
	switch res := a.checkCaptcha(r, loginForm, auth.AddressKey(ip), a.LoginCaptchaFails); {
	case res.Lockout > 0:
		return redirect(w, r, "/mod/login?failed=captchalock")
	case !res.Passed:
		return redirect(w, r, "/mod/login?failed=captcha")
	}
	user, err := a.Store.UserByName(r.Context(), r.PostFormValue("username"))
	if err != nil {
		return err
	}
	if user == nil || !auth.CheckPassword(user.PasswordHash, r.PostFormValue("password")) {
		a.Logins.Fail(ip)
		return redirect(w, r, "/mod/login?failed=true")
	}
	a.Logins.Reset(ip)
	a.setCookie(w, sessionCookie, a.Sessions.Issue(user.Username, sessionTTL), sessionTTL)
	return redirect(w, r, "/mod")
}

func (a *App) Logout(w http.ResponseWriter, r *http.Request) error {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", Expires: time.Now().Add(-24 * time.Hour), MaxAge: -1})
	return redirect(w, r, "/mod/login")
}

func (a *App) Panel(w http.ResponseWriter, r *http.Request) error {
	if _, err := a.guard(w, r); err != nil {
		return guarded(err)
	}
	return a.html(w, r, view.ModLayout(a.modPage(r, "panel")))
}

func (a *App) DeleteScreen(w http.ResponseWriter, r *http.Request) error {
	if _, err := a.guard(w, r); err != nil {
		return guarded(err)
	}
	return a.html(w, r, view.ModLayout(a.modPage(r, "delete")))
}
