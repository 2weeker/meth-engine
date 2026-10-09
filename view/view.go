package view

import (
	"embed"
	"html/template"
	"net/url"
	"slices"
	"strconv"
	"time"

	"meth-enginev2/config"
	"meth-enginev2/helper/dubs"
	"meth-enginev2/helper/formatter"
	"meth-enginev2/helper/themes"
	"meth-enginev2/model"
)

//go:embed static/css static/fonts/*.woff2 static/assets static/favicon.ico static/favicon.png static/favicon.gif static/apple-touch-icon.png static/robots.txt
var Static embed.FS

const HoneypotField = "website"

type PostView struct {
	ID         int64
	ParentID   *int64
	Message    template.HTML
	Sage       bool
	Time       string
	Dubs       string
	Selected   bool
	Mod        bool
	IP         string
	PosterID   string
	IDLink     bool
	ParentHref string
	ReplyHref  string
	Children   []*PostView
	Tags       []TagLink
}

func (v *PostView) IDString() string { return strconv.FormatInt(v.ID, 10) }

func (v *PostView) ParentString() string {
	if v.ParentID == nil {
		return ""
	}
	return strconv.FormatInt(*v.ParentID, 10)
}

type TagLink struct {
	Name, Href string
	Current    bool
}

type Composer struct {
	Open    bool
	Message string
	Sage    bool
	Tags    string
	Error   string
}

type Theme struct {
	Name, Label string
	Current     bool
}

func Stylesheet(theme string) string {
	return "/css/" + themes.Resolve(theme, "") + ".css"
}

func ThemesFor(current string) []Theme {
	out := make([]Theme, len(themes.All))
	for i, t := range themes.All {
		out[i] = Theme{Name: t.Name, Label: t.Label, Current: t.Name == current}
	}
	return out
}

type SiteTag struct {
	Name, Href string
	Posts      int
}

type Page struct {
	Site        config.Site
	SiteTags    []SiteTag
	Theme       string
	Themes      []Theme
	ReturnTo    string
	BaseURL     string
	History     string
	ReplyTo     string
	Threads     []*PostView
	Stats       string
	Composer    Composer
	Captcha     *Captcha
	CSRF        string
	MaxChars    int
	Placeholder string
	TagFilter   string
	TagsHint    string
}

type ModPage struct {
	Site          config.Site
	Theme         string
	Screen        string
	CSRF          string
	ID, IP        string
	Status        string
	Failed        bool
	Wait          string
	Captcha       *Captcha
	CaptchaFailed bool
	CaptchaLocked string
	Filters       []model.Filter
	FilterForm    FilterForm
	Bans          []model.Ban
	Users         []model.User
	Tags          []model.TagCount
	TagsOn        bool
}

type Captcha struct {
	ID            string
	Width, Height int
}

type FilterForm struct {
	Regex, Action, Replacement, BanDays, Note string
}

func FormatTime(t time.Time) string { return t.UTC().Format("2006-01-02 15:04:05 UTC") }

var ShowTags bool

func Tree(posts []model.Post, selected *int64, mod bool) []*PostView {
	return tree(posts, selected, mod, false, "")
}

func tree(posts []model.Post, selected *int64, mod, away bool, current string) []*PostView {
	links := formatter.Links{Away: away}
	byID := map[int64]*PostView{}
	order := make([]*PostView, 0, len(posts))
	for i := range posts {
		p := posts[i]
		id := strconv.FormatInt(p.ID, 10)
		v := &PostView{ID: p.ID, ParentID: p.ParentID, Message: template.HTML(formatter.Format(p.Message, links)),
			Sage: p.Sage && p.ParentID != nil, Time: FormatTime(p.CreatedAt), Mod: mod, IP: p.IPAddress, PosterID: p.PosterID, IDLink: true,
			Selected:  selected != nil && *selected == p.ID,
			ReplyHref: "/" + id + "#reply-" + id}
		if n, _ := dubs.Check(p.ID); n > 1 {
			v.Dubs = "(x" + strconv.Itoa(n) + ")"
		}
		if ShowTags {
			for _, t := range p.Tags {
				l := TagLink{Name: t, Href: TagHref(t), Current: t == current}
				if l.Current {
					l.Href = "/"
				}
				v.Tags = append(v.Tags, l)
			}
		}
		if p.ParentID != nil {
			v.ParentHref = "#post-" + strconv.FormatInt(*p.ParentID, 10)
		}
		byID[p.ID] = v
		order = append(order, v)
	}
	var roots []*PostView
	for _, v := range order {
		if v.ParentID != nil {
			if parent, ok := byID[*v.ParentID]; ok {
				parent.Children = append(parent.Children, v)
				continue
			}
		}
		roots = append(roots, v)
	}
	return roots
}

func History(posts []model.Post, mod bool) []*PostView {
	views := make([]*PostView, 0, len(posts))
	for _, p := range posts {
		v := tree([]model.Post{p}, nil, mod, true, "")[0]
		v.IDLink = false
		if p.ParentID != nil {
			v.ParentHref = "/#post-" + strconv.FormatInt(*p.ParentID, 10)
		}
		views = append(views, v)
	}
	return views
}

func TagHref(tag string) string {
	return "/?tag=" + url.QueryEscape(tag)
}

func Tagged(posts []model.Post, tag string, selected *int64, mod bool) []*PostView {
	var keep []model.Post
	kept := map[int64]bool{}
	for _, p := range posts {
		if slices.Contains(p.Tags, tag) {
			keep = append(keep, p)
			kept[p.ID] = true
		}
	}
	roots := tree(keep, selected, mod, true, tag)
	for _, v := range roots {
		if v.ParentID != nil && !kept[*v.ParentID] {
			v.ParentHref = "/#post-" + strconv.FormatInt(*v.ParentID, 10)
		}
	}
	return roots
}
