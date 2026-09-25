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

func TestSanitizeHTMLURLSchemes(t *testing.T) {
	in := `<a href="&#106;avascript:alert(1)">a</a>` +
		`<a href="jav&#x09;ascript:alert(1)">b</a>` +
		"<a href=\"jav\tascript:alert(1)\">c</a>" +
		`<img src="data:text/html;base64,AAAA">` +
		`<a href="vbscript:x">d</a>` +
		`<a href="https://ok">e</a>` +
		`<a href="/relative">f</a>` +
		`<a href="#top">g</a>` +
		`<a href="mailto:a@b.test">h</a>` +
		`<img src="cid:image1">`
	out := SanitizeHTML(in)
	for _, bad := range []string{"javascript:", "vbscript:", "data:text/html"} {
		if strings.Contains(strings.ToLower(out), strings.ToLower(bad)) {
			t.Fatalf("output still contains %q: %s", bad, out)
		}
	}
	for _, good := range []string{
		`href="https://ok"`, `href="/relative"`, `href="#top"`,
		`href="mailto:a@b.test"`, `src="cid:image1"`,
	} {
		if !strings.Contains(out, good) {
			t.Fatalf("output lost %q: %s", good, out)
		}
	}
	// Every dangerous variant must have been rewritten to href/src="#".
	if strings.Count(out, `="#"`) != 5 {
		t.Fatalf("expected 5 rewritten attributes, got: %s", out)
	}
}

// TestSanitizeHTMLSeparatorsAndURLAttributes covers the bypasses found in the
// final review: "/" (or a closing quote) as an attribute separator, URL
// attributes other than href/src, and srcdoc.
func TestSanitizeHTMLSeparatorsAndURLAttributes(t *testing.T) {
	cases := map[string][]string{
		`<svg/onload=alert(1)>`:                                             {"onload", "alert(1)"},
		`<a/href="javascript:alert(1)">x</a>`:                               {"javascript:"},
		`<svg><a xlink:href="javascript:alert(1)">x</a></svg>`:              {"javascript:"},
		`<button formaction="javascript:alert(1)">x</button>`:               {"javascript:"},
		`<iframe srcdoc="&lt;img src=x onerror=alert(1)&gt;"></iframe>`:     {"srcdoc", "onerror"},
		`<div srcdoc="&lt;script&gt;alert(1)&lt;/script&gt;">x</div>`:       {"srcdoc", "alert(1)"},
		`<a href="https://ok"onclick="alert(1)">x</a>`:                      {"onclick", "alert(1)"},
		`<p/ONMOUSEOVER='alert(1)'>x</p>`:                                   {"onmouseover", "alert(1)"},
		`<form><input type=submit formaction=javascript:alert(1)></form>`:   {"javascript:"},
		`<video poster="javascript:alert(1)"></video>`:                      {"javascript:"},
		`<table background="javascript:alert(1)"></table>`:                  {"javascript:"},
		`<object data="javascript:alert(1)"></object><x data="vbscript:y">`: {"javascript:", "vbscript:"},
		`<a/href="https://ok"/onclick="alert(1)"/title="t">x</a>`:           {"onclick", "alert(1)"},
		`<isindex action="javascript:alert(1)">`:                            {"javascript:"},
	}
	for in, bads := range cases {
		out := strings.ToLower(SanitizeHTML(in))
		for _, bad := range bads {
			if strings.Contains(out, bad) {
				t.Errorf("SanitizeHTML(%s) = %s: still contains %q", in, out, bad)
			}
		}
	}
	// Benign content survives, including "/" inside URLs.
	in := `<a href="https://ok.test/a/b?c=d">x</a><img src="cid:img1"/><p class="x">online=yes</p>`
	out := SanitizeHTML(in)
	for _, good := range []string{`href="https://ok.test/a/b?c=d"`, `src="cid:img1"`, `<p class="x">`} {
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
