package posterid

import (
	"regexp"
	"testing"
	"time"
)

func TestFor(t *testing.T) {
	key := []byte("k")
	day := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	a := For(key, "1.2.3.4", day)
	if len(a) != 8 {
		t.Fatalf("want 8 chars, got %q", a)
	}
	if !regexp.MustCompile(`^[A-Za-z0-9_-]{8}$`).MatchString(a) {
		t.Errorf("not urlsafe: %q", a)
	}
	if For(key, "1.2.3.4", day.Add(3*time.Hour)) != a {
		t.Error("should be stable within a day")
	}
	if For(key, "1.2.3.5", day) == a {
		t.Error("should differ per address")
	}

	if For(key, "1.2.3.4", time.Date(2026, 9, 21, 23, 59, 59, 0, time.UTC)) != a {
		t.Error("should hold until midnight UTC")
	}
	if For(key, "1.2.3.4", day.Add(24*time.Hour)) == a {
		t.Error("should rotate at midnight UTC")
	}
	if For([]byte("other"), "1.2.3.4", day) == a {
		t.Error("changing the key should change the ID")
	}
	if len(For(key, "", day)) != 8 {
		t.Error("an address-less post still gets an ID")
	}
}
