package inbound

import "strings"

// IgnoreReason reports why a message must not become a ticket or reply, or "".
func IgnoreReason(p *Parsed, ownAddress string) string {
	from := strings.ToLower(strings.TrimSpace(p.FromAddress))
	if from == strings.ToLower(strings.TrimSpace(ownAddress)) {
		return "sent by our own address"
	}
	if as := strings.ToLower(p.AutoSubmitted); as != "" && as != "no" {
		return "auto-submitted: " + as
	}
	switch strings.ToLower(p.Precedence) {
	case "bulk", "list", "junk":
		return "precedence: " + strings.ToLower(p.Precedence)
	}
	if s := strings.ToLower(p.AutoResponseSuppress); strings.Contains(s, "all") || strings.Contains(s, "autoreply") {
		return "x-auto-response-suppress"
	}
	local := from
	if at := strings.Index(from, "@"); at >= 0 {
		local = from[:at]
	}
	switch local {
	case "mailer-daemon", "postmaster", "noreply", "no-reply", "donotreply", "do-not-reply":
		return "sender " + local
	}
	if strings.HasPrefix(strings.ToLower(p.ContentType), "multipart/report") {
		return "delivery report"
	}
	return ""
}
