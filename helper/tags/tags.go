package tags

import (
	"errors"
	"strings"
	"unicode/utf8"

	"meth-enginev2/helper/slug"
)

var (
	ErrTooMany = errors.New("too many tags")

	ErrTooLong = errors.New("a tag is too long")
)

func Normalize(tag string) string {
	return slug.Make(strings.TrimLeft(strings.TrimSpace(tag), "#"))
}

func Parse(input string, max, length int) ([]string, error) {
	fields := strings.FieldsFunc(input, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r' })
	var out []string
	seen := map[string]bool{}
	for _, f := range fields {
		t := Normalize(f)
		if t == "" || seen[t] {
			continue
		}
		if utf8.RuneCountInString(t) > length {
			return nil, ErrTooLong
		}
		seen[t] = true
		out = append(out, t)
	}
	if len(out) > max {
		return nil, ErrTooMany
	}
	return out, nil
}
