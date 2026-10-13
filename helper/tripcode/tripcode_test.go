package tripcode

import (
	"crypto/des"
	"strings"
	"testing"
)

func TestCryptCoreIsDES(t *testing.T) {
	for _, key := range []string{"abc", "tea", "12345678", "zz{|}~"} {
		k := make([]byte, 8)
		for i := 0; i < len(key) && i < 8; i++ {
			k[i] = key[i] << 1
		}
		c, err := des.NewCipher(k)
		if err != nil {
			t.Fatal(err)
		}
		want := make([]byte, 8)
		for range 25 {
			c.Encrypt(want, want)
		}
		bits := cryptBlock([]byte(key), "..")
		var got [8]byte
		for i, b := range bits {
			got[i/8] |= b << (7 - i%8)
		}
		if string(got[:]) != string(want) {
			t.Errorf("%q: DES core %x, want %x", key, got, want)
		}
	}
}

func TestCryptMatchesLibc(t *testing.T) {
	for _, c := range []struct{ key, salt, want string }{
		{"abc", "..", "..4WjmMO32dBQ"},
		{"x", "zz", "zzXjar5EX/ECI"},
		{"tea", "ea", "eauWokonZwxw2"},
		{"a", "H.", "H.6ZnBI2EKkq."},
	} {
		if got := Crypt([]byte(c.key), c.salt); got != c.want {
			t.Errorf("crypt(%q, %q) = %q, want %q", c.key, c.salt, got, c.want)
		}
	}
}

func TestMake(t *testing.T) {
	for _, c := range []struct{ secret, want string }{
		{"tea", "WokonZwxw2"},
		{"a", "ZnBI2EKkq."},
		{"faggot", "Ep8pui8Vw2"},
		{"pass word", "WmqpBmd3xM"},
		{"12345678", "WBRXcNtpf."},
		{"123456789", "WBRXcNtpf."},
		{"1234567" + strings.Repeat("&", 100000), Make("1234567&")},
		{"ﾃｽﾄ" + strings.Repeat("あ", 20), Make("ﾃｽﾄあああああ")},
		{"zz{|}~", "NerILmwPnc"},
		{"<script>", "Z.da602.KQ"},
		{`a"b&c`, ".5HnWOZ4h6"},
		{":;<=>?@", "djbe4UiURM"},
		{"[\\]^_`x", "WkozLxYjo."},
		{"ﾃｽﾄ", "kDK4STUJ8A"},
		{"テスト", "SQ2Wyjdi7M"},
		{"é", "cPUZU5OGFs"},
	} {
		if got := Make(c.secret); got != c.want {
			t.Errorf("Make(%q) = %q, want %q", c.secret, got, c.want)
		}
	}
	if Make("") != "" {
		t.Error("an empty secret makes a tripcode")
	}
}

func TestParse(t *testing.T) {
	for _, c := range []struct{ field, name, trip string }{
		{"Anon#tea", "Anon", "WokonZwxw2"},
		{"#tea", "", "WokonZwxw2"},
		{"  Big   Anon \t#tea", "Big Anon", "WokonZwxw2"},
		{"Anon#te#a", "Anon", Make("te#a")},
		{"Anon", "Anon", ""},
		{"Anon#", "Anon", ""},
		{"", "", ""},
		{"Fake◆WokonZwxw2#x", "Fake◇WokonZwxw2", Make("x")},
		{strings.Repeat("n", 40) + "#x", strings.Repeat("n", MaxName), Make("x")},
		{"A​non\x07#tea", "A non", "WokonZwxw2"},
	} {
		name, trip := Parse(c.field)
		if name != c.name || trip != c.trip {
			t.Errorf("Parse(%q) = %q, %q; want %q, %q", c.field, name, trip, c.name, c.trip)
		}
	}
	if got := NameOf("Anon#secret"); got != "Anon" {
		t.Errorf("NameOf keeps %q, want the name without its secret", got)
	}
	if got := CleanName("a\xff\xfeb#1"); got != "ab#1" {
		t.Errorf("CleanName(%q) = %q, want invalid bytes dropped and '#' kept", "a\xff\xfeb#1", got)
	}
}
