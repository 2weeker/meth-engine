package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func HashPassword(password string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(h), err
}

func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

type Sessions struct{ secret []byte }

func NewSessions(secret []byte) *Sessions { return &Sessions{secret: secret} }

func (s *Sessions) sign(payload string) string {
	m := hmac.New(sha256.New, s.secret)
	m.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

func (s *Sessions) Issue(username string, ttl time.Duration) string {
	payload := base64.RawURLEncoding.EncodeToString([]byte(username)) + "." + strconv.FormatInt(time.Now().Add(ttl).Unix(), 10)
	return payload + "." + s.sign(payload)
}

func (s *Sessions) Verify(token string) (string, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", false
	}
	payload := parts[0] + "." + parts[1]
	if subtle.ConstantTimeCompare([]byte(s.sign(payload)), []byte(parts[2])) != 1 {
		return "", false
	}
	exp, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return "", false
	}
	user, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || len(user) == 0 {
		return "", false
	}
	return string(user), true
}

type LoginThrottle struct {
	mu    sync.Mutex
	fails map[string]*failure
	free  int
	base  time.Duration
	max   time.Duration
}

type failure struct {
	count int
	until time.Time
}

func NewLoginThrottle() *LoginThrottle {
	return &LoginThrottle{fails: map[string]*failure{}, free: 3, base: 2 * time.Second, max: 5 * time.Minute}
}

func (t *LoginThrottle) Allow(ip string) (time.Duration, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	f, ok := t.fails[ip]
	if !ok || time.Now().After(f.until) {
		return 0, true
	}
	return time.Until(f.until), false
}

func (t *LoginThrottle) Fail(ip string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	f := t.fails[ip]
	if f == nil {
		f = &failure{}
		t.fails[ip] = f
	}
	f.count++
	if f.count > t.free {
		d := t.base << uint(f.count-t.free-1)
		if d > t.max || d <= 0 {
			d = t.max
		}
		f.until = time.Now().Add(d)
	}
}

func (t *LoginThrottle) Reset(ip string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.fails, ip)
}

func AddressKey(ip string) string {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return ip
	}
	addr = addr.Unmap()
	if addr.Is4() {
		return addr.String()
	}
	prefix, err := addr.Prefix(64)
	if err != nil {
		return ip
	}
	return prefix.String()
}

const FailureMemory = time.Hour

type Failures struct {
	mu      sync.Mutex
	limit   int
	lockout time.Duration
	counts  map[string]*failures
	now     func() time.Time
}

type failures struct {
	count int
	last  time.Time
	until time.Time
}

func NewFailures(limit int, lockout time.Duration) *Failures {
	if limit < 1 {
		limit = 1
	}
	return &Failures{limit: limit, lockout: lockout, counts: map[string]*failures{}, now: time.Now}
}

func (f *Failures) Locked(addr string) (time.Duration, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	st, ok := f.counts[addr]
	if now := f.now(); ok && now.Before(st.until) {
		return st.until.Sub(now), true
	}
	return 0, false
}

func (f *Failures) Fail(addr string) time.Duration {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := f.now()
	st := f.counts[addr]
	switch {
	case st != nil && now.Before(st.until):
		return 0
	case st == nil || now.Sub(st.last) > FailureMemory:
		st = &failures{}
		f.counts[addr] = st
	}
	st.count++
	st.last = now

	if len(f.counts) > 10000 {
		for k, v := range f.counts {
			if now.Sub(v.last) > FailureMemory && !now.Before(v.until) {
				delete(f.counts, k)
			}
		}
	}
	if st.count < f.limit || f.lockout <= 0 {
		return 0
	}
	st.count = 0
	st.until = now.Add(f.lockout)
	return f.lockout
}

func (f *Failures) Succeed(addr string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if st, ok := f.counts[addr]; ok && !f.now().Before(st.until) {
		delete(f.counts, addr)
	}
}

type Bursts struct {
	mu     sync.Mutex
	posts  int
	within time.Duration
	wait   time.Duration
	seen   map[string]*burst
	now    func() time.Time
}

type burst struct {
	times []time.Time
	until time.Time
}

func NewBursts(posts int, within, wait time.Duration) *Bursts {
	return &Bursts{posts: max(posts, 1), within: within, wait: wait, seen: map[string]*burst{}, now: time.Now}
}

func (b *Bursts) Waiting(addr string) (time.Duration, bool) {
	if b == nil || b.wait <= 0 {
		return 0, false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if st, ok := b.seen[addr]; ok {
		if now := b.now(); now.Before(st.until) {
			return st.until.Sub(now), true
		}
	}
	return 0, false
}

func (b *Bursts) Posted(addr string) {
	if b == nil || b.wait <= 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.now()
	st := b.seen[addr]
	if st == nil {
		st = &burst{}
		b.seen[addr] = st
		b.forget(now)
	}
	keep := st.times[:0]
	for _, t := range st.times {
		if now.Sub(t) < b.within {
			keep = append(keep, t)
		}
	}
	st.times = append(keep, now)
	if len(st.times) >= b.posts {
		st.times = nil
		st.until = now.Add(b.wait)
	}
}

func (b *Bursts) forget(now time.Time) {
	if len(b.seen) <= 10000 {
		return
	}
	for k, st := range b.seen {
		if !now.Before(st.until) && (len(st.times) == 0 || now.Sub(st.times[len(st.times)-1]) >= b.within) {
			delete(b.seen, k)
		}
	}
}
