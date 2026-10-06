package handler

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"net/url"
	"regexp"
)

const csrfCookie = "csrf_"

var tokenRe = regexp.MustCompile(`^[0-9a-f]{64}$`)

func newToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

type CSRFOptions struct {
	Secure bool
	Scheme func(*http.Request) string
}

func sameOrigin(o CSRFOptions, r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		if ref := r.Header.Get("Referer"); ref != "" {
			u, err := url.Parse(ref)
			if err != nil {
				return false
			}
			origin = u.Scheme + "://" + u.Host
		}
	}
	scheme := o.Scheme(r)
	if origin == "" {
		return scheme != "https"
	}
	return origin == scheme+"://"+r.Host
}

func CSRF(o CSRFOptions, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := ""
		if c, err := r.Cookie(csrfCookie); err == nil && tokenRe.MatchString(c.Value) {
			token = c.Value
		}
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			if token == "" && !Quiet(r.URL.Path) {
				token = newToken()
				http.SetCookie(w, &http.Cookie{Name: csrfCookie, Value: token, Path: "/", HttpOnly: true,
					SameSite: http.SameSiteLaxMode, Secure: o.Secure, MaxAge: 86400})
			}
		default:
			sent := r.PostFormValue("_csrf")
			if token == "" || !sameOrigin(o, r) || subtle.ConstantTimeCompare([]byte(sent), []byte(token)) != 1 {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, WithCSRF(r, token))
	})
}

type csrfKey struct{}

func WithCSRF(r *http.Request, token string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), csrfKey{}, token))
}

func CSRFToken(r *http.Request) string {
	t, _ := r.Context().Value(csrfKey{}).(string)
	return t
}
