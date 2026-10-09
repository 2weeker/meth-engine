package themes

import (
	"io/fs"
	"os"
	"sort"
	"strings"
	"testing"
)

func TestLookup(t *testing.T) {
	for in, want := range map[string]string{
		"ratwires": "ratwires", "meth": "meth", "cyb": "cyb", "coffee": "coffee",
		"main": "ratwires", "bm": "meth",
		"": "", "../x": "", "nope": "", "Coffee": "",
	} {
		got, ok := Lookup(in)
		if got != want || ok != (want != "") {
			t.Errorf("Lookup(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
	if _, ok := Lookup(Fallback); !ok {
		t.Errorf("Fallback %q is not a theme", Fallback)
	}
}

func TestResolve(t *testing.T) {
	for _, tc := range []struct{ value, fallback, want string }{
		{"cyb", "coffee", "cyb"}, {"bm", "coffee", "meth"}, {"", "ratwires", "ratwires"},
		{"nope", "macos", "macos"}, {"", "", "coffee"}, {"x", "y", "coffee"},
	} {
		if got := Resolve(tc.value, tc.fallback); got != tc.want {
			t.Errorf("Resolve(%q, %q) = %q, want %q", tc.value, tc.fallback, got, tc.want)
		}
	}
}

func TestSortedByLabel(t *testing.T) {
	labels := make([]string, len(All))
	for i, th := range All {
		labels[i] = strings.ToLower(th.Label)
	}
	if !sort.StringsAreSorted(labels) {
		t.Errorf("themes are not alphabetical: %v", labels)
	}
}

func TestEveryThemeHasAStylesheet(t *testing.T) {
	css := os.DirFS("../../view/static/css")
	for _, name := range Names() {
		if _, err := fs.Stat(css, name+".css"); err != nil {
			t.Errorf("theme %q: %v", name, err)
		}
	}
	files, _ := fs.Glob(css, "*.css")
	if len(files) != len(All) {
		t.Errorf("view/static/css has %d stylesheets, %d themes: %v", len(files), len(All), files)
	}
}
