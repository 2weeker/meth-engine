package posterror

import "strconv"

var messages = map[string]string{
	"captcha":     "Incorrect or expired CAPTCHA",
	"empty":       "Message empty",
	"long":        "Message too long",
	"filter":      "You just posted cringe, you are going to lose subscribers",
	"missing":     "The post you're trying to reply to does not exist",
	"banned":      "This address is banned",
	"captchalock": "Too many wrong captchas: your address is locked out of posting for a while",
	"burst":       "You're posting quickly: wait a moment and try again",
	"modonly":     "Only moderators can post here",
	"tags":        "Too many tags",
	"taglong":     "A tag is too long",
	"invalid":     "Post rejected",
}

func Message(code string) string { return messages[code] }

func Known(code string) bool { _, ok := messages[code]; return ok }

const maxWait = 7 * 24 * 60 * 60

func CaptchaLockoutMessage(wait string, login bool) string {
	left, ok := timeLeft(wait)
	switch {
	case login && ok:
		return "Too many wrong captchas: the login is locked for " + left
	case login:
		return "Too many wrong captchas: the login is locked for a while"
	case ok:
		return "Too many wrong captchas: Wait " + left
	}
	return messages["captchalock"]
}

func BurstMessage(wait string) string {
	if left, ok := timeLeft(wait); ok {
		return "Wait " + left
	}
	return messages["burst"]
}

func timeLeft(wait string) (string, bool) {
	n, err := strconv.Atoi(wait)
	if err != nil || n < 1 || n > maxWait {
		return "", false
	}
	unit := func(n int, name string) string {
		if n == 1 {
			return "1 " + name
		}
		return strconv.Itoa(n) + " " + name + "s"
	}
	switch {
	case n < 120:
		return unit(n, "second"), true
	case n < 2*60*60:
		return unit((n+59)/60, "minute"), true
	default:
		return unit((n+3599)/3600, "hour"), true
	}
}

func TagsMessage(long bool, max, length int) string {
	if long {
		return "A tag is too long: tags hold up to " + strconv.Itoa(length) + " characters"
	}
	if max == 1 {
		return "Too many tags: a post takes 1 tag"
	}
	return "Too many tags: a post takes up to " + strconv.Itoa(max)
}
