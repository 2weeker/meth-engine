package controller

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"meth-enginev2/helper/netban"
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

func banTarget(raw string, wide bool, self string) (string, string) {
	if strings.TrimSpace(raw) == "" {
		return "", "Enter an address"
	}
	entry, err := netban.Normalize(raw)
	if err != nil {
		return "", "Not banned: " + err.Error()
	}
	if wide {
		entry = netban.Widen(entry, 64)
	}
	if netban.Covers(entry, self) {
		return "", "You cannot ban yourself"
	}
	return entry, ""
}

func (a *App) BanCreate(w http.ResponseWriter, r *http.Request) error {
	user, err := a.guard(w, r)
	if err != nil {
		return guarded(err)
	}
	ip, status := banTarget(r.PostFormValue("ip_address"), r.PostFormValue("range") == "64", a.ClientIP(r))
	if status != "" {
		return a.banScreen(w, r, status)
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
	entry := strings.TrimSpace(r.PostFormValue("ip_address"))
	if n, err := netban.Normalize(entry); err == nil {
		entry = n
	}
	if err := a.Store.DeleteBan(r.Context(), entry); err != nil {
		return a.banScreen(w, r, "No such ban")
	}
	return a.banScreen(w, r, "Ban removed")
}
