package importer

import (
	"context"
	"errors"
	"fmt"

	"github.com/grandpine/ticket-api/internal/attachment"
	"github.com/grandpine/ticket-api/internal/db"
)

type step func(ctx context.Context, src *Source, w *Writer, lk *Lookup, rep *Report) error

// Run copies the osTicket database described by opts into target. The report
// is always returned, even when err is not nil, so callers can print progress.
//
// In dry-run mode Sink keeps every step inside one outer transaction and Close
// rolls it back, so the steps read each other's rows exactly as in a real run
// and the report is identical; only the commit is withheld.
func Run(ctx context.Context, opts Options, target db.Beginner, store attachment.Storage) (*Report, error) {
	rep := NewReport()
	if err := opts.Validate(); err != nil {
		return rep, err
	}
	if opts.DryRun {
		store = discardStorage{}
	}
	src, err := OpenSource(ctx, opts.MySQLDSN, opts.Prefix, opts.Location())
	if err != nil {
		return rep, err
	}
	defer src.Close()

	sink := NewSink(target, opts.Batch, opts.DryRun)
	defer sink.Close(ctx)
	if err := sink.Preflight(ctx); err != nil {
		return rep, err
	}
	lk := NewLookup()
	named := []struct {
		name string
		fn   step
	}{
		{"priorities", importPriorities},
		{"statuses", importStatuses},
		{"departments", importDepartmentsPass1},
		{"staff", importStaff},
		{"department managers", importDepartmentsPass2},
		{"topics", importTopics},
		{"tickets", importTickets},
		{"thread entries", importEntries},
		{"files", func(ctx context.Context, src *Source, w *Writer, lk *Lookup, rep *Report) error {
			return importFiles(ctx, src, w, lk, rep, store, opts.FilesDir)
		}},
		{"events", importEvents},
		{"last response", func(ctx context.Context, _ *Source, w *Writer, _ *Lookup, _ *Report) error {
			return setLastResponse(ctx, w)
		}},
	}
	for _, s := range named {
		if err := sink.Step(ctx, func(w *Writer) error { return s.fn(ctx, src, w, lk, rep) }); err != nil {
			return rep, fmt.Errorf("step %s: %w", s.name, err)
		}
	}
	if err := sink.ResetSequences(ctx); err != nil {
		return rep, err
	}
	return rep, nil
}

// IsPreflight reports whether err came from the pre-flight check.
func IsPreflight(err error) bool { return errors.Is(err, ErrTargetNotEmpty) }
