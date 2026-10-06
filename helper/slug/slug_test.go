package slug

import "testing"

func TestMake(t *testing.T) {
	for in, want := range map[string]string{"Tech": "tech", "  Board 4 ": "board-4", "BLAME!": "blame", "///": "", "general": "general"} {
		if got := Make(in); got != want {
			t.Errorf("%q: want %q got %q", in, want, got)
		}
	}
}
