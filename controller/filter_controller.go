package controller

import (
	"net/http"
	"strconv"
	"strings"

	"meth-enginev2/model"
	"meth-enginev2/view"
)

func (a *App) filterScreen(w http.ResponseWriter, r *http.Request, status string, form ...view.FilterForm) error {
	p := a.modPage(r, "filter")
	p.Status = status
	if len(form) > 0 {
		p.FilterForm = form[0]
	}
	var err error
	if p.Filters, err = a.Store.Filters(r.Context()); err != nil {
		return err
	}
	return a.html(w, r, view.ModLayout(p))
}

func (a *App) FilterScreen(w http.ResponseWriter, r *http.Request) error {
	if _, err := a.guard(w, r); err != nil {
		return guarded(err)
	}
	return a.filterScreen(w, r, "")
}

func (a *App) FilterCreate(w http.ResponseWriter, r *http.Request) error {
	if _, err := a.guard(w, r); err != nil {
		return guarded(err)
	}
	typed := view.FilterForm{
		Regex:       r.PostFormValue("filter_regex"),
		Action:      r.PostFormValue("filter_action"),
		Replacement: r.PostFormValue("filter_replacement"),
		BanDays:     strings.TrimSpace(r.PostFormValue("filter_ban_days")),
		Note:        r.PostFormValue("filter_note"),
	}
	f := model.NewFilter{Regex: typed.Regex, Action: model.FilterAction(typed.Action),
		Replacement: typed.Replacement, Note: typed.Note}
	if typed.BanDays != "" && f.Action == model.FilterBan {
		n, err := strconv.Atoi(typed.BanDays)
		if err != nil {
			return a.filterScreen(w, r, "Filter not created: ban days must be a number", typed)
		}
		f.BanDays = n
	}
	if err := a.Store.CreateFilter(r.Context(), f); err != nil {
		return a.filterScreen(w, r, "Filter not created: "+err.Error(), typed)
	}
	return a.filterScreen(w, r, "Filter created")
}

func (a *App) FilterDelete(w http.ResponseWriter, r *http.Request) error {
	if _, err := a.guard(w, r); err != nil {
		return guarded(err)
	}
	id, _ := strconv.ParseInt(r.PostFormValue("filter_id"), 10, 64)
	if err := a.Store.DeleteFilter(r.Context(), id); err != nil {
		return a.filterScreen(w, r, "No such filter")
	}
	return a.filterScreen(w, r, "Filter deleted")
}
