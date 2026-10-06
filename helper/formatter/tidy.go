package formatter

import (
	"strings"
	"unicode"
)

func Tidy(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	blank := false
	for _, line := range lines {
		line = strings.TrimRightFunc(line, invisible)
		if strings.TrimLeftFunc(line, invisible) == "" {
			blank = len(out) > 0
			continue
		}
		if blank {
			out = append(out, "")
			blank = false
		}
		out = append(out, line)
	}

	return strings.Join(out, "\n")
}

func invisible(r rune) bool {
	if unicode.IsSpace(r) {
		return true
	}
	switch r {
	case 0x00AD,
		0x034F,
		0x115F, 0x1160,
		0x180E,
		0x200B, 0x200C, 0x200D,
		0x2060, 0x2061, 0x2062, 0x2063, 0x2064,
		0x2800,
		0x3164,
		0xFEFF,
		0xFFA0:
		return true
	}
	return false
}
