package importer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOptionsValidateDefaults(t *testing.T) {
	o := Options{MySQLDSN: "u:p@tcp(h:3306)/db"}
	if err := o.Validate(); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if o.Prefix != "ost_" || o.Timezone != "UTC" || o.Batch != 500 {
		t.Fatalf("defaults not applied: %+v", o)
	}
	if o.Location().String() != "UTC" {
		t.Fatalf("location = %s", o.Location())
	}
}

func TestOptionsValidateErrors(t *testing.T) {
	file := filepath.Join(t.TempDir(), "plain")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		o    Options
		want string
	}{
		{"empty dsn", Options{}, "--mysql-dsn is required"},
		{"batch", Options{MySQLDSN: "x", Batch: -1}, "--batch must be at least 1"},
		{"tz", Options{MySQLDSN: "x", Timezone: "Mars/Olympus"}, "--timezone"},
		{"prefix", Options{MySQLDSN: "x", Prefix: "ost`;"}, "--prefix"},
		{"files-dir missing", Options{MySQLDSN: "x", FilesDir: filepath.Join(t.TempDir(), "nope")}, "is not a directory"},
		{"files-dir is a file", Options{MySQLDSN: "x", FilesDir: file}, "is not a directory"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.o.Validate()
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("got %v, want containing %q", err, c.want)
			}
		})
	}
}

func TestOptionsFilesDirExists(t *testing.T) {
	o := Options{MySQLDSN: "x", FilesDir: t.TempDir()}
	if err := o.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestOptionsTimezone(t *testing.T) {
	o := Options{MySQLDSN: "x", Timezone: "Asia/Kuala_Lumpur"}
	if err := o.Validate(); err != nil {
		t.Fatal(err)
	}
	if o.Location().String() != "Asia/Kuala_Lumpur" {
		t.Fatalf("location = %s", o.Location())
	}
}
