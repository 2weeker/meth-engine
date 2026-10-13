package controller

import (
	"encoding/base64"
	"net/http"
	"strings"
	"time"

	"meth-enginev2/helper/tripcode"
)

const nameCookie = "name"

const nameCookieTTL = 365 * 24 * time.Hour

const maxNameField = 128

func (a *App) rememberName(w http.ResponseWriter, field string) {
	if r := []rune(field); len(r) > maxNameField {
		field = string(r[:maxNameField])
	}
	a.setCookie(w, nameCookie, base64.RawURLEncoding.EncodeToString([]byte(field)), nameCookieTTL)
}

func forgetName(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: nameCookie, Value: "", Path: "/", Expires: time.Now().Add(-24 * time.Hour), MaxAge: -1})
}

func rememberedName(r *http.Request) string {
	c, err := r.Cookie(nameCookie)
	if err != nil {
		return ""
	}
	b, err := base64.RawURLEncoding.DecodeString(c.Value)
	if err != nil {
		return ""
	}
	field := strings.ToValidUTF8(string(b), "")
	if r := []rune(field); len(r) > maxNameField {
		field = string(r[:maxNameField])
	}
	return field
}

func composerName(remembered, typed string, kept bool) string {
	if kept && remembered != "" && tripcode.NameOf(remembered) == typed {
		return remembered
	}
	return typed
}
