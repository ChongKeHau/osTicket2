// Package importer copies an osTicket MySQL database into the API's PostgreSQL schema.
package importer

import (
	"errors"
	"fmt"
	"regexp"
	"time"
)

var prefixPattern = regexp.MustCompile(`^[A-Za-z0-9_]*$`)

// Options are the import-osticket command's inputs.
type Options struct {
	MySQLDSN string
	Prefix   string
	FilesDir string
	Timezone string
	Batch    int
	DryRun   bool

	loc *time.Location
}

// Validate applies defaults and checks the options. It must run before Location.
func (o *Options) Validate() error {
	if o.MySQLDSN == "" {
		return errors.New("--mysql-dsn is required")
	}
	if o.Prefix == "" {
		o.Prefix = "ost_"
	}
	if !prefixPattern.MatchString(o.Prefix) {
		return fmt.Errorf("--prefix %q must contain only letters, digits and underscores", o.Prefix)
	}
	if o.Timezone == "" {
		o.Timezone = "UTC"
	}
	if o.Batch == 0 {
		o.Batch = 500
	}
	if o.Batch < 1 {
		return errors.New("--batch must be at least 1")
	}
	loc, err := time.LoadLocation(o.Timezone)
	if err != nil {
		return fmt.Errorf("--timezone %q: %w", o.Timezone, err)
	}
	o.loc = loc
	return nil
}

// Location is the zone MySQL datetimes are interpreted in.
func (o *Options) Location() *time.Location {
	if o.loc == nil {
		return time.UTC
	}
	return o.loc
}
