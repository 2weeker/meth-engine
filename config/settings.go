package config

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"html/template"
	"log"
	"meth-enginev2/helper/themes"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"meth-enginev2/helper/applog"
)

type Site struct {
	Title       string `yaml:"title"`
	Description string `yaml:"description"`

	URL string `yaml:"url"`

	Banner struct {
		Logo      string    `yaml:"logo"`
		Text      string    `yaml:"text"`
		Addresses []Address `yaml:"addresses"`
		Links     []Link    `yaml:"links"`
	} `yaml:"banner"`

	Captcha *bool `yaml:"captcha"`

	CaptchaLockout CaptchaLockout `yaml:"captcha_lockout"`

	CaptchaPostLimit PostLimit `yaml:"captcha_post_limit"`

	MaxChars *int `yaml:"max_chars"`

	MaxPosts *int `yaml:"max_posts"`

	Tags *TagsSetting `yaml:"tags"`

	Webring []Link `yaml:"webring"`

	Theme string `yaml:"theme"`

	Boards         yaml.Node `yaml:"boards"`
	Overboard      yaml.Node `yaml:"overboard"`
	CaptchaVersion yaml.Node `yaml:"captcha_version"`
	LoginCaptcha   yaml.Node `yaml:"login_captcha"`

	RateLimit yaml.Node `yaml:"rate_limit"`

	AboutHTML template.HTML `yaml:"about"`
}

const maxLockout = 7 * 24 * time.Hour

type CaptchaLockout struct {
	Posting Lockout `yaml:"posting"`

	Login Lockout `yaml:"login"`
}

type Lockout struct {
	Failures *int      `yaml:"failures"`
	Lockout  *Duration `yaml:"lockout"`

	login bool
}

const (
	defaultPostFailures  = 5
	defaultPostLockout   = time.Minute
	defaultLoginFailures = 3
	defaultLoginLockout  = 15 * time.Minute
	maxCaptchaFailures   = 100
)

func (l Lockout) Limit() int {
	switch {
	case l.Failures != nil:
		return *l.Failures
	case l.login:
		return defaultLoginFailures
	}
	return defaultPostFailures
}

func (l Lockout) Duration() time.Duration {
	switch {
	case l.Lockout != nil:
		return l.Lockout.Duration
	case l.login:
		return defaultLoginLockout
	}
	return defaultPostLockout
}

func (l Lockout) check(side string) error {
	if n := l.Limit(); n < 1 || n > maxCaptchaFailures {
		return fmt.Errorf("captcha_lockout.%s.failures must be between 1 and %d", side, maxCaptchaFailures)
	}
	if d := l.Duration(); d < 0 || d > maxLockout {
		return fmt.Errorf("captcha_lockout.%s.lockout must be between 0 and 168h", side)
	}
	return nil
}

type PostLimit struct {
	Posts  *int      `yaml:"posts"`
	Within *Duration `yaml:"within"`
	Wait   *Duration `yaml:"wait"`
}

const (
	defaultBurstPosts  = 5
	defaultBurstWithin = time.Minute
	defaultBurstWait   = 30 * time.Second
	maxBurstPosts      = 1000
)

func (p PostLimit) Count() int {
	if p.Posts != nil {
		return *p.Posts
	}
	return defaultBurstPosts
}

func (p PostLimit) Window() time.Duration {
	if p.Within != nil {
		return p.Within.Duration
	}
	return defaultBurstWithin
}

func (p PostLimit) Pause() time.Duration {
	if p.Wait != nil {
		return p.Wait.Duration
	}
	return defaultBurstWait
}

func (p PostLimit) check() error {
	if n := p.Count(); n < 1 || n > maxBurstPosts {
		return fmt.Errorf("captcha_post_limit.posts must be between 1 and %d", maxBurstPosts)
	}
	if d := p.Window(); d < time.Second || d > 24*time.Hour {
		return errors.New("captcha_post_limit.within must be between 1s and 24h")
	}
	if d := p.Pause(); d < 0 || d > 24*time.Hour {
		return errors.New("captcha_post_limit.wait must be between 0 (no limit) and 24h")
	}
	return nil
}

func (c Config) PostLimit() PostLimit { return c.Site.CaptchaPostLimit }

const (
	DefaultMaxChars = 1024
	maxMaxChars     = 100000
)

func (c Config) MaxChars() int { return c.Site.maxChars() }

const (
	DefaultMaxPosts = 254
	maxMaxPosts     = 10000
)

func (s Site) PostCap() int {
	if s.MaxPosts != nil {
		return *s.MaxPosts
	}
	return DefaultMaxPosts
}

