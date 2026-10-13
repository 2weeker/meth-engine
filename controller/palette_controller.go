package controller

import (
	"net/http"
	"net/url"
	"time"

	"meth-enginev2/helper/palette"
	"meth-enginev2/view"
)

const paletteCookie = "palette"

const paletteTTL = 365 * 24 * time.Hour

func (a *App) customPalette(r *http.Request) palette.Palette {
	c, err := r.Cookie(paletteCookie)
	if err != nil {
		return palette.Palette{}
	}
	return palette.Decode(c.Value)
}

func (a *App) editor(r *http.Request) view.Editor {
	q := r.URL.Query()
	custom := a.customPalette(r)
	base := a.Palettes[a.theme(r)]
	e := view.Editor{ReturnTo: returnPath(q.Get("return_to")), Active: !custom.Empty(), Tiles: palette.Tiles, Invalid: q.Get("error") != ""}
	for _, f := range palette.Fields {
		v, ok := custom.Colors[f.Key]
		if !ok {
			v, ok = base.Colors[f.Key]
		}
		if !ok {
			v = "#000000"
		}
		e.Fields = append(e.Fields, view.EditorField{Key: f.Key, Label: f.Label, Value: v})
	}
	e.Tile = custom.Tile
	if e.Tile == "" {
		e.Tile = base.Tile
	}
	if e.Tile == "" {
		e.Tile = "bg"
	}
	e.Image, e.Fit = custom.Image, custom.Fit
	return e
}

func (a *App) PaletteEditor(w http.ResponseWriter, r *http.Request) error {
	return a.html(w, r, view.ThemeEditor(a.page(r), a.editor(r)))
}

func (a *App) SetPalette(w http.ResponseWriter, r *http.Request) error {
	back := returnPath(r.PostFormValue("return_to"))
	p, err := palette.Parse(r.PostForm)
	if err != nil {
		return redirect(w, r, "/theme/custom?error=1&return_to="+url.QueryEscape(back))
	}
	p = p.Over(a.Palettes[a.theme(r)])
	if p.Empty() {
		forgetPalette(w)
	} else {
		a.setCookie(w, paletteCookie, p.Encode(), paletteTTL)
	}
	return redirect(w, r, back)
}

func (a *App) ResetPalette(w http.ResponseWriter, r *http.Request) error {
	forgetPalette(w)
	return redirect(w, r, returnPath(r.PostFormValue("return_to")))
}

func forgetPalette(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: paletteCookie, Value: "", Path: "/", Expires: time.Now().Add(-24 * time.Hour), MaxAge: -1})
}
