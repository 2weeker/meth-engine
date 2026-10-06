package posterror

import "testing"

func TestCaptchaLockoutMessage(t *testing.T) {
	for _, c := range []struct {
		wait  string
		login bool
		want  string
	}{
		{"60", false, "Too many wrong captchas: Wait 60 seconds"},
		{"900", true, "Too many wrong captchas: the login is locked for 15 minutes"},
		{"", false, Message("captchalock")},
		{"<b>", true, "Too many wrong captchas: the login is locked for a while"},
	} {
		if got := CaptchaLockoutMessage(c.wait, c.login); got != c.want {
			t.Errorf("CaptchaLockoutMessage(%q, %v) = %q, want %q", c.wait, c.login, got, c.want)
		}
	}
	if !Known("captchalock") {
		t.Error("captchalock must be a known refusal")
	}
}

func TestBurstMessage(t *testing.T) {
	for wait, want := range map[string]string{
		"22":   "Wait 22 seconds",
		"1":    "Wait 1 second",
		"":     "You're posting quickly: wait a moment and try again",
		"junk": "You're posting quickly: wait a moment and try again",
	} {
		if got := BurstMessage(wait); got != want {
			t.Errorf("BurstMessage(%q) = %q, want %q", wait, got, want)
		}
	}
	if !Known("burst") {
		t.Error("burst should be a known code")
	}
}

func TestTagsMessage(t *testing.T) {
	for _, c := range []struct {
		long        bool
		max, length int
		want        string
	}{
		{false, 3, 24, "Too many tags: a post takes up to 3"},
		{false, 1, 24, "Too many tags: a post takes 1 tag"},
		{true, 3, 24, "A tag is too long: tags hold up to 24 characters"},
	} {
		if got := TagsMessage(c.long, c.max, c.length); got != c.want {
			t.Errorf("TagsMessage(%v, %d, %d) = %q, want %q", c.long, c.max, c.length, got, c.want)
		}
	}
	if !Known("tags") || !Known("taglong") {
		t.Error("both tag refusals are known codes")
	}
}