func (s Site) maxChars() int {
	if s.MaxChars != nil {
		return *s.MaxChars
	}
	return DefaultMaxChars
}

func (s *Site) fillAbout() {
	s.AboutHTML = template.HTML(strings.NewReplacer(
		"{max_posts}", strconv.Itoa(s.PostCap()),
		"{max_chars}", strconv.Itoa(s.maxChars()),
	).Replace(string(s.AboutHTML)))
}

func checkMaxChars(p *int, where string) error {
	if p != nil && (*p < 1 || *p > maxMaxChars) {
		return fmt.Errorf("%s: max_chars must be between 1 and %d", where, maxMaxChars)
	}
	return nil
}

func (p *PostLimit) UnmarshalYAML(n *yaml.Node) error {
	if err := knownKeys(n, "captcha_post_limit", "posts", "within", "wait"); err != nil {
		return err
	}
	type plain PostLimit
	return n.Decode((*plain)(p))
}

func (c *CaptchaLockout) UnmarshalYAML(n *yaml.Node) error {
	if err := knownKeys(n, "captcha_lockout", "posting", "login"); err != nil {
		return err
	}
	type plain CaptchaLockout
	if err := n.Decode((*plain)(c)); err != nil {
		return err
	}
	c.Login.login = true
	return nil
}

func (l *Lockout) UnmarshalYAML(n *yaml.Node) error {
	if err := knownKeys(n, "captcha_lockout", "failures", "lockout"); err != nil {
		return err
	}
	type plain Lockout
	return n.Decode((*plain)(l))
}

func knownKeys(n *yaml.Node, section string, keys ...string) error {
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: %s must be a mapping", n.Line, section)
	}
	for i := 0; i < len(n.Content); i += 2 {
		if k := n.Content[i].Value; !slices.Contains(keys, k) {
			return fmt.Errorf("line %d: %s has no setting %q (it has %s)", n.Content[i].Line, section, k, strings.Join(keys, ", "))
		}
	}
	return nil
}

type TagsSetting struct {
	On     bool
	Max    *int `yaml:"max"`
	Length *int `yaml:"length"`
}

const (
	DefaultMaxTags   = 3
	DefaultTagLength = 24
	maxMaxTags       = 10
	maxTagLength     = 48
)

func (bt *TagsSetting) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		switch strings.ToLower(strings.TrimSpace(n.Value)) {
		case "true", "yes", "on":
			*bt = TagsSetting{On: true}
		case "false", "no", "off":
			*bt = TagsSetting{}
		default:
			return fmt.Errorf("line %d: tags is true, false, or a mapping of max and length, not %q", n.Line, n.Value)
		}
		return nil
	}
	if err := knownKeys(n, "tags", "max", "length"); err != nil {
		return err
	}
	var limits struct {
		Max    *int `yaml:"max"`
		Length *int `yaml:"length"`
	}
	if err := n.Decode(&limits); err != nil {
		return err
	}
	*bt = TagsSetting{On: true, Max: limits.Max, Length: limits.Length}
	return nil
}

type TagLimits struct {
	Max    int
	Length int
}

func (bt *TagsSetting) limits() TagLimits {
	l := TagLimits{Max: DefaultMaxTags, Length: DefaultTagLength}
	if bt.Max != nil {
		l.Max = *bt.Max
	}
	if bt.Length != nil {
		l.Length = *bt.Length
	}
	return l
}

func (c Config) Tags() (TagLimits, bool) {
	if c.Site.Tags != nil && c.Site.Tags.On {
		return c.Site.Tags.limits(), true
	}
	return TagLimits{}, false
}

type Duration struct{ time.Duration }

func (d *Duration) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.ScalarNode {
		return fmt.Errorf("line %d: want a duration like 10s", n.Line)
	}
	v, err := time.ParseDuration(n.Value)
	if err != nil {
		return fmt.Errorf("line %d: %q is not a duration like 10s or 1m", n.Line, n.Value)
	}
	d.Duration = v
	return nil
}

const (
	defaultLogo      = "/assets/logo.gif"
	defaultShareLogo = "/assets/share.png"
	ShareWidth       = 1200
	ShareHeight      = 630
)

var logoTypes = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true, ".svg": true, ".avif": true}

func (s Site) LogoURL() string {
	if s.Banner.Logo == "" {
		return defaultLogo
	}
	return "/site-logo" + strings.ToLower(filepath.Ext(s.Banner.Logo))
}

func (s Site) ShareImagePath() string {
	if s.Banner.Logo == "" {
		return defaultShareLogo
	}
	return s.LogoURL()
}

