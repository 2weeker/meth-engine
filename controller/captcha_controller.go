package controller

import (
	"net/http"

	"meth-enginev2/helper/captcha/captchav1"
	"meth-enginev2/view"
)

type form int

const (
	postingForm form = iota
	loginForm
)

func binding(f form, addr string) captchav1.Binding {
	if f == loginForm {
		return captchav1.Binding{Purpose: captchav1.Login, Addr: addr}
	}
	return captchav1.Binding{Purpose: captchav1.Post, Addr: addr}
}

func (a *App) issueCaptcha(f form, addr string) *view.Captcha {
	return &view.Captcha{ID: a.Captcha.Issue(binding(f, addr)), Width: captchav1.Width, Height: captchav1.Height}
}

func (a *App) verifyCaptcha(r *http.Request, f form, addr string) bool {
	return a.Captcha.Verify(r.PostFormValue("captcha_id"), r.PostFormValue("captcha_value"), binding(f, addr))
}
