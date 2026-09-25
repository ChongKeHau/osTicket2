package importer

import "testing"

func TestCleanText(t *testing.T) {
	cases := []struct {
		in, want string
		changed  bool
	}{
		{"hello, wörld", "hello, wörld", false},
		{"", "", false},
		{"bad\x00byte", "badbyte", true},
		{"bad\xffbyte", "bad�byte", true},
		{"a\x00b\xff", "ab�", true},
	}
	for _, c := range cases {
		got, changed := cleanText(c.in)
		if got != c.want || changed != c.changed {
			t.Errorf("cleanText(%q) = %q, %v; want %q, %v", c.in, got, changed, c.want, c.changed)
		}
	}
}
