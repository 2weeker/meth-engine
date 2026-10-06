package tags

import (
	"errors"
	"slices"
	"testing"
)

func TestParse(t *testing.T) {
	for _, c := range []struct {
		in   string
		want []string
		err  error
	}{
		{"", nil, nil},
		{"   ", nil, nil},
		{"music", []string{"music"}, nil},
		{"Music  #Lo-Fi, vinyl", []string{"music", "lo-fi", "vinyl"}, nil},
		{"music,music MUSIC", []string{"music"}, nil},
		{"New Wave", []string{"new", "wave"}, nil},
		{"rock&roll", []string{"rock-roll"}, nil},
		{"### !!!", nil, nil},
		{"a b c d", nil, ErrTooMany},
		{"abcdefghijk", nil, ErrTooLong},
		{"a,a,a,a,b", []string{"a", "b"}, nil},
	} {
		got, err := Parse(c.in, 3, 10)
		if !errors.Is(err, c.err) || !slices.Equal(got, c.want) {
			t.Errorf("Parse(%q) = %q, %v; want %q, %v", c.in, got, err, c.want, c.err)
		}
	}
	if Normalize("#Lo-Fi") != "lo-fi" || Normalize("!!!") != "" {
		t.Error("Normalize")
	}
}
