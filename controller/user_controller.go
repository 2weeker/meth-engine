package controller

import (
	"net/http"
	"strings"

	"meth-enginev2/helper/auth"
	"meth-enginev2/view"
)

func (a *App) userScreen(w http.ResponseWriter, r *http.Request, status string) error {
	p := a.modPage(r, "user")
	p.Status = status
	var err error
	if p.Users, err = a.Store.Users(r.Context()); err != nil {
		return err
	}
	return a.html(w, r, view.ModLayout(p))
}

func (a *App) UserScreen(w http.ResponseWriter, r *http.Request) error {
	if _, err := a.guard(w, r); err != nil {
		return guarded(err)
	}
	return a.userScreen(w, r, "")
}

func (a *App) UserCreate(w http.ResponseWriter, r *http.Request) error {
	if _, err := a.guard(w, r); err != nil {
		return guarded(err)
	}
	name, pw := strings.TrimSpace(r.PostFormValue("username")), r.PostFormValue("password")
	if name == "" || pw == "" {
		return a.userScreen(w, r, "Username and password are both required")
	}
	hash, err := auth.HashPassword(pw)
	if err != nil {
		return err
	}
	if err := a.Store.CreateUser(r.Context(), name, hash); err != nil {
		return a.userScreen(w, r, "User not created (name taken?)")
	}
	return a.userScreen(w, r, "User "+name+" created")
}

func (a *App) UserDelete(w http.ResponseWriter, r *http.Request) error {
	me, err := a.guard(w, r)
	if err != nil {
		return guarded(err)
	}
	name := strings.TrimSpace(r.PostFormValue("username"))
	if name == me {
		return a.userScreen(w, r, "You cannot delete the account you are logged in as")
	}
	if err := a.Store.DeleteUser(r.Context(), name); err != nil {
		return a.userScreen(w, r, "No such user")
	}
	return a.userScreen(w, r, "User "+name+" deleted")
}
