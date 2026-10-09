package controller

import (
	"context"
	"net/http"
	"regexp"
	"strconv"

	"meth-enginev2/core/posterror"
	"meth-enginev2/handler"
	"meth-enginev2/helper/auth"
	"meth-enginev2/helper/tags"
	"meth-enginev2/model"
	"meth-enginev2/view"
)

func (a *App) stats(ctx context.Context) string {
	posts, identities, err := a.Store.Stats(ctx)
	if err != nil {
		return statsLine(0, 0)
	}
	return statsLine(posts, identities)
}

func statsLine(posts, identities int) string {
	return strconv.Itoa(posts) + " " + plural(posts, "post") + " made per hour with " +
		strconv.Itoa(identities) + " " + plural(identities, "identity")
}

func plural(n int, word string) string {
	if n == 1 {
		return word
	}
	if word == "identity" {
		return "identities"
	}
	return word + "s"
}

func (a *App) Index(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	var replyTo *int64
	if raw := r.PathValue("id"); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return handler.ErrNotFound
		}
		replyTo = &id
	}

	_, mod := a.modUser(r)
	page := a.page(r)
	if replyTo != nil {
		page.ReplyTo = strconv.FormatInt(*replyTo, 10)
	}
	if !mod && a.Captcha != nil {
		page.Captcha = a.issueCaptcha(postingForm, auth.AddressKey(a.ClientIP(r)))
	}

	posts, err := a.Store.Posts(ctx)
	if err != nil {
		return err
	}
	page.Threads = view.Tree(posts, replyTo, mod)
	q := r.URL.Query()
	limits, tagsOn := a.Cfg.Tags()
	if tagsOn {
		page.TagFilter = tags.Normalize(q.Get("tag"))
		if page.TagFilter != "" {
			page.Threads = view.Tagged(posts, page.TagFilter, replyTo, mod)
		}
		page.TagsHint = tagsHint(limits)
	}
	page.MaxChars = a.Cfg.MaxChars()
	page.Placeholder = a.placeholder()
	if msg := q.Get("msg"); msg != "" || q.Get("error") != "" {
		errText := posterror.Message(q.Get("error"))
		switch q.Get("error") {
		case "captchalock":
			errText = posterror.CaptchaLockoutMessage(q.Get("wait"), false)
		case "burst":
			errText = posterror.BurstMessage(q.Get("wait"))
		case "tags", "taglong":
			if tagsOn {
				errText = posterror.TagsMessage(q.Get("error") == "taglong", limits.Max, limits.Length)
			}
		}
		page.Composer = view.Composer{Open: true, Message: msg, Sage: q.Get("sage") == "true", Tags: q.Get("tags"), Error: errText}
	}
	page.Stats = a.stats(ctx)
	return a.html(w, r, view.Layout(page))
}

func (a *App) OldBoard(w http.ResponseWriter, r *http.Request) error {
	path := "/"
	if id := r.PathValue("id"); id != "" {
		if _, err := strconv.ParseInt(id, 10, 64); err == nil {
			path += id
		}
	}
	if q := r.URL.RawQuery; q != "" {
		path += "?" + q
	}
	http.Redirect(w, r, path, http.StatusMovedPermanently)
	return nil
}

var posterIDRe = regexp.MustCompile(`^[A-Za-z0-9_-]{8}$`)

func (a *App) History(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	id := r.PathValue("id")
	var posts []model.Post
	if posterIDRe.MatchString(id) {
		var err error
		if posts, err = a.Store.PostsByPosterID(ctx, id); err != nil {
			return err
		}
	} else {
		id = ""
	}
	_, mod := a.modUser(r)
	page := a.page(r)
	page.History, page.Threads = id, view.History(posts, mod)
	if id == "" {
		asked := []rune(r.PathValue("id"))
		page.History = string(asked[:min(len(asked), 16)])
	}
	page.Stats = strconv.Itoa(len(posts)) + " " + plural(len(posts), "post") + " by ID:" + page.History
	return a.html(w, r, view.Layout(page))
}
