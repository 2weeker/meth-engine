package formatter

import (
	"regexp"
	"strconv"
	"strings"
)

type Links struct {
	Away bool
}

var (
	urlRe   = regexp.MustCompile(`((?:\w+://)[\w./%\-:/=#?&]+)`)
	greenRe = regexp.MustCompile(`(?m)^&gt;(.*?)$`)
	blueRe  = regexp.MustCompile(`(?m)^&lt;(.*?)$`)
	escaper = strings.NewReplacer(">", "&gt;", "<", "&lt;", "+", "&#43;")

	linkRe = regexp.MustCompile(`&gt;&gt;([0-9]+)`)
)

func Escape(s string) string { return escaper.Replace(s) }

func Format(s string, l Links) string {
	e := Escape(Tidy(s))
	e = urlRe.ReplaceAllString(e, `<a href="${1}">${1}</a>`)
	e = wrapPairs(e, "[[", "]]", "<button>", "</button>")
	e = postLinks(e, l)
	e = greenRe.ReplaceAllString(e, `<span class="greentext">&gt;${1}</span>`)
	e = blueRe.ReplaceAllString(e, `<span class="bluetext">&lt;${1}</span>`)
	e = strings.ReplaceAll(e, "\n", "<br/>")
	e = wrapPairs(e, "`", "`", "<code>", "</code>")
	e = wrapPairs(e, "==", "==", `<span class="redtext">`, "</span>")
	e = wrapPairs(e, "%%", "%%", `<span class="spoiler">`, "</span>")
	e = wrapPairs(e, "$$", "$$", `<span class="shaketext">`, "</span>")
	e = wrapPairs(e, "$", "$", `<span class="rainbowtext">`, "</span>")
	e = wrapPairs(e, "^^", "^^", `<span class="threedtext">`, "</span>")
	e = wrapPairs(e, "!!", "!!", `<span class="glow">`, "</span>")
	e = wrapPairs(e, "**", "**", "<b>", "</b>")
	e = wrapPairs(e, "*", "*", "<i>", "</i>")

	return strings.ReplaceAll(e, `\`, "")
}

func postLinks(e string, l Links) string {
	return linkRe.ReplaceAllStringFunc(e, func(m string) string {
		return `<a href="` + postHref(l, linkRe.FindStringSubmatch(m)[1]) + `" class="quotelink">` + m + `</a>`
	})
}

func postHref(l Links, n string) string {

	if v, err := strconv.ParseInt(n, 10, 64); err == nil {
		n = strconv.FormatInt(v, 10)
	}
	if l.Away {
		return "/#post-" + n
	}
	return "#post-" + n
}

func wrapPairs(s, open, close, openTag, closeTag string) string {
	var b strings.Builder
	i := 0
	for i < len(s) {
		start := findUnescaped(s, open, i)
		if start < 0 {
			b.WriteString(s[i:])
			break
		}
		cstart := start + len(open)
		end := findUnescaped(s, close, cstart)
		if end >= 0 && strings.IndexByte(s[cstart:end], '\n') >= 0 {
			end = -1
		}
		if end < 0 {
			b.WriteString(s[i : start+1])
			i = start + 1
			continue
		}
		b.WriteString(s[i:start])
		b.WriteString(openTag)
		b.WriteString(s[cstart:end])
		b.WriteString(closeTag)
		i = end + len(close)
	}
	return b.String()
}

func findUnescaped(s, marker string, from int) int {
	for from <= len(s)-len(marker) {
		p := strings.Index(s[from:], marker)
		if p < 0 {
			return -1
		}
		p += from
		if p == 0 || s[p-1] != '\\' {
			return p
		}
		from = p + 1
	}
	return -1
}
