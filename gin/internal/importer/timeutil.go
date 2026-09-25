package importer

import "time"

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
