package routes

import (
	"io/fs"
	"log"
	"mime"
	"net/http"
	"path"
	"strings"

	"meth-enginev2/controller"
	"meth-enginev2/handler"
	"meth-enginev2/helper/stylesheets"
)

const bodyLimit = 1 << 20

func cached(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "public, max-age=3600")
		next.ServeHTTP(w, r)
	})
}

func Build(a *controller.App, assets fs.FS) http.Handler {
	mux := http.NewServeMux()
	h := handler.HTTP

	mux.Handle("GET /healthz", h(a.Healthz))
	mux.Handle("GET /captcha/{file}", h(a.CaptchaImage))

	sheets, err := stylesheets.Build(assets, "static/css")
	if err != nil {
		log.Fatal(err)
	}
	mux.Handle("GET /css/{file}", h(func(w http.ResponseWriter, r *http.Request) error {
		s, ok := sheets[r.PathValue("file")]
		if !ok {
			return handler.ErrNotFound
		}
		w.Header().Set("Cache-Control", "public, max-age=3600")
		w.Header().Set("ETag", s.ETag)
		if r.Header.Get("If-None-Match") == s.ETag {
			w.WriteHeader(http.StatusNotModified)
			return nil
		}
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		_, err := w.Write(s.Body)
		return err
	}))
	for _, dir := range []string{"assets", "fonts"} {
		sub, err := fs.Sub(assets, "static/"+dir)
		if err != nil {
			log.Fatal(err)
		}
		mux.Handle("GET /"+dir+"/", cached(http.StripPrefix("/"+dir+"/", http.FileServerFS(sub))))
	}
	if logo := a.Cfg.Site.Banner.Logo; logo != "" {
		mux.Handle("GET "+a.Cfg.Site.LogoURL(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "public, max-age=3600")
			http.ServeFile(w, r, logo)
		}))
	}
	for _, file := range []string{"favicon.ico", "favicon.png", "favicon.gif", "apple-touch-icon.png", "robots.txt"} {
		mux.Handle("GET /"+file, h(func(w http.ResponseWriter, r *http.Request) error {
			b, err := fs.ReadFile(assets, "static/"+file)
			if err != nil {
				return handler.ErrNotFound
			}
			if ct := mime.TypeByExtension(path.Ext(file)); ct != "" {
				w.Header().Set("Content-Type", ct)
			}
			w.Header().Set("Cache-Control", "public, max-age=3600")
			_, err = w.Write(b)
			return err
		}))
	}

	mux.Handle("GET /mod", h(a.Panel))
	mux.Handle("GET /mod/login", h(a.LoginPage))
	mux.Handle("POST /mod/authenticate", h(a.Authenticate))
	mux.Handle("POST /mod/logout", h(a.Logout))
	mux.Handle("GET /mod/delete", h(a.DeleteScreen))
	mux.Handle("GET /mod/filter", h(a.FilterScreen))
	mux.Handle("GET /mod/user", h(a.UserScreen))
	mux.Handle("GET /mod/ban", h(a.BanScreen))
	mux.Handle("GET /mod/tags", h(a.TagScreen))

	mux.Handle("POST /post/create/{id}", h(a.CreatePost))
	mux.Handle("POST /post/create", h(a.CreatePost))
	mux.Handle("POST /post/delete", h(a.DeletePost))
	mux.Handle("POST /filter/create", h(a.FilterCreate))
	mux.Handle("POST /filter/delete", h(a.FilterDelete))
	mux.Handle("POST /user/create", h(a.UserCreate))
	mux.Handle("POST /user/delete", h(a.UserDelete))
	mux.Handle("POST /tags/delete", h(a.TagDelete))
	mux.Handle("POST /ban/create", h(a.BanCreate))
	mux.Handle("POST /ban/delete", h(a.BanDelete))

	mux.Handle("GET /style/{name}", h(a.SetTheme))
	mux.Handle("GET /id/{id}", h(a.History))
	mux.Handle("GET /b/{board}", h(a.OldBoard))
	mux.Handle("GET /b/{board}/{id}", h(a.OldBoard))
	mux.Handle("GET /overboard", h(a.OldBoard))
	mux.Handle("GET /{id}", h(a.Index))
	mux.Handle("GET /{$}", h(a.Index))

	guarded := handler.CSRF(handler.CSRFOptions{Secure: a.Cfg.Env == "production", Scheme: a.Scheme}, mux)
	var app http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "script-src 'none'; object-src 'none'; base-uri 'self'")
		r.Body = http.MaxBytesReader(w, r.Body, bodyLimit)
		guarded.ServeHTTP(w, r)
	})
	return handler.Requests(a.Cfg.Log.SlowRequest, a.ClientIP)(handler.Recover(app))
}
