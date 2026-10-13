package tripcode

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/encoding/japanese"
)

const MaxName = 32

const Mark = "◆"

func Parse(field string) (name, trip string) {
	before, secret, _ := strings.Cut(field, "#")
	return CleanName(before), Make(secret)
}

func NameOf(field string) string {
	before, _, _ := strings.Cut(field, "#")
	return CleanName(before)
}

func Make(secret string) string {
	if secret == "" {
		return ""
	}
	if r := []rune(secret); len(r) > 8 {
		secret = string(r[:8])
	}
	key := shiftJIS(strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(secret))
	salt := []byte((string(key) + "H.")[1:3])
	for i, c := range salt {
		switch {
		case c < '.' || c > 'z':
			salt[i] = '.'
		case strings.IndexByte(":;<=>?@[\\]^_`", c) >= 0:
			salt[i] = "ABCDEFGabcdef"[strings.IndexByte(":;<=>?@[\\]^_`", c)]
		}
	}
	h := Crypt(key, string(salt))
	return h[len(h)-10:]
}

func shiftJIS(s string) []byte {
	enc := japanese.ShiftJIS.NewEncoder()
	var out []byte
	for _, r := range s {
		b, err := enc.Bytes([]byte(string(r)))
		if err != nil || r == utf8.RuneError {
			b = []byte{'?'}
		}
		out = append(out, b...)
	}
	return out
}

func CleanName(s string) string {
	s = strings.ToValidUTF8(s, "")
	s = strings.Join(strings.FieldsFunc(s, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsControl(r) || unicode.Is(unicode.Cf, r)
	}), " ")
	s = strings.ReplaceAll(s, Mark, "◇")
	if r := []rune(s); len(r) > MaxName {
		s = strings.TrimSpace(string(r[:MaxName]))
	}
	return s
}
