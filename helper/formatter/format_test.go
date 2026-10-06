package formatter

import "testing"

func TestFormat(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"plain", "hello", "hello"},
		{"escape only > < +", "a > b < c + d", "a &gt; b &lt; c &#43; d"},
		{"ampersand and quotes untouched", `a & "b"`, `a & "b"`},
		{"newline", "one\ntwo", "one<br/>two"},
		{"greentext keeps marker, per line", ">be me\n>ok", `<span class="greentext">&gt;be me</span><br/><span class="greentext">&gt;ok</span>`},
		{"bluetext", "<blue", `<span class="bluetext">&lt;blue</span>`},
		{"a line starting with a tag becomes bluetext", "<script>", `<span class="bluetext">&lt;script&gt;</span>`},
		{"bold and italic", "**b** *i*", "<b>b</b> <i>i</i>"},
		{"doubled forms win", "$$s$$ $r$", `<span class="shaketext">s</span> <span class="rainbowtext">r</span>`},
		{"code does not protect its contents (original bug)", "`**x**`", "<code><b>x</b></code>"},
		{"red spoiler 3d glow", "==r== %%s%% ^^t^^ !!g!!", `<span class="redtext">r</span> <span class="spoiler">s</span> <span class="threedtext">t</span> <span class="glow">g</span>`},
		{"button", "[[go]]", "<button>go</button>"},
		{"button cannot cross a line", "[[a\nb]]", "[[a<br/>b]]"},
		{"url with query and ampersand", "see http://x.y/a?b=1&c=2 ok", `see <a href="http://x.y/a?b=1&c=2">http://x.y/a?b=1&c=2</a> ok`},
		{"backslashes are stripped", `a\\b`, "ab"},

		{"escaped bold still italicises (original behaviour)", `\**not**`, "*<i>not</i>*"},

		{"unclosed double marker becomes an empty italic (original behaviour)", "**open", "<i></i>open"},
		{"trailing backslash escapes the closer", "`a\\`", "`a`"},
	}
	for _, c := range cases {
		if got := Format(c.in, Links{}); got != c.want {
			t.Errorf("%s:\n  in   %q\n  want %q\n  got  %q", c.name, c.in, c.want, got)
		}
	}
}

func TestPostLinks(t *testing.T) {
	onPage := Links{}
	away := Links{Away: true}
	cases := []struct {
		name string
		l    Links
		in   string
		want string
	}{
		{"on the main page: a jump down the page", onPage, ">>12",
			`<a href="#post-12" class="quotelink">&gt;&gt;12</a>`},
		{"anywhere else: the main page at the post", away, ">>12",
			`<a href="/#post-12" class="quotelink">&gt;&gt;12</a>`},
		{"the old board form is text now", onPage, ">>>/spam/45",
			`<span class="greentext">&gt;&gt;&gt;/spam/45</span>`},
		{"a line opening with a link is not greentext", onPage, ">>12 agreed\n>be me",
			`<a href="#post-12" class="quotelink">&gt;&gt;12</a> agreed<br/><span class="greentext">&gt;be me</span>`},
		{"links inside greentext stay links", onPage, "> told >>12 so",
			`<span class="greentext">&gt; told <a href="#post-12" class="quotelink">&gt;&gt;12</a> so</span>`},
		{"several on a line, mid-sentence", onPage, "a>>1 b >>2",
			`a<a href="#post-1" class="quotelink">&gt;&gt;1</a> b <a href="#post-2" class="quotelink">&gt;&gt;2</a>`},
		{"other markup wraps a link", onPage, "**>>12**",
			`<b><a href="#post-12" class="quotelink">&gt;&gt;12</a></b>`},
		{"leading zeros name the real anchor", onPage, ">>007",
			`<a href="#post-7" class="quotelink">&gt;&gt;007</a>`},
		{">> with no number is text", onPage, "a >> b",
			`a &gt;&gt; b`},
	}
	for _, c := range cases {
		if got := Format(c.in, c.l); got != c.want {
			t.Errorf("%s:\n  in   %q\n  want %q\n  got  %q", c.name, c.in, c.want, got)
		}
	}
}

func TestTidy(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"nothing to do", "one\ntwo", "one\ntwo"},
		{"top and bottom", "\n\n  \nhello\n\n\n", "hello"},
		{"a run between paragraphs becomes one blank line", "a\n\n\n\n\nb", "a\n\nb"},
		{"one blank line is kept", "a\n\nb", "a\n\nb"},
		{"lines of spaces and tabs are blank", "a\n \t \n   \nb", "a\n\nb"},
		{"invisible characters are blank", "a\n\u200b\n\u2800\u3164\n\ufeff\nb", "a\n\nb"},
		{"trailing spaces go, indentation stays", "  indented   \nnext\t", "  indented\nnext"},
		{"windows line endings", "a\r\n\r\n\r\nb\r\n", "a\n\nb"},
		{"only blank space is empty", "\n \u200b\n\u2800\n", ""},
		{"the example from the report", "\n\n\nno text written here, just whitespace \n\n\nactual text test post\n\n\n\n\nactual text test post\n\n\n\n\nhitting enter here\n\n\n",
			"no text written here, just whitespace\n\nactual text test post\n\nactual text test post\n\nhitting enter here"},
	}
	for _, c := range cases {
		if got := Tidy(c.in); got != c.want {
			t.Errorf("%s:\n  in   %q\n  want %q\n  got  %q", c.name, c.in, c.want, got)
		}
	}

	if got := Format("\n\nhi\n\n\n\nthere\n\n", Links{}); got != "hi<br/><br/>there" {
		t.Errorf("Format did not tidy: %q", got)
	}
}
