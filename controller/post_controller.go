package controller

import (
	"context"
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
	"meth-enginev2/helper/tripcode"
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

func (a *App) screen(ctx context.Context, ip string, isMod bool, text, where string) (out, code string, err error) {
	res, err := a.Store.ApplyFilters(ctx, text)
	if err != nil {
		return text, "", err
	}
	switch res.Action {
	case model.FilterBan:
		if isMod {
			return text, "filter", nil
		}
		if err := a.Store.CreateBan(ctx, ip, filterBanReason(res.By), "filter", banExpiry(res.BanDays)); err != nil {
			return text, "", err
		}
		slog.Info("filter banned an address", "filter", res.By.ID, "ip", ip, "days", res.BanDays, "in", where)
		return text, "banned", nil
	case model.FilterReject:
		return text, "filter", nil
	}
	return res.Message, "", nil
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
	var name, trip, nameField string
	if a.Cfg.Tripcodes() {
		nameField = r.PostFormValue("name")
		name, trip = tripcode.Parse(nameField)
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
		if name != "" {
			q.Set("name", name)
			if nameField == rememberedName(r) {
				q.Set("kept", "1")
			}
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
	case name != "" && trip == "":
		return refuse("tripcode")
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
	message, code, err := a.screen(ctx, ip, isMod, msg, "message")
	switch {
	case err != nil:
		return err
	case code != "":
		return refuse(code)
	}
	if message != msg {
		message = formatter.Tidy(message)
		switch {
		case message == "":
			return refuse("empty")
		case utf8.RuneCountInString(message) > a.Cfg.MaxChars():
			return refuse("long")
		}
	}

	if len(postTags) > 0 {
		out, code, err := a.screen(ctx, ip, isMod, strings.Join(postTags, " "), "tags")
		switch {
		case err != nil:
			return err
		case code != "":
			return refuse(code)
		}
		if postTags, err = tags.Parse(out, tagLimits.Max, tagLimits.Length); err != nil {
			return refuse("taglong")
		}
	}
	if name != "" {
		out, code, err := a.screen(ctx, ip, isMod, name, "name")
		switch {
		case err != nil:
			return err
		case code != "":
			return refuse(code)
		}
		name = tripcode.CleanName(out)
	}

	np := model.NewPost{ParentID: parent, Message: message, Sage: sage, IP: ip,
		PosterID: posterid.For(a.Cfg.PosterIDSecret, ip, time.Now()), MaxPosts: a.Cfg.Site.PostCap(), Tags: postTags,
		Name: name, Tripcode: trip}
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
	if a.Cfg.Tripcodes() {
		switch {
		case trip != "":
			a.rememberName(w, nameField)
		case strings.TrimSpace(nameField) == "":
			forgetName(w)
		}
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
