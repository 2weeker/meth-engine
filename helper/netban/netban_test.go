package netban

import "testing"

func TestNormalize(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"203.0.113.7", "203.0.113.7"},
		{" 203.0.113.7 ", "203.0.113.7"},
		{"::ffff:203.0.113.7", "203.0.113.7"},
		{"203.0.113.77/24", "203.0.113.0/24"},
		{"203.0.113.7/32", "203.0.113.7"},
		{"2001:DB8:1:2:3:4:5:6", "2001:db8:1:2:3:4:5:6"},
		{"2001:db8:1:2:3:4:5:6/64", "2001:db8:1:2::/64"},
		{"2001:db8::1/128", "2001:db8::1"},
		{"::ffff:203.0.113.0/120", "203.0.113.0/24"},
		{"fe80::1%eth0", "fe80::1"},
	} {
		if got, err := Normalize(c.in); err != nil || got != c.want {
			t.Errorf("Normalize(%q) = %q, %v; want %q", c.in, got, err, c.want)
		}
	}
	for _, bad := range []string{"", "banana", "1.2.3.999", "1.2.3.4/33", "10.0.0.0/8", "0.0.0.0/0", "2001:db8::/16", "::/0", "::ffff:1.2.3.4/64", "1.2.3.4/abc"} {
		if got, err := Normalize(bad); err == nil {
			t.Errorf("Normalize(%q) accepted as %q", bad, got)
		}
	}
}

func TestWiden(t *testing.T) {
	if got := Widen("2001:db8:1:2:3:4:5:6", 64); got != "2001:db8:1:2::/64" {
		t.Errorf("Widen v6 = %q", got)
	}
	if got := Widen("203.0.113.7", 64); got != "203.0.113.7" {
		t.Errorf("an IPv4 address is not widened: %q", got)
	}
	if got := Widen("2001:db8::/48", 64); got != "2001:db8::/48" {
		t.Errorf("a range is left alone: %q", got)
	}
}

func TestCovers(t *testing.T) {
	for _, c := range []struct {
		entry, ip string
		want      bool
	}{
		{"203.0.113.7", "203.0.113.7", true},
		{"203.0.113.7", "203.0.113.8", false},
		{"203.0.113.0/24", "203.0.113.200", true},
		{"203.0.113.0/24", "203.0.114.1", false},
		{"203.0.113.0/24", "::ffff:203.0.113.9", true},
		{"2001:db8:1:2::/64", "2001:db8:1:2:ffff::1", true},
		{"2001:db8:1:2::/64", "2001:db8:1:3::1", false},
		{"2001:db8::1", "2001:DB8::1", true},
		{"legacy junk", "legacy junk", true},
		{"legacy junk", "203.0.113.7", false},
		{"203.0.113.0/24", "not an ip", false},
	} {
		if got := Covers(c.entry, c.ip); got != c.want {
			t.Errorf("Covers(%q, %q) = %v, want %v", c.entry, c.ip, got, c.want)
		}
	}
}
