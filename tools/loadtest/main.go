package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var (
	base     = flag.String("url", "http://localhost:3100", "engine to load")
	readers  = flag.Int("readers", 60, "users browsing pages")
	posters  = flag.Int("posters", 20, "users writing posts")
	duration = flag.Duration("duration", 2*time.Minute, "how long to run")
	think    = flag.Duration("think", 3*time.Second, "mean pause between a reader's page views")
	postGap  = flag.Duration("post-gap", 12*time.Second, "mean pause between a poster's posts (keep above the post limit's wait)")
	stall    = flag.Duration("stall", time.Second, "report any request slower than this")
)

type series struct {
	mu   sync.Mutex
	durs []time.Duration
	errs int
}

type metrics struct {
	mu     sync.Mutex
	byKind map[string]*series
	stalls []string
}

func (m *metrics) get(kind string) *series {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.byKind[kind]
	if s == nil {
		s = &series{}
		m.byKind[kind] = s
	}
	return s
}

func (m *metrics) record(kind string, d time.Duration, err error, detail string) {
	s := m.get(kind)
	s.mu.Lock()
	if err != nil {
		s.errs++
	} else {
		s.durs = append(s.durs, d)
	}
	s.mu.Unlock()
	if err != nil || d > *stall {
		m.mu.Lock()
		msg := fmt.Sprintf("%s %-12s %7s %s", time.Now().Format("15:04:05.000"), kind, d.Round(time.Millisecond), detail)
		if err != nil {
			msg += " ERR " + err.Error()
		}
		m.stalls = append(m.stalls, msg)
		m.mu.Unlock()
	}
}

func pct(d []time.Duration, p float64) time.Duration {
	if len(d) == 0 {
		return 0
	}
	i := int(float64(len(d)-1) * p)
	return d[i]
}

func (m *metrics) report(elapsed time.Duration) {
	m.mu.Lock()
	kinds := make([]string, 0, len(m.byKind))
	for k := range m.byKind {
		kinds = append(kinds, k)
	}
	m.mu.Unlock()
	sort.Strings(kinds)
	fmt.Printf("\n%-13s %7s %6s %8s %8s %8s %8s %8s %6s\n", "kind", "count", "rps", "p50", "p95", "p99", "max", ">stall", "errs")
	for _, k := range kinds {
		s := m.get(k)
		s.mu.Lock()
		d := append([]time.Duration(nil), s.durs...)
		errs := s.errs
		s.mu.Unlock()
		sort.Slice(d, func(i, j int) bool { return d[i] < d[j] })
		slow := 0
		for _, v := range d {
			if v > *stall {
				slow++
			}
		}
		r := time.Millisecond
		fmt.Printf("%-13s %7d %6.1f %8s %8s %8s %8s %8d %6d\n", k, len(d), float64(len(d))/elapsed.Seconds(),
			pct(d, .5).Round(r), pct(d, .95).Round(r), pct(d, .99).Round(r), pct(d, 1).Round(r), slow, errs)
	}
}

type user struct {
	c  *http.Client
	ip string
	m  *metrics
}

var ipSeq atomic.Int64

func newUser(m *metrics) *user {
	jar, _ := cookiejar.New(nil)
	n := ipSeq.Add(1)
	return &user{
		c: &http.Client{
			Jar:     jar,
			Timeout: 30 * time.Second,

			Transport: &http.Transport{MaxConnsPerHost: 6, MaxIdleConnsPerHost: 6, IdleConnTimeout: 90 * time.Second},
		},
		ip: fmt.Sprintf("10.%d.%d.%d", n>>16&255, n>>8&255, n&255+1),
		m:  m,
	}
}

func (u *user) do(kind string, req *http.Request) (string, *http.Response, error) {
	req.Header.Set("X-Forwarded-For", u.ip)
	req.Header.Set("User-Agent", "meth-loadtest")
	start := time.Now()
	res, err := u.c.Do(req)
	var body []byte
	if err == nil {
		body, err = io.ReadAll(res.Body)
		res.Body.Close()
		if err == nil && res.StatusCode >= 400 {
			err = fmt.Errorf("HTTP %d", res.StatusCode)
		}
	}
	u.m.record(kind, time.Since(start), err, req.Method+" "+req.URL.RequestURI())
	return string(body), res, err
}

