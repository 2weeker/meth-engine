package auth

import (
	"testing"
	"time"
)

func TestBursts(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	b := NewBursts(3, time.Minute, 30*time.Second)
	b.now = func() time.Time { return now }
	const alice, bob = "203.0.113.7", "198.51.100.9"
	step := func(d time.Duration) { now = now.Add(d) }

	b.Posted(alice)
	step(10 * time.Second)
	b.Posted(alice)
	if _, w := b.Waiting(alice); w {
		t.Fatal("two posts in a minute is under the limit")
	}
	step(10 * time.Second)
	b.Posted(alice)
	wait, w := b.Waiting(alice)
	if !w || wait != 30*time.Second {
		t.Fatalf("the third post within a minute: waiting %v for %v, want 30s", w, wait)
	}
	if _, w := b.Waiting(bob); w {
		t.Error("another address has its own count")
	}

	step(20 * time.Second)
	if wait, _ := b.Waiting(alice); wait != 10*time.Second {
		t.Errorf("20s in: %v left, want 10s", wait)
	}
	step(10 * time.Second)
	if _, w := b.Waiting(alice); w {
		t.Fatal("the wait should be over")
	}

	b.Posted(alice)
	b.Posted(alice)
	if _, w := b.Waiting(alice); w {
		t.Error("after the wait the count starts again from nothing")
	}

	b2 := NewBursts(3, time.Minute, 30*time.Second)
	b2.now = func() time.Time { return now }
	for range 10 {
		b2.Posted(bob)
		step(31 * time.Second)
		if _, w := b2.Waiting(bob); w {
			t.Fatal("two posts a minute, never three, should never wait")
		}
	}

	b3 := NewBursts(2, time.Minute, 30*time.Second)
	b3.now = func() time.Time { return now }
	for round := range 3 {
		b3.Posted(alice)
		b3.Posted(alice)
		if wait, _ := b3.Waiting(alice); wait != 30*time.Second {
			t.Errorf("round %d: wait %v, want 30s every time", round, wait)
		}
		step(30 * time.Second)
	}

	var off *Bursts
	off.Posted(alice)
	if _, w := off.Waiting(alice); w {
		t.Error("a nil Bursts limits nothing")
	}
	zero := NewBursts(1, time.Minute, 0)
	zero.Posted(alice)
	if _, w := zero.Waiting(alice); w {
		t.Error("a zero wait is no limit")
	}
}
