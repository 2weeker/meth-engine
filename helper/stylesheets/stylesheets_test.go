package stylesheets

import (
	"os"
	"strings"
	"testing"
	"testing/fstest"
)

func TestBuildInlinesImports(t *testing.T) {
	fsys := fstest.MapFS{
		"css/main.css":          {Data: []byte("/* main */\n@import \"./main/a.css\";\n@import url(\"main/b.css\");\n")},
		"css/main/a.css":        {Data: []byte(".a {}\n")},
		"css/main/b.css":        {Data: []byte("@import \"./deeper/c.css\";\n.b {}\n")},
		"css/main/deeper/c.css": {Data: []byte(".c {}\n")},
		"css/theme.css":         {Data: []byte("/* theme */\n@import \"./main.css\";\n\n.theme {}\n")},
	}
	sheets, err := Build(fsys, "css")
	if err != nil {
		t.Fatal(err)
	}

	if len(sheets) != 2 {
		t.Fatalf("want main.css and theme.css, got %v", keys(sheets))
	}
	if got, want := string(sheets["main.css"].Body), "/* main */\n.a {}\n.c {}\n.b {}\n"; got != want {
		t.Errorf("main.css:\n%q\nwant\n%q", got, want)
	}
	if got, want := string(sheets["theme.css"].Body), "/* theme */\n/* main */\n.a {}\n.c {}\n.b {}\n\n.theme {}\n"; got != want {
		t.Errorf("theme.css:\n%q\nwant\n%q", got, want)
	}
	if sheets["theme.css"].ETag == "" || sheets["theme.css"].ETag == sheets["main.css"].ETag {
		t.Error("each sheet needs its own ETag")
	}
}

func TestBuildErrors(t *testing.T) {
	for name, files := range map[string]fstest.MapFS{
		"missing file":       {"css/main.css": {Data: []byte("@import \"./nope.css\";\n")}},
		"import cycle":       {"css/main.css": {Data: []byte("@import \"./a/x.css\";\n")}, "css/a/x.css": {Data: []byte("@import \"../main.css\";\n")}},
		"escapes the folder": {"css/main.css": {Data: []byte("@import \"../secret.css\";\n")}, "secret.css": {Data: []byte("x")}},
		"remote import":      {"css/main.css": {Data: []byte("@import \"https://evil.test/x.css\";\n")}},
		"media-qualified":    {"css/main.css": {Data: []byte("@import \"./a.css\" screen;\n")}, "css/a.css": {Data: []byte("x")}},
	} {
		if _, err := Build(files, "css"); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func TestRealStylesheets(t *testing.T) {
	sheets, err := Build(os.DirFS("../../view"), "static/css")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"ratwires.css", "coffee.css"} {
		s, ok := sheets[name]
		if !ok {
			t.Errorf("%s missing", name)
			continue
		}

		if rest := commentRe.ReplaceAllString(string(s.Body), ""); strings.Contains(rest, "@import") {
			t.Errorf("%s still has an @import", name)
		}
	}
}

func keys(m map[string]Sheet) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}
