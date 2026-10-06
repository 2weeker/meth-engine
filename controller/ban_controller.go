package controller

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"meth-enginev2/view"
)

func (a *App) banScreen(w http.ResponseWriter, r *http.Request, status string) error {
	p := a.modPage(r, "ban")
	p.Status = status
	var err error
	if p.Bans, err = a.Store.Bans(r.Context()); err != nil {
		return err
	}
	return a.html(w, r, view.ModLayout(p))
}

func (a *App) BanScreen(w http.ResponseWriter, r *http.Request) error {
	if _, err := a.guard(w, r); err != nil {
		return guarded(err)
	}
	return a.banScreen(w, r, "")
}

func (a *App) BanCreate(w http.ResponseWriter, r *http.Request) error {
	user, err := a.guard(w, r)
	if err != nil {
		return guarded(err)
	}
	ip := strings.TrimSpace(r.PostFormValue("ip_address"))
	if ip == "" {
		return a.banScreen(w, r, "Enter an address")
	}
	if ip == a.ClientIP(r) {
		return a.banScreen(w, r, "You cannot ban yourself")
	}
	var expires *time.Time
	if days, err := strconv.Atoi(r.PostFormValue("days")); err == nil && days > 0 {
		t := time.Now().Add(time.Duration(days) * 24 * time.Hour)
		expires = &t
	}
	if err := a.Store.CreateBan(r.Context(), ip, r.PostFormValue("reason"), user, expires); err != nil {
		return err
	}
	return a.banScreen(w, r, "Banned "+ip)
}

func (a *App) BanDelete(w http.ResponseWriter, r *http.Request) error {
	if _, err := a.guard(w, r); err != nil {
		return guarded(err)
	}
	if err := a.Store.DeleteBan(r.Context(), strings.TrimSpace(r.PostFormValue("ip_address"))); err != nil {
		return a.banScreen(w, r, "No such ban")
	}
	return a.banScreen(w, r, "Ban removed")
}
