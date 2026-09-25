package importer

import (
	"strings"
	"time"
)

// cleanText makes s storable in a Postgres text column, which (unlike MySQL)
// rejects NUL bytes and invalid UTF-8: NULs are dropped and each invalid
// sequence becomes U+FFFD. The bool reports whether anything changed.
func cleanText(s string) (string, bool) {
	out := strings.ToValidUTF8(strings.ReplaceAll(s, "\x00", ""), "�")
	return out, out != s
}

// textCleaner cleans several fields of one source row and remembers whether
// any of them changed, so the row is noted once.
type textCleaner struct{ changed bool }

func (c *textCleaner) clean(s string) string {
	out, changed := cleanText(s)
	c.changed = c.changed || changed
	return out
}

// note records a "text sanitised" note for the row when a field changed.
func (c *textCleaner) note(rep *Report, e Entity, id int64) {
	if c.changed {
		rep.Note(e, id, "text sanitised")
	}
}

// orZero returns t, or fallback when t is the zero time (MySQL NULL or 0000-00-00).
func orZero(t, fallback time.Time) time.Time {
	if t.IsZero() {
		return fallback
	}
	return t
}

// nullTime turns the zero time into a SQL NULL.
func nullTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}