func (s Site) ShareImageBuiltIn() bool { return s.Banner.Logo == "" }

type Address struct {
	Label   string `yaml:"label"`
	Address string `yaml:"address"`
}

type Link struct {
	Label string `yaml:"label"`
	URL   string `yaml:"url"`
}

type Config struct {
	Env         string
	Addr        string
	DatabaseURL string

	Secret []byte

	PosterIDSecret []byte

	ModUsername, ModPassword string

	TrustedProxy bool

	CaptchaEnabled bool

	Log applog.Options

	PprofAddr string

	Site Site
}

const DefaultPath = "config.yaml"

var truthy = map[string]bool{"1": true, "true": true, "yes": true, "on": true}

func Load(path string) (Config, error) {
	logOpts, err := LogOptions()
	if err != nil {
		return Config{}, err
	}
	c := Config{
		Log:          logOpts,
		PprofAddr:    os.Getenv("METH_PPROF_ADDR"),
		Env:          env("METH_ENV", "development"),
		Addr:         ListenAddr(),
		DatabaseURL:  env("DATABASE_URL", "postgres://postgres@localhost:5432/meth"),
		ModUsername:  os.Getenv("METH_MOD_USERNAME"),
		ModPassword:  os.Getenv("METH_MOD_PASSWORD"),
		TrustedProxy: truthy[strings.ToLower(os.Getenv("METH_TRUSTED_PROXY"))],
	}

	if os.Getenv("METH_CAPTCHA_ENABLED") != "" {
		return c, errors.New("METH_CAPTCHA_ENABLED was removed; remove it from the environment and set captcha: true or false in config.yaml")
	}
	if os.Getenv("METH_POST_COOLDOWN") != "" {
		return c, errors.New("METH_POST_COOLDOWN was removed along with the rate limit; remove it from the environment (posts are on the captcha; see captcha_post_limit in config.yaml)")
	}

	if s := os.Getenv("METH_SECRET"); s != "" {
		c.Secret = []byte(s)
	} else {
		c.Secret = randomKey()
		if c.Env == "production" {
			log.Print("WARNING: METH_SECRET is unset; moderator sessions will not survive a restart")
		}
	}
	if s := os.Getenv("METH_POSTER_ID_SECRET"); s != "" {
		c.PosterIDSecret = []byte(s)
	} else {
		c.PosterIDSecret = c.Secret
	}

	site, err := LoadSite(path)
	if err != nil {
		return c, err
	}
	c.Site = site
	c.CaptchaEnabled = site.Captcha == nil || *site.Captcha
	return c, nil
}

func ListenAddr() string { return env("METH_ADDR", ":"+env("PORT", "3000")) }

func LogOptions() (applog.Options, error) {
	o := applog.Options{SlowRequest: 500 * time.Millisecond, SlowQuery: 100 * time.Millisecond, StatsInterval: time.Minute}
	if v := os.Getenv("METH_LOG_LEVEL"); v != "" {
		l, err := applog.ParseLevel(v)
		if err != nil {
			return o, fmt.Errorf("METH_LOG_LEVEL %q: want debug, info, warn or error", v)
		}
		o.Level = l
	}
	switch f := strings.ToLower(os.Getenv("METH_LOG_FORMAT")); f {
	case "", "text":
	case "json":
		o.JSON = true
	default:
		return o, fmt.Errorf("METH_LOG_FORMAT %q: want text or json", f)
	}
	for _, d := range []struct {
		env string
		dst *time.Duration
	}{{"METH_SLOW_REQUEST", &o.SlowRequest}, {"METH_SLOW_QUERY", &o.SlowQuery}, {"METH_STATS_INTERVAL", &o.StatsInterval}} {
		if v := os.Getenv(d.env); v != "" {
			p, err := time.ParseDuration(v)
			if err != nil || p < 0 {
				return o, fmt.Errorf("%s %q is not a duration", d.env, v)
			}
			*d.dst = p
		}
	}
	return o, nil
}

func DefaultSite() Site {
	var s Site
	s.Title = "meth"
	s.Description = "The graffiti wall of the internet"
	s.AboutHTML = "An anonymous textboard with no JavaScript. The oldest post is purged at the {max_posts}-post limit."
	return s
}