func (u *user) get(kind, path string) (string, error) {
	req, _ := http.NewRequest("GET", *base+path, nil)
	body, _, err := u.do(kind, req)
	return body, err
}

var (
	csrfRe   = regexp.MustCompile(`name="_csrf" value="([^"]+)"`)
	postIDRe = regexp.MustCompile(`id="post-(\d+)"`)
)

func sleepJitter(ctx context.Context, mean time.Duration) bool {
	d := time.Duration(float64(mean) * (0.5 + rand.Float64()))
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}

var loadTags = []string{"load", "test", "synth", "vinyl"}

func pick[T any](xs []T) T { return xs[rand.IntN(len(xs))] }

func postIDs(html string) []string {
	var ids []string
	for _, m := range postIDRe.FindAllStringSubmatch(html, -1) {
		ids = append(ids, m[1])
	}
	return ids
}

func reader(ctx context.Context, m *metrics) {
	u := newUser(m)
	for sleepJitter(ctx, *think) {
		switch r := rand.IntN(10); {
		case r < 7:
			html, err := u.get("main", "/")
			if ids := postIDs(html); err == nil && len(ids) > 0 && rand.IntN(2) == 0 {
				u.get("thread", "/"+pick(ids))
			}
		case r < 9:
			u.get("tag", "/?tag="+pick(loadTags))
		default:
			u.get("static", "/css/coffee.css")
		}
	}
}

func poster(ctx context.Context, m *metrics) {
	u := newUser(m)

	if !sleepJitter(ctx, *postGap) {
		return
	}
	for {
		html, err := u.get("main", "/")
		if err == nil {
			tok := csrfRe.FindStringSubmatch(html)
			ids := postIDs(html)
			if tok != nil {
				form := url.Values{"_csrf": {tok[1]},
					"msg": {fmt.Sprintf("load test %s at %s\n>greentext line %d", u.ip, time.Now().Format(time.RFC3339Nano), rand.IntN(1e6))}}
				action := "/post/create"
				if len(ids) > 0 && rand.IntN(3) > 0 {
					parent := pick(ids)
					form.Set("parent", parent)
					action += "/" + parent
				}
				if rand.IntN(5) == 0 {
					form.Set("sage", "true")
				}
				if rand.IntN(2) == 0 {
					form.Set("tags", pick(loadTags))
				}
				req, _ := http.NewRequest("POST", *base+action, strings.NewReader(form.Encode()))
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				req.Header.Set("Origin", *base)
				req.Header.Set("Referer", *base+"/")

				_, res, err := u.do("post", req)
				if err == nil {
					if code := res.Request.URL.Query().Get("error"); code != "" {
						m.record("post-refused:"+code, 0, nil, "")
					}
				}
			}
		}
		if !sleepJitter(ctx, *postGap) {
			return
		}
	}
}

func main() {
	flag.Parse()
	if _, err := http.Get(*base + "/robots.txt"); err != nil {
		fmt.Fprintln(os.Stderr, "engine unreachable:", err)
		os.Exit(1)
	}
	m := &metrics{byKind: map[string]*series{}}
	ctx, cancel := context.WithTimeout(context.Background(), *duration)
	defer cancel()

	var wg sync.WaitGroup
	start := time.Now()
	spawn := func(n int, f func(context.Context, *metrics)) {
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); f(ctx, m) }()
		}
	}
	spawn(*readers, reader)
	spawn(*posters, poster)
	fmt.Printf("loading %s: %d readers, %d posters for %s\n", *base, *readers, *posters, *duration)

	tick := time.NewTicker(15 * time.Second)
	defer tick.Stop()
	go func() {
		for range tick.C {
			if ctx.Err() != nil {
				return
			}
			fmt.Printf("  %s elapsed\n", time.Since(start).Round(time.Second))
		}
	}()
	wg.Wait()
	elapsed := time.Since(start)

	m.report(elapsed)
	m.mu.Lock()
	fmt.Printf("\n%d requests over %s or failed:\n", len(m.stalls), *stall)
	for i, s := range m.stalls {
		if i == 40 {
			fmt.Printf("  ... %d more\n", len(m.stalls)-40)
			break
		}
		fmt.Println("  " + s)
	}
	m.mu.Unlock()
}
