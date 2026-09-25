package inbound

import (
	"regexp"
	"strings"

	"github.com/grandpine/ticket-api/internal/mail"
)

var (
	rePrefix = regexp.MustCompile(`(?i)^\s*(re|fwd?|aw|wg)\s*:\s*`)
	reTag    = regexp.MustCompile(`\[#([^\]\s]+)\]`)
)

// CleanSubject removes reply/forward prefixes and ticket tags.
func CleanSubject(s string) string {
	s = reTag.ReplaceAllString(s, " ")
	for {
		t := rePrefix.ReplaceAllString(s, "")
		if t == s {
			break
		}
		s = t
	}
	s = strings.Join(strings.Fields(s), " ")
	if s == "" {
		return "(no subject)"
	}
	return s
}

// NumberFromSubject returns the number inside the first [#…] tag.
func NumberFromSubject(s string) string {
	m := reTag.FindStringSubmatch(s)
	if m == nil {
		return ""
	}
	return m[1]
}

// TicketIDsFromRefs extracts our ticket ids from In-Reply-To/References values.
func TicketIDsFromRefs(refs []string) []int64 {
	seen := map[int64]bool{}
	var out []int64
	for _, r := range refs {
		if id, ok := mail.TicketIDFromMessageID(r); ok && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}
