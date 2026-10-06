package config

import (
	"fmt"
	"io"
)

func (c Config) Report(w io.Writer) {
	captchaState := "off"
	if c.CaptchaEnabled {
		captchaState = "on"
	}
	fmt.Fprintf(w, "configuration ok: %q, env %s, captcha %s\n", c.Site.Title, c.Env, captchaState)
	if pl := c.PostLimit(); pl.Pause() > 0 {
		fmt.Fprintf(w, "  post limit: %s within %s, then a %s wait\n", posts(pl.Count()), pl.Window(), pl.Pause())
	} else {
		fmt.Fprintln(w, "  post limit: none")
	}
	if c.CaptchaEnabled {
		for _, side := range []struct {
			name string
			l    Lockout
		}{{"posting", c.Site.CaptchaLockout.Posting}, {"login", c.Site.CaptchaLockout.Login}} {
			if side.l.Duration() > 0 {
				fmt.Fprintf(w, "  captcha lockout, %s: %d wrong captchas lock out for %s\n", side.name, side.l.Limit(), side.l.Duration())
			} else {
				fmt.Fprintf(w, "  captcha lockout, %s: off\n", side.name)
			}
		}
	}
	fmt.Fprintf(w, "  post length: %d characters\n", c.MaxChars())
	if l, on := c.Tags(); on {
		fmt.Fprintf(w, "  tags: up to %d of %d characters\n", l.Max, l.Length)
	} else {
		fmt.Fprintln(w, "  tags: off")
	}
	fmt.Fprintf(w, "  posts kept: %d, then the oldest is purged\n", c.Site.PostCap())
}

func (c Config) Protection() string {
	limit := "no post limit"
	if pl := c.PostLimit(); pl.Pause() > 0 {
		limit = fmt.Sprintf("%s within %s then %s", posts(pl.Count()), pl.Window(), pl.Pause())
	}
	if !c.CaptchaEnabled {
		return "captcha off, " + limit
	}
	return "captcha on, " + limit
}

func posts(n int) string {
	if n == 1 {
		return "1 post"
	}
	return fmt.Sprintf("%d posts", n)
}