func LoadSite(path string) (Site, error) {
	explicit := path != ""
	if !explicit {
		path = os.Getenv("METH_CONFIG")
		explicit = path != ""
	}
	if path == "" {
		path = DefaultPath
	}

	s := DefaultSite()
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := yaml.Unmarshal(data, &s); err != nil {
			return s, fmt.Errorf("%s: %w", path, err)
		}
	case errors.Is(err, os.ErrNotExist) && !explicit:
		log.Printf("no %s; using the default site text", path)
	default:
		return s, err
	}
	if strings.TrimSpace(s.Title) == "" {
		return s, fmt.Errorf("%s: title must not be empty", path)
	}
	for _, r := range []struct {
		key, gone string
		node      yaml.Node
	}{
		{"rate_limit", "the rate limit was removed and posts are on the captcha (see captcha_post_limit)", s.RateLimit},
		{"boards", "boards were removed and the site is one stream of posts: set captcha_version, captcha_post_limit, max_chars and tags at the top level instead", s.Boards},
		{"overboard", "the overboard was removed along with the boards", s.Overboard},
		{"captcha_version", "captchav1 is the only captcha; captcha: true or false is the switch", s.CaptchaVersion},
		{"login_captcha", "captchav1 is the only captcha; captcha: true or false is the switch", s.LoginCaptcha},
	} {
		if r.node.Kind != 0 {
			return s, fmt.Errorf("%s: line %d: %s is no longer a setting; %s, so remove it", path, r.node.Line, r.key, r.gone)
		}
	}
	if s.Theme == "" {
		s.Theme = themes.Fallback
	} else if name, ok := themes.Lookup(s.Theme); ok {
		s.Theme = name
	} else {
		return s, fmt.Errorf("%s: theme %q is not one of %s", path, s.Theme, strings.Join(themes.Names(), ", "))
	}
	if s.Tags != nil && s.Tags.On {
		if l := s.Tags.limits(); l.Max < 1 || l.Max > maxMaxTags {
			return s, fmt.Errorf("%s: tags.max must be between 1 and %d", path, maxMaxTags)
		} else if l.Length < 1 || l.Length > maxTagLength {
			return s, fmt.Errorf("%s: tags.length must be between 1 and %d", path, maxTagLength)
		}
	}
	s.CaptchaLockout.Login.login = true
	for side, l := range map[string]Lockout{"posting": s.CaptchaLockout.Posting, "login": s.CaptchaLockout.Login} {
		if err := l.check(side); err != nil {
			return s, fmt.Errorf("%s: %w", path, err)
		}
	}
	if err := s.CaptchaPostLimit.check(); err != nil {
		return s, fmt.Errorf("%s: %w", path, err)
	}
	if err := checkMaxChars(s.MaxChars, "site"); err != nil {
		return s, fmt.Errorf("%s: %w", path, err)
	}
	if n := s.MaxPosts; n != nil && (*n < 1 || *n > maxMaxPosts) {
		return s, fmt.Errorf("%s: max_posts must be between 1 and %d", path, maxMaxPosts)
	}
	s.fillAbout()
	if s.URL != "" {
		u, err := url.Parse(s.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return s, fmt.Errorf("%s: url must be an absolute http(s) URL, e.g. https://meth.example", path)
		}
		s.URL = strings.TrimRight(s.URL, "/")
	}
	if s.Banner.Logo != "" {
		logo := s.Banner.Logo
		if !filepath.IsAbs(logo) {
			logo = filepath.Join(filepath.Dir(path), logo)
		}
		if !logoTypes[strings.ToLower(filepath.Ext(logo))] {
			return s, fmt.Errorf("%s: banner.logo must be a png, jpg, gif, webp, avif or svg file", path)
		}
		info, err := os.Stat(logo)
		if err != nil {
			return s, fmt.Errorf("%s: banner.logo: %w", path, err)
		}
		if !info.Mode().IsRegular() {
			return s, fmt.Errorf("%s: banner.logo %s is not a file", path, logo)
		}
		s.Banner.Logo = logo
	}
	for i, a := range s.Banner.Addresses {
		if a.Label == "" || a.Address == "" {
			return s, fmt.Errorf("%s: banner.addresses[%d] needs a label and an address", path, i)
		}
	}
	for i, l := range s.Banner.Links {
		if l.Label == "" || l.URL == "" {
			return s, fmt.Errorf("%s: banner.links[%d] needs a label and a url", path, i)
		}
	}
	for i, l := range s.Webring {
		if strings.TrimSpace(l.Label) == "" || l.URL == "" {
			return s, fmt.Errorf("%s: webring[%d] needs a label and a url", path, i)
		}

		if u, err := url.Parse(l.URL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return s, fmt.Errorf("%s: webring[%d] url must be an absolute http(s) URL, e.g. https://friend.example", path, i)
		}
	}
	return s, nil
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func randomKey() []byte {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		log.Fatal(err)
	}
	return []byte(hex.EncodeToString(b))
}
