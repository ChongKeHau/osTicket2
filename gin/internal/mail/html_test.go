package mail

import (
	"strings"
	"testing"
)

func TestSanitizeHTML(t *testing.T) {
	in := `<p onclick="x()">Hi <b>there</b></p><script>alert(1)</script><style>p{}</style><a href="javascript:alert(1)">x</a><a href="https://ok">ok</a><img src="JAVASCRIPT:bad" onerror="e()"><iframe src="a"></iframe>`
	out := SanitizeHTML(in)
	for _, bad := range []string{"<script", "alert(1)", "<style", "onclick", "onerror", "javascript:", "JAVASCRIPT:", "<iframe"} {
		if strings.Contains(out, bad) {
			t.Fatalf("output still contains %q: %s", bad, out)
		}
	}
	for _, good := range []string{"<p>", "<b>there</b>", `href="https://ok"`} {
		if !strings.Contains(out, good) {
			t.Fatalf("output lost %q: %s", good, out)
		}
	}
}

func TestHTMLToText(t *testing.T) {
	in := "<p>Hello &amp; welcome</p><div>Line<br>break</div><ul><li>one</li><li>two</li></ul><script>x</script>"
	got := HTMLToText(in)
	want := "Hello & welcome\nLine\nbreak\none\ntwo"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestBodyVars(t *testing.T) {
	text, html := BodyVars("a < b\nsecond", "text")
	if text != "a < b\nsecond" || string(html) != "a &lt; b<br>second" {
		t.Fatalf("text body = %q / %q", text, html)
	}
	text, html = BodyVars("<p>Hi</p><script>x</script>", "html")
	if text != "Hi" || strings.Contains(string(html), "script") || !strings.Contains(string(html), "<p>Hi</p>") {
		t.Fatalf("html body = %q / %q", text, html)
	}
}
