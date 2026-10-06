package slug

import (
	"regexp"
	"strings"
)

var nonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

func Make(name string) string {
	return strings.Trim(nonAlnum.ReplaceAllString(strings.ToLower(strings.TrimSpace(name)), "-"), "-")
}
