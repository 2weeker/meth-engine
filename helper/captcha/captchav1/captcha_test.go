package captchav1

import (
	"bytes"
	"image/png"
	"regexp"
	"strings"
	"testing"
	"time"
)

const here = "203.0.113.7"

var (
	postHere  = Binding{Purpose: Post, Addr: here}
	loginHere = Binding{Purpose: Login, Addr: here}
)

func testStore(capacity int) (*Store, *time.Time) {
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	s := New(capacity)
	s.now = func() time.Time { return now }
	return s, &now
}

func answer(t *testing.T, s *Store, id string) string {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.challenges[id]
	if !ok {
		t.Fatalf("no challenge %q", id)
	}
	return c.answer
}

func TestIssue(t *testing.T) {
	s, _ := testStore(0)
	idRe, answerRe := regexp.MustCompile(`^[0-9a-f]{32}$`), regexp.MustCompile(`^[a-ikm-pr-z02-9]{6}$`)
	ids, answers := map[string]bool{}, map[string]bool{}
	for i := 0; i < 200; i++ {
		id := s.Issue(postHere)
		if !idRe.MatchString(id) {
			t.Fatalf("id %q: want 32 hex characters, not a guessable sequence", id)
		}
		a := answer(t, s, id)
		if !answerRe.MatchString(a) {
			t.Fatalf("answer %q: want six of a-z and 0-9, none of them q, l, j or 1", a)
		}
		ids[id], answers[a] = true, true
	}
	if len(ids) != 200 || len(answers) < 195 {
		t.Errorf("ids and answers must not repeat: %d ids, %d answers", len(ids), len(answers))
	}
}

func TestVerify(t *testing.T) {
	s, _ := testStore(0)

	id := s.Issue(postHere)
	a := answer(t, s, id)
	if !s.Verify(id, a, postHere) {
		t.Error("the right answer is accepted")
	}
	if s.Verify(id, a, postHere) {
		t.Error("single use: the same captcha must not pass twice")
	}

	id = s.Issue(postHere)
	a = answer(t, s, id)
	if s.Verify(id, "zzzzzz", postHere) {
		t.Error("a wrong answer is refused")
	}
	if s.Verify(id, a, postHere) {
		t.Error("a wrong answer must burn the captcha: the right answer came too late")
	}

	id = s.Issue(postHere)
	if !s.Verify(id, "  "+strings.ToUpper(answer(t, s, id))+" ", postHere) {
		t.Error("case and surrounding spaces must not matter")
	}

	for name, try := range map[string][2]string{
		"unknown id": {"0123456789abcdef0123456789abcdef", "abcdef"},
		"empty id":   {"", "abcdef"},
		"both empty": {"", ""},
	} {
		if s.Verify(try[0], try[1], postHere) {
			t.Errorf("%s must be refused", name)
		}
	}

	id = s.Issue(postHere)
	if s.Verify(id, "", postHere) {
		t.Error("an empty answer is refused")
	}
}

func TestVerifyIsBound(t *testing.T) {
	s, _ := testStore(0)
	for name, elsewhere := range map[string]Binding{
		"the login":         {Purpose: Login, Addr: here},
		"another address":   {Purpose: Post, Addr: "198.51.100.9"},
		"no address at all": {Purpose: Post},
	} {
		id := s.Issue(postHere)
		a := answer(t, s, id)
		if s.Verify(id, a, elsewhere) {
			t.Errorf("a posting-form captcha for spam at %s must not pass at %s", here, name)
		}
		if s.Verify(id, a, postHere) {
			t.Errorf("%s: trying it there must have used it up", name)
		}
	}
	id := s.Issue(loginHere)
	if s.Verify(id, answer(t, s, id), postHere) {
		t.Error("a login captcha must not pass on the posting form")
	}
	id = s.Issue(loginHere)
	if !s.Verify(id, answer(t, s, id), loginHere) {
		t.Error("the login captcha passes at the login, from the same address")
	}
}

