// Package mail renders notifications, queues them in the outbox, and sends them over SMTP.
package mail

import (
	"html"
	"html/template"
	"regexp"
	"strings"
)

var (
	reDropElements    = regexp.MustCompile(`(?is)<(script|style|iframe|object|embed|form)\b[^>]*>.*?</\s*(script|style|iframe|object|embed|form)\s*>`)
	reDropSelfClosing = regexp.MustCompile(`(?is)<(script|style|iframe|object|embed|form)\b[^>]*/?>`)
	reOnAttr          = regexp.MustCompile(`(?is)\s+on[a-z]+\s*=\s*("[^"]*"|'[^']*'|[^\s>]+)`)
	reJSURL           = regexp.MustCompile(`(?is)\s+(href|src)\s*=\s*("\s*javascript:[^"]*"|'\s*javascript:[^']*'|javascript:[^\s>]+)`)
	reBreaks          = regexp.MustCompile(`(?is)<br\s*/?>|</p\s*>|</div\s*>|</li\s*>|</tr\s*>|</h[1-6]\s*>`)
	reTags            = regexp.MustCompile(`(?s)<[^>]+>`)
	reBlankRuns       = regexp.MustCompile(`\n{3,}`)
	reTrailingWS      = regexp.MustCompile(`[ \t]+\n`)
)

// SanitizeHTML strips active content from untrusted HTML. It is a
// defence-in-depth filter for mail bodies (mail clients apply their own), not
// a full HTML sanitiser; the React app sanitises again before rendering.
func SanitizeHTML(s string) string {
	s = reDropElements.ReplaceAllString(s, "")
	s = reDropSelfClosing.ReplaceAllString(s, "")
	s = reOnAttr.ReplaceAllString(s, "")
	s = reJSURL.ReplaceAllString(s, ` $1="#"`)
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
