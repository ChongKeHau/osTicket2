package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/grandpine/ticket-api/internal/attachment"
	"github.com/grandpine/ticket-api/internal/db"
	"github.com/grandpine/ticket-api/internal/importer"
)

// exitError carries a process exit code out of run.
type exitError struct {
	code int
	err  error
}

func (e *exitError) Error() string {
	if e.err == nil {
		return fmt.Sprintf("exit %d", e.code)
	}
	return e.err.Error()
}

func (e *exitError) Unwrap() error { return e.err }

// exitCodeFor maps an import outcome to the documented exit codes.
func exitCodeFor(rep *importer.Report, err error) int {
	switch {
	case err != nil && importer.IsPreflight(err):
		return 2
	case err != nil:
		return 1
	case rep != nil && rep.NeedsAttention():
		return 3
	default:
		return 0
	}
}

func parseImportFlags(args []string) (importer.Options, error) {
	fs := flag.NewFlagSet("import-osticket", flag.ContinueOnError)
	var o importer.Options
	fs.StringVar(&o.MySQLDSN, "mysql-dsn", "", "MySQL DSN, e.g. user:pass@tcp(host:3306)/osticket")
	fs.StringVar(&o.Prefix, "prefix", "ost_", "osTicket table prefix")
	fs.StringVar(&o.FilesDir, "files-dir", "", "directory of the osTicket filesystem storage plugin")
	fs.StringVar(&o.Timezone, "timezone", "UTC", "zone that MySQL datetimes are in")
	fs.IntVar(&o.Batch, "batch", 500, "rows per insert batch")
	fs.BoolVar(&o.DryRun, "dry-run", false, "read and validate everything, write nothing")
	if err := fs.Parse(args); err != nil {
		return o, err
	}
	if err := o.Validate(); err != nil {
		return o, err
	}
	return o, nil
}

func importOsticket(ctx context.Context, args []string) error {
	opts, err := parseImportFlags(args)
	if err != nil {
		return &exitError{code: 1, err: err}
	}
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		return &exitError{code: 1, err: errors.New("DATABASE_URL is required")}
	}
	storageDir := os.Getenv("STORAGE_DIR")
	if storageDir == "" {
		storageDir = "./storage"
	}
	pool, err := db.NewPool(ctx, url)
	if err != nil {
		return &exitError{code: 1, err: err}
	}
	defer pool.Close()
	store, err := attachment.NewLocalStorage(storageDir)
	if err != nil {
		return &exitError{code: 1, err: err}
	}
	rep, runErr := importer.Run(ctx, opts, pool, store)
	fmt.Print(rep.String())
	if opts.DryRun && runErr == nil {
		fmt.Println("dry run: nothing was written")
	}
	code := exitCodeFor(rep, runErr)
	if code == 0 {
		return nil
	}
	return &exitError{code: code, err: runErr}
}