func TestImageIsBoundToTheAddress(t *testing.T) {
	s, _ := testStore(0)
	id := s.Issue(postHere)
	if _, ok := s.Image(id, "198.51.100.9"); ok {
		t.Error("another address must not get the image")
	}
	if _, ok := s.Image(id, here); !ok {
		t.Error("the address it was issued to gets the image")
	}

	if !s.Verify(id, answer(t, s, id), postHere) {
		t.Error("a refused image fetch must not use the captcha up")
	}
}

func TestExpiry(t *testing.T) {
	s, now := testStore(0)
	id := s.Issue(postHere)
	a := answer(t, s, id)
	*now = now.Add(Lifetime - time.Second)
	if _, ok := s.Image(id, here); !ok {
		t.Error("still valid just inside its lifetime")
	}
	*now = now.Add(2 * time.Second)
	if _, ok := s.Image(id, here); ok {
		t.Error("the image of an expired captcha is not served")
	}
	if s.Verify(id, a, postHere) {
		t.Error("an expired captcha is refused, right answer or not")
	}

	for i := 0; i < 50; i++ {
		s.Issue(postHere)
	}
	*now = now.Add(Lifetime + time.Second)
	s.Issue(postHere)
	if n := s.Len(); n != 1 {
		t.Errorf("after the sweep only the new captcha remains, got %d", n)
	}
}

func TestCapacity(t *testing.T) {
	s, now := testStore(10)
	first := s.Issue(postHere)
	var last string
	for i := 0; i < 20; i++ {
		*now = now.Add(time.Millisecond)
		last = s.Issue(postHere)
	}
	if n := s.Len(); n != 10 {
		t.Errorf("want 10 held, got %d", n)
	}
	if _, ok := s.Image(first, here); ok {
		t.Error("the oldest captcha should have been dropped")
	}
	if _, ok := s.Image(last, here); !ok {
		t.Error("the newest captcha must survive")
	}
}

func TestImage(t *testing.T) {
	s, _ := testStore(0)
	id := s.Issue(postHere)
	data, ok := s.Image(id, here)
	if !ok {
		t.Fatal("no image")
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("not a PNG: %v", err)
	}
	if b := img.Bounds(); b.Dx() != Width || b.Dy() != Height {
		t.Errorf("want %dx%d, got %dx%d", Width, Height, b.Dx(), b.Dy())
	}
	again, _ := s.Image(id, here)
	if !bytes.Equal(data, again) {
		t.Error("the same captcha must render the same image every time it is fetched")
	}
	other, _ := s.Image(s.Issue(postHere), here)
	if bytes.Equal(data, other) {
		t.Error("two captchas must not share an image")
	}
	if _, ok := s.Image("0123456789abcdef0123456789abcdef", here); ok {
		t.Error("an unknown id has no image")
	}

	if !s.Verify(id, answer(t, s, id), postHere) {
		t.Error("fetching the image must not consume the captcha")
	}
}

func TestAnswersUseTheWholeAlphabet(t *testing.T) {
	counts := map[rune]int{}
	const draws = 64000
	for _, r := range randomLetters(draws) {
		counts[r]++
	}
	want := draws / len(Alphabet)
	for _, r := range Alphabet {
		if n := counts[r]; n < want*9/10 || n > want*11/10 {
			t.Errorf("%q drawn %d times in %d; want about %d", r, n, draws, want)
		}
	}
	if len(counts) != len(Alphabet) {
		t.Errorf("drew %d different characters, want %d", len(counts), len(Alphabet))
	}
	for _, r := range "023456789" {
		if counts[r] == 0 {
			t.Errorf("the digit %q never turned up", r)
		}
	}
	for _, r := range "qlj1" {
		if counts[r] != 0 {
			t.Errorf("%q must never turn up", r)
		}
	}
}
