package controller

import (
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"meth-enginev2/handler"
	"meth-enginev2/helper/themes"
	"meth-enginev2/view"
)

const themeCookie = "theme_name"

var returnPathRe = regexp.MustCompile(`^/(?:[0-9]+|id/[A-Za-z0-9_-]{1,16}|mod(?:/[a-z]+)?)?$`)

func (a *App) theme(r *http.Request) string {
	c, err := r.Cookie(themeCookie)
	if err != nil {
		return themes.Resolve("", a.Cfg.Site.Theme)
	}
	return themes.Resolve(c.Value, a.Cfg.Site.Theme)
}

func (a *App) page(r *http.Request) view.Page {
	theme := a.theme(r)
	return view.Page{Site: a.Cfg.Site, Theme: theme, Themes: view.ThemesFor(theme), ReturnTo: r.URL.RequestURI(),
		BaseURL: a.baseURL(r), CSRF: handler.CSRFToken(r), SiteTags: a.siteTags(r)}
}

func (a *App) SetTheme(w http.ResponseWriter, r *http.Request) error {
	a.setCookie(w, themeCookie, themes.Resolve(r.PathValue("name"), a.Cfg.Site.Theme), 365*24*time.Hour)
	http.Redirect(w, r, returnPath(r.URL.Query().Get("return_to")), http.StatusSeeOther)
	return nil
}

func returnPath(target string) string {
	if target == "" || strings.ContainsAny(target, "\\\r\n") {
		return "/"
	}
	u, err := url.Parse(target)
	if err != nil || u.Scheme != "" || u.Host != "" || !returnPathRe.MatchString(u.Path) {
		return "/"
	}
	if u.RawQuery != "" {
		return u.Path + "?" + u.RawQuery
	}
	return u.Path
}

// match tag buttons in the banner with the current theme
func (a *App) siteTags(r *http.Request) []view.SiteTag {
	if _, on := a.Cfg.Tags(); !on || a.Store == nil {
		return nil
	}
	counts, err := a.Store.TagCounts(r.Context())
	if err != nil {
		slog.Error("listing tags", "err", err)
		return nil
	}
	out := make([]view.SiteTag, len(counts))
	for i, c := range counts {
		out[i] = view.SiteTag{Name: c.Tag, Href: view.TagHref(c.Tag), Posts: c.Posts}
	}
	return out
}
