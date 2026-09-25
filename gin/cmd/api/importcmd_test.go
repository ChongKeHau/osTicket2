package main

import (
	"errors"
	"fmt"
	"testing"

	"github.com/grandpine/ticket-api/internal/importer"
)

func TestExitCodeFor(t *testing.T) {
	clean := importer.NewReport()
	attention := importer.NewReport()
	attention.Skip(importer.EntityAttachments, 1, "file not found on disk")
	expected := importer.NewReport()
	expected.Skip(importer.EntityTickets, 1, importer.ReasonDeletedStatus)
	cases := []struct {
		rep  *importer.Report
		err  error
		want int
	}{
		{clean, nil, 0},
		{expected, nil, 0},
		{attention, nil, 3},
		{clean, fmt.Errorf("wrapped: %w", importer.ErrTargetNotEmpty), 2},
		{attention, errors.New("boom"), 1},
		{nil, errors.New("boom"), 1},
	}
	for i, c := range cases {
		if got := exitCodeFor(c.rep, c.err); got != c.want {
			t.Errorf("case %d: got %d want %d", i, got, c.want)
		}
	}
}

func TestImportFlagsRequireDSN(t *testing.T) {
	_, err := parseImportFlags([]string{"--prefix", "x_"})
	if err == nil {
		t.Fatal("missing dsn must fail")
	}
	o, err := parseImportFlags([]string{"--mysql-dsn", "u:p@tcp(h)/d", "--files-dir", "/tmp/f", "--timezone", "UTC", "--batch", "10", "--dry-run"})
	if err != nil || o.FilesDir != "/tmp/f" || o.Batch != 10 || !o.DryRun || o.Prefix != "ost_" {
		t.Fatalf("opts = %+v, %v", o, err)
	}
}
