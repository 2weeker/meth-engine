package palette

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

type Field struct {
	Key, Label string
}

var Fields = []Field{
	{"text", "Text"},
	{"text-strong", "Strong text"},
	{"heading-text", "Heading text"},
	{"link", "Message link"},
	{"link-hover", "Message link, hovered"},
	{"greentext", "Greentext"},
	{"bluetext", "Bluetext"},
	{"ground", "Page background"},
	{"accent", "Accent"},
	{"frame", "Frame"},
	{"chrome", "Chrome"},
	{"bevel-light", "Bevel, lit edge"},
	{"bevel-dark", "Bevel, shaded edge"},
	{"tab-face", "Tab face"},
	{"tab-light", "Tab highlight"},
	{"field", "Field background"},
	{"input-text", "Field text"},
}

var withRGB = map[string]bool{"text": true, "accent": true, "chrome": true, "frame": true}

var Tiles = []string{"bg", "meth", "dark", "coffee", "macos", "angelic", "none"}

const TileKey = "tile"

type Palette struct {
	Colors map[string]string
	Tile   string
}

func (p Palette) Over(base Palette) Palette {
	out := Palette{Colors: map[string]string{}, Tile: p.Tile}
	for k, v := range p.Colors {
		if base.Colors[k] != v {
			out.Colors[k] = v
		}
	}
	if out.Tile == base.Tile {
		out.Tile = ""
	}
	return out
}

func (p Palette) Empty() bool { return len(p.Colors) == 0 && p.Tile == "" }

var hexRe = regexp.MustCompile(`^#[0-9a-f]{6}$`)

var shortHexRe = regexp.MustCompile(`^[0-9a-f]{3}$`)

func normalize(v string) (string, bool) {
	v = strings.ToLower(strings.TrimSpace(v))
	if hexRe.MatchString(v) {
		return v, true
	}
	if len(v) == 4 && v[0] == '#' {
		short := strings.ToLower(v[1:])
		if shortHexRe.MatchString(short) {
			return "#" + string([]byte{short[0], short[0], short[1], short[1], short[2], short[2]}), true
		}
	}
	if h, ok := named[v]; ok {
		return h, true
	}
	return "", false
}

var named = map[string]string{
	"black": "#000000", "white": "#ffffff", "red": "#ff0000", "lime": "#00ff00", "aqua": "#00ffff",
	"yellow": "#ffff00", "deeppink": "#ff1493", "lightpink": "#ffb6c1", "cyan": "#00ffff",
}

func known(key string) bool {
	for _, f := range Fields {
		if f.Key == key {
			return true
		}
	}
	return false
}

func tileOK(t string) bool {
	for _, x := range Tiles {
		if x == t {
			return true
		}
	}
	return false
}

func Parse(form url.Values) (Palette, error) {
	p := Palette{Colors: map[string]string{}}
	for _, f := range Fields {
		v := form.Get(f.Key)
		if v == "" {
			continue
		}
		h, ok := normalize(v)
		if !ok {
			return Palette{}, fmt.Errorf("%s: %q is not a colour like #d99a5b", f.Key, v)
		}
		p.Colors[f.Key] = h
	}
	if t := form.Get(TileKey); t != "" {
		if !tileOK(t) {
			return Palette{}, fmt.Errorf("tile %q is not one of %s", t, strings.Join(Tiles, ", "))
		}
		p.Tile = t
	}
	return p, nil
}

func (p Palette) Encode() string {
	v := url.Values{}
	for _, f := range Fields {
		if h, ok := p.Colors[f.Key]; ok {
			v.Set(f.Key, h[1:])
		}
	}
	if p.Tile != "" {
		v.Set(TileKey, p.Tile)
	}
	return v.Encode()
}

func Decode(s string) Palette {
	v, err := url.ParseQuery(s)
	if err != nil {
		return Palette{}
	}
	for _, f := range Fields {
		if h := v.Get(f.Key); h != "" {
			v.Set(f.Key, "#"+h)
		}
	}
	p, err := Parse(v)
	if err != nil {
		return Palette{}
	}
	return p
}

func (p Palette) CSS() string {
	if p.Empty() {
		return ""
	}
	var b strings.Builder
	b.WriteString(":root{")
	for _, f := range Fields {
		h, ok := p.Colors[f.Key]
		if !ok {
			continue
		}
		fmt.Fprintf(&b, "--theme-%s:%s;", f.Key, h)
		if withRGB[f.Key] {
			r, _ := strconv.ParseUint(h[1:3], 16, 8)
			g, _ := strconv.ParseUint(h[3:5], 16, 8)
			bl, _ := strconv.ParseUint(h[5:7], 16, 8)
			fmt.Fprintf(&b, "--theme-%s-rgb:%d,%d,%d;", f.Key, r, g, bl)
		}
	}
	switch {
	case p.Tile == "":
	case p.Tile == "none":
		b.WriteString("--theme-background-image:none;")
	default:
		fmt.Fprintf(&b, "--theme-background-image:url(/assets/%s.png);", p.Tile)
	}
	b.WriteString("}")
	return b.String()
}

var declRe = regexp.MustCompile(`--theme-([a-z-]+)\s*:\s*([^;}]+)[;}]`)

var tileRe = regexp.MustCompile(`url\(/assets/([a-z]+)\.png\)`)

func Extract(css []byte) Palette {
	p := Palette{Colors: map[string]string{}}
	for _, m := range declRe.FindAllSubmatch(css, -1) {
		key, val := string(m[1]), strings.TrimSpace(string(m[2]))
		switch {
		case key == "background-image":
			if val == "none" {
				p.Tile = "none"
			} else if t := tileRe.FindStringSubmatch(val); t != nil && tileOK(t[1]) {
				p.Tile = t[1]
			}
		case known(key):
			if h, ok := normalize(val); ok {
				p.Colors[key] = h
			}
		}
	}
	return p
}
