package dubs

import "testing"

func TestCheck(t *testing.T) {
	for _, c := range []struct {
		id      int64
		checked int
		clear   bool
	}{{25565, 1, false}, {399, 2, false}, {300, 2, true}, {333, 3, false}, {3000, 3, true}, {7, 1, false}} {
		got, clear := Check(c.id)
		if got != c.checked || clear != c.clear {
			t.Errorf("%d: want (%d,%v) got (%d,%v)", c.id, c.checked, c.clear, got, clear)
		}
	}
}
