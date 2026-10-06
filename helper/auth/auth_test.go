package auth

import (
	"testing"
	"time"
)

func TestPassword(t *testing.T) {
	h, err := HashPassword("tfwnogf")
	if err != nil {
		t.Fatal(err)
	}
	if !CheckPassword(h, "tfwnogf") || CheckPassword(h, "wrong") {
		t.Fatal("bcrypt round trip failed")
	}
}

func TestSessions(t *testing.T) {
	s := NewSessions([]byte("secret"))
	tok := s.Issue("wojak", time.Hour)
	if u, ok := s.Verify(tok); !ok || u != "wojak" {
		t.Fatalf("verify: %q %v", u, ok)
	}
	if _, ok := s.Verify(tok + "x"); ok {
		t.Error("tampered signature accepted")
	}
	if _, ok := NewSessions([]byte("other")).Verify(tok); ok {
		t.Error("token from another secret accepted")
	}
	if _, ok := s.Verify(s.Issue("wojak", -time.Second)); ok {
		t.Error("expired token accepted")
	}
	if _, ok := s.Verify("garbage"); ok {
		t.Error("garbage accepted")
	}
}

func TestLoginThrottle(t *testing.T) {
	th := NewLoginThrottle()
	for i := 0; i < 3; i++ {
		th.Fail("1.2.3.4")
	}
	if _, ok := th.Allow("1.2.3.4"); !ok {
		t.Fatal("three failures should still be free")
	}
	th.Fail("1.2.3.4")
	if wait, ok := th.Allow("1.2.3.4"); ok || wait <= 0 {
		t.Fatal("fourth failure should impose a delay")
	}
	if _, ok := th.Allow("5.6.7.8"); !ok {
		t.Error("throttle must be per address")
	}
	th.Reset("1.2.3.4")
	if _, ok := th.Allow("1.2.3.4"); !ok {
		t.Error("reset should clear the delay")
	}
}

func TestAddressKey(t *testing.T) {
	for in, want := range map[string]string{
		"203.0.113.7": "203.0.113.7",

		"2001:db8:aaaa:bbbb:1:2:3:4":          "2001:db8:aaaa:bbbb::/64",
		"2001:db8:aaaa:bbbb:ffff:ffff:ffff:1": "2001:db8:aaaa:bbbb::/64",
		"2001:db8:aaaa:cccc::1":               "2001:db8:aaaa:cccc::/64",
		"::1":                                 "::/64",

		"::ffff:203.0.113.7": "203.0.113.7",

		"unknown": "unknown",
		"":        "",
	} {
		if got := AddressKey(in); got != want {
			t.Errorf("AddressKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFailures(t *testing.T) {
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	f := NewFailures(3, 5*time.Minute)
	f.now = func() time.Time { return now }

	for i := 1; i <= 2; i++ {
		if d := f.Fail("a"); d != 0 {
			t.Fatalf("failure %d: no lockout yet, got %v", i, d)
		}
		if _, locked := f.Locked("a"); locked {
			t.Fatalf("failure %d: not locked yet", i)
		}
	}

	if d := f.Fail("a"); d != 5*time.Minute {
		t.Fatalf("third failure: want a 5m lockout, got %v", d)
	}
	if wait, locked := f.Locked("a"); !locked || wait != 5*time.Minute {
		t.Fatalf("locked for 5m: %v %v", wait, locked)
	}
	now = now.Add(2 * time.Minute)
	if wait, _ := f.Locked("a"); wait != 3*time.Minute {
		t.Errorf("two minutes in, three are left: %v", wait)
	}

	f.Fail("a")
	if wait, _ := f.Locked("a"); wait != 3*time.Minute {
		t.Errorf("a failure during the lockout must not change it: %v", wait)
	}
	if _, locked := f.Locked("b"); locked {
		t.Error("another address is unaffected")
	}

	now = now.Add(3 * time.Minute)
	if _, locked := f.Locked("a"); locked {
		t.Fatal("the lockout is over once served")
	}
	f.Fail("a")
	f.Fail("a")
	if d := f.Fail("a"); d != 5*time.Minute {
		t.Errorf("the next lockout is the same 5m, got %v", d)
	}

	now = now.Add(time.Hour)
	f.Fail("c")
	f.Fail("c")
	f.Succeed("c")
	if d := f.Fail("c"); d != 0 {
		t.Errorf("after a success the count starts again: %v", d)
	}

	f.Fail("d")
	f.Fail("d")
	now = now.Add(FailureMemory + time.Second)
	if d := f.Fail("d"); d != 0 {
		t.Errorf("old failures are forgotten after %v: %v", FailureMemory, d)
	}

	one := NewFailures(1, time.Minute)
	if d := one.Fail("x"); d != time.Minute {
		t.Errorf("failures: 1 locks out on the first: %v", d)
	}

	off := NewFailures(3, 0)
	for i := 0; i < 10; i++ {
		if d := off.Fail("x"); d != 0 {
			t.Fatalf("lockouts off, got %v", d)
		}
	}
	if _, locked := off.Locked("x"); locked {
		t.Error("lockouts off: never locked")
	}
}
