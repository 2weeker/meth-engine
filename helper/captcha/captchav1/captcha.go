package captchav1

import (
	"bytes"
	"container/list"
	"crypto/rand"
	"crypto/subtle"
	"encoding/binary"
	"encoding/hex"
	"image/png"
	"strings"
	"sync"
	"time"

	"meth-enginev2/helper/captcha/captchaimg"
)

const (
	Width  = 192
	Height = 64
)

var canvas = captchaimg.Canvas{Width: Width, Height: Height}

const (
	Length = 6

	Lifetime = 5 * time.Minute

	DefaultCapacity = 50000
)

type Purpose string

const (
	Post  Purpose = "post"
	Login Purpose = "login"
)

type Binding struct {
	Purpose Purpose
	Addr    string
}

type challenge struct {
	answer  string
	bound   Binding
	seed    uint64
	expires time.Time
	queued  *list.Element
}

type Store struct {
	mu         sync.Mutex
	capacity   int
	challenges map[string]challenge
	queue      *list.List
	now        func() time.Time
}

func New(capacity int) *Store {
	if capacity <= 0 {
		capacity = DefaultCapacity
	}
	return &Store{capacity: capacity, challenges: map[string]challenge{}, queue: list.New(), now: time.Now}
}

func (s *Store) Issue(b Binding) string {
	var raw [16 + 8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		panic("captchav1: no randomness: " + err.Error())
	}
	id := hex.EncodeToString(raw[:16])
	c := challenge{answer: randomLetters(Length), bound: b, seed: binary.LittleEndian.Uint64(raw[16:])}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweep()
	for len(s.challenges) >= s.capacity {
		s.drop(s.queue.Front().Value.(string))
	}
	c.expires = s.now().Add(Lifetime)
	c.queued = s.queue.PushBack(id)
	s.challenges[id] = c
	return id
}

func (s *Store) Verify(id, value string, b Binding) bool {
	s.mu.Lock()
	s.sweep()
	c, ok := s.challenges[id]
	if ok {
		s.drop(id)
	}
	expired := ok && !s.now().Before(c.expires)
	s.mu.Unlock()

	value = strings.ToLower(strings.TrimSpace(value))
	if !ok || expired || c.bound != b || b.Addr == "" || value == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(value), []byte(c.answer)) == 1
}

func (s *Store) Image(id, addr string) ([]byte, bool) {
	s.mu.Lock()
	c, ok := s.challenges[id]
	ok = ok && s.now().Before(c.expires) && addr != "" && c.bound.Addr == addr
	s.mu.Unlock()
	if !ok {
		return nil, false
	}
	var buf bytes.Buffer
	if err := (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(&buf, captchaimg.Draw(c.answer, c.seed, canvas)); err != nil {
		return nil, false
	}
	return buf.Bytes(), true
}

func (s *Store) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.challenges)
}

func (s *Store) sweep() {
	now := s.now()
	for e := s.queue.Front(); e != nil; e = s.queue.Front() {
		id := e.Value.(string)
		if now.Before(s.challenges[id].expires) {
			return
		}
		s.drop(id)
	}
}

func (s *Store) drop(id string) {
	if c, ok := s.challenges[id]; ok {
		s.queue.Remove(c.queued)
		delete(s.challenges, id)
	}
}

const Alphabet = "abcdefghikmnoprstuvwxyz023456789"

func randomLetters(n int) string {
	fair := 256 - 256%len(Alphabet)
	out := make([]byte, 0, n)
	var buf [16]byte
	for len(out) < n {
		if _, err := rand.Read(buf[:]); err != nil {
			panic("captchav1: no randomness: " + err.Error())
		}
		for _, b := range buf {
			if int(b) < fair && len(out) < n {
				out = append(out, Alphabet[int(b)%len(Alphabet)])
			}
		}
	}
	return string(out)
}
