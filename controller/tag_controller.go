package controller

import (
	"log/slog"
	"net/http"
	"strconv"

	"meth-enginev2/helper/tags"
	"meth-enginev2/view"
)

func (a *App) tagScreen(w http.ResponseWriter, r *http.Request, status string) error {
	p := a.modPage(r, "tags")
	p.Status = status
	_, p.TagsOn = a.Cfg.Tags()
	counts, err := a.Store.TagCounts(r.Context())
	if err != nil {
		return err
	}
	p.Tags = counts
	return a.html(w, r, view.ModLayout(p))
}

func (a *App) TagScreen(w http.ResponseWriter, r *http.Request) error {
	if _, err := a.guard(w, r); err != nil {
		return guarded(err)
	}
	return a.tagScreen(w, r, "")
}

func (a *App) TagDelete(w http.ResponseWriter, r *http.Request) error {
	if _, err := a.guard(w, r); err != nil {
		return guarded(err)
	}
	tag := tags.Normalize(r.PostFormValue("tag"))
	if tag == "" {
		return a.tagScreen(w, r, "No such tag")
	}
	n, err := a.Store.DeleteTag(r.Context(), tag)
	if err != nil {
		return err
	}
	if n == 0 {
		return a.tagScreen(w, r, "No post carries "+tag)
	}
	slog.Info("tag deleted", "tag", tag, "posts", n)
	return a.tagScreen(w, r, "Tag "+tag+" removed from "+strconv.FormatInt(n, 10)+" "+plural(int(n), "post"))
}
