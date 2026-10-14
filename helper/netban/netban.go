package netban

import (
	"errors"
	"net/netip"
	"strings"
)

const (
	MinBits4 = 16
	MinBits6 = 32
)

var ErrInvalid = errors.New("not an address or a range like 2001:db8::/64")

func Normalize(s string) (string, error) {
	s = strings.TrimSpace(s)
	if !strings.Contains(s, "/") {
		a, err := netip.ParseAddr(s)
		if err != nil {
			return "", ErrInvalid
		}
		return a.WithZone("").Unmap().String(), nil
	}
	p, err := netip.ParsePrefix(s)
	if err != nil {
		return "", ErrInvalid
	}
	a, bits := p.Addr(), p.Bits()
	if a.Is4In6() {
		if bits < 96 {
			return "", ErrInvalid
		}
		a, bits = a.Unmap(), bits-96
	}
	switch {
	case a.Is4() && bits < MinBits4:
		return "", errors.New("an IPv4 range wider than /16 is too broad to ban")
	case a.Is6() && bits < MinBits6:
		return "", errors.New("an IPv6 range wider than /32 is too broad to ban")
	}
	if bits == a.BitLen() {
		return a.String(), nil
	}
	return netip.PrefixFrom(a, bits).Masked().String(), nil
}

func Widen(entry string, bits6 int) string {
	a, err := netip.ParseAddr(entry)
	if err != nil || !a.Is6() {
		return entry
	}
	return netip.PrefixFrom(a, bits6).Masked().String()
}

func Covers(entry, ip string) bool {
	if entry == ip {
		return true
	}
	a, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}
	a = a.WithZone("").Unmap()
	if strings.Contains(entry, "/") {
		p, err := netip.ParsePrefix(entry)
		return err == nil && p.Contains(a)
	}
	e, err := netip.ParseAddr(entry)
	return err == nil && e.WithZone("").Unmap() == a
}
