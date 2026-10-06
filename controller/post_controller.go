package controller

import (
	"errors"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"meth-enginev2/helper/auth"
	"meth-enginev2/helper/formatter"
	"meth-enginev2/helper/posterid"
	"meth-enginev2/helper/tags"
	"meth-enginev2/model"
	"meth-enginev2/view"
)

const HoneypotField = view.HoneypotField

func filterBanReason(f model.Filter) string {
	reason := "filter #" + strconv.FormatInt(f.ID, 10)
	if f.Note != "" {
		reason += ": " + f.Note
	}
	return reason
}

func banExpiry(days int) *time.Time {
	if days <= 0 {
		return nil
	}
	t := time.Now().Add(time.Duration(days) * 24 * time.Hour)
	return &t
}

func (a *App) CreatePost(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	ip := a.ClientIP(r)
	msg := formatter.Tidy(r.PostFormValue("msg"))
	var parent *int64
	if v := r.PostFormValue("parent"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			parent = &id
		}
	}
	sage := parent != nil && r.PostFormValue("sage") == "true"
	tagLimits, tagged := a.Cfg.Tags()
	tagsTyped := ""
	if tagged {
		tagsTyped = strings.TrimSpace(r.PostFormValue("tags"))
	}

	refuse := func(code string, extra ...string) error {
		q := url.Values{"msg": {msg}, "error": {code}}
		for i := 0; i+1 < len(extra); i += 2 {
			q.Set(extra[i], extra[i+1])
		}
		if sage {
			q.Set("sage", "true")
		}
		if tagsTyped != "" {
			q.Set("tags", tagsTyped)
		}
		path := "/"
		if parent != nil {
			path += strconv.FormatInt(*parent, 10)
		}
		return redirect(w, r, path+"?"+q.Encode())
	}

	if r.PostFormValue(HoneypotField) != "" {
		slog.Info("honeypot filled; post refused", "ip", ip)
		return refuse("invalid")
	}

	_, isMod := a.modUser(r)
	limitBursts := !isMod
	if limitBursts {
		if wait, waiting := a.Bursts.Waiting(auth.AddressKey(ip)); waiting {
			return refuse("burst", "wait", strconv.Itoa(int(math.Ceil(wait.Seconds()))))
		}
	}
	if !isMod {
		switch res := a.checkCaptcha(r, postingForm, auth.AddressKey(ip), a.PostCaptchaFails); {
		case res.Lockout > 0:
			return refuse("captchalock", "wait", strconv.Itoa(int(math.Ceil(res.Lockout.Seconds()))))
		case !res.Passed:
			return refuse("captcha")
		}
	}
	switch {
	case msg == "":
		return refuse("empty")
	case utf8.RuneCountInString(msg) > a.Cfg.MaxChars():
		return refuse("long")
	}
	postTags, err := tags.Parse(tagsTyped, tagLimits.Max, tagLimits.Length)
	switch {
	case errors.Is(err, tags.ErrTooMany):
		return refuse("tags")
	case errors.Is(err, tags.ErrTooLong):
		return refuse("taglong")
	}
	if banned, err := a.Store.Banned(ctx, ip); err != nil {
		return err
	} else if banned {
		return refuse("banned")
	}
	filtered, err := a.Store.ApplyFilters(ctx, msg)
	if err != nil {
		return err
	}
	switch filtered.Action {
	case model.FilterBan:
		if isMod {
			return refuse("filter")
		}
		if err := a.Store.CreateBan(ctx, ip, filterBanReason(filtered.By), "filter", banExpiry(filtered.BanDays)); err != nil {
			return err
		}
		slog.Info("filter banned an address", "filter", filtered.By.ID, "ip", ip, "days", filtered.BanDays)
		return refuse("banned")
	case model.FilterReject:
		return refuse("filter")
	case model.FilterReplace:
		filtered.Message = formatter.Tidy(filtered.Message)
		switch {
		case filtered.Message == "":
			return refuse("empty")
		case utf8.RuneCountInString(filtered.Message) > a.Cfg.MaxChars():
			return refuse("long")
		}
	}

	if len(postTags) > 0 {
		ft, err := a.Store.ApplyFilters(ctx, strings.Join(postTags, " "))
		if err != nil {
			return err
		}
		switch ft.Action {
		case model.FilterBan:
			if isMod {
				return refuse("filter")
			}
			if err := a.Store.CreateBan(ctx, ip, filterBanReason(ft.By), "filter", banExpiry(ft.BanDays)); err != nil {
				return err
			}
			slog.Info("filter banned an address", "filter", ft.By.ID, "ip", ip, "days", ft.BanDays, "in", "tags")
			return refuse("banned")
		case model.FilterReject:
			return refuse("filter")
		case model.FilterReplace:
			if postTags, err = tags.Parse(ft.Message, tagLimits.Max, tagLimits.Length); err != nil {
				return refuse("taglong")
			}
		}
	}

	np := model.NewPost{ParentID: parent, Message: filtered.Message, Sage: sage, IP: ip,
		PosterID: posterid.For(a.Cfg.PosterIDSecret, ip, time.Now()), MaxPosts: a.Cfg.Site.PostCap(), Tags: postTags}
	id, err := a.Store.CreatePost(ctx, np)
	if errors.Is(err, model.ErrMissingParent) {
		return refuse("missing")
	}
	if err != nil {
		return err
	}
	if limitBursts {
		a.Bursts.Posted(auth.AddressKey(ip))
	}
	n := strconv.FormatInt(id, 10)
	return redirect(w, r, "/"+n+"#reply-"+n)
}

func (a *App) DeletePost(w http.ResponseWriter, r *http.Request) error {
	if _, err := a.guard(w, r); err != nil {
		return err
	}
	p := a.modPage(r, "delete")
	id, err := strconv.ParseInt(r.PostFormValue("post_id"), 10, 64)
	if err != nil {
		p.Status = "Choose a post"
		return a.html(w, r, view.ModLayout(p))
	}
	switch err := a.Store.DeletePost(r.Context(), id); {
	case errors.Is(err, model.ErrNotFound):
		p.Status = "No such post"
	case err != nil:
		return err
	default:
		p.Status = "Post " + strconv.FormatInt(id, 10) + " deleted"
	}
	return a.html(w, r, view.ModLayout(p))
}
