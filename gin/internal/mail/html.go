// Package mail renders notifications, queues them in the outbox, and sends them over SMTP.
package mail

import (
	"html"
	"html/template"
	"regexp"
	"strings"
	"unicode"
)

var (
	reDropElements    = regexp.MustCompile(`(?is)<(script|style|iframe|object|embed|form)\b[^>]*>.*?</\s*(script|style|iframe|object|embed|form)\s*>`)
	reDropSelfClosing = regexp.MustCompile(`(?is)<(script|style|iframe|object|embed|form)\b[^>]*/?>`)
	reOnAttr          = regexp.MustCompile(`(?is)\s+on[a-z]+\s*=\s*("[^"]*"|'[^']*'|[^\s>]+)`)
	reURLAttr         = regexp.MustCompile(`(?is)\s+(href|src)\s*=\s*("[^"]*"|'[^']*'|[^\s>]+)`)
	reBreaks          = regexp.MustCompile(`(?is)<br\s*/?>|</p\s*>|</div\s*>|</li\s*>|</tr\s*>|</h[1-6]\s*>`)
	reTags            = regexp.MustCompile(`(?s)<[^>]+>`)
	reBlankRuns       = regexp.MustCompile(`\n{3,}`)
	reTrailingWS      = regexp.MustCompile(`[ \t]+\n`)
)

// allowedURLSchemes are the only URL schemes left untouched by SanitizeHTML
// in href/src attributes. Anything else (javascript:, data:, vbscript:, ...)
// is rewritten to "#", however it is obfuscated (entities, control
// characters, embedded whitespace).
var allowedURLSchemes = map[string]bool{
	"http": true, "https": true, "mailto": true, "tel": true, "cid": true,
}

// normalizeURLAttr decodes entities and strips control characters and
// whitespace so obfuscated schemes (e.g. "&#106;avascript:", "jav\tascript:")
// collapse to their plain form before the scheme is inspected.
func normalizeURLAttr(raw string) string {
	decoded := html.UnescapeString(raw)
	var b strings.Builder
	for _, r := range decoded {
		if r < 0x20 || r == 0x7f || unicode.IsSpace(r) {
			continue
		}
		b.WriteRune(r)
	}
	return strings.ToLower(b.String())
}

// hasDisallowedScheme reports whether norm (already normalized) names a URL
// scheme that is not in allowedURLSchemes. Values with no scheme (relative
// URLs, anchors, protocol-relative URLs) are never flagged.
func hasDisallowedScheme(norm string) bool {
	colon := strings.IndexByte(norm, ':')
	if colon < 0 {
		return false
	}
	if boundary := strings.IndexAny(norm, "/?#"); boundary >= 0 && boundary < colon {
		return false
	}
	return !allowedURLSchemes[norm[:colon]]
}

// SanitizeHTML strips active content from untrusted HTML. It is a
// defence-in-depth filter for mail bodies (mail clients apply their own), not
// a full HTML sanitiser; the React app sanitises again before rendering.
func SanitizeHTML(s string) string {
	s = reDropElements.ReplaceAllString(s, "")
	s = reDropSelfClosing.ReplaceAllString(s, "")
	s = reOnAttr.ReplaceAllString(s, "")
	s = reURLAttr.ReplaceAllStringFunc(s, func(m string) string {
		sub := reURLAttr.FindStringSubmatch(m)
		name, val := sub[1], sub[2]
		if len(val) >= 2 && (val[0] == '"' || val[0] == '\'') && val[len(val)-1] == val[0] {
			val = val[1 : len(val)-1]
		}
		if hasDisallowedScheme(normalizeURLAttr(val)) {
			return " " + name + `="#"`
		}
		return m
	})
	return s
}

// HTMLToText turns HTML into readable plain text: block ends become newlines,
// tags are dropped, entities decoded, whitespace collapsed.
func HTMLToText(s string) string {
	s = reDropElements.ReplaceAllString(s, "")
	s = reBreaks.ReplaceAllString(s, "\n")
	s = reTags.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = reTrailingWS.ReplaceAllString(s, "\n")
	s = reBlankRuns.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}

// BodyVars derives the text and HTML forms of a thread entry body.
func BodyVars(body, format string) (string, template.HTML) {
	if format == "html" {
		clean := SanitizeHTML(body)
		return HTMLToText(clean), template.HTML(clean)
	}
	escaped := template.HTMLEscapeString(body)
	escaped = strings.ReplaceAll(strings.ReplaceAll(escaped, "\r\n", "\n"), "\n", "<br>")
	return body, template.HTML(escaped)
}
