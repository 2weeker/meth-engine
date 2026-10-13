package palette

import (
	"net/url"
	"strings"
	"testing"
)

func TestParseAndRoundTrip(t *testing.T) {
	form := url.Values{"text": {"#F2DFC6"}, "accent": {"#d99a5b"}, "greentext": {"#abc"}, "tile": {"coffee"}}
	p, err := Parse(form)
	if err != nil {
		t.Fatal(err)
	}
	if p.Colors["text"] != "#f2dfc6" || p.Colors["greentext"] != "#aabbcc" || p.Tile != "coffee" || len(p.Colors) != 3 {
		t.Errorf("parsed %+v", p)
	}
	enc := p.Encode()
	if strings.Contains(enc, "#") || !strings.Contains(enc, "text=f2dfc6") {
		t.Errorf("cookie value %q", enc)
	}
	back := Decode(enc)
	if back.Colors["text"] != "#f2dfc6" || back.Colors["accent"] != "#d99a5b" || back.Tile != "coffee" {
		t.Errorf("decoded %+v", back)
	}
	for _, bad := range []url.Values{
		{"text": {"red; } body { display:none"}},
		{"text": {"url(http://evil.example)"}},
		{"text": {"#12345"}},
		{"tile": {"../../etc/passwd"}},
		{"tile": {"evil"}},
	} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("%v accepted", bad)
		}
	}
	if !Decode("text=zzzzzz").Empty() || !Decode("%zz").Empty() || !Decode("").Empty() {
		t.Error("a bad cookie should decode to nothing")
	}
	if _, err := Parse(url.Values{"nonsense": {"x"}}); err != nil {
		t.Error("unknown fields are ignored")
	}
}

func TestImageURL(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"https://pics.example/tile.png", "https://pics.example/tile.png"},
		{"  https://pics.example/a%20b.png?x=1&y=2#f ", "https://pics.example/a%20b.png?x=1&y=2#f"},
		{"https://pics.example/a b.png", "https://pics.example/a%20b.png"},
		{`https://pics.example/a")}body{display:none}`, "https://pics.example/a%22%29%7Dbody%7Bdisplay:none%7D"},
		{"https://pics.example/a\\b.png", "https://pics.example/a%5Cb.png"},
		{"", ""},
	} {
		got, err := ImageURL(c.in)
		if err != nil || got != c.want {
			t.Errorf("ImageURL(%q) = %q, %v; want %q", c.in, got, err, c.want)
		}
		if strings.ContainsAny(got, "\"'()\\ \n") {
			t.Errorf("ImageURL(%q) = %q can break out of url(\"...\")", c.in, got)
		}
	}
	for _, bad := range []string{
		"http://pics.example/tile.png",
		"javascript:alert(1)",
		"data:image/png;base64,AAAA",
		"//pics.example/tile.png",
		"https:///tile.png",
		"https://user:pw@pics.example/tile.png",
		"https://pics.example/a'b.png",
		"https://pics.example/(a).png",
		"https://pics.example/" + strings.Repeat("a", MaxImageURL),
	} {
		if got, err := ImageURL(bad); err == nil {
			t.Errorf("ImageURL(%q) accepted as %q", bad, got)
		}
	}
	p, err := Parse(url.Values{"image": {"https://pics.example/t.png"}, "fit": {"cover"}, "tile": {"dark"}})
	if err != nil || p.Image != "https://pics.example/t.png" || p.Fit != "cover" {
		t.Fatalf("parsed %+v, %v", p, err)
	}
	back := Decode(p.Encode())
	if back.Image != p.Image || back.Fit != "cover" || back.Tile != "dark" {
		t.Errorf("round trip %+v", back)
	}
	if got := p.CSS(); got != `:root{--theme-background-image:url("https://pics.example/t.png");}body{background-size:cover;background-repeat:no-repeat;background-position:center;background-attachment:fixed;}` {
		t.Errorf("cover CSS: %s", got)
	}
	if p, _ := Parse(url.Values{"fit": {"cover"}}); p.Fit != "" || !p.Empty() {
		t.Errorf("fit without an image means nothing: %+v", p)
	}
	if _, err := Parse(url.Values{"image": {"https://x.example/"}, "fit": {"stretch"}}); err == nil {
		t.Error("an unknown fit is accepted")
	}
}

func TestCSS(t *testing.T) {
	p := Palette{Colors: map[string]string{"accent": "#d99a5b", "link": "#f8cd9f"}, Tile: "none"}
	want := ":root{--theme-link:#f8cd9f;--theme-accent:#d99a5b;--theme-accent-rgb:217,154,91;--theme-background-image:none;}"
	if got := p.CSS(); got != want {
		t.Errorf("CSS()\n got %s\nwant %s", got, want)
	}
	if got := (Palette{Tile: "dark"}).CSS(); got != ":root{--theme-background-image:url(/assets/dark.png);}" {
		t.Errorf("tile only: %s", got)
	}
	if (Palette{}).CSS() != "" {
		t.Error("an empty palette emits CSS")
	}
}

func TestExtract(t *testing.T) {
	css := []byte(`:root { --theme-text: #f6b3e5; --theme-greentext: lime; --theme-bluetext:aqua;
	--theme-background-image: url(/assets/bg.png); --theme-post-border: rgba(0,0,0,.2); }
	:root { --theme-text: #FFF; --theme-background-image: url(/assets/coffee.png) }`)
	p := Extract(css)
	if p.Colors["text"] != "#ffffff" || p.Colors["greentext"] != "#00ff00" || p.Colors["bluetext"] != "#00ffff" || p.Tile != "coffee" {
		t.Errorf("extracted %+v", p)
	}
	if _, ok := p.Colors["post-border"]; ok {
		t.Error("only editable fields are extracted")
	}
	if Extract([]byte(`:root{--theme-background-image:none}`)).Tile != "none" {
		t.Error("a theme with no tile")
	}
}
