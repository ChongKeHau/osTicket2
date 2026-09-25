# osTicket MySQL Import Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A one-shot `api import-osticket` command that copies an osTicket MySQL database into the Go API's PostgreSQL schema, preserving ids and ticket numbers, copying attachment bytes, and printing a per-entity report.

**Architecture:** A new package `gin/internal/importer` with a MySQL `Source` (plain `database/sql` reads, prefix-aware), a Postgres `Sink` (multi-row inserts with `OVERRIDING SYSTEM VALUE`, one transaction per step, sequence reset at the end, dry-run mode), a `Lookup` of source-to-target id maps, pure mapping functions per entity with their own unit tests, and `Run` orchestrating ten steps. The CLI subcommand parses flags, runs pre-flight, prints the `Report`, and maps outcomes to exit codes.

**Tech Stack:** Go 1.26, pgx v5 (existing), `github.com/go-sql-driver/mysql` (new), testcontainers-go with the existing Postgres module and the new `modules/mysql` module for tests, `golang.org/x/crypto/bcrypt` (existing).

**Spec:** `docs/superpowers/specs/2026-09-25-osticket-import-design.md`

## Global Constraints

- All work is under `gin/`; run `go`/`make` commands from `gin/`. Module path `github.com/grandpine/ticket-api`. Do not touch `gin/db/migrations/*` (frozen).
- New dependencies allowed: exactly `github.com/go-sql-driver/mysql` and `github.com/testcontainers/testcontainers-go/modules/mysql` (v0.44.0, matching the existing Postgres module). Nothing else.
- Target rows are written only with `INSERT … OVERRIDING SYSTEM VALUE` for preserved ids, one transaction per step, committed before the next step.
- Source: a live MySQL connection given by `--mysql-dsn`; table prefix `--prefix` default `ost_`; `--timezone` default `UTC` interprets MySQL datetimes; `--batch` default 500, minimum 1; `--dry-run` writes nothing (no rows, no files).
- Pre-flight refuses a non-empty target: `staff`, `ticket`, `thread_entry`, `file` must have zero rows; `department` and `help_topic` must have exactly the seed row each (`Support`, `General Inquiry`).
- Exit codes: 0 success; 1 aborted on error; 2 pre-flight refused; 3 completed with skipped tickets or attachments for reasons other than `deleted status`.
- Enum values must be exactly the Postgres enums: `ticket_state` (`open`,`resolved`,`closed`), `ticket_source` (`web`,`api`,`phone`,`other`), `thread_entry_type` (`message`,`response`,`note`), `body_format` (`html`,`text`), `ticket_event_kind` (`created`,`assigned`,`unassigned`,`status_changed`,`transferred`,`closed`,`reopened`,`edited`).
- Row-level problems skip the row with a reason recorded in the `Report`; they never abort. Database and storage errors abort the current step and stop.
- Storage keys are 32 random bytes hex-encoded (64 chars), like the upload service; `file.sha256` and `file.size` come from `Storage.Put`.
- `go build ./... && go vet ./... && gofmt -l ./cmd ./internal` clean after every task; `make test` (serialized, several minutes, Docker required) stays strictly above 75% coverage.
- Every commit message ends with `Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>`.

## Review Focus

1. A source with a status named `Resolved` whose osTicket state is `closed` must land in the API's `resolved` state, not `closed` (Task 4 test `TestMapStatus`).
2. A thread entry whose `pid` points to a later id (child inserted before parent) must still get its parent link, not a foreign-key failure (Task 6 test `TestImportEntriesParentLinks`, two-pass update).
3. A ticket whose requester user has no email row at all must import with a placeholder address rather than being dropped or crashing on a nil (Task 5 test `TestMapTicketRequesterFallback`).
4. Running the import twice, or against a database with an admin already created by `create-admin`, must refuse before writing anything (Task 3 test `TestPreflightRefusesNonEmpty`).
5. After import, creating a new ticket through the API must not collide with an imported id (Task 7 test `TestResetSequences`, and the end-to-end assertion in Task 8).

---

### Task 1: Options, Report, Lookup, and dependencies

**Files:**
- Modify: `gin/go.mod`, `gin/go.sum` (via `go get`)
- Create: `gin/internal/importer/options.go`
- Create: `gin/internal/importer/report.go`
- Create: `gin/internal/importer/lookup.go`
- Test: `gin/internal/importer/options_test.go`, `gin/internal/importer/report_test.go`

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces:
  - `type Options struct { MySQLDSN, Prefix, FilesDir, Timezone string; Batch int; DryRun bool }` with `func (o *Options) Validate() error` (defaults `Prefix` to `ost_`, `Timezone` to `UTC`, `Batch` to 500; errors on empty DSN, `Batch < 1`, unknown timezone) and `func (o *Options) Location() *time.Location` (valid after `Validate`).
  - `type Entity string` constants `EntityPriorities, EntityStatuses, EntityDepartments, EntityStaff, EntityTopics, EntityTickets, EntityEntries, EntityFiles, EntityAttachments, EntityEvents`.
  - `type Report` with `NewReport() *Report`, `(r *Report) Read(e Entity)`, `Written(e Entity)`, `Merged(e Entity)`, `Skip(e Entity, id int64, reason string)`, `Counter(e Entity) *Counter` (fields `Read, Written, Merged, Skipped int; Reasons map[string]int; Samples []Sample` where `Sample{ID int64; Reason string}`), `(r *Report) NeedsAttention() bool` (true when tickets were skipped for any reason other than `ReasonDeletedStatus`, or any attachment was skipped), `(r *Report) String() string`.
  - Reason constants: `ReasonDeletedStatus = "deleted status"`, `ReasonFilesDirMissing = "files-dir not given"`.
  - `type IDMap map[int64]int64` and `type Lookup struct { Priorities, Statuses, Departments, Staff, Topics, Tickets, Entries, Files IDMap; DeletedStatus map[int64]bool; DefaultPriority, DefaultDept, DefaultStatus int64 }` with `NewLookup() *Lookup` initialising every map.

- [ ] **Step 1: Add the dependencies**

Run from `gin/`:

```bash
go get github.com/go-sql-driver/mysql@latest
go get github.com/testcontainers/testcontainers-go/modules/mysql@v0.44.0
go mod tidy
```

Expected: `go.mod` gains both requires (the MySQL testcontainer module may sit under `// indirect` until Task 2 imports it; that is fine).

- [ ] **Step 2: Write the failing tests**

Create `gin/internal/importer/options_test.go`:

```go
package importer

import (
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
	cases := []struct {
		name string
		o    Options
		want string
	}{
		{"empty dsn", Options{}, "--mysql-dsn is required"},
		{"batch", Options{MySQLDSN: "x", Batch: -1}, "--batch must be at least 1"},
		{"tz", Options{MySQLDSN: "x", Timezone: "Mars/Olympus"}, "--timezone"},
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

func TestOptionsTimezone(t *testing.T) {
	o := Options{MySQLDSN: "x", Timezone: "Asia/Kuala_Lumpur"}
	if err := o.Validate(); err != nil {
		t.Fatal(err)
	}
	if o.Location().String() != "Asia/Kuala_Lumpur" {
		t.Fatalf("location = %s", o.Location())
	}
}
```

Create `gin/internal/importer/report_test.go`:

```go
package importer

import (
	"strings"
	"testing"
)

func TestReportCountsAndSamples(t *testing.T) {
	r := NewReport()
	r.Read(EntityTickets)
	r.Read(EntityTickets)
	r.Written(EntityTickets)
	r.Skip(EntityTickets, 7, ReasonDeletedStatus)
	for i := int64(0); i < 25; i++ {
		r.Skip(EntityAttachments, i, "file missing")
	}
	c := r.Counter(EntityTickets)
	if c.Read != 2 || c.Written != 1 || c.Skipped != 1 || c.Reasons[ReasonDeletedStatus] != 1 {
		t.Fatalf("ticket counter = %+v", c)
	}
	a := r.Counter(EntityAttachments)
	if a.Skipped != 25 || len(a.Samples) != 20 || a.Samples[0].ID != 0 {
		t.Fatalf("attachment counter = %+v", a)
	}
	if !r.NeedsAttention() {
		t.Fatal("attachment skips should need attention")
	}
}

func TestReportNeedsAttentionOnlyForRealSkips(t *testing.T) {
	r := NewReport()
	r.Skip(EntityTickets, 1, ReasonDeletedStatus)
	r.Skip(EntityEvents, 1, "unmapped event")
	if r.NeedsAttention() {
		t.Fatal("deleted-status tickets and event skips are expected")
	}
	r.Skip(EntityTickets, 2, "unknown department")
	if !r.NeedsAttention() {
		t.Fatal("a ticket skipped for another reason needs attention")
	}
}

func TestReportString(t *testing.T) {
	r := NewReport()
	r.Read(EntityStaff)
	r.Merged(EntityPriorities)
	r.Skip(EntityStaff, 3, "duplicate email")
	s := r.String()
	for _, want := range []string{"entity", "read", "written", "merged", "skipped", "staff", "priorities", "duplicate email", "#3"} {
		if !strings.Contains(s, want) {
			t.Fatalf("report missing %q:\n%s", want, s)
		}
	}
}

func TestLookupInitialised(t *testing.T) {
	lk := NewLookup()
	lk.Staff[1] = 1
	lk.DeletedStatus[4] = true
	if lk.Tickets == nil || lk.Files == nil || len(lk.Staff) != 1 {
		t.Fatal("maps must be initialised")
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `cd gin && go test ./internal/importer/`
Expected: FAIL to compile (undefined `Options`, `NewReport`, …).

- [ ] **Step 4: Write the code**

Create `gin/internal/importer/options.go`:

```go
// Package importer copies an osTicket MySQL database into the API's PostgreSQL schema.
package importer

import (
	"errors"
	"fmt"
	"time"
)

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
```

Create `gin/internal/importer/report.go`:

```go
package importer

import (
	"fmt"
	"sort"
	"strings"
)

// Entity names one imported table group, in report order.
type Entity string

const (
	EntityPriorities  Entity = "priorities"
	EntityStatuses    Entity = "statuses"
	EntityDepartments Entity = "departments"
	EntityStaff       Entity = "staff"
	EntityTopics      Entity = "topics"
	EntityTickets     Entity = "tickets"
	EntityEntries     Entity = "entries"
	EntityFiles       Entity = "files"
	EntityAttachments Entity = "attachments"
	EntityEvents      Entity = "events"
)

var entityOrder = []Entity{
	EntityPriorities, EntityStatuses, EntityDepartments, EntityStaff, EntityTopics,
	EntityTickets, EntityEntries, EntityFiles, EntityAttachments, EntityEvents,
}

// Skip reasons that the report treats as expected rather than problems.
const (
	ReasonDeletedStatus   = "deleted status"
	ReasonFilesDirMissing = "files-dir not given"
)

const maxSamples = 20

// Sample is one skipped source row.
type Sample struct {
	ID     int64
	Reason string
}

// Counter tallies one entity.
type Counter struct {
	Read, Written, Merged, Skipped int
	Reasons                        map[string]int
	Samples                        []Sample
}

// Report collects counters for every entity.
type Report struct {
	counters map[Entity]*Counter
}

// NewReport returns an empty report with a counter per entity.
func NewReport() *Report {
	r := &Report{counters: map[Entity]*Counter{}}
	for _, e := range entityOrder {
		r.counters[e] = &Counter{Reasons: map[string]int{}}
	}
	return r
}

// Counter returns the entity's counter (created on demand for unknown entities).
func (r *Report) Counter(e Entity) *Counter {
	c, ok := r.counters[e]
	if !ok {
		c = &Counter{Reasons: map[string]int{}}
		r.counters[e] = c
	}
	return c
}

func (r *Report) Read(e Entity)    { r.Counter(e).Read++ }
func (r *Report) Written(e Entity) { r.Counter(e).Written++ }
func (r *Report) Merged(e Entity)  { r.Counter(e).Merged++ }

// Skip records a skipped row with its reason, keeping the first samples.
func (r *Report) Skip(e Entity, id int64, reason string) {
	c := r.Counter(e)
	c.Skipped++
	c.Reasons[reason]++
	if len(c.Samples) < maxSamples {
		c.Samples = append(c.Samples, Sample{ID: id, Reason: reason})
	}
}

// NeedsAttention reports whether tickets were skipped for anything other than a
// deleted status, or any attachment was skipped. Those are the exit-code-3 cases.
func (r *Report) NeedsAttention() bool {
	t := r.Counter(EntityTickets)
	for reason, n := range t.Reasons {
		if reason != ReasonDeletedStatus && n > 0 {
			return true
		}
	}
	return r.Counter(EntityAttachments).Skipped > 0
}

// String renders a table plus the skip samples.
func (r *Report) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%-12s %8s %8s %8s %8s\n", "entity", "read", "written", "merged", "skipped")
	for _, e := range entityOrder {
		c := r.counters[e]
		fmt.Fprintf(&b, "%-12s %8d %8d %8d %8d\n", e, c.Read, c.Written, c.Merged, c.Skipped)
	}
	for _, e := range entityOrder {
		c := r.counters[e]
		if c.Skipped == 0 {
			continue
		}
		reasons := make([]string, 0, len(c.Reasons))
		for k := range c.Reasons {
			reasons = append(reasons, k)
		}
		sort.Strings(reasons)
		fmt.Fprintf(&b, "\n%s skipped:\n", e)
		for _, k := range reasons {
			fmt.Fprintf(&b, "  %6d  %s\n", c.Reasons[k], k)
		}
		for _, s := range c.Samples {
			fmt.Fprintf(&b, "    #%d: %s\n", s.ID, s.Reason)
		}
	}
	return b.String()
}
```

Create `gin/internal/importer/lookup.go`:

```go
package importer

// IDMap maps a source (MySQL) id to the target (Postgres) id.
type IDMap map[int64]int64

// Lookup carries the id maps and defaults built by earlier steps for later ones.
type Lookup struct {
	Priorities, Statuses, Departments, Staff, Topics, Tickets, Entries, Files IDMap
	// DeletedStatus marks source status ids whose state is "deleted"; tickets in them are skipped.
	DeletedStatus map[int64]bool
	// Seed ids used as fallbacks: priority "normal", department "Support", status "Open".
	DefaultPriority, DefaultDept, DefaultStatus int64
}

// NewLookup returns a Lookup with every map initialised.
func NewLookup() *Lookup {
	return &Lookup{
		Priorities: IDMap{}, Statuses: IDMap{}, Departments: IDMap{}, Staff: IDMap{},
		Topics: IDMap{}, Tickets: IDMap{}, Entries: IDMap{}, Files: IDMap{},
		DeletedStatus: map[int64]bool{},
	}
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `cd gin && go test ./internal/importer/ && go vet ./internal/importer/`
Expected: PASS (6 tests).

- [ ] **Step 6: Commit**

```bash
git add gin/go.mod gin/go.sum gin/internal/importer/options.go gin/internal/importer/report.go gin/internal/importer/lookup.go gin/internal/importer/options_test.go gin/internal/importer/report_test.go
git commit -m "feat(api): importer options, report, and lookup scaffolding

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 2: MySQL test fixture and the Source reader

**Files:**
- Create: `gin/db/testdata/osticket-mysql/schema.sql`
- Create: `gin/db/testdata/osticket-mysql/fixtures.sql`
- Create: `gin/internal/importer/source.go`
- Create: `gin/internal/importer/mysql_test.go` (test helper starting the container)
- Test: `gin/internal/importer/source_test.go`

**Interfaces:**
- Consumes: `Options.Location()` (Task 1).
- Produces:
  - `type Source struct` with `OpenSource(ctx context.Context, dsn, prefix string, loc *time.Location) (*Source, error)` (forces `parseTime=true` and `loc` on the DSN, pings) and `(s *Source) Close() error`.
  - Row types (all fields exported, MySQL zero dates arrive as zero `time.Time`): `SrcPriority{ID int64; Name string; Urgency int32; Color string}`, `SrcStatus{ID int64; Name, State string; Sort int32}`, `SrcDepartment{ID, ManagerID int64; Name string; IsPublic bool; Created, Updated time.Time}`, `SrcStaff{ID, DeptID int64; Username, Email, Passwd, FirstName, LastName string; IsAdmin, IsActive bool; Created, Updated time.Time}`, `SrcStaffDept{StaffID, DeptID int64}`, `SrcTopic{ID, DeptID, PriorityID int64; Name string; Flags uint32; Sort int32; Created, Updated time.Time}`, `SrcUser{ID, DefaultEmailID int64; Name string}`, `SrcUserEmail{ID, UserID int64; Address string}`, `SrcFormAnswer{TicketID int64; Field, Value string; ValueID *int64}`, `SrcTicket{ID int64; Number string; UserID, UserEmailID, StatusID, DeptID, TopicID, StaffID, SLAID, TeamID, EmailID int64; Flags uint32; IPAddress, Source, SourceExtra string; IsAnswered bool; DueDate, Closed, LastUpdate, Created, Updated time.Time}`, `SrcThread{ID, TicketID int64}`, `SrcEntry{ID, ThreadID, PID, StaffID int64; Type, Poster, Title, Body, Format string; Created, Updated time.Time}`, `SrcAttachment{EntryID, FileID int64; Name string; Inline bool; File SrcFile}`, `SrcFile{ID int64; Backend, Type, Key, Name string; Size int64; Created time.Time}`, `SrcEvent{ID, ThreadID, StaffID int64; Name, Data string; Annulled bool; Timestamp time.Time}`.
  - Readers: `Priorities(ctx) ([]SrcPriority, error)`, `Statuses(ctx) ([]SrcStatus, error)`, `Departments(ctx) ([]SrcDepartment, error)`, `Staff(ctx) ([]SrcStaff, error)`, `StaffDepts(ctx) ([]SrcStaffDept, error)`, `Topics(ctx) ([]SrcTopic, error)`, `Users(ctx) (map[int64]SrcUser, error)`, `UserEmails(ctx) (map[int64]SrcUserEmail, error)`, `FormAnswers(ctx) (map[int64]map[string]SrcFormAnswer, error)` (ticket id → field name → answer, fields `subject` and `priority` only), `Tickets(ctx, fn func(SrcTicket) error) error` (ordered by id), `Threads(ctx) (map[int64]int64, error)` (thread id → ticket id, `object_type = 'T'`), `Entries(ctx, fn func(SrcEntry) error) error` (ordered by thread id, id), `Attachments(ctx, fn func(SrcAttachment) error) error` (type `H`, joined to `file`, ordered by entry id), `OpenChunks(ctx, fileID int64) (io.ReadCloser, error)` (concatenates `file_chunk` rows ordered by `chunk_id`), `Events(ctx, fn func(SrcEvent) error) error` (joined to `event`, ordered by timestamp, id).
  - Test helper (test-only): `mysqlDSN(t *testing.T) string` starting one `mysql:8.0` container per test binary loaded with the two SQL files, and `fixturePrefix = "ost_"`.

- [ ] **Step 1: Write the MySQL schema fixture**

Create `gin/db/testdata/osticket-mysql/schema.sql` (trimmed from osTicket 1.18 `install-mysql.sql`, prefix `ost_`, only the columns the importer reads plus NOT NULL ones):

```sql
CREATE TABLE ost_ticket_priority (
  priority_id tinyint(4) NOT NULL AUTO_INCREMENT,
  priority varchar(60) NOT NULL DEFAULT '',
  priority_desc varchar(30) NOT NULL DEFAULT '',
  priority_color varchar(7) NOT NULL DEFAULT '',
  priority_urgency tinyint(1) unsigned NOT NULL DEFAULT 0,
  ispublic tinyint(1) NOT NULL DEFAULT 1,
  PRIMARY KEY (priority_id)
);
CREATE TABLE ost_ticket_status (
  id int(11) NOT NULL AUTO_INCREMENT,
  name varchar(60) NOT NULL DEFAULT '',
  state varchar(16) DEFAULT NULL,
  mode int(11) unsigned NOT NULL DEFAULT 0,
  flags int(11) unsigned NOT NULL DEFAULT 0,
  sort int(11) unsigned NOT NULL DEFAULT 0,
  properties text NOT NULL,
  created datetime NOT NULL,
  updated datetime NOT NULL,
  PRIMARY KEY (id)
);
CREATE TABLE ost_department (
  id int(11) unsigned NOT NULL AUTO_INCREMENT,
  pid int(11) unsigned DEFAULT NULL,
  manager_id int(10) unsigned NOT NULL DEFAULT 0,
  flags int(10) unsigned NOT NULL DEFAULT 0,
  name varchar(128) NOT NULL DEFAULT '',
  signature text NOT NULL,
  ispublic tinyint(1) unsigned NOT NULL DEFAULT 1,
  updated datetime NOT NULL,
  created datetime NOT NULL,
  PRIMARY KEY (id)
);
CREATE TABLE ost_staff (
  staff_id int(11) unsigned NOT NULL AUTO_INCREMENT,
  dept_id int(10) unsigned NOT NULL DEFAULT 0,
  role_id int(10) unsigned NOT NULL DEFAULT 0,
  username varchar(32) NOT NULL DEFAULT '',
  firstname varchar(32) DEFAULT NULL,
  lastname varchar(32) DEFAULT NULL,
  passwd varchar(128) DEFAULT NULL,
  backend varchar(32) DEFAULT NULL,
  email varchar(255) DEFAULT NULL,
  signature text NOT NULL,
  isactive tinyint(1) NOT NULL DEFAULT 1,
  isadmin tinyint(1) NOT NULL DEFAULT 0,
  created datetime NOT NULL,
  updated datetime NOT NULL,
  PRIMARY KEY (staff_id)
);
CREATE TABLE ost_staff_dept_access (
  staff_id int(10) unsigned NOT NULL DEFAULT 0,
  dept_id int(10) unsigned NOT NULL DEFAULT 0,
  role_id int(10) unsigned NOT NULL DEFAULT 0,
  flags int(10) unsigned NOT NULL DEFAULT 1,
  PRIMARY KEY (staff_id, dept_id)
);
CREATE TABLE ost_help_topic (
  topic_id int(11) unsigned NOT NULL AUTO_INCREMENT,
  topic_pid int(10) unsigned NOT NULL DEFAULT 0,
  ispublic tinyint(1) unsigned NOT NULL DEFAULT 1,
  flags int(10) unsigned DEFAULT 0,
  status_id int(10) unsigned NOT NULL DEFAULT 0,
  priority_id int(10) unsigned NOT NULL DEFAULT 0,
  dept_id int(10) unsigned NOT NULL DEFAULT 0,
  staff_id int(10) unsigned NOT NULL DEFAULT 0,
  sort int(10) unsigned NOT NULL DEFAULT 0,
  topic varchar(128) NOT NULL DEFAULT '',
  created datetime NOT NULL,
  updated datetime NOT NULL,
  PRIMARY KEY (topic_id)
);
CREATE TABLE ost_user (
  id int(10) unsigned NOT NULL AUTO_INCREMENT,
  org_id int(10) unsigned NOT NULL DEFAULT 0,
  default_email_id int(10) NOT NULL DEFAULT 0,
  status int(11) unsigned NOT NULL DEFAULT 0,
  name varchar(128) NOT NULL,
  created datetime NOT NULL,
  updated datetime NOT NULL,
  PRIMARY KEY (id)
);
CREATE TABLE ost_user_email (
  id int(10) unsigned NOT NULL AUTO_INCREMENT,
  user_id int(10) unsigned NOT NULL,
  flags int(10) unsigned NOT NULL DEFAULT 0,
  address varchar(255) NOT NULL,
  PRIMARY KEY (id)
);
CREATE TABLE ost_form (
  id int(11) unsigned NOT NULL AUTO_INCREMENT,
  pid int(10) unsigned DEFAULT NULL,
  type varchar(8) NOT NULL DEFAULT 'G',
  flags int(10) unsigned NOT NULL DEFAULT 1,
  title varchar(255) NOT NULL,
  PRIMARY KEY (id)
);
CREATE TABLE ost_form_field (
  id int(11) unsigned NOT NULL AUTO_INCREMENT,
  form_id int(10) unsigned NOT NULL,
  flags int(10) unsigned DEFAULT 1,
  type varchar(255) NOT NULL DEFAULT 'text',
  label varchar(255) NOT NULL,
  name varchar(64) NOT NULL,
  sort int(11) unsigned NOT NULL DEFAULT 0,
  PRIMARY KEY (id)
);
CREATE TABLE ost_form_entry (
  id int(11) unsigned NOT NULL AUTO_INCREMENT,
  form_id int(11) unsigned NOT NULL,
  object_id int(11) unsigned DEFAULT NULL,
  object_type char(1) NOT NULL DEFAULT 'T',
  sort int(11) unsigned NOT NULL DEFAULT 1,
  PRIMARY KEY (id)
);
CREATE TABLE ost_form_entry_values (
  entry_id int(11) unsigned NOT NULL,
  field_id int(11) unsigned NOT NULL,
  value text,
  value_id int(11),
  PRIMARY KEY (entry_id, field_id)
);
CREATE TABLE ost_ticket (
  ticket_id int(11) unsigned NOT NULL AUTO_INCREMENT,
  ticket_pid int(11) unsigned DEFAULT NULL,
  number varchar(20),
  user_id int(11) unsigned NOT NULL DEFAULT 0,
  user_email_id int(11) unsigned NOT NULL DEFAULT 0,
  status_id int(10) unsigned NOT NULL DEFAULT 0,
  dept_id int(10) unsigned NOT NULL DEFAULT 0,
  sla_id int(10) unsigned NOT NULL DEFAULT 0,
  topic_id int(10) unsigned NOT NULL DEFAULT 0,
  staff_id int(10) unsigned NOT NULL DEFAULT 0,
  team_id int(10) unsigned NOT NULL DEFAULT 0,
  email_id int(11) unsigned NOT NULL DEFAULT 0,
  flags int(10) unsigned NOT NULL DEFAULT 0,
  ip_address varchar(64) NOT NULL DEFAULT '',
  source enum('Web','Email','Phone','API','Other') NOT NULL DEFAULT 'Other',
  source_extra varchar(40) DEFAULT NULL,
  isoverdue tinyint(1) unsigned NOT NULL DEFAULT 0,
  isanswered tinyint(1) unsigned NOT NULL DEFAULT 0,
  duedate datetime DEFAULT NULL,
  closed datetime DEFAULT NULL,
  lastupdate datetime DEFAULT NULL,
  created datetime NOT NULL,
  updated datetime NOT NULL,
  PRIMARY KEY (ticket_id)
);
CREATE TABLE ost_thread (
  id int(11) unsigned NOT NULL AUTO_INCREMENT,
  object_id int(11) unsigned NOT NULL,
  object_type char(1) NOT NULL,
  extra text,
  lastresponse datetime DEFAULT NULL,
  lastmessage datetime DEFAULT NULL,
  created datetime NOT NULL,
  PRIMARY KEY (id)
);
CREATE TABLE ost_thread_entry (
  id int(11) unsigned NOT NULL AUTO_INCREMENT,
  pid int(11) unsigned NOT NULL DEFAULT 0,
  thread_id int(11) unsigned NOT NULL DEFAULT 0,
  staff_id int(11) unsigned NOT NULL DEFAULT 0,
  user_id int(11) unsigned NOT NULL DEFAULT 0,
  type char(1) NOT NULL DEFAULT '',
  flags int(11) unsigned NOT NULL DEFAULT 0,
  poster varchar(128) NOT NULL DEFAULT '',
  source varchar(32) NOT NULL DEFAULT '',
  title varchar(255),
  body text NOT NULL,
  format varchar(16) NOT NULL DEFAULT 'html',
  ip_address varchar(64) NOT NULL DEFAULT '',
  created datetime NOT NULL,
  updated datetime NOT NULL,
  PRIMARY KEY (id)
);
CREATE TABLE ost_attachment (
  id int(10) unsigned NOT NULL AUTO_INCREMENT,
  object_id int(11) unsigned NOT NULL,
  type char(1) NOT NULL,
  file_id int(11) unsigned NOT NULL,
  name varchar(255) DEFAULT NULL,
  inline tinyint(1) unsigned NOT NULL DEFAULT 0,
  lang varchar(16),
  PRIMARY KEY (id)
);
CREATE TABLE ost_file (
  id int(11) NOT NULL AUTO_INCREMENT,
  ft char(1) NOT NULL DEFAULT 'T',
  bk char(1) NOT NULL DEFAULT 'D',
  type varchar(255) NOT NULL DEFAULT '',
  size bigint(20) unsigned NOT NULL DEFAULT 0,
  `key` varchar(86) NOT NULL,
  signature varchar(86) NOT NULL,
  name varchar(255) NOT NULL DEFAULT '',
  attrs varchar(255),
  created datetime NOT NULL,
  PRIMARY KEY (id)
);
CREATE TABLE ost_file_chunk (
  file_id int(11) NOT NULL,
  chunk_id int(11) NOT NULL,
  filedata longblob NOT NULL,
  PRIMARY KEY (file_id, chunk_id)
);
CREATE TABLE ost_event (
  id int(10) unsigned NOT NULL AUTO_INCREMENT,
  name varchar(60) NOT NULL,
  description varchar(60) DEFAULT NULL,
  PRIMARY KEY (id)
);
CREATE TABLE ost_thread_event (
  id int(10) unsigned NOT NULL AUTO_INCREMENT,
  thread_id int(11) unsigned NOT NULL DEFAULT 0,
  thread_type char(1) NOT NULL DEFAULT '',
  event_id int(11) unsigned DEFAULT NULL,
  staff_id int(11) unsigned NOT NULL,
  team_id int(11) unsigned NOT NULL,
  dept_id int(11) unsigned NOT NULL,
  topic_id int(11) unsigned NOT NULL,
  data varchar(1024) DEFAULT NULL,
  username varchar(128) NOT NULL DEFAULT 'SYSTEM',
  uid int(11) unsigned DEFAULT NULL,
  uid_type char(1) NOT NULL DEFAULT 'S',
  annulled tinyint(1) unsigned NOT NULL DEFAULT 0,
  timestamp datetime NOT NULL,
  PRIMARY KEY (id)
);
```

- [ ] **Step 2: Write the fixture rows**

Create `gin/db/testdata/osticket-mysql/fixtures.sql`. The password hash below is a cost-10 bcrypt hash of `agentpass1` in PHP's `$2y$` form; Go's bcrypt verifies it (checked when the plan was written):

```sql
INSERT INTO ost_ticket_priority VALUES
  (1,'Low','Low','#DDFFDD',1,1),
  (2,'Normal','Normal','#FFFFFF',2,1),
  (3,'High','High','#FEE7E7',3,1),
  (4,'Emergency','Emergency','#FEE7E7',4,1),
  (5,'VIP','Very important','#FFD700',5,0);
INSERT INTO ost_ticket_status (id,name,state,mode,flags,sort,properties,created,updated) VALUES
  (1,'Open','open',3,0,1,'{}','2020-01-01 00:00:00','2020-01-01 00:00:00'),
  (2,'Resolved','closed',3,0,2,'{}','2020-01-01 00:00:00','2020-01-01 00:00:00'),
  (3,'Closed','closed',3,0,3,'{}','2020-01-01 00:00:00','2020-01-01 00:00:00'),
  (4,'Archived','archived',3,0,4,'{}','2020-01-01 00:00:00','2020-01-01 00:00:00'),
  (5,'Deleted','deleted',3,0,5,'{}','2020-01-01 00:00:00','2020-01-01 00:00:00'),
  (6,'Waiting','open',3,0,6,'{}','2020-01-01 00:00:00','2020-01-01 00:00:00');
INSERT INTO ost_department (id,pid,manager_id,flags,name,signature,ispublic,updated,created) VALUES
  (1,NULL,1,0,'Support','',1,'2020-01-01 00:00:00','2020-01-01 00:00:00'),
  (2,NULL,0,0,'Billing','',0,'2020-01-02 00:00:00','2020-01-02 00:00:00'),
  (3,1,0,0,'billing ','',1,'2020-01-03 00:00:00','2020-01-03 00:00:00');
INSERT INTO ost_staff (staff_id,dept_id,role_id,username,firstname,lastname,passwd,backend,email,signature,isactive,isadmin,created,updated) VALUES
  (1,1,1,'admin','Ada','Admin','$2y$10$uBGMLSBZVCGO.AIPCUG6V.pCoB3BL30EoiTv11HjETcXGxkjbyl.a','local','ada@example.test','',1,1,'2020-01-01 00:00:00','2020-01-01 00:00:00'),
  (2,2,2,'bob','Bob','Billing','$2y$10$uBGMLSBZVCGO.AIPCUG6V.pCoB3BL30EoiTv11HjETcXGxkjbyl.a','local','ADA@example.test','',1,0,'2020-01-02 00:00:00','2020-01-02 00:00:00'),
  (3,9,2,'ldapuser','Lea','Ldap','','ldap',NULL,'',0,0,'2020-01-03 00:00:00','2020-01-03 00:00:00');
INSERT INTO ost_staff_dept_access VALUES (1,2,1,1),(2,1,2,1),(2,9,2,1);
INSERT INTO ost_help_topic (topic_id,topic_pid,ispublic,flags,status_id,priority_id,dept_id,staff_id,sort,topic,created,updated) VALUES
  (1,0,1,2,0,2,1,0,1,'General Inquiry','2020-01-01 00:00:00','2020-01-01 00:00:00'),
  (2,0,1,0,0,3,2,0,2,'Refunds','2020-01-01 00:00:00','2020-01-01 00:00:00'),
  (3,0,1,2,0,0,9,0,3,'Orphan','2020-01-01 00:00:00','2020-01-01 00:00:00');
INSERT INTO ost_user (id,org_id,default_email_id,status,name,created,updated) VALUES
  (1,0,1,0,'Pat Requester','2020-01-01 00:00:00','2020-01-01 00:00:00'),
  (2,0,2,0,'No Email','2020-01-01 00:00:00','2020-01-01 00:00:00'),
  (3,0,3,0,'Fallback Person','2020-01-01 00:00:00','2020-01-01 00:00:00');
INSERT INTO ost_user_email (id,user_id,flags,address) VALUES
  (1,1,0,'pat@example.test'),
  (3,3,0,'fallback@example.test');
INSERT INTO ost_form (id,pid,type,flags,title) VALUES (2,NULL,'T',1,'Ticket Details');
INSERT INTO ost_form_field (id,form_id,flags,type,label,name,sort) VALUES
  (20,2,1,'text','Issue Summary','subject',1),
  (22,2,1,'priority','Priority Level','priority',2);
INSERT INTO ost_form_entry (id,form_id,object_id,object_type,sort) VALUES
  (100,2,1,'T',1),(101,2,2,'T',1),(102,2,3,'T',1),(103,2,4,'T',1),(104,2,5,'T',1);
INSERT INTO ost_form_entry_values (entry_id,field_id,value,value_id) VALUES
  (100,20,'Printer on fire',NULL),(100,22,'{"3":"High"}',3),
  (101,20,'Refund please',NULL),(101,22,'{"2":"Normal"}',2),
  (102,20,'',NULL),
  (103,20,'Deleted ticket',NULL),(103,22,'{"1":"Low"}',1),
  (104,20,'Unknown dept',NULL);
INSERT INTO ost_ticket (ticket_id,ticket_pid,number,user_id,user_email_id,status_id,dept_id,sla_id,topic_id,staff_id,team_id,email_id,flags,ip_address,source,source_extra,isoverdue,isanswered,duedate,closed,lastupdate,created,updated) VALUES
  (1,NULL,'100001',1,1,1,1,1,1,1,0,0,0,'10.0.0.1','Web',NULL,0,0,'2020-02-01 09:00:00',NULL,'2020-01-10 12:00:00','2020-01-10 10:00:00','2020-01-10 12:00:00'),
  (2,NULL,'100002',3,0,2,2,0,2,0,0,0,0,'','Email',NULL,0,1,NULL,'2020-01-12 15:00:00','2020-01-12 15:00:00','2020-01-11 10:00:00','2020-01-12 15:00:00'),
  (3,NULL,'',2,0,4,1,0,0,0,0,0,0,'','Phone','ext 12',0,0,NULL,'2020-01-13 15:00:00',NULL,'2020-01-13 10:00:00','2020-01-13 15:00:00'),
  (4,NULL,'100004',1,1,5,1,0,0,0,0,0,0,'','Web',NULL,0,0,NULL,NULL,NULL,'2020-01-14 10:00:00','2020-01-14 10:00:00'),
  (5,NULL,'100001',1,1,6,9,0,3,9,0,0,0,'','API',NULL,0,0,NULL,NULL,NULL,'2020-01-15 10:00:00','2020-01-15 10:00:00');
INSERT INTO ost_thread (id,object_id,object_type,extra,lastresponse,lastmessage,created) VALUES
  (10,1,'T',NULL,NULL,NULL,'2020-01-10 10:00:00'),
  (20,2,'T',NULL,NULL,NULL,'2020-01-11 10:00:00'),
  (30,3,'T',NULL,NULL,NULL,'2020-01-13 10:00:00'),
  (40,4,'T',NULL,NULL,NULL,'2020-01-14 10:00:00'),
  (50,5,'T',NULL,NULL,NULL,'2020-01-15 10:00:00');
INSERT INTO ost_thread_entry (id,pid,thread_id,staff_id,user_id,type,flags,poster,source,title,body,format,ip_address,created,updated) VALUES
  (1,0,10,0,1,'M',0,'Pat Requester','Web','Printer on fire','<p>Help, smoke everywhere</p>','html','','2020-01-10 10:00:00','2020-01-10 10:00:00'),
  (2,1,10,1,0,'R',0,'Ada Admin','Web',NULL,'On my way','text','','2020-01-10 11:00:00','2020-01-10 11:00:00'),
  (3,0,10,1,0,'N',0,'Ada Admin','Web','Ops note','Ordered extinguisher','text','','2020-01-10 11:30:00','2020-01-10 11:30:00'),
  (4,5,10,0,1,'M',0,'Pat Requester','Web',NULL,'Child before parent','text','','2020-01-10 12:00:00','2020-01-10 12:00:00'),
  (5,0,10,1,0,'R',0,'Ada Admin','Web',NULL,'Parent with higher id','markdown','','2020-01-10 12:30:00','2020-01-10 12:30:00'),
  (6,0,20,0,3,'M',0,'Fallback Person','Email',NULL,'Refund please','text','','2020-01-11 10:00:00','2020-01-11 10:00:00'),
  (7,0,20,2,0,'R',0,'Bob Billing','Web',NULL,'Refunded','text','','2020-01-12 15:00:00','2020-01-12 15:00:00'),
  (8,0,30,0,2,'M',0,'No Email','Phone',NULL,'Called in','text','','2020-01-13 10:00:00','2020-01-13 10:00:00'),
  (9,0,40,0,1,'M',0,'Pat Requester','Web',NULL,'Deleted body','text','','2020-01-14 10:00:00','2020-01-14 10:00:00'),
  (11,0,10,0,0,'X',0,'System','Web',NULL,'Unknown type','text','','2020-01-10 13:00:00','2020-01-10 13:00:00');
INSERT INTO ost_file (id,ft,bk,type,size,`key`,signature,name,attrs,created) VALUES
  (1,'T','D','text/plain',11,'chunkedkey1','sig1','notes.txt',NULL,'2020-01-10 11:30:00'),
  (2,'T','F','image/png',4,'fskey2','sig2','pixel.png',NULL,'2020-01-12 15:00:00'),
  (3,'T','S','application/pdf',9,'s3key3','sig3','remote.pdf',NULL,'2020-01-12 15:00:00'),
  (4,'T','F','text/plain',5,'missingkey4','sig4','gone.txt',NULL,'2020-01-12 15:00:00');
INSERT INTO ost_file_chunk VALUES (1,0,'hello '),(1,1,'world');
INSERT INTO ost_attachment (id,object_id,type,file_id,name,inline,lang) VALUES
  (1,3,'H',1,'renamed-notes.txt',0,NULL),
  (2,7,'H',2,NULL,1,NULL),
  (3,7,'H',3,NULL,0,NULL),
  (4,7,'H',4,NULL,0,NULL),
  (5,9,'H',1,NULL,0,NULL),
  (6,3,'H',1,'dup-of-same-file.txt',0,NULL),
  (7,99,'F',1,NULL,0,NULL);
INSERT INTO ost_event (id,name,description) VALUES
  (1,'created',NULL),(2,'closed',NULL),(3,'reopened',NULL),(4,'assigned',NULL),(5,'transferred',NULL),(6,'edited',NULL),(7,'viewed',NULL),(8,'released',NULL);
INSERT INTO ost_thread_event (id,thread_id,thread_type,event_id,staff_id,team_id,dept_id,topic_id,data,username,uid,uid_type,annulled,timestamp) VALUES
  (1,10,'T',1,0,0,1,1,NULL,'SYSTEM',NULL,'S',0,'2020-01-10 10:00:00'),
  (2,10,'T',4,1,0,1,1,'{"staff":1}','admin',1,'S',0,'2020-01-10 10:30:00'),
  (3,10,'T',5,1,0,1,1,'{"dept":2}','admin',1,'S',0,'2020-01-10 10:40:00'),
  (4,10,'T',7,1,0,1,1,NULL,'admin',1,'S',0,'2020-01-10 10:50:00'),
  (5,10,'T',4,1,0,1,1,'{"staff":0}','admin',1,'S',0,'2020-01-10 10:55:00'),
  (6,10,'T',6,1,0,1,1,'not json','admin',1,'S',0,'2020-01-10 10:56:00'),
  (7,20,'T',2,2,0,2,2,NULL,'bob',2,'S',0,'2020-01-12 15:00:00'),
  (8,20,'T',3,2,0,2,2,NULL,'bob',2,'S',1,'2020-01-12 16:00:00'),
  (9,40,'T',1,0,0,1,0,NULL,'SYSTEM',NULL,'S',0,'2020-01-14 10:00:00'),
  (10,10,'T',8,1,0,1,1,NULL,'admin',1,'S',0,'2020-01-10 10:57:00');
```

Fixture intent, for the tests that follow: priorities 1–4 merge with the seed, 5 is new. Statuses: Resolved (state closed) must become `resolved`; Archived → `closed`; Deleted marks ticket 4 as skipped; Waiting is a new `open` status. Departments: Support merges with the seed; `billing ` trims to a duplicate of Billing and becomes `Billing (2)`; department 3's parent is dropped. Staff: bob's email duplicates ada's case-insensitively → placeholder; ldapuser has an empty password, unknown dept 9 → seed dept, and an access row to unknown dept 9. Topics: General Inquiry merges; Refunds inactive (flags 0); Orphan's dept 9 → null. Tickets: 1 normal with a due date; 2 resolved with no `user_email_id` (falls back to default email) and `Email` source; 3 archived with empty number, empty subject, requester with no email row; 4 deleted → skipped; 5 duplicate number, unknown dept and staff, topic 3, and status Waiting. Entries: 4 has a parent (5) with a higher id; 5 has format `markdown` → `html`; 11 has an unknown type → skipped; 9 belongs to the deleted ticket. Files: 1 chunked in two chunks (`hello world`), 2 on disk (the test writes 4 bytes to `<files-dir>/fskey2`), 3 backend `S` → skipped, 4 on disk but missing → skipped; attachment 5 belongs to the deleted ticket's entry; attachment 6 reuses file 1 on the same entry (one `file` row, second `attachment` row); attachment 7 is not type `H`. Events: viewed and released are unmapped, event 5 is an unassign, event 6 has non-JSON data, event 8 is annulled, event 9 belongs to the deleted ticket.

- [ ] **Step 3: Write the container helper and the failing source test**

Create `gin/internal/importer/mysql_test.go`:

```go
package importer

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/grandpine/ticket-api/internal/db/testutil"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/mysql"
	"github.com/testcontainers/testcontainers-go/wait"
)

const fixturePrefix = "ost_"

var (
	mysqlOnce sync.Once
	mysqlDSNv string
	mysqlErr  error
)

// mysqlDSN starts one MySQL container per test binary, loaded with the osTicket fixture.
func mysqlDSN(t *testing.T) string {
	t.Helper()
	mysqlOnce.Do(func() {
		ctx := context.Background()
		mig, err := testutil.MigrationsDir()
		if err != nil {
			mysqlErr = err
			return
		}
		data := filepath.Join(filepath.Dir(mig), "testdata", "osticket-mysql")
		ctr, err := mysql.Run(ctx, "mysql:8.0",
			mysql.WithDatabase("osticket"),
			mysql.WithUsername("ost"),
			mysql.WithPassword("ost"),
			mysql.WithScripts(filepath.Join(data, "schema.sql"), filepath.Join(data, "fixtures.sql")),
			testcontainers.WithWaitStrategy(wait.ForLog("port: 3306  MySQL Community Server").WithStartupTimeout(180*time.Second)),
		)
		if err != nil {
			mysqlErr = err
			return
		}
		mysqlDSNv, mysqlErr = ctr.ConnectionString(ctx)
	})
	if mysqlErr != nil {
		t.Fatalf("mysql container: %v", mysqlErr)
	}
	return mysqlDSNv
}

func openTestSource(t *testing.T) *Source {
	t.Helper()
	s, err := OpenSource(context.Background(), mysqlDSN(t), fixturePrefix, time.UTC)
	if err != nil {
		t.Fatalf("open source: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}
```

Create `gin/internal/importer/source_test.go`:

```go
package importer

import (
	"context"
	"io"
	"testing"
	"time"
)

func TestSourceReadsReferenceTables(t *testing.T) {
	s := openTestSource(t)
	ctx := context.Background()
	pr, err := s.Priorities(ctx)
	if err != nil || len(pr) != 5 || pr[4].Name != "VIP" || pr[4].Urgency != 5 {
		t.Fatalf("priorities = %+v, %v", pr, err)
	}
	st, err := s.Statuses(ctx)
	if err != nil || len(st) != 6 || st[1].State != "closed" || st[1].Name != "Resolved" {
		t.Fatalf("statuses = %+v, %v", st, err)
	}
	de, err := s.Departments(ctx)
	if err != nil || len(de) != 3 || de[0].ManagerID != 1 || de[2].Name != "billing " || de[1].IsPublic {
		t.Fatalf("departments = %+v, %v", de, err)
	}
	sf, err := s.Staff(ctx)
	if err != nil || len(sf) != 3 || sf[2].Email != "" || sf[2].Passwd != "" || !sf[0].IsAdmin || sf[2].IsActive {
		t.Fatalf("staff = %+v, %v", sf, err)
	}
	sd, err := s.StaffDepts(ctx)
	if err != nil || len(sd) != 3 {
		t.Fatalf("staff depts = %+v, %v", sd, err)
	}
	tp, err := s.Topics(ctx)
	if err != nil || len(tp) != 3 || tp[0].Flags != 2 || tp[1].PriorityID != 3 {
		t.Fatalf("topics = %+v, %v", tp, err)
	}
}

func TestSourceReadsTicketsAndForms(t *testing.T) {
	s := openTestSource(t)
	ctx := context.Background()
	users, err := s.Users(ctx)
	if err != nil || len(users) != 3 || users[3].DefaultEmailID != 3 {
		t.Fatalf("users = %+v, %v", users, err)
	}
	emails, err := s.UserEmails(ctx)
	if err != nil || len(emails) != 2 || emails[1].Address != "pat@example.test" {
		t.Fatalf("emails = %+v, %v", emails, err)
	}
	forms, err := s.FormAnswers(ctx)
	if err != nil || forms[1]["subject"].Value != "Printer on fire" || forms[1]["priority"].ValueID == nil || *forms[1]["priority"].ValueID != 3 {
		t.Fatalf("forms = %+v, %v", forms, err)
	}
	if _, ok := forms[3]["priority"]; ok {
		t.Fatal("ticket 3 has no priority answer")
	}
	var tickets []SrcTicket
	if err := s.Tickets(ctx, func(tk SrcTicket) error { tickets = append(tickets, tk); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(tickets) != 5 || tickets[0].Number != "100001" || tickets[0].Source != "Web" || tickets[1].Closed.IsZero() || !tickets[0].Closed.IsZero() {
		t.Fatalf("tickets = %+v", tickets)
	}
	want := time.Date(2020, 2, 1, 9, 0, 0, 0, time.UTC)
	if !tickets[0].DueDate.Equal(want) {
		t.Fatalf("due = %v, want %v", tickets[0].DueDate, want)
	}
	if tickets[2].SourceExtra != "ext 12" || tickets[1].UserEmailID != 0 {
		t.Fatalf("tickets = %+v", tickets)
	}
}

func TestSourceTimezoneInterpretsDatetimes(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Kuala_Lumpur")
	s, err := OpenSource(context.Background(), mysqlDSN(t), fixturePrefix, loc)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var first SrcTicket
	_ = s.Tickets(context.Background(), func(tk SrcTicket) error {
		if first.ID == 0 {
			first = tk
		}
		return nil
	})
	// 2020-01-10 10:00 in Kuala Lumpur (UTC+8) is 02:00 UTC.
	if got := first.Created.UTC(); got != time.Date(2020, 1, 10, 2, 0, 0, 0, time.UTC) {
		t.Fatalf("created = %v", got)
	}
}

func TestSourceReadsThreadsFilesEvents(t *testing.T) {
	s := openTestSource(t)
	ctx := context.Background()
	threads, err := s.Threads(ctx)
	if err != nil || len(threads) != 5 || threads[10] != 1 || threads[50] != 5 {
		t.Fatalf("threads = %+v, %v", threads, err)
	}
	var entries []SrcEntry
	if err := s.Entries(ctx, func(e SrcEntry) error { entries = append(entries, e); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 10 || entries[0].Type != "M" || entries[1].PID != 1 || entries[4].Format != "markdown" {
		t.Fatalf("entries = %+v", entries)
	}
	var atts []SrcAttachment
	if err := s.Attachments(ctx, func(a SrcAttachment) error { atts = append(atts, a); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(atts) != 6 || atts[0].Name != "renamed-notes.txt" || atts[0].File.Backend != "D" || atts[1].File.Key != "fskey2" || !atts[1].Inline {
		t.Fatalf("attachments = %+v", atts)
	}
	rc, err := s.OpenChunks(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(rc)
	_ = rc.Close()
	if string(b) != "hello world" {
		t.Fatalf("chunks = %q", b)
	}
	var events []SrcEvent
	if err := s.Events(ctx, func(e SrcEvent) error { events = append(events, e); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(events) != 10 || events[0].Name != "created" || events[1].Data != `{"staff":1}` || !events[7].Annulled {
		t.Fatalf("events = %+v", events)
	}
}
```

- [ ] **Step 4: Run the tests to verify they fail**

Run: `cd gin && go test ./internal/importer/ -run TestSource`
Expected: FAIL to compile (undefined `OpenSource`, row types).

- [ ] **Step 5: Write the Source**

Create `gin/internal/importer/source.go`:

```go
package importer

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"time"

	"github.com/go-sql-driver/mysql"
)

// Source reads an osTicket MySQL database. Table names are prefixed with Prefix.
type Source struct {
	db     *sql.DB
	prefix string
}

// OpenSource connects to MySQL, forcing parseTime and the given zone for datetimes.
func OpenSource(ctx context.Context, dsn, prefix string, loc *time.Location) (*Source, error) {
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return nil, fmt.Errorf("mysql dsn: %w", err)
	}
	cfg.ParseTime = true
	cfg.Loc = loc
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return nil, fmt.Errorf("mysql open: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("mysql ping: %w", err)
	}
	return &Source{db: db, prefix: prefix}, nil
}

// Close releases the connection pool.
func (s *Source) Close() error { return s.db.Close() }

func (s *Source) t(name string) string { return "`" + s.prefix + name + "`" }

// Row types. Zero MySQL datetimes arrive as the zero time.Time.

type SrcPriority struct {
	ID      int64
	Name    string
	Urgency int32
	Color   string
}

type SrcStatus struct {
	ID          int64
	Name, State string
	Sort        int32
}

type SrcDepartment struct {
	ID, ManagerID    int64
	Name             string
	IsPublic         bool
	Created, Updated time.Time
}

type SrcStaff struct {
	ID, DeptID                                     int64
	Username, Email, Passwd, FirstName, LastName string
	IsAdmin, IsActive                              bool
	Created, Updated                               time.Time
}

type SrcStaffDept struct{ StaffID, DeptID int64 }

type SrcTopic struct {
	ID, DeptID, PriorityID int64
	Name                   string
	Flags                  uint32
	Sort                   int32
	Created, Updated       time.Time
}

type SrcUser struct {
	ID, DefaultEmailID int64
	Name               string
}

type SrcUserEmail struct {
	ID, UserID int64
	Address    string
}

type SrcFormAnswer struct {
	TicketID     int64
	Field, Value string
	ValueID      *int64
}

type SrcTicket struct {
	ID                                                                   int64
	Number                                                               string
	UserID, UserEmailID, StatusID, DeptID, TopicID, StaffID, SLAID, TeamID, EmailID int64
	Flags                                                                uint32
	IPAddress, Source, SourceExtra                                       string
	IsAnswered                                                           bool
	DueDate, Closed, LastUpdate, Created, Updated                        time.Time
}

type SrcEntry struct {
	ID, ThreadID, PID, StaffID          int64
	Type, Poster, Title, Body, Format string
	Created, Updated                    time.Time
}

type SrcFile struct {
	ID                        int64
	Backend, Type, Key, Name string
	Size                      int64
	Created                   time.Time
}

type SrcAttachment struct {
	EntryID, FileID int64
	Name            string
	Inline          bool
	File            SrcFile
}

type SrcEvent struct {
	ID, ThreadID, StaffID int64
	Name, Data            string
	Annulled              bool
	Timestamp             time.Time
}

// nt unwraps a nullable datetime; NULL and zero dates both become the zero time.
func nt(v sql.NullTime) time.Time {
	if !v.Valid {
		return time.Time{}
	}
	return v.Time
}

func ns(v sql.NullString) string {
	if !v.Valid {
		return ""
	}
	return v.String
}

func (s *Source) Priorities(ctx context.Context) ([]SrcPriority, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT priority_id, priority, priority_urgency, priority_color FROM "+s.t("ticket_priority")+" ORDER BY priority_id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SrcPriority
	for rows.Next() {
		var p SrcPriority
		if err := rows.Scan(&p.ID, &p.Name, &p.Urgency, &p.Color); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Source) Statuses(ctx context.Context) ([]SrcStatus, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, name, COALESCE(state, ''), sort FROM "+s.t("ticket_status")+" ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SrcStatus
	for rows.Next() {
		var st SrcStatus
		if err := rows.Scan(&st.ID, &st.Name, &st.State, &st.Sort); err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

func (s *Source) Departments(ctx context.Context) ([]SrcDepartment, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, manager_id, name, ispublic, created, updated FROM "+s.t("department")+" ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SrcDepartment
	for rows.Next() {
		var d SrcDepartment
		var c, u sql.NullTime
		if err := rows.Scan(&d.ID, &d.ManagerID, &d.Name, &d.IsPublic, &c, &u); err != nil {
			return nil, err
		}
		d.Created, d.Updated = nt(c), nt(u)
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Source) Staff(ctx context.Context) ([]SrcStaff, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT staff_id, dept_id, username, COALESCE(email,''), COALESCE(passwd,''), COALESCE(firstname,''), COALESCE(lastname,''), isadmin, isactive, created, updated FROM "+s.t("staff")+" ORDER BY staff_id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SrcStaff
	for rows.Next() {
		var st SrcStaff
		var c, u sql.NullTime
		if err := rows.Scan(&st.ID, &st.DeptID, &st.Username, &st.Email, &st.Passwd, &st.FirstName, &st.LastName, &st.IsAdmin, &st.IsActive, &c, &u); err != nil {
			return nil, err
		}
		st.Created, st.Updated = nt(c), nt(u)
		out = append(out, st)
	}
	return out, rows.Err()
}

func (s *Source) StaffDepts(ctx context.Context) ([]SrcStaffDept, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT staff_id, dept_id FROM "+s.t("staff_dept_access")+" ORDER BY staff_id, dept_id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SrcStaffDept
	for rows.Next() {
		var sd SrcStaffDept
		if err := rows.Scan(&sd.StaffID, &sd.DeptID); err != nil {
			return nil, err
		}
		out = append(out, sd)
	}
	return out, rows.Err()
}

func (s *Source) Topics(ctx context.Context) ([]SrcTopic, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT topic_id, dept_id, priority_id, topic, COALESCE(flags,0), sort, created, updated FROM "+s.t("help_topic")+" ORDER BY topic_id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SrcTopic
	for rows.Next() {
		var tp SrcTopic
		var c, u sql.NullTime
		if err := rows.Scan(&tp.ID, &tp.DeptID, &tp.PriorityID, &tp.Name, &tp.Flags, &tp.Sort, &c, &u); err != nil {
			return nil, err
		}
		tp.Created, tp.Updated = nt(c), nt(u)
		out = append(out, tp)
	}
	return out, rows.Err()
}

func (s *Source) Users(ctx context.Context) (map[int64]SrcUser, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, default_email_id, name FROM "+s.t("user"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]SrcUser{}
	for rows.Next() {
		var u SrcUser
		if err := rows.Scan(&u.ID, &u.DefaultEmailID, &u.Name); err != nil {
			return nil, err
		}
		out[u.ID] = u
	}
	return out, rows.Err()
}

func (s *Source) UserEmails(ctx context.Context) (map[int64]SrcUserEmail, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, user_id, address FROM "+s.t("user_email"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]SrcUserEmail{}
	for rows.Next() {
		var e SrcUserEmail
		if err := rows.Scan(&e.ID, &e.UserID, &e.Address); err != nil {
			return nil, err
		}
		out[e.ID] = e
	}
	return out, rows.Err()
}

// FormAnswers returns the ticket form's subject and priority answers keyed by ticket id.
func (s *Source) FormAnswers(ctx context.Context) (map[int64]map[string]SrcFormAnswer, error) {
	q := "SELECT e.object_id, f.name, COALESCE(v.value,''), v.value_id FROM " + s.t("form_entry") + " e JOIN " + s.t("form_entry_values") + " v ON v.entry_id = e.id JOIN " + s.t("form_field") + " f ON f.id = v.field_id WHERE e.object_type = 'T' AND f.name IN ('subject','priority')"
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]map[string]SrcFormAnswer{}
	for rows.Next() {
		var a SrcFormAnswer
		var vid sql.NullInt64
		if err := rows.Scan(&a.TicketID, &a.Field, &a.Value, &vid); err != nil {
			return nil, err
		}
		if vid.Valid {
			v := vid.Int64
			a.ValueID = &v
		}
		if out[a.TicketID] == nil {
			out[a.TicketID] = map[string]SrcFormAnswer{}
		}
		out[a.TicketID][a.Field] = a
	}
	return out, rows.Err()
}

func (s *Source) Tickets(ctx context.Context, fn func(SrcTicket) error) error {
	q := "SELECT ticket_id, COALESCE(number,''), user_id, user_email_id, status_id, dept_id, topic_id, staff_id, sla_id, team_id, email_id, flags, ip_address, source, COALESCE(source_extra,''), isanswered, duedate, closed, lastupdate, created, updated FROM " + s.t("ticket") + " ORDER BY ticket_id"
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var tk SrcTicket
		var due, closed, last, c, u sql.NullTime
		if err := rows.Scan(&tk.ID, &tk.Number, &tk.UserID, &tk.UserEmailID, &tk.StatusID, &tk.DeptID, &tk.TopicID, &tk.StaffID, &tk.SLAID, &tk.TeamID, &tk.EmailID, &tk.Flags, &tk.IPAddress, &tk.Source, &tk.SourceExtra, &tk.IsAnswered, &due, &closed, &last, &c, &u); err != nil {
			return err
		}
		tk.DueDate, tk.Closed, tk.LastUpdate, tk.Created, tk.Updated = nt(due), nt(closed), nt(last), nt(c), nt(u)
		if err := fn(tk); err != nil {
			return err
		}
	}
	return rows.Err()
}

// Threads maps thread id to ticket id for ticket threads.
func (s *Source) Threads(ctx context.Context) (map[int64]int64, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, object_id FROM "+s.t("thread")+" WHERE object_type = 'T'")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]int64{}
	for rows.Next() {
		var id, obj int64
		if err := rows.Scan(&id, &obj); err != nil {
			return nil, err
		}
		out[id] = obj
	}
	return out, rows.Err()
}

func (s *Source) Entries(ctx context.Context, fn func(SrcEntry) error) error {
	q := "SELECT id, thread_id, pid, staff_id, type, poster, title, body, format, created, updated FROM " + s.t("thread_entry") + " ORDER BY thread_id, id"
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var e SrcEntry
		var title sql.NullString
		var c, u sql.NullTime
		if err := rows.Scan(&e.ID, &e.ThreadID, &e.PID, &e.StaffID, &e.Type, &e.Poster, &title, &e.Body, &e.Format, &c, &u); err != nil {
			return err
		}
		e.Title, e.Created, e.Updated = ns(title), nt(c), nt(u)
		if err := fn(e); err != nil {
			return err
		}
	}
	return rows.Err()
}

// Attachments streams thread-entry attachments (type 'H') joined to their file rows.
func (s *Source) Attachments(ctx context.Context, fn func(SrcAttachment) error) error {
	q := "SELECT a.object_id, a.file_id, COALESCE(a.name,''), a.inline, f.bk, f.type, f.`key`, f.name, f.size, f.created FROM " + s.t("attachment") + " a JOIN " + s.t("file") + " f ON f.id = a.file_id WHERE a.type = 'H' ORDER BY a.object_id, a.id"
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var a SrcAttachment
		var c sql.NullTime
		if err := rows.Scan(&a.EntryID, &a.FileID, &a.Name, &a.Inline, &a.File.Backend, &a.File.Type, &a.File.Key, &a.File.Name, &a.File.Size, &c); err != nil {
			return err
		}
		a.File.ID, a.File.Created = a.FileID, nt(c)
		if err := fn(a); err != nil {
			return err
		}
	}
	return rows.Err()
}

// OpenChunks returns a reader over the file's chunks in order.
func (s *Source) OpenChunks(ctx context.Context, fileID int64) (io.ReadCloser, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT filedata FROM "+s.t("file_chunk")+" WHERE file_id = ? ORDER BY chunk_id", fileID)
	if err != nil {
		return nil, err
	}
	return &chunkReader{rows: rows}, nil
}

type chunkReader struct {
	rows *sql.Rows
	buf  []byte
	done bool
}

func (c *chunkReader) Read(p []byte) (int, error) {
	for len(c.buf) == 0 {
		if c.done {
			return 0, io.EOF
		}
		if !c.rows.Next() {
			c.done = true
			if err := c.rows.Err(); err != nil {
				return 0, err
			}
			return 0, io.EOF
		}
		if err := c.rows.Scan(&c.buf); err != nil {
			return 0, err
		}
	}
	n := copy(p, c.buf)
	c.buf = c.buf[n:]
	return n, nil
}

func (c *chunkReader) Close() error { return c.rows.Close() }

func (s *Source) Events(ctx context.Context, fn func(SrcEvent) error) error {
	q := "SELECT te.id, te.thread_id, te.staff_id, ev.name, COALESCE(te.data,''), te.annulled, te.timestamp FROM " + s.t("thread_event") + " te JOIN " + s.t("event") + " ev ON ev.id = te.event_id WHERE te.thread_type = 'T' ORDER BY te.timestamp, te.id"
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var e SrcEvent
		var ts sql.NullTime
		if err := rows.Scan(&e.ID, &e.ThreadID, &e.StaffID, &e.Name, &e.Data, &e.Annulled, &ts); err != nil {
			return err
		}
		e.Timestamp = nt(ts)
		if err := fn(e); err != nil {
			return err
		}
	}
	return rows.Err()
}
```

Note on `chunkReader.Scan(&c.buf)`: scanning a `longblob` into `[]byte` copies the driver buffer, so the slice stays valid after `Next`.

- [ ] **Step 6: Run the tests to verify they pass**

Run: `cd gin && go test ./internal/importer/ -run TestSource -v 2>&1 | tail -20`
Expected: PASS (4 tests). The first run pulls `mysql:8.0` and takes a minute or two. If `wait.ForLog("port: 3306  MySQL Community Server")` never matches, use the module's default wait strategy by removing the `WithWaitStrategy` option (the mysql module already waits for readiness).

- [ ] **Step 7: Vet, tidy, commit**

Run: `cd gin && go vet ./... && gofmt -l ./cmd ./internal && go mod tidy && git diff --stat go.mod`

```bash
git add gin/go.mod gin/go.sum gin/db/testdata/osticket-mysql/schema.sql gin/db/testdata/osticket-mysql/fixtures.sql gin/internal/importer/source.go gin/internal/importer/mysql_test.go gin/internal/importer/source_test.go
git commit -m "feat(api): osTicket MySQL source reader with container-backed fixture

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 3: Postgres Sink, pre-flight, and sequence reset

**Files:**
- Create: `gin/internal/importer/sink.go`
- Test: `gin/internal/importer/sink_test.go`

**Interfaces:**
- Consumes: `db.Beginner` from `internal/db` (existing); `testutil.Tx(t)` for tests.
- Produces:
  - `type Sink struct` with `NewSink(b db.Beginner, batch int, dryRun bool) *Sink`.
  - `(s *Sink) Step(ctx context.Context, fn func(w *Writer) error) error`: runs `fn` in one transaction (commit on nil, rollback on error). In dry-run mode every step is a savepoint inside one outer transaction that is never committed, so later steps see earlier rows (foreign keys resolve) and nothing reaches the database.
  - `(s *Sink) Close(ctx context.Context) error`: rolls back the dry-run outer transaction, if any. Always call it (deferred) after the steps.
  - `type Writer struct` (valid only inside `Step`): `(w *Writer) Insert(ctx, table string, cols []string, rows [][]any) error` (multi-row `INSERT INTO table (cols) OVERRIDING SYSTEM VALUE VALUES (...)` in batches of `batch` rows; no-op on empty rows), `(w *Writer) Exec(ctx, sql string, args ...any) error`, `(w *Writer) QueryRow(ctx, sql string, args ...any) pgx.Row`, `(w *Writer) Query(ctx, sql string, args ...any) (pgx.Rows, error)`.
  - `(s *Sink) Preflight(ctx context.Context) error`: returns `ErrTargetNotEmpty` wrapped with the offending table when the target is not a fresh seed database.
  - `var ErrTargetNotEmpty = errors.New("target database is not empty")`.
  - `(s *Sink) ResetSequences(ctx context.Context) error`: `setval` for every table in `identityTables` to `max(id)` (or 1 with is_called false when empty); no-op in dry-run. `var identityTables = []string{"department","staff","refresh_token","ticket_priority","ticket_status","help_topic","ticket","thread_entry","ticket_event","file"}`.

- [ ] **Step 1: Write the failing test**

Create `gin/internal/importer/sink_test.go`:

```go
package importer

import (
	"context"
	"errors"
	"testing"

	"github.com/grandpine/ticket-api/internal/db/testutil"
)

func TestSinkInsertPreservesIDsInBatches(t *testing.T) {
	tx := testutil.Tx(t)
	ctx := context.Background()
	s := NewSink(tx, 2, false)
	err := s.Step(ctx, func(w *Writer) error {
		return w.Insert(ctx, "department", []string{"id", "name", "is_public"}, [][]any{
			{int64(50), "Fifty", true}, {int64(51), "Fifty-one", false}, {int64(52), "Fifty-two", true},
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	var n int
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM department WHERE id IN (50,51,52)").Scan(&n); err != nil || n != 3 {
		t.Fatalf("count = %d, %v", n, err)
	}
	var name string
	if err := tx.QueryRow(ctx, "SELECT name FROM department WHERE id = 51").Scan(&name); err != nil || name != "Fifty-one" {
		t.Fatalf("name = %q, %v", name, err)
	}
}

func TestSinkStepRollsBackOnError(t *testing.T) {
	tx := testutil.Tx(t)
	ctx := context.Background()
	s := NewSink(tx, 500, false)
	boom := errors.New("boom")
	err := s.Step(ctx, func(w *Writer) error {
		if err := w.Insert(ctx, "department", []string{"id", "name"}, [][]any{{int64(60), "Sixty"}}); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	var n int
	_ = tx.QueryRow(ctx, "SELECT count(*) FROM department WHERE id = 60").Scan(&n)
	if n != 0 {
		t.Fatal("row should have been rolled back")
	}
}

func TestSinkDryRunWritesNothing(t *testing.T) {
	tx := testutil.Tx(t)
	ctx := context.Background()
	s := NewSink(tx, 500, true)
	if err := s.Step(ctx, func(w *Writer) error {
		return w.Insert(ctx, "department", []string{"id", "name"}, [][]any{{int64(70), "Seventy"}})
	}); err != nil {
		t.Fatal(err)
	}
	// A later step sees the earlier step's row, so foreign keys resolve during a dry run.
	if err := s.Step(ctx, func(w *Writer) error {
		var n int
		if err := w.QueryRow(ctx, "SELECT count(*) FROM department WHERE id = 70").Scan(&n); err != nil {
			return err
		}
		if n != 1 {
			return errors.New("row from the previous step not visible")
		}
		return w.Insert(ctx, "staff", []string{"id", "username", "email", "password_hash", "primary_dept_id"}, [][]any{{int64(70), "dry", "dry@example.test", "h", int64(70)}})
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(ctx); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = tx.QueryRow(ctx, "SELECT count(*) FROM department WHERE id = 70").Scan(&n)
	if n != 0 {
		t.Fatal("dry run must not commit")
	}
	_ = tx.QueryRow(ctx, "SELECT count(*) FROM staff WHERE id = 70").Scan(&n)
	if n != 0 {
		t.Fatal("dry run must not commit staff either")
	}
}

func TestPreflightRefusesNonEmpty(t *testing.T) {
	tx := testutil.Tx(t)
	ctx := context.Background()
	s := NewSink(tx, 500, false)
	if err := s.Preflight(ctx); err != nil {
		t.Fatalf("fresh seed database should pass: %v", err)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO staff (username, email, password_hash, primary_dept_id) VALUES ('x','x@example.test','h',(SELECT id FROM department LIMIT 1))"); err != nil {
		t.Fatal(err)
	}
	err := s.Preflight(ctx)
	if !errors.Is(err, ErrTargetNotEmpty) || err.Error() == ErrTargetNotEmpty.Error() {
		t.Fatalf("want wrapped ErrTargetNotEmpty naming the table, got %v", err)
	}
}

func TestPreflightRefusesExtraDepartment(t *testing.T) {
	tx := testutil.Tx(t)
	ctx := context.Background()
	if _, err := tx.Exec(ctx, "INSERT INTO department (name) VALUES ('Extra')"); err != nil {
		t.Fatal(err)
	}
	if err := NewSink(tx, 500, false).Preflight(ctx); !errors.Is(err, ErrTargetNotEmpty) {
		t.Fatalf("got %v", err)
	}
}

func TestResetSequences(t *testing.T) {
	tx := testutil.Tx(t)
	ctx := context.Background()
	s := NewSink(tx, 500, false)
	if err := s.Step(ctx, func(w *Writer) error {
		return w.Insert(ctx, "department", []string{"id", "name"}, [][]any{{int64(900), "Nine hundred"}})
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.ResetSequences(ctx); err != nil {
		t.Fatal(err)
	}
	var id int64
	if err := tx.QueryRow(ctx, "INSERT INTO department (name) VALUES ('Next') RETURNING id").Scan(&id); err != nil {
		t.Fatal(err)
	}
	if id != 901 {
		t.Fatalf("next id = %d, want 901", id)
	}
	// An empty table starts at 1 after the reset.
	if err := tx.QueryRow(ctx, "INSERT INTO file (key, name, mime, size, sha256) VALUES ('abcdef', 'a', 'text/plain', 1, 'x') RETURNING id").Scan(&id); err != nil {
		t.Fatal(err)
	}
	if id != 1 {
		t.Fatalf("file id = %d, want 1", id)
	}
}
```

Note: `setval` is not transactional, so `TestResetSequences` leaves sequences moved forward in the shared test database. That is harmless for other tests (they only ever need fresh ids).

- [ ] **Step 2: Run to verify it fails**

Run: `cd gin && go test ./internal/importer/ -run 'TestSink|TestPreflight|TestResetSequences'`
Expected: FAIL to compile (undefined `NewSink`).

- [ ] **Step 3: Write the Sink**

Create `gin/internal/importer/sink.go`:

```go
package importer

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/grandpine/ticket-api/internal/db"
	"github.com/jackc/pgx/v5"
)

// ErrTargetNotEmpty is returned by Preflight when the target already has data.
var ErrTargetNotEmpty = errors.New("target database is not empty")

// identityTables are reset by ResetSequences after ids were inserted explicitly.
var identityTables = []string{
	"department", "staff", "refresh_token", "ticket_priority", "ticket_status",
	"help_topic", "ticket", "thread_entry", "ticket_event", "file",
}

// Sink writes to the target Postgres database, one transaction per Step.
// In dry-run mode the steps run as savepoints inside one outer transaction
// that Close rolls back, so nothing is committed but later steps still see
// earlier rows.
type Sink struct {
	db     db.Beginner
	batch  int
	dryRun bool
	outer  pgx.Tx
}

// NewSink wraps a pool or transaction. batch is the number of rows per INSERT.
func NewSink(b db.Beginner, batch int, dryRun bool) *Sink {
	if batch < 1 {
		batch = 1
	}
	return &Sink{db: b, batch: batch, dryRun: dryRun}
}

// Writer is the per-step handle. It is only valid inside Step.
type Writer struct {
	tx    pgx.Tx
	batch int
}

// Step runs fn in a transaction and commits when fn returns nil. In dry-run
// mode the transaction is a savepoint of the outer transaction instead.
func (s *Sink) Step(ctx context.Context, fn func(w *Writer) error) error {
	var parent db.Beginner = s.db
	if s.dryRun {
		if s.outer == nil {
			outer, err := s.db.Begin(ctx)
			if err != nil {
				return fmt.Errorf("begin dry run: %w", err)
			}
			s.outer = outer
		}
		parent = s.outer
	}
	tx, err := parent.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	if err := fn(&Writer{tx: tx, batch: s.batch}); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// Close rolls back the dry-run outer transaction. It is a no-op otherwise.
func (s *Sink) Close(ctx context.Context) error {
	if s.outer == nil {
		return nil
	}
	err := s.outer.Rollback(ctx)
	s.outer = nil
	return err
}

// Insert writes rows with explicit ids in batches.
func (w *Writer) Insert(ctx context.Context, table string, cols []string, rows [][]any) error {
	for start := 0; start < len(rows); start += w.batch {
		end := start + w.batch
		if end > len(rows) {
			end = len(rows)
		}
		chunk := rows[start:end]
		var sb strings.Builder
		fmt.Fprintf(&sb, "INSERT INTO %s (%s) OVERRIDING SYSTEM VALUE VALUES ", table, strings.Join(cols, ", "))
		args := make([]any, 0, len(chunk)*len(cols))
		for i, row := range chunk {
			if len(row) != len(cols) {
				return fmt.Errorf("insert %s: row %d has %d values for %d columns", table, start+i, len(row), len(cols))
			}
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString("(")
			for j := range row {
				if j > 0 {
					sb.WriteString(", ")
				}
				fmt.Fprintf(&sb, "$%d", len(args)+1)
				args = append(args, row[j])
			}
			sb.WriteString(")")
		}
		if _, err := w.tx.Exec(ctx, sb.String(), args...); err != nil {
			return fmt.Errorf("insert %s: %w", table, err)
		}
	}
	return nil
}

func (w *Writer) Exec(ctx context.Context, sql string, args ...any) error {
	_, err := w.tx.Exec(ctx, sql, args...)
	return err
}

func (w *Writer) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return w.tx.QueryRow(ctx, sql, args...)
}

func (w *Writer) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return w.tx.Query(ctx, sql, args...)
}

// Preflight checks that the target holds only the seed rows.
func (s *Sink) Preflight(ctx context.Context) error {
	checks := []struct {
		table string
		want  int
	}{
		{"staff", 0}, {"ticket", 0}, {"thread_entry", 0}, {"file", 0}, {"department", 1}, {"help_topic", 1},
	}
	for _, c := range checks {
		var n int
		if err := s.db.QueryRow(ctx, "SELECT count(*) FROM "+c.table).Scan(&n); err != nil {
			return fmt.Errorf("preflight %s: %w", c.table, err)
		}
		if n != c.want {
			return fmt.Errorf("%w: %s has %d rows (expected %d)", ErrTargetNotEmpty, c.table, n, c.want)
		}
	}
	return nil
}

// ResetSequences moves every identity sequence past the highest inserted id.
func (s *Sink) ResetSequences(ctx context.Context) error {
	if s.dryRun {
		return nil
	}
	for _, t := range identityTables {
		q := fmt.Sprintf("SELECT setval(pg_get_serial_sequence('%s', 'id'), COALESCE(MAX(id), 1), MAX(id) IS NOT NULL) FROM %s", t, t)
		if _, err := s.db.Exec(ctx, q); err != nil {
			return fmt.Errorf("reset sequence %s: %w", t, err)
		}
	}
	return nil
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd gin && go test ./internal/importer/ -run 'TestSink|TestPreflight|TestResetSequences' -v 2>&1 | tail -15`
Expected: PASS (6 tests). `testutil.Tx` hands a `pgx.Tx` whose `Begin` creates a savepoint, so `Step` commits release savepoints and the test's rollback discards everything. `pgx.Tx` satisfies `db.Beginner`, which is what lets the dry-run outer transaction act as the parent.

- [ ] **Step 5: Vet and commit**

```bash
git add gin/internal/importer/sink.go gin/internal/importer/sink_test.go
git commit -m "feat(api): importer sink with batched id-preserving inserts and preflight

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 4: Reference data steps: priorities, statuses, departments, staff, topics

**Files:**
- Modify: `gin/internal/importer/lookup.go` (add `PendingManagers`, `TakenIDs`, `allocID`)
- Modify: `gin/internal/importer/report.go` (add `Note`, adjust `String`)
- Modify: `gin/internal/importer/report_test.go` (add `TestReportNote`)
- Create: `gin/internal/importer/timeutil.go`, `priorities.go`, `statuses.go`, `departments.go`, `staff.go`, `topics.go`
- Test: `gin/internal/importer/reference_test.go` (pure), `gin/internal/importer/reference_db_test.go` (both containers)

**Interfaces:**
- Consumes: `Source` readers (Task 2), `Sink`/`Writer` (Task 3), `Lookup`, `Report` (Task 1), `auth.HashPassword` and `auth.CheckPassword` from `internal/auth`.
- Produces (every step has the shape `func importX(ctx context.Context, src *Source, w *Writer, lk *Lookup, rep *Report) error` and runs inside `sink.Step`):
  - `Report.Note(e Entity, id int64, note string)`: records a sample without counting a skip.
  - `Lookup.PendingManagers map[int64]int64` (target dept id → source manager staff id); `Lookup.TakenIDs map[string]map[int64]bool` (table → ids already used by seed rows); `(lk *Lookup) allocID(table string, srcID int64) int64`: returns `srcID` unless a seed row already uses it, in which case it returns the next free id above every id seen so far for that table, and records the id as taken either way.
  - `mapStatusState(name, state string) (string, bool)`; `dedupeName(name string, taken map[string]bool) string`; `staffEmail(username, email string, taken map[string]bool) (string, bool)`; `staffPassword(passwd string) (string, bool, error)`; `topicActive(flags uint32) bool`; `orZero(t, fallback time.Time) time.Time`; `nullTime(t time.Time) *time.Time`.
  - Steps: `importPriorities` (sets `lk.DefaultPriority` = seed `normal`), `importStatuses` (sets `lk.DefaultStatus` = seed `Open`), `importDepartmentsPass1` (sets `lk.DefaultDept` = seed `Support`), `importStaff`, `importDepartmentsPass2`, `importTopics`.

- [ ] **Step 1: Extend Lookup and Report**

In `gin/internal/importer/lookup.go` add the fields and method:

```go
	// PendingManagers holds target department id → source manager staff id, applied after staff import.
	PendingManagers map[int64]int64
	// TakenIDs holds, per table, the ids already present in the target (seed rows and inserted rows).
	TakenIDs map[string]map[int64]bool
```

initialise both in `NewLookup` (`PendingManagers: map[int64]int64{}, TakenIDs: map[string]map[int64]bool{}`), and add:

```go
// allocID keeps the source id when it is free in the target table, otherwise
// hands out the next id above everything seen so far. Either way the id is
// recorded as taken. Seed rows must be registered first with markTaken.
func (lk *Lookup) allocID(table string, srcID int64) int64 {
	taken := lk.TakenIDs[table]
	if taken == nil {
		taken = map[int64]bool{}
		lk.TakenIDs[table] = taken
	}
	id := srcID
	if taken[id] {
		id = 0
		for k := range taken {
			if k > id {
				id = k
			}
		}
		id++
	}
	taken[id] = true
	return id
}

// markTaken registers an id that already exists in the target table.
func (lk *Lookup) markTaken(table string, id int64) {
	if lk.TakenIDs[table] == nil {
		lk.TakenIDs[table] = map[int64]bool{}
	}
	lk.TakenIDs[table][id] = true
}
```

In `gin/internal/importer/report.go` add:

```go
// Note records an informational sample (renames, placeholders) without counting a skip.
func (r *Report) Note(e Entity, id int64, note string) {
	c := r.Counter(e)
	if len(c.Samples) < maxSamples {
		c.Samples = append(c.Samples, Sample{ID: id, Reason: note})
	}
}
```

and in `String()` change the per-entity guard so notes print too:

```go
		if c.Skipped == 0 && len(c.Samples) == 0 {
			continue
		}
		if c.Skipped == 0 {
			fmt.Fprintf(&b, "\n%s notes:\n", e)
		} else {
			fmt.Fprintf(&b, "\n%s skipped:\n", e)
		}
```

(the reasons loop stays; it prints nothing when `Reasons` is empty).

Append to `gin/internal/importer/report_test.go`:

```go
func TestReportNote(t *testing.T) {
	r := NewReport()
	r.Note(EntityDepartments, 3, "renamed to Billing (2)")
	c := r.Counter(EntityDepartments)
	if c.Skipped != 0 || len(c.Samples) != 1 || !strings.Contains(r.String(), "renamed to Billing (2)") {
		t.Fatalf("note not recorded: %+v\n%s", c, r.String())
	}
	if r.NeedsAttention() {
		t.Fatal("notes never need attention")
	}
}

func TestLookupAllocID(t *testing.T) {
	lk := NewLookup()
	lk.markTaken("ticket_priority", 1)
	lk.markTaken("ticket_priority", 2)
	if got := lk.allocID("ticket_priority", 5); got != 5 {
		t.Fatalf("free id kept: %d", got)
	}
	if got := lk.allocID("ticket_priority", 2); got != 6 {
		t.Fatalf("collision moves above max: %d", got)
	}
	if got := lk.allocID("ticket_priority", 6); got != 7 {
		t.Fatalf("second collision: %d", got)
	}
}
```

- [ ] **Step 2: Write the failing pure tests**

Create `gin/internal/importer/reference_test.go`:

```go
package importer

import (
	"strings"
	"testing"
	"time"

	"github.com/grandpine/ticket-api/internal/auth"
)

func TestMapStatus(t *testing.T) {
	cases := []struct {
		name, state, want string
		ok                bool
	}{
		{"Open", "open", "open", true},
		{"Waiting", "open", "open", true},
		{"Resolved", "closed", "resolved", true},
		{"resolved", "closed", "resolved", true},
		{"Closed", "closed", "closed", true},
		{"Archived", "archived", "closed", true},
		{"Deleted", "deleted", "", false},
		{"Weird", "", "open", true},
	}
	for _, c := range cases {
		got, ok := mapStatusState(c.name, c.state)
		if got != c.want || ok != c.ok {
			t.Errorf("%s/%s = %q,%v want %q,%v", c.name, c.state, got, ok, c.want, c.ok)
		}
	}
}

func TestDedupeName(t *testing.T) {
	taken := map[string]bool{"support": true}
	if got := dedupeName("Billing", taken); got != "Billing" {
		t.Fatal(got)
	}
	if got := dedupeName("billing ", taken); got != "billing (2)" {
		t.Fatal(got)
	}
	if got := dedupeName("BILLING", taken); got != "BILLING (3)" {
		t.Fatal(got)
	}
	if got := dedupeName(" Support", taken); got != "Support (2)" {
		t.Fatal(got)
	}
}

func TestStaffEmail(t *testing.T) {
	taken := map[string]bool{}
	if e, replaced := staffEmail("ada", "Ada@Example.test", taken); e != "Ada@Example.test" || replaced {
		t.Fatal(e, replaced)
	}
	if e, replaced := staffEmail("bob", "ADA@example.test", taken); e != "bob@imported.invalid" || !replaced {
		t.Fatal(e, replaced)
	}
	if e, replaced := staffEmail("lea", "", taken); e != "lea@imported.invalid" || !replaced {
		t.Fatal(e, replaced)
	}
}

func TestStaffPassword(t *testing.T) {
	const phpHash = "$2y$10$uBGMLSBZVCGO.AIPCUG6V.pCoB3BL30EoiTv11HjETcXGxkjbyl.a"
	h, reset, err := staffPassword(phpHash)
	if err != nil || reset || h != phpHash {
		t.Fatal(h, reset, err)
	}
	if !auth.CheckPassword(h, "agentpass1") {
		t.Fatal("Go bcrypt must verify a PHP $2y$ hash")
	}
	h2, reset, err := staffPassword("")
	if err != nil || !reset || !strings.HasPrefix(h2, "$2a$") {
		t.Fatal(h2, reset, err)
	}
	if auth.CheckPassword(h2, "") {
		t.Fatal("random hash must not verify the empty password")
	}
}

func TestTopicActiveAndTimes(t *testing.T) {
	if topicActive(0) || !topicActive(2) || !topicActive(3) || topicActive(1) {
		t.Fatal("flag bit 2 is active")
	}
	fb := time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)
	if orZero(time.Time{}, fb) != fb || orZero(fb.Add(time.Hour), fb) != fb.Add(time.Hour) {
		t.Fatal("orZero")
	}
	if nullTime(time.Time{}) != nil || nullTime(fb) == nil {
		t.Fatal("nullTime")
	}
}
```

- [ ] **Step 3: Run to verify it fails**

Run: `cd gin && go test ./internal/importer/ -run 'TestMapStatus|TestDedupe|TestStaff|TestTopicActive|TestReportNote|TestLookupAllocID'`
Expected: FAIL to compile.

- [ ] **Step 4: Write the helpers and steps**

Create `gin/internal/importer/timeutil.go`:

```go
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
```

Add to `gin/internal/importer/lookup.go`:

```go
// seedByName loads the target table's existing rows as lower(name) → id and marks their ids taken.
func seedByName(ctx context.Context, w *Writer, lk *Lookup, table string) (map[string]int64, error) {
	rows, err := w.Query(ctx, "SELECT id, lower(name) FROM "+table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		out[name] = id
		lk.markTaken(table, id)
	}
	return out, rows.Err()
}
```

(add `import "context"` to `lookup.go`).

Create `gin/internal/importer/priorities.go`:

```go
package importer

import (
	"context"
	"strings"
)

func importPriorities(ctx context.Context, src *Source, w *Writer, lk *Lookup, rep *Report) error {
	seed, err := seedByName(ctx, w, lk, "ticket_priority")
	if err != nil {
		return err
	}
	lk.DefaultPriority = seed["normal"]
	items, err := src.Priorities(ctx)
	if err != nil {
		return err
	}
	var batch [][]any
	for _, p := range items {
		rep.Read(EntityPriorities)
		name := strings.TrimSpace(p.Name)
		key := strings.ToLower(name)
		if id, ok := seed[key]; ok {
			lk.Priorities[p.ID] = id
			rep.Merged(EntityPriorities)
			continue
		}
		id := lk.allocID("ticket_priority", p.ID)
		seed[key] = id
		lk.Priorities[p.ID] = id
		batch = append(batch, []any{id, name, p.Urgency, p.Color})
		rep.Written(EntityPriorities)
	}
	return w.Insert(ctx, "ticket_priority", []string{"id", "name", "urgency", "color"}, batch)
}
```

Create `gin/internal/importer/statuses.go`:

```go
package importer

import (
	"context"
	"strings"
)

// mapStatusState maps an osTicket status to the API's ticket_state. ok is false for deleted.
func mapStatusState(name, state string) (string, bool) {
	switch strings.ToLower(state) {
	case "deleted":
		return "", false
	case "closed":
		if strings.EqualFold(strings.TrimSpace(name), "resolved") {
			return "resolved", true
		}
		return "closed", true
	case "archived":
		return "closed", true
	default:
		return "open", true
	}
}

func importStatuses(ctx context.Context, src *Source, w *Writer, lk *Lookup, rep *Report) error {
	seed, err := seedByName(ctx, w, lk, "ticket_status")
	if err != nil {
		return err
	}
	lk.DefaultStatus = seed["open"]
	items, err := src.Statuses(ctx)
	if err != nil {
		return err
	}
	var batch [][]any
	for _, st := range items {
		rep.Read(EntityStatuses)
		state, ok := mapStatusState(st.Name, st.State)
		if !ok {
			lk.DeletedStatus[st.ID] = true
			rep.Skip(EntityStatuses, st.ID, ReasonDeletedStatus)
			continue
		}
		name := strings.TrimSpace(st.Name)
		key := strings.ToLower(name)
		if id, ok := seed[key]; ok {
			lk.Statuses[st.ID] = id
			rep.Merged(EntityStatuses)
			continue
		}
		id := lk.allocID("ticket_status", st.ID)
		seed[key] = id
		lk.Statuses[st.ID] = id
		batch = append(batch, []any{id, name, state, st.Sort})
		rep.Written(EntityStatuses)
	}
	return w.Insert(ctx, "ticket_status", []string{"id", "name", "state", "sort_order"}, batch)
}
```

Create `gin/internal/importer/departments.go`:

```go
package importer

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// dedupeName trims and makes the name unique (case-insensitive) with a " (n)" suffix.
func dedupeName(name string, taken map[string]bool) string {
	base := strings.TrimSpace(name)
	cand := base
	for n := 2; taken[strings.ToLower(cand)]; n++ {
		cand = fmt.Sprintf("%s (%d)", base, n)
	}
	taken[strings.ToLower(cand)] = true
	return cand
}

func importDepartmentsPass1(ctx context.Context, src *Source, w *Writer, lk *Lookup, rep *Report) error {
	seed, err := seedByName(ctx, w, lk, "department")
	if err != nil {
		return err
	}
	lk.DefaultDept = seed["support"]
	taken := map[string]bool{}
	for name := range seed {
		taken[name] = true
	}
	items, err := src.Departments(ctx)
	if err != nil {
		return err
	}
	now := time.Now()
	var batch [][]any
	for _, d := range items {
		rep.Read(EntityDepartments)
		trimmed := strings.TrimSpace(d.Name)
		if id, ok := seed[strings.ToLower(trimmed)]; ok {
			lk.Departments[d.ID] = id
			if d.ManagerID != 0 {
				lk.PendingManagers[id] = d.ManagerID
			}
			rep.Merged(EntityDepartments)
			continue
		}
		name := dedupeName(d.Name, taken)
		if name != trimmed {
			rep.Note(EntityDepartments, d.ID, "renamed to "+name)
		}
		id := lk.allocID("department", d.ID)
		lk.Departments[d.ID] = id
		if d.ManagerID != 0 {
			lk.PendingManagers[id] = d.ManagerID
		}
		created := orZero(d.Created, now)
		batch = append(batch, []any{id, name, d.IsPublic, created, orZero(d.Updated, created)})
		rep.Written(EntityDepartments)
	}
	return w.Insert(ctx, "department", []string{"id", "name", "is_public", "created_at", "updated_at"}, batch)
}

// importDepartmentsPass2 sets manager_id now that staff exist.
func importDepartmentsPass2(ctx context.Context, _ *Source, w *Writer, lk *Lookup, _ *Report) error {
	for deptID, srcManager := range lk.PendingManagers {
		staffID, ok := lk.Staff[srcManager]
		if !ok {
			continue
		}
		if err := w.Exec(ctx, "UPDATE department SET manager_id = $1 WHERE id = $2", staffID, deptID); err != nil {
			return err
		}
	}
	return nil
}
```

Create `gin/internal/importer/staff.go`:

```go
package importer

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"time"

	"github.com/grandpine/ticket-api/internal/auth"
)

// staffEmail returns the address to store and whether it was replaced with a placeholder.
func staffEmail(username, email string, taken map[string]bool) (string, bool) {
	e := strings.TrimSpace(email)
	key := strings.ToLower(e)
	if e == "" || taken[key] {
		p := username + "@imported.invalid"
		taken[strings.ToLower(p)] = true
		return p, true
	}
	taken[key] = true
	return e, false
}

// staffPassword keeps a stored bcrypt hash, or generates an unusable random one.
func staffPassword(passwd string) (string, bool, error) {
	if strings.HasPrefix(passwd, "$2") {
		return passwd, false, nil
	}
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", false, err
	}
	h, err := auth.HashPassword(hex.EncodeToString(b[:]))
	if err != nil {
		return "", false, err
	}
	return h, true, nil
}

func importStaff(ctx context.Context, src *Source, w *Writer, lk *Lookup, rep *Report) error {
	items, err := src.Staff(ctx)
	if err != nil {
		return err
	}
	access, err := src.StaffDepts(ctx)
	if err != nil {
		return err
	}
	taken := map[string]bool{}
	now := time.Now()
	primary := map[int64]int64{}
	var batch [][]any
	for _, st := range items {
		rep.Read(EntityStaff)
		email, replaced := staffEmail(st.Username, st.Email, taken)
		if replaced {
			rep.Note(EntityStaff, st.ID, "email replaced with "+email)
		}
		hash, reset, err := staffPassword(st.Passwd)
		if err != nil {
			return err
		}
		if reset {
			rep.Note(EntityStaff, st.ID, "password reset required")
		}
		dept, ok := lk.Departments[st.DeptID]
		if !ok {
			dept = lk.DefaultDept
			rep.Note(EntityStaff, st.ID, "primary department missing, using seed department")
		}
		id := lk.allocID("staff", st.ID)
		lk.Staff[st.ID] = id
		primary[id] = dept
		created := orZero(st.Created, now)
		batch = append(batch, []any{id, st.Username, email, hash, st.FirstName, st.LastName, st.IsAdmin, st.IsActive, dept, created, orZero(st.Updated, created)})
		rep.Written(EntityStaff)
	}
	if err := w.Insert(ctx, "staff", []string{"id", "username", "email", "password_hash", "first_name", "last_name", "is_admin", "is_active", "primary_dept_id", "created_at", "updated_at"}, batch); err != nil {
		return err
	}
	// Memberships: each primary department, plus every access row with a known department.
	type pair struct{ s, d int64 }
	seen := map[pair]bool{}
	var members [][]any
	add := func(s, d int64) {
		if !seen[pair{s, d}] {
			seen[pair{s, d}] = true
			members = append(members, []any{s, d})
		}
	}
	for id, d := range primary {
		add(id, d)
	}
	for _, a := range access {
		s, ok := lk.Staff[a.StaffID]
		if !ok {
			continue
		}
		if d, ok := lk.Departments[a.DeptID]; ok {
			add(s, d)
		}
	}
	return w.Insert(ctx, "staff_department", []string{"staff_id", "dept_id"}, members)
}
```

`staff_department` has no identity column; `OVERRIDING SYSTEM VALUE` is accepted on any insert. Map iteration order for `primary` is random, which is fine because the rows are independent.

Create `gin/internal/importer/topics.go`:

```go
package importer

import (
	"context"
	"strings"
	"time"
)

// topicActive is osTicket's Topic::FLAG_ACTIVE (0x0002).
func topicActive(flags uint32) bool { return flags&2 != 0 }

func importTopics(ctx context.Context, src *Source, w *Writer, lk *Lookup, rep *Report) error {
	seed, err := seedByName(ctx, w, lk, "help_topic")
	if err != nil {
		return err
	}
	items, err := src.Topics(ctx)
	if err != nil {
		return err
	}
	now := time.Now()
	var batch [][]any
	for _, tp := range items {
		rep.Read(EntityTopics)
		name := strings.TrimSpace(tp.Name)
		if id, ok := seed[strings.ToLower(name)]; ok {
			lk.Topics[tp.ID] = id
			rep.Merged(EntityTopics)
			continue
		}
		var dept, prio *int64
		if d, ok := lk.Departments[tp.DeptID]; ok {
			dept = &d
		}
		if p, ok := lk.Priorities[tp.PriorityID]; ok {
			prio = &p
		}
		id := lk.allocID("help_topic", tp.ID)
		lk.Topics[tp.ID] = id
		created := orZero(tp.Created, now)
		batch = append(batch, []any{id, name, dept, prio, topicActive(tp.Flags), tp.Sort, created, orZero(tp.Updated, created)})
		rep.Written(EntityTopics)
	}
	return w.Insert(ctx, "help_topic", []string{"id", "name", "dept_id", "priority_id", "is_active", "sort_order", "created_at", "updated_at"}, batch)
}
```

- [ ] **Step 5: Run the pure tests**

Run: `cd gin && go test ./internal/importer/ -run 'TestMapStatus|TestDedupe|TestStaff|TestTopicActive|TestReport|TestLookup'`
Expected: PASS.

- [ ] **Step 6: Write the failing database step test**

Create `gin/internal/importer/reference_db_test.go`:

```go
package importer

import (
	"context"
	"testing"

	"github.com/grandpine/ticket-api/internal/auth"
	"github.com/grandpine/ticket-api/internal/db/testutil"
)

// runReferenceSteps imports everything up to topics inside the test transaction.
func runReferenceSteps(t *testing.T, sink *Sink, src *Source, lk *Lookup, rep *Report) {
	t.Helper()
	ctx := context.Background()
	steps := []func(context.Context, *Source, *Writer, *Lookup, *Report) error{
		importPriorities, importStatuses, importDepartmentsPass1, importStaff, importDepartmentsPass2, importTopics,
	}
	for i, step := range steps {
		if err := sink.Step(ctx, func(w *Writer) error { return step(ctx, src, w, lk, rep) }); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
	}
}

func TestReferenceSteps(t *testing.T) {
	tx := testutil.Tx(t)
	ctx := context.Background()
	src := openTestSource(t)
	sink := NewSink(tx, 2, false)
	lk, rep := NewLookup(), NewReport()
	runReferenceSteps(t, sink, src, lk, rep)

	// Priorities: 1-4 merge with the seed, VIP is new and keeps id 5.
	var n int
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM ticket_priority").Scan(&n); err != nil || n != 5 {
		t.Fatalf("priorities = %d, %v", n, err)
	}
	if lk.Priorities[5] != 5 || lk.Priorities[2] != lk.DefaultPriority || rep.Counter(EntityPriorities).Merged != 4 {
		t.Fatalf("priority map = %+v, report %+v", lk.Priorities, rep.Counter(EntityPriorities))
	}
	// Statuses: Resolved is resolved, Archived closed, Waiting open, Deleted skipped.
	var state string
	if err := tx.QueryRow(ctx, "SELECT state FROM ticket_status WHERE id = $1", lk.Statuses[2]).Scan(&state); err != nil || state != "resolved" {
		t.Fatalf("resolved state = %q, %v", state, err)
	}
	if err := tx.QueryRow(ctx, "SELECT state FROM ticket_status WHERE id = $1", lk.Statuses[4]).Scan(&state); err != nil || state != "closed" {
		t.Fatalf("archived state = %q, %v", state, err)
	}
	if err := tx.QueryRow(ctx, "SELECT state FROM ticket_status WHERE id = $1", lk.Statuses[6]).Scan(&state); err != nil || state != "open" {
		t.Fatalf("waiting state = %q, %v", state, err)
	}
	if !lk.DeletedStatus[5] || rep.Counter(EntityStatuses).Reasons[ReasonDeletedStatus] != 1 {
		t.Fatal("deleted status not recorded")
	}
	// Departments: Support merged, Billing kept, "billing " renamed, manager set in pass 2.
	var name string
	var mgr *int64
	if err := tx.QueryRow(ctx, "SELECT name FROM department WHERE id = $1", lk.Departments[3]).Scan(&name); err != nil || name != "billing (2)" {
		t.Fatalf("renamed dept = %q, %v", name, err)
	}
	if err := tx.QueryRow(ctx, "SELECT manager_id FROM department WHERE id = $1", lk.DefaultDept).Scan(&mgr); err != nil || mgr == nil || *mgr != lk.Staff[1] {
		t.Fatalf("support manager = %v, %v", mgr, err)
	}
	if lk.Departments[1] != lk.DefaultDept || lk.Departments[2] != 2 {
		t.Fatalf("dept map = %+v", lk.Departments)
	}
	// Staff: passwords kept, duplicate email replaced, ldap user reset and moved to the seed dept.
	var hash, email string
	var prim int64
	if err := tx.QueryRow(ctx, "SELECT password_hash, email FROM staff WHERE id = $1", lk.Staff[1]).Scan(&hash, &email); err != nil || !auth.CheckPassword(hash, "agentpass1") || email != "ada@example.test" {
		t.Fatalf("ada = %q %q, %v", hash, email, err)
	}
	if err := tx.QueryRow(ctx, "SELECT email FROM staff WHERE id = $1", lk.Staff[2]).Scan(&email); err != nil || email != "bob@imported.invalid" {
		t.Fatalf("bob email = %q, %v", email, err)
	}
	if err := tx.QueryRow(ctx, "SELECT password_hash, primary_dept_id FROM staff WHERE id = $1", lk.Staff[3]).Scan(&hash, &prim); err != nil || prim != lk.DefaultDept || auth.CheckPassword(hash, "") {
		t.Fatalf("ldap user = %q %d, %v", hash, prim, err)
	}
	// ada: Support (primary) + Billing; bob: Billing (primary) + Support; lea: Support. Dept 9 rows dropped.
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM staff_department").Scan(&n); err != nil || n != 5 {
		t.Fatalf("memberships = %d, %v", n, err)
	}
	// Topics: General Inquiry merged, Refunds inactive with dept Billing, Orphan dept null.
	var active bool
	var dept *int64
	if err := tx.QueryRow(ctx, "SELECT is_active, dept_id FROM help_topic WHERE id = $1", lk.Topics[2]).Scan(&active, &dept); err != nil || active || dept == nil || *dept != 2 {
		t.Fatalf("refunds = %v %v, %v", active, dept, err)
	}
	if err := tx.QueryRow(ctx, "SELECT is_active, dept_id FROM help_topic WHERE id = $1", lk.Topics[3]).Scan(&active, &dept); err != nil || !active || dept != nil {
		t.Fatalf("orphan = %v %v, %v", active, dept, err)
	}
	if rep.Counter(EntityTopics).Merged != 1 || rep.Counter(EntityTopics).Written != 2 {
		t.Fatalf("topic report = %+v", rep.Counter(EntityTopics))
	}
}
```

- [ ] **Step 7: Run to verify it passes**

Run: `cd gin && go test ./internal/importer/ -run TestReferenceSteps -v 2>&1 | tail -15`
Expected: PASS. If the memberships count differs, print `SELECT staff_id, dept_id FROM staff_department ORDER BY 1,2` and reconcile against the comment before changing either side.

- [ ] **Step 8: Vet, full package test, commit**

Run: `cd gin && go vet ./... && gofmt -l ./cmd ./internal && go test ./internal/importer/`

```bash
git add gin/internal/importer/
git commit -m "feat(api): import priorities, statuses, departments, staff, and topics

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 5: Tickets

**Files:**
- Create: `gin/internal/importer/tickets.go`
- Test: `gin/internal/importer/tickets_test.go` (pure), `gin/internal/importer/tickets_db_test.go`

**Interfaces:**
- Consumes: `Source.Users/UserEmails/FormAnswers/Tickets` (Task 2), `Writer` (Task 3), `Lookup` maps and defaults, `Report.Note/Skip` (Task 4), `orZero/nullTime`.
- Produces:
  - `mapSource(s string) string` (Web→web, Phone→phone, API→api, else other).
  - `requesterEmail(tk SrcTicket, users map[int64]SrcUser, emails map[int64]SrcUserEmail) (string, bool)`: `user_email_id` row, else the user's default email row, else `unknown-<ticket id>@imported.invalid`; the bool is true when the placeholder was used.
  - `ticketNumber(tk SrcTicket, taken map[string]bool) (string, bool)`: empty → zero-padded 6-digit id; duplicates → `<number>-<id>`; bool true when changed.
  - `ticketExtra(tk SrcTicket) ([]byte, error)`: JSON object of the dropped columns, only non-zero/non-empty values, `{}` when none.
  - `importTickets(ctx, src, w, lk, rep) error`: fills `lk.Tickets` (source id → target id) and writes `ticket` rows with `last_response_at` NULL (set in Task 7).

- [ ] **Step 1: Write the failing pure tests**

Create `gin/internal/importer/tickets_test.go`:

```go
package importer

import (
	"encoding/json"
	"testing"
)

func TestMapSource(t *testing.T) {
	for in, want := range map[string]string{"Web": "web", "Phone": "phone", "API": "api", "Email": "other", "Other": "other", "": "other", "web": "web"} {
		if got := mapSource(in); got != want {
			t.Errorf("%q = %q want %q", in, got, want)
		}
	}
}

func TestMapTicketRequesterFallback(t *testing.T) {
	users := map[int64]SrcUser{1: {ID: 1, DefaultEmailID: 1, Name: "Pat"}, 2: {ID: 2, DefaultEmailID: 2, Name: "No Email"}, 3: {ID: 3, DefaultEmailID: 3}}
	emails := map[int64]SrcUserEmail{1: {ID: 1, UserID: 1, Address: "pat@example.test"}, 3: {ID: 3, UserID: 3, Address: "fallback@example.test"}}
	if e, ph := requesterEmail(SrcTicket{ID: 1, UserID: 1, UserEmailID: 1}, users, emails); e != "pat@example.test" || ph {
		t.Fatal(e, ph)
	}
	if e, ph := requesterEmail(SrcTicket{ID: 2, UserID: 3, UserEmailID: 0}, users, emails); e != "fallback@example.test" || ph {
		t.Fatal("default email fallback", e, ph)
	}
	if e, ph := requesterEmail(SrcTicket{ID: 3, UserID: 2}, users, emails); e != "unknown-3@imported.invalid" || !ph {
		t.Fatal("placeholder", e, ph)
	}
	if e, ph := requesterEmail(SrcTicket{ID: 4, UserID: 99}, users, emails); e != "unknown-4@imported.invalid" || !ph {
		t.Fatal("missing user", e, ph)
	}
}

func TestTicketNumber(t *testing.T) {
	taken := map[string]bool{}
	if n, changed := ticketNumber(SrcTicket{ID: 1, Number: "100001"}, taken); n != "100001" || changed {
		t.Fatal(n, changed)
	}
	if n, changed := ticketNumber(SrcTicket{ID: 3, Number: ""}, taken); n != "000003" || !changed {
		t.Fatal(n, changed)
	}
	if n, changed := ticketNumber(SrcTicket{ID: 5, Number: "100001"}, taken); n != "100001-5" || !changed {
		t.Fatal(n, changed)
	}
}

func TestTicketExtra(t *testing.T) {
	b, err := ticketExtra(SrcTicket{ID: 1, Source: "Email"})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if len(m) != 1 || m["source"] != "Email" {
		t.Fatalf("extra = %v", m)
	}
	b, _ = ticketExtra(SrcTicket{ID: 1, SLAID: 2, TeamID: 3, Flags: 4, IPAddress: "10.0.0.1", SourceExtra: "ext", Source: "Web", EmailID: 6, UserID: 7})
	_ = json.Unmarshal(b, &m)
	for _, k := range []string{"sla_id", "team_id", "flags", "ip_address", "source_extra", "source", "email_id", "user_id"} {
		if _, ok := m[k]; !ok {
			t.Fatalf("extra missing %s: %v", k, m)
		}
	}
	if b, _ = ticketExtra(SrcTicket{}); string(b) != "{}" {
		t.Fatalf("empty extra = %s", b)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd gin && go test ./internal/importer/ -run 'TestMapSource|TestMapTicket|TestTicketNumber|TestTicketExtra'`
Expected: FAIL to compile.

- [ ] **Step 3: Write tickets.go**

```go
package importer

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func mapSource(s string) string {
	switch strings.ToLower(s) {
	case "web":
		return "web"
	case "phone":
		return "phone"
	case "api":
		return "api"
	default:
		return "other"
	}
}

// requesterEmail resolves the ticket's email row, then the user's default, then a placeholder.
func requesterEmail(tk SrcTicket, users map[int64]SrcUser, emails map[int64]SrcUserEmail) (string, bool) {
	if e, ok := emails[tk.UserEmailID]; ok && strings.TrimSpace(e.Address) != "" {
		return strings.TrimSpace(e.Address), false
	}
	if u, ok := users[tk.UserID]; ok {
		if e, ok := emails[u.DefaultEmailID]; ok && strings.TrimSpace(e.Address) != "" {
			return strings.TrimSpace(e.Address), false
		}
	}
	return fmt.Sprintf("unknown-%d@imported.invalid", tk.ID), true
}

// ticketNumber keeps the source number when present and unique.
func ticketNumber(tk SrcTicket, taken map[string]bool) (string, bool) {
	n := strings.TrimSpace(tk.Number)
	changed := false
	if n == "" {
		n = fmt.Sprintf("%06d", tk.ID)
		changed = true
	}
	if taken[n] {
		n = fmt.Sprintf("%s-%d", n, tk.ID)
		changed = true
	}
	taken[n] = true
	return n, changed
}

// ticketExtra keeps the dropped columns as JSON so nothing is lost.
func ticketExtra(tk SrcTicket) ([]byte, error) {
	m := map[string]any{}
	if tk.SLAID != 0 {
		m["sla_id"] = tk.SLAID
	}
	if tk.TeamID != 0 {
		m["team_id"] = tk.TeamID
	}
	if tk.Flags != 0 {
		m["flags"] = tk.Flags
	}
	if tk.IPAddress != "" {
		m["ip_address"] = tk.IPAddress
	}
	if tk.SourceExtra != "" {
		m["source_extra"] = tk.SourceExtra
	}
	if tk.Source != "" {
		m["source"] = tk.Source
	}
	if tk.EmailID != 0 {
		m["email_id"] = tk.EmailID
	}
	if tk.UserID != 0 {
		m["user_id"] = tk.UserID
	}
	return json.Marshal(m)
}

func importTickets(ctx context.Context, src *Source, w *Writer, lk *Lookup, rep *Report) error {
	users, err := src.Users(ctx)
	if err != nil {
		return err
	}
	emails, err := src.UserEmails(ctx)
	if err != nil {
		return err
	}
	forms, err := src.FormAnswers(ctx)
	if err != nil {
		return err
	}
	// Target topic → default priority, for tickets without a priority answer.
	topicPriority := map[int64]int64{}
	rows, err := w.Query(ctx, "SELECT id, priority_id FROM help_topic WHERE priority_id IS NOT NULL")
	if err != nil {
		return err
	}
	for rows.Next() {
		var id, p int64
		if err := rows.Scan(&id, &p); err != nil {
			rows.Close()
			return err
		}
		topicPriority[id] = p
	}
	rows.Close()
	rows, err = w.Query(ctx, "SELECT number FROM ticket")
	if err != nil {
		return err
	}
	taken := map[string]bool{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			rows.Close()
			return err
		}
		taken[n] = true
	}
	rows.Close()

	now := time.Now()
	var batch [][]any
	err = src.Tickets(ctx, func(tk SrcTicket) error {
		rep.Read(EntityTickets)
		if lk.DeletedStatus[tk.StatusID] {
			rep.Skip(EntityTickets, tk.ID, ReasonDeletedStatus)
			return nil
		}
		status, ok := lk.Statuses[tk.StatusID]
		if !ok {
			status = lk.DefaultStatus
			rep.Note(EntityTickets, tk.ID, "unknown status, using Open")
		}
		dept, ok := lk.Departments[tk.DeptID]
		if !ok {
			dept = lk.DefaultDept
			rep.Note(EntityTickets, tk.ID, "unknown department, using seed department")
		}
		var topic *int64
		if t, ok := lk.Topics[tk.TopicID]; ok {
			topic = &t
		}
		var assignee *int64
		if s, ok := lk.Staff[tk.StaffID]; ok {
			assignee = &s
		}
		answers := forms[tk.ID]
		subject := strings.TrimSpace(answers["subject"].Value)
		if subject == "" {
			subject = "(no subject)"
			rep.Note(EntityTickets, tk.ID, "empty subject")
		}
		priority := lk.DefaultPriority
		if a, ok := answers["priority"]; ok && a.ValueID != nil {
			if p, ok := lk.Priorities[*a.ValueID]; ok {
				priority = p
			}
		} else if topic != nil {
			if p, ok := topicPriority[*topic]; ok {
				priority = p
			}
		}
		number, changed := ticketNumber(tk, taken)
		if changed {
			rep.Note(EntityTickets, tk.ID, "number set to "+number)
		}
		email, placeholder := requesterEmail(tk, users, emails)
		if placeholder {
			rep.Note(EntityTickets, tk.ID, "requester email replaced with "+email)
		}
		name := ""
		if u, ok := users[tk.UserID]; ok {
			name = u.Name
		}
		extra, err := ticketExtra(tk)
		if err != nil {
			return err
		}
		id := lk.allocID("ticket", tk.ID)
		lk.Tickets[tk.ID] = id
		created := orZero(tk.Created, now)
		batch = append(batch, []any{
			id, number, subject, status, dept, topic, priority, assignee, name, email, mapSource(tk.Source),
			tk.IsAnswered, nullTime(tk.DueDate), nullTime(tk.Closed), orZero(tk.LastUpdate, created), extra,
			created, orZero(tk.Updated, created),
		})
		rep.Written(EntityTickets)
		return nil
	})
	if err != nil {
		return err
	}
	return w.Insert(ctx, "ticket", []string{
		"id", "number", "subject", "status_id", "dept_id", "topic_id", "priority_id", "assigned_staff_id",
		"requester_name", "requester_email", "source", "is_answered", "due_at", "closed_at", "last_message_at",
		"extra", "created_at", "updated_at",
	}, batch)
}
```

`extra` is passed as `[]byte`; pgx encodes it into `jsonb` when the parameter is a byte slice containing JSON text.

- [ ] **Step 4: Run the pure tests**

Run: `cd gin && go test ./internal/importer/ -run 'TestMapSource|TestMapTicket|TestTicketNumber|TestTicketExtra'`
Expected: PASS.

- [ ] **Step 5: Write the failing database test**

Create `gin/internal/importer/tickets_db_test.go`:

```go
package importer

import (
	"context"
	"testing"
	"time"

	"github.com/grandpine/ticket-api/internal/db/testutil"
)

func TestImportTickets(t *testing.T) {
	tx := testutil.Tx(t)
	ctx := context.Background()
	src := openTestSource(t)
	sink := NewSink(tx, 2, false)
	lk, rep := NewLookup(), NewReport()
	runReferenceSteps(t, sink, src, lk, rep)
	if err := sink.Step(ctx, func(w *Writer) error { return importTickets(ctx, src, w, lk, rep) }); err != nil {
		t.Fatal(err)
	}
	c := rep.Counter(EntityTickets)
	if c.Read != 5 || c.Written != 4 || c.Skipped != 1 || c.Reasons[ReasonDeletedStatus] != 1 {
		t.Fatalf("ticket counter = %+v", c)
	}
	if _, ok := lk.Tickets[4]; ok {
		t.Fatal("deleted ticket must not be mapped")
	}
	var number, subject, email, source string
	var priority, status, dept int64
	var assignee *int64
	var due *time.Time
	q := "SELECT number, subject, priority_id, status_id, dept_id, assigned_staff_id, requester_email, source, due_at FROM ticket WHERE id = $1"
	if err := tx.QueryRow(ctx, q, lk.Tickets[1]).Scan(&number, &subject, &priority, &status, &dept, &assignee, &email, &source, &due); err != nil {
		t.Fatal(err)
	}
	if number != "100001" || subject != "Printer on fire" || priority != lk.Priorities[3] || status != lk.Statuses[1] || dept != lk.DefaultDept || assignee == nil || *assignee != lk.Staff[1] || email != "pat@example.test" || source != "web" || due == nil {
		t.Fatalf("ticket 1 = %s %s %d %d %d %v %s %s %v", number, subject, priority, status, dept, assignee, email, source, due)
	}
	// Ticket 2: Resolved, no user_email_id → default email, Email source → other, priority from answer.
	if err := tx.QueryRow(ctx, q, lk.Tickets[2]).Scan(&number, &subject, &priority, &status, &dept, &assignee, &email, &source, &due); err != nil {
		t.Fatal(err)
	}
	if status != lk.Statuses[2] || email != "fallback@example.test" || source != "other" || assignee != nil || priority != lk.Priorities[2] {
		t.Fatalf("ticket 2 = %d %s %s %v %d", status, email, source, assignee, priority)
	}
	// Ticket 3: archived → closed status id 4, empty number → 000003, empty subject, no email row → placeholder, phone.
	if err := tx.QueryRow(ctx, q, lk.Tickets[3]).Scan(&number, &subject, &priority, &status, &dept, &assignee, &email, &source, &due); err != nil {
		t.Fatal(err)
	}
	if number != "000003" || subject != "(no subject)" || email != "unknown-3@imported.invalid" || source != "phone" || status != lk.Statuses[4] || priority != lk.DefaultPriority {
		t.Fatalf("ticket 3 = %s %s %s %s %d %d", number, subject, email, source, status, priority)
	}
	// Ticket 5: duplicate number, unknown dept and staff → seed dept, unassigned; topic 3 has no priority → seed normal.
	if err := tx.QueryRow(ctx, q, lk.Tickets[5]).Scan(&number, &subject, &priority, &status, &dept, &assignee, &email, &source, &due); err != nil {
		t.Fatal(err)
	}
	if number != "100001-5" || dept != lk.DefaultDept || assignee != nil || priority != lk.DefaultPriority || status != lk.Statuses[6] || source != "api" {
		t.Fatalf("ticket 5 = %s %d %v %d %d %s", number, dept, assignee, priority, status, source)
	}
	var extra map[string]any
	if err := tx.QueryRow(ctx, "SELECT extra FROM ticket WHERE id = $1", lk.Tickets[3]).Scan(&extra); err != nil || extra["source_extra"] != "ext 12" || extra["source"] != "Phone" {
		t.Fatalf("extra = %v, %v", extra, err)
	}
}
```

- [ ] **Step 6: Run to verify it passes**

Run: `cd gin && go test ./internal/importer/ -run TestImportTickets -v 2>&1 | tail -15`
Expected: PASS.

- [ ] **Step 7: Vet and commit**

Run: `cd gin && go vet ./... && gofmt -l ./cmd ./internal`

```bash
git add gin/internal/importer/tickets.go gin/internal/importer/tickets_test.go gin/internal/importer/tickets_db_test.go
git commit -m "feat(api): import tickets with requester, form data, and extras

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 6: Thread entries, files, and attachments

**Files:**
- Create: `gin/internal/importer/thread.go`
- Create: `gin/internal/importer/files.go`
- Test: `gin/internal/importer/thread_test.go` (pure), `gin/internal/importer/thread_db_test.go`

**Interfaces:**
- Consumes: `Source.Threads/Entries/Attachments/OpenChunks` (Task 2), `Writer` (Task 3), `Lookup.Tickets` (Task 5), `attachment.Storage` (`Put(ctx, key, r) (size int64, sha256Hex string, err error)`, `Open`, `Delete`) from `internal/attachment`.
- Produces:
  - `mapEntryType(t string) (string, bool)` (M→message, R→response, N→note, else false); `mapFormat(f string) string` (`text` → text, else html).
  - `importEntries(ctx, src, w, lk, rep) error`: two passes (insert with `parent_id` NULL, then `UPDATE … SET parent_id` for pids that name an imported entry of the same thread); fills `lk.Entries`.
  - `newStorageKey() (string, error)`: 32 random bytes hex.
  - `type discardStorage struct{}` implementing `attachment.Storage` for dry runs (`Put` hashes and counts, `Open`/`Delete` return an error).
  - `importFiles(ctx context.Context, src *Source, w *Writer, lk *Lookup, rep *Report, store attachment.Storage, filesDir string) error`: one `file` row per source file id (`lk.Files`), `attachment` rows deduplicated per (entry, file), bytes from chunks or `filesDir`.

- [ ] **Step 1: Write the failing pure tests**

Create `gin/internal/importer/thread_test.go`:

```go
package importer

import (
	"bytes"
	"context"
	"regexp"
	"testing"
)

func TestMapEntryTypeAndFormat(t *testing.T) {
	for in, want := range map[string]string{"M": "message", "R": "response", "N": "note"} {
		if got, ok := mapEntryType(in); !ok || got != want {
			t.Errorf("%s = %s,%v", in, got, ok)
		}
	}
	if _, ok := mapEntryType("X"); ok {
		t.Error("X must not map")
	}
	if mapFormat("text") != "text" || mapFormat("html") != "html" || mapFormat("markdown") != "html" || mapFormat("") != "html" {
		t.Error("format mapping")
	}
}

func TestStorageKeyAndDiscard(t *testing.T) {
	k, err := newStorageKey()
	if err != nil || !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(k) {
		t.Fatal(k, err)
	}
	var d discardStorage
	n, sum, err := d.Put(context.Background(), k, bytes.NewReader([]byte("hello world")))
	if err != nil || n != 11 || sum != "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9" {
		t.Fatal(n, sum, err)
	}
	if _, err := d.Open(context.Background(), k); err == nil {
		t.Fatal("discard storage cannot open")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd gin && go test ./internal/importer/ -run 'TestMapEntryType|TestStorageKey'`
Expected: FAIL to compile.

- [ ] **Step 3: Write thread.go**

```go
package importer

import (
	"context"
	"strings"
	"time"
)

func mapEntryType(t string) (string, bool) {
	switch t {
	case "M":
		return "message", true
	case "R":
		return "response", true
	case "N":
		return "note", true
	default:
		return "", false
	}
}

func mapFormat(f string) string {
	if strings.EqualFold(f, "text") {
		return "text"
	}
	return "html"
}

func importEntries(ctx context.Context, src *Source, w *Writer, lk *Lookup, rep *Report) error {
	threads, err := src.Threads(ctx)
	if err != nil {
		return err
	}
	type parentLink struct{ child, parent, thread int64 }
	var links []parentLink
	entryThread := map[int64]int64{} // target entry id → source thread id
	now := time.Now()
	var batch [][]any
	err = src.Entries(ctx, func(e SrcEntry) error {
		rep.Read(EntityEntries)
		srcTicket, ok := threads[e.ThreadID]
		if !ok {
			rep.Skip(EntityEntries, e.ID, "thread is not a ticket thread")
			return nil
		}
		ticketID, ok := lk.Tickets[srcTicket]
		if !ok {
			rep.Skip(EntityEntries, e.ID, "ticket not imported")
			return nil
		}
		kind, ok := mapEntryType(e.Type)
		if !ok {
			rep.Skip(EntityEntries, e.ID, "unsupported entry type "+e.Type)
			return nil
		}
		var staff *int64
		if s, ok := lk.Staff[e.StaffID]; ok {
			staff = &s
		}
		var title *string
		if t := strings.TrimSpace(e.Title); t != "" {
			title = &t
		}
		id := lk.allocID("thread_entry", e.ID)
		lk.Entries[e.ID] = id
		entryThread[id] = e.ThreadID
		if e.PID != 0 {
			links = append(links, parentLink{child: id, parent: e.PID, thread: e.ThreadID})
		}
		created := orZero(e.Created, now)
		batch = append(batch, []any{id, ticketID, kind, staff, e.Poster, title, e.Body, mapFormat(e.Format), created, orZero(e.Updated, created)})
		rep.Written(EntityEntries)
		return nil
	})
	if err != nil {
		return err
	}
	if err := w.Insert(ctx, "thread_entry", []string{"id", "ticket_id", "type", "staff_id", "poster", "title", "body", "format", "created_at", "updated_at"}, batch); err != nil {
		return err
	}
	// Second pass: parents may have higher ids than their children, so link after all rows exist.
	for _, l := range links {
		parent, ok := lk.Entries[l.parent]
		if !ok || entryThread[parent] != l.thread {
			continue
		}
		if err := w.Exec(ctx, "UPDATE thread_entry SET parent_id = $1 WHERE id = $2", parent, l.child); err != nil {
			return err
		}
	}
	return nil
}
```

- [ ] **Step 4: Write files.go**

```go
package importer

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/grandpine/ticket-api/internal/attachment"
)

// newStorageKey mirrors the upload service: 32 random bytes, hex encoded.
func newStorageKey() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// discardStorage hashes and counts bytes without keeping them (dry runs).
type discardStorage struct{}

func (discardStorage) Put(_ context.Context, _ string, r io.Reader) (int64, string, error) {
	h := sha256.New()
	n, err := io.Copy(h, r)
	if err != nil {
		return 0, "", err
	}
	return n, hex.EncodeToString(h.Sum(nil)), nil
}

func (discardStorage) Open(context.Context, string) (io.ReadCloser, error) {
	return nil, errors.New("dry run: nothing stored")
}

func (discardStorage) Delete(context.Context, string) error { return nil }

var _ attachment.Storage = discardStorage{}

// openSourceFile returns the bytes of an osTicket file from its backend.
func openSourceFile(ctx context.Context, src *Source, f SrcFile, filesDir string) (io.ReadCloser, string, error) {
	switch f.Backend {
	case "D":
		rc, err := src.OpenChunks(ctx, f.ID)
		return rc, "", err
	case "F":
		if filesDir == "" {
			return nil, ReasonFilesDirMissing, nil
		}
		for _, p := range []string{filepath.Join(filesDir, f.Key), filepath.Join(filesDir, f.Key[:min(2, len(f.Key))], f.Key)} {
			if fh, err := os.Open(p); err == nil {
				return fh, "", nil
			}
		}
		return nil, "file not found on disk", nil
	default:
		return nil, "unsupported storage backend " + f.Backend, nil
	}
}

func importFiles(ctx context.Context, src *Source, w *Writer, lk *Lookup, rep *Report, store attachment.Storage, filesDir string) error {
	type pair struct{ entry, file int64 }
	seen := map[pair]bool{}
	// Files that could not be copied, so later attachments of the same file skip with the same reason.
	failed := map[int64]string{}
	entryStaff := map[int64]*int64{}
	rows, err := w.Query(ctx, "SELECT id, staff_id FROM thread_entry")
	if err != nil {
		return err
	}
	for rows.Next() {
		var id int64
		var staff *int64
		if err := rows.Scan(&id, &staff); err != nil {
			rows.Close()
			return err
		}
		entryStaff[id] = staff
	}
	rows.Close()

	now := time.Now()
	var attachments [][]any
	err = src.Attachments(ctx, func(a SrcAttachment) error {
		rep.Read(EntityAttachments)
		entryID, ok := lk.Entries[a.EntryID]
		if !ok {
			rep.Note(EntityAttachments, a.FileID, fmt.Sprintf("entry %d not imported", a.EntryID))
			return nil
		}
		if reason, ok := failed[a.FileID]; ok {
			rep.Skip(EntityAttachments, a.FileID, reason)
			return nil
		}
		if _, ok := lk.Files[a.FileID]; !ok {
			rep.Read(EntityFiles)
			rc, reason, err := openSourceFile(ctx, src, a.File, filesDir)
			if err != nil {
				return err
			}
			if reason != "" {
				failed[a.FileID] = reason
				rep.Skip(EntityFiles, a.FileID, reason)
				rep.Skip(EntityAttachments, a.FileID, reason)
				return nil
			}
			key, err := newStorageKey()
			if err != nil {
				rc.Close()
				return err
			}
			size, sum, err := store.Put(ctx, key, rc)
			rc.Close()
			if err != nil {
				return fmt.Errorf("store file %d: %w", a.FileID, err)
			}
			if size != a.File.Size {
				rep.Note(EntityFiles, a.FileID, fmt.Sprintf("size %d differs from source %d", size, a.File.Size))
			}
			name := strings.TrimSpace(a.File.Name)
			if name == "" {
				name = key
			}
			mime := a.File.Type
			if mime == "" {
				mime = "application/octet-stream"
			}
			id := lk.allocID("file", a.FileID)
			lk.Files[a.FileID] = id
			if err := w.Insert(ctx, "file", []string{"id", "key", "name", "mime", "size", "sha256", "backend", "uploaded_by", "created_at"},
				[][]any{{id, key, name, mime, size, sum, "local", entryStaff[entryID], orZero(a.File.Created, now)}}); err != nil {
				return err
			}
			rep.Written(EntityFiles)
		}
		p := pair{entryID, lk.Files[a.FileID]}
		if seen[p] {
			rep.Note(EntityAttachments, a.FileID, fmt.Sprintf("duplicate attachment on entry %d", a.EntryID))
			return nil
		}
		seen[p] = true
		attachments = append(attachments, []any{entryID, lk.Files[a.FileID], a.Inline})
		rep.Written(EntityAttachments)
		return nil
	})
	if err != nil {
		return err
	}
	return w.Insert(ctx, "attachment", []string{"thread_entry_id", "file_id", "inline"}, attachments)
}
```

`attachment.name` in the source overrides the display name only in osTicket's UI; the API's `file.name` is the file's name, so the source `file.name` is used (the spec's "attachment.name if set" is applied to nothing else because the API's `attachment` table has no name column). Record this in the task report.

- [ ] **Step 5: Run the pure tests**

Run: `cd gin && go test ./internal/importer/ -run 'TestMapEntryType|TestStorageKey'`
Expected: PASS.

- [ ] **Step 6: Write the failing database test**

Create `gin/internal/importer/thread_db_test.go`:

```go
package importer

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/grandpine/ticket-api/internal/attachment"
	"github.com/grandpine/ticket-api/internal/db/testutil"
)

func setupThroughTickets(t *testing.T) (*Sink, *Source, *Lookup, *Report) {
	t.Helper()
	tx := testutil.Tx(t)
	ctx := context.Background()
	src := openTestSource(t)
	sink := NewSink(tx, 3, false)
	lk, rep := NewLookup(), NewReport()
	runReferenceSteps(t, sink, src, lk, rep)
	if err := sink.Step(ctx, func(w *Writer) error { return importTickets(ctx, src, w, lk, rep) }); err != nil {
		t.Fatal(err)
	}
	return sink, src, lk, rep
}
```

The same file continues:

```go
func TestImportEntriesParentLinks(t *testing.T) {
	sink, src, lk, rep := setupThroughTickets(t)
	ctx := context.Background()
	if err := sink.Step(ctx, func(w *Writer) error { return importEntries(ctx, src, w, lk, rep) }); err != nil {
		t.Fatal(err)
	}
	c := rep.Counter(EntityEntries)
	// 10 read: 8 written (ticket 1: 1,2,3,4,5; ticket 2: 6,7; ticket 3: 8), entry 9 (deleted ticket) and 11 (type X) skipped.
	if c.Read != 10 || c.Written != 8 || c.Skipped != 2 {
		t.Fatalf("entries = %+v", c)
	}
	var err error
	var parent *int64
	var kind, format string
	if err = sink.db.QueryRow(ctx, "SELECT parent_id, type, format FROM thread_entry WHERE id = $1", lk.Entries[4]).Scan(&parent, &kind, &format); err != nil {
		t.Fatal(err)
	}
	if parent == nil || *parent != lk.Entries[5] || kind != "message" || format != "text" {
		t.Fatalf("entry 4 = parent %v type %s format %s", parent, kind, format)
	}
	if err = sink.db.QueryRow(ctx, "SELECT parent_id, format FROM thread_entry WHERE id = $1", lk.Entries[5]).Scan(&parent, &format); err != nil || parent != nil || format != "html" {
		t.Fatalf("entry 5 = %v %s, %v", parent, format, err)
	}
	if err = sink.db.QueryRow(ctx, "SELECT parent_id FROM thread_entry WHERE id = $1", lk.Entries[2]).Scan(&parent); err != nil || parent == nil || *parent != lk.Entries[1] {
		t.Fatalf("entry 2 parent = %v, %v", parent, err)
	}
	var staff *int64
	var title *string
	if err = sink.db.QueryRow(ctx, "SELECT staff_id, title FROM thread_entry WHERE id = $1", lk.Entries[3]).Scan(&staff, &title); err != nil || staff == nil || *staff != lk.Staff[1] || title == nil || *title != "Ops note" {
		t.Fatalf("entry 3 = %v %v, %v", staff, title, err)
	}
}

func TestImportFilesBothBackends(t *testing.T) {
	sink, src, lk, rep := setupThroughTickets(t)
	ctx := context.Background()
	if err := sink.Step(ctx, func(w *Writer) error { return importEntries(ctx, src, w, lk, rep) }); err != nil {
		t.Fatal(err)
	}
	filesDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(filesDir, "fskey2"), []byte("PNG!"), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := attachment.NewLocalStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := sink.Step(ctx, func(w *Writer) error { return importFiles(ctx, src, w, lk, rep, store, filesDir) }); err != nil {
		t.Fatal(err)
	}
	f := rep.Counter(EntityFiles)
	a := rep.Counter(EntityAttachments)
	// Files: 1 (chunks) and 2 (disk) written; 3 (backend S) and 4 (missing) skipped.
	if f.Written != 2 || f.Skipped != 2 {
		t.Fatalf("files = %+v", f)
	}
	// Attachments: 6 read; entry3/file1 and entry7/file2 written; file 3 and 4 skipped; attachment 5 (deleted ticket) and 6 (duplicate) noted.
	if a.Read != 6 || a.Written != 2 || a.Skipped != 2 || len(a.Samples) != 4 {
		t.Fatalf("attachments = %+v", a)
	}
	var key, name, mime, sum string
	var size int64
	var by *int64
	if err := sink.db.QueryRow(ctx, "SELECT key, name, mime, size, sha256, uploaded_by FROM file WHERE id = $1", lk.Files[1]).Scan(&key, &name, &mime, &size, &sum, &by); err != nil {
		t.Fatal(err)
	}
	if name != "notes.txt" || mime != "text/plain" || size != 11 || sum != "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9" || by == nil || *by != lk.Staff[1] {
		t.Fatalf("file 1 = %s %s %d %s %v", name, mime, size, sum, by)
	}
	rc, err := store.Open(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(rc)
	rc.Close()
	if string(b) != "hello world" {
		t.Fatalf("stored bytes = %q", b)
	}
	if err := sink.db.QueryRow(ctx, "SELECT key, size FROM file WHERE id = $1", lk.Files[2]).Scan(&key, &size); err != nil || size != 4 {
		t.Fatalf("file 2 = %d, %v", size, err)
	}
	rc, _ = store.Open(ctx, key)
	b, _ = io.ReadAll(rc)
	rc.Close()
	if string(b) != "PNG!" {
		t.Fatalf("disk bytes = %q", b)
	}
	var n int
	var inline bool
	if err := sink.db.QueryRow(ctx, "SELECT count(*) FROM attachment").Scan(&n); err != nil || n != 2 {
		t.Fatalf("attachment rows = %d, %v", n, err)
	}
	if err := sink.db.QueryRow(ctx, "SELECT inline FROM attachment WHERE thread_entry_id = $1", lk.Entries[7]).Scan(&inline); err != nil || !inline {
		t.Fatalf("inline = %v, %v", inline, err)
	}
	if !rep.NeedsAttention() {
		t.Fatal("skipped attachments need attention")
	}
}

func TestImportFilesWithoutFilesDir(t *testing.T) {
	sink, src, lk, rep := setupThroughTickets(t)
	ctx := context.Background()
	if err := sink.Step(ctx, func(w *Writer) error { return importEntries(ctx, src, w, lk, rep) }); err != nil {
		t.Fatal(err)
	}
	if err := sink.Step(ctx, func(w *Writer) error { return importFiles(ctx, src, w, lk, rep, discardStorage{}, "") }); err != nil {
		t.Fatal(err)
	}
	if rep.Counter(EntityFiles).Reasons[ReasonFilesDirMissing] != 2 || rep.Counter(EntityFiles).Written != 1 {
		t.Fatalf("files = %+v", rep.Counter(EntityFiles))
	}
}
```

In `setupThroughTickets`, `sink.db` is the test transaction; the tests read through it. Since `Sink.db` is unexported but the tests are in the same package, that access is fine.

- [ ] **Step 7: Run to verify it passes**

Run: `cd gin && go test ./internal/importer/ -run 'TestImportEntries|TestImportFiles' -v 2>&1 | tail -20`
Expected: PASS (3 tests).

- [ ] **Step 8: Vet and commit**

Run: `cd gin && go vet ./... && gofmt -l ./cmd ./internal`

```bash
git add gin/internal/importer/thread.go gin/internal/importer/files.go gin/internal/importer/thread_test.go gin/internal/importer/thread_db_test.go
git commit -m "feat(api): import thread entries, files, and attachments

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 7: Events, last response, and the Run orchestrator

**Files:**
- Create: `gin/internal/importer/events.go`
- Create: `gin/internal/importer/importer.go`
- Test: `gin/internal/importer/events_test.go` (pure), `gin/internal/importer/importer_test.go` (end to end)

**Interfaces:**
- Consumes: everything above; `attachment.Storage`; `db.Beginner`.
- Produces:
  - `mapEvent(name, data string) (kind string, payload []byte, ok bool)`: created/closed/reopened/transferred/edited map to the same kind; `assigned` becomes `unassigned` when the decoded data has no positive `staff`; unknown names return ok=false. `payload` is the data parsed as a JSON object, `{"raw": "<data>"}` when it is not an object, `{}` when empty.
  - `importEvents(ctx, src, w, lk, rep) error`.
  - `setLastResponse(ctx context.Context, w *Writer) error`: one UPDATE that sets `ticket.last_response_at` to the newest `response` entry per ticket.
  - `Run(ctx context.Context, opts Options, target db.Beginner, store attachment.Storage) (*Report, error)`: validates options, opens the source, builds the sink, runs pre-flight, runs the ten steps, sets last response, resets sequences, closes the source. The report is always non-nil, even on error. In dry-run mode `store` is replaced by `discardStorage{}`.

- [ ] **Step 1: Write the failing pure test**

Create `gin/internal/importer/events_test.go`:

```go
package importer

import "testing"

func TestMapEvent(t *testing.T) {
	cases := []struct {
		name, data, kind, payload string
		ok                        bool
	}{
		{"created", "", "created", `{}`, true},
		{"closed", "", "closed", `{}`, true},
		{"reopened", "", "reopened", `{}`, true},
		{"transferred", `{"dept":2}`, "transferred", `{"dept":2}`, true},
		{"edited", "not json", "edited", `{"raw":"not json"}`, true},
		{"assigned", `{"staff":1}`, "assigned", `{"staff":1}`, true},
		{"assigned", `{"staff":0}`, "unassigned", `{"staff":0}`, true},
		{"assigned", ``, "unassigned", `{}`, true},
		{"assigned", `{"team":3}`, "unassigned", `{"team":3}`, true},
		{"viewed", "", "", "", false},
		{"released", "", "", "", false},
	}
	for _, c := range cases {
		kind, payload, ok := mapEvent(c.name, c.data)
		if ok != c.ok || kind != c.kind || (ok && string(payload) != c.payload) {
			t.Errorf("%s/%s = %s %s %v, want %s %s %v", c.name, c.data, kind, payload, ok, c.kind, c.payload, c.ok)
		}
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd gin && go test ./internal/importer/ -run TestMapEvent`
Expected: FAIL to compile.

- [ ] **Step 3: Write events.go**

```go
package importer

import (
	"context"
	"encoding/json"
	"time"
)

var eventKinds = map[string]string{
	"created": "created", "closed": "closed", "reopened": "reopened",
	"assigned": "assigned", "transferred": "transferred", "edited": "edited",
}

// mapEvent maps an osTicket event name and data blob to a ticket_event kind and JSON payload.
func mapEvent(name, data string) (string, []byte, bool) {
	kind, ok := eventKinds[name]
	if !ok {
		return "", nil, false
	}
	var obj map[string]any
	payload := []byte("{}")
	if data != "" {
		if err := json.Unmarshal([]byte(data), &obj); err == nil && obj != nil {
			payload = []byte(data)
		} else {
			payload, _ = json.Marshal(map[string]string{"raw": data})
		}
	}
	if kind == "assigned" {
		staff, _ := obj["staff"].(float64)
		if staff <= 0 {
			kind = "unassigned"
		}
	}
	return kind, payload, true
}

func importEvents(ctx context.Context, src *Source, w *Writer, lk *Lookup, rep *Report) error {
	threads, err := src.Threads(ctx)
	if err != nil {
		return err
	}
	now := time.Now()
	var batch [][]any
	err = src.Events(ctx, func(e SrcEvent) error {
		rep.Read(EntityEvents)
		if e.Annulled {
			rep.Skip(EntityEvents, e.ID, "annulled")
			return nil
		}
		ticketID, ok := lk.Tickets[threads[e.ThreadID]]
		if !ok {
			rep.Skip(EntityEvents, e.ID, "ticket not imported")
			return nil
		}
		kind, payload, ok := mapEvent(e.Name, e.Data)
		if !ok {
			rep.Skip(EntityEvents, e.ID, "unmapped event "+e.Name)
			return nil
		}
		var staff *int64
		if s, ok := lk.Staff[e.StaffID]; ok {
			staff = &s
		}
		id := lk.allocID("ticket_event", e.ID)
		batch = append(batch, []any{id, ticketID, staff, kind, payload, orZero(e.Timestamp, now)})
		rep.Written(EntityEvents)
		return nil
	})
	if err != nil {
		return err
	}
	return w.Insert(ctx, "ticket_event", []string{"id", "ticket_id", "staff_id", "kind", "data", "created_at"}, batch)
}

// setLastResponse derives last_response_at from the newest response entry of each ticket.
func setLastResponse(ctx context.Context, w *Writer) error {
	return w.Exec(ctx, `UPDATE ticket t SET last_response_at = r.latest
		FROM (SELECT ticket_id, max(created_at) AS latest FROM thread_entry WHERE type = 'response' GROUP BY ticket_id) r
		WHERE r.ticket_id = t.id`)
}
```

- [ ] **Step 4: Write importer.go**

```go
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
```

In dry-run mode `Sink` keeps every step inside one outer transaction and `Close` rolls it back, so the steps read each other's rows exactly as in a real run and the report is identical; only the commit is withheld. Say so in a comment above `Run`.

- [ ] **Step 5: Run the pure test**

Run: `cd gin && go test ./internal/importer/ -run TestMapEvent && go build ./...`
Expected: PASS and clean build.

- [ ] **Step 6: Write the failing end-to-end test**

Create `gin/internal/importer/importer_test.go`:

```go
package importer

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/grandpine/ticket-api/internal/attachment"
	"github.com/grandpine/ticket-api/internal/auth"
	"github.com/grandpine/ticket-api/internal/db"
	"github.com/grandpine/ticket-api/internal/db/testutil"
)

func TestRunEndToEnd(t *testing.T) {
	tx := testutil.Tx(t)
	ctx := context.Background()
	filesDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(filesDir, "fskey2"), []byte("PNG!"), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := attachment.NewLocalStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	rep, err := Run(ctx, Options{MySQLDSN: mysqlDSN(t), Prefix: fixturePrefix, FilesDir: filesDir, Batch: 2}, tx, store)
	if err != nil {
		t.Fatalf("run: %v\n%s", err, rep)
	}
	t.Log("\n" + rep.String())
	if rep.Counter(EntityTickets).Written != 4 || rep.Counter(EntityEntries).Written != 8 || rep.Counter(EntityFiles).Written != 2 {
		t.Fatalf("counts:\n%s", rep)
	}
	// Events: 10 read; written created(1), assigned(2), transferred(3), unassigned(5), edited(6), closed(7) = 6; skipped viewed, annulled, deleted-ticket, released.
	ev := rep.Counter(EntityEvents)
	if ev.Written != 6 || ev.Skipped != 4 {
		t.Fatalf("events = %+v", ev)
	}
	var kind string
	var data map[string]any
	if err := tx.QueryRow(ctx, "SELECT kind, data FROM ticket_event WHERE id = 5").Scan(&kind, &data); err != nil || kind != "unassigned" {
		t.Fatalf("event 5 = %s %v, %v", kind, data, err)
	}
	if err := tx.QueryRow(ctx, "SELECT kind, data FROM ticket_event WHERE id = 6").Scan(&kind, &data); err != nil || kind != "edited" || data["raw"] != "not json" {
		t.Fatalf("event 6 = %s %v, %v", kind, data, err)
	}
	// last_response_at: ticket 1's newest response is entry 5 at 12:30; ticket 3 has none.
	var last *time.Time
	if err := tx.QueryRow(ctx, "SELECT last_response_at FROM ticket WHERE id = 1").Scan(&last); err != nil || last == nil || !last.Equal(time.Date(2020, 1, 10, 12, 30, 0, 0, time.UTC)) {
		t.Fatalf("ticket 1 last response = %v, %v", last, err)
	}
	if err := tx.QueryRow(ctx, "SELECT last_response_at FROM ticket WHERE id = 3").Scan(&last); err != nil || last != nil {
		t.Fatalf("ticket 3 last response = %v, %v", last, err)
	}
	// Round trip through the API's own queries.
	q := db.New(tx)
	tk, err := q.GetTicket(ctx, 1)
	if err != nil || tk.Number != "100001" || tk.Subject != "Printer on fire" {
		t.Fatalf("GetTicket = %+v, %v", tk, err)
	}
	st, err := q.GetStaffByUsername(ctx, "admin")
	if err != nil || !auth.CheckPassword(st.PasswordHash, "agentpass1") {
		t.Fatalf("staff login = %+v, %v", st, err)
	}
	// Sequences were reset: a new ticket gets an id above the imported ones.
	var id int64
	if err := tx.QueryRow(ctx, `INSERT INTO ticket (number, subject, status_id, dept_id, priority_id, requester_email)
		VALUES ('900000', 'new', (SELECT id FROM ticket_status LIMIT 1), (SELECT id FROM department LIMIT 1), (SELECT id FROM ticket_priority LIMIT 1), 'n@example.test') RETURNING id`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if id <= 5 {
		t.Fatalf("new ticket id = %d, want above the imported ids", id)
	}
	if !rep.NeedsAttention() {
		t.Fatal("two attachments were skipped, so the run needs attention")
	}
}

func TestRunDryRun(t *testing.T) {
	tx := testutil.Tx(t)
	ctx := context.Background()
	rep, err := Run(ctx, Options{MySQLDSN: mysqlDSN(t), Prefix: fixturePrefix, DryRun: true}, tx, nil)
	if err != nil {
		t.Fatalf("dry run: %v\n%s", err, rep)
	}
	if rep.Counter(EntityTickets).Read != 5 || rep.Counter(EntityTickets).Written != 4 {
		t.Fatalf("dry-run counts:\n%s", rep)
	}
	var n int
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM ticket").Scan(&n); err != nil || n != 0 {
		t.Fatalf("dry run wrote %d tickets", n)
	}
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM staff").Scan(&n); err != nil || n != 0 {
		t.Fatalf("dry run wrote %d staff", n)
	}
}

func TestRunRefusesNonEmptyTarget(t *testing.T) {
	tx := testutil.Tx(t)
	ctx := context.Background()
	if _, err := tx.Exec(ctx, "INSERT INTO department (name) VALUES ('Extra')"); err != nil {
		t.Fatal(err)
	}
	rep, err := Run(ctx, Options{MySQLDSN: mysqlDSN(t), Prefix: fixturePrefix}, tx, nil)
	if !IsPreflight(err) || rep == nil {
		t.Fatalf("err = %v", err)
	}
}
```

Check the sqlc query names before running: `grep -n 'func (q \*Queries) Get' gin/internal/db/*.sql.go | grep -i 'ticket\|staff'`. Use the existing names for "get ticket by id" and "get staff by username" (they exist because the ticket service and login use them); adjust the two calls if the generated names differ, and say so in the report.

- [ ] **Step 7: Run to verify it passes**

Run: `cd gin && go test ./internal/importer/ -run 'TestRun' -v 2>&1 | tail -30`
Expected: PASS (3 tests) with the report printed by `t.Log`.

- [ ] **Step 8: Vet, package test, commit**

Run: `cd gin && go vet ./... && gofmt -l ./cmd ./internal && go test ./internal/importer/`

```bash
git add gin/internal/importer/events.go gin/internal/importer/importer.go gin/internal/importer/events_test.go gin/internal/importer/importer_test.go
git commit -m "feat(api): import events, derive last response, and orchestrate the run

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 8: CLI subcommand, exit codes, README, full gate

**Files:**
- Create: `gin/cmd/api/importcmd.go`
- Modify: `gin/cmd/api/main.go` (`main`, `run`)
- Create: `gin/cmd/api/importcmd_test.go`
- Modify: `gin/README.md`

**Interfaces:**
- Consumes: `importer.Run`, `importer.Options`, `importer.Report`, `importer.IsPreflight`, `attachment.NewLocalStorage`, `db.NewPool`.
- Produces: `api import-osticket` with flags `--mysql-dsn`, `--prefix`, `--files-dir`, `--timezone`, `--batch`, `--dry-run`; `type exitError struct { code int; err error }` with `Error()`; `exitCodeFor(rep *importer.Report, err error) int` (2 for pre-flight, 1 for any other error, 3 when `rep.NeedsAttention()`, else 0). The command reads `DATABASE_URL` (required) and `STORAGE_DIR` (default `./storage`) directly, so `JWT_SECRET` is not needed for an import.

- [ ] **Step 1: Write the failing test**

Create `gin/cmd/api/importcmd_test.go`:

```go
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
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd gin && go test ./cmd/api/`
Expected: FAIL to compile.

- [ ] **Step 3: Write importcmd.go**

```go
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
```

- [ ] **Step 4: Wire main.go**

In `gin/cmd/api/main.go`, change `main` to honour the exit code:

```go
func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if err := run(os.Args[1:]); err != nil {
		var ee *exitError
		if errors.As(err, &ee) {
			if ee.err != nil {
				slog.Error("fatal", "err", ee.err)
			}
			os.Exit(ee.code)
		}
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}
```

and in `run`, handle the new command before `config.Load` (it needs neither `JWT_SECRET` nor the shared pool):

```go
	switch cmd {
	case "serve", "create-admin", "gc-files", "import-osticket":
	default:
		return fmt.Errorf("unknown command %q (expected serve, create-admin, gc-files or import-osticket)", cmd)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if cmd == "import-osticket" {
		return importOsticket(ctx, args)
	}

	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	…
```

(remove the now-duplicated `ctx, stop := …` lines that sat after `config.Load`, and update the inner `default:` message to match).

- [ ] **Step 5: Run the tests and build**

Run: `cd gin && go test ./cmd/api/ && go build ./... && go vet ./... && gofmt -l ./cmd ./internal`
Expected: PASS, clean.

- [ ] **Step 6: Document**

Add to `gin/README.md`, after the `create-admin` paragraph, a section:

```markdown
## Importing an existing osTicket database

`import-osticket` copies an osTicket (1.18) MySQL database into a fresh API database:
departments, staff (passwords carry over), help topics, priorities, statuses, tickets with
their threads and attachments, and ticket history. Run it once, against a target that has
only the seed rows (it refuses otherwise), with the API stopped:

    DATABASE_URL=postgres://... STORAGE_DIR=./storage \
      go run ./cmd/api import-osticket \
        --mysql-dsn 'ost:secret@tcp(127.0.0.1:3306)/osticket' \
        --prefix ost_ --timezone Asia/Kuala_Lumpur \
        --files-dir /var/www/osticket/attachments   # only if the filesystem storage plugin was used

Flags: `--prefix` (default `ost_`), `--timezone` (zone of the MySQL datetimes, default `UTC`),
`--files-dir` (needed for files stored by the filesystem plugin; database-stored files need
nothing), `--batch` (rows per insert, default 500), `--dry-run` (validate and report only).

Ids and ticket numbers are preserved. The report lists, per entity, rows read, written,
merged into seed rows, and skipped with reasons. Exit codes: 0 done; 1 aborted (earlier
steps stay committed: drop and re-migrate the target before retrying); 2 target not empty;
3 done but some tickets or attachments were skipped for reasons other than a deleted
status, listed in the report. Not imported: SLAs, teams, collaborators, custom form fields
other than subject and priority, canned responses, knowledge base, email settings.
```

- [ ] **Step 7: Full gate**

Run: `cd gin && make test 2>&1 | tail -5`
Expected: every package passes and the last line reports coverage above 75%. The importer package's DB tests start both containers; allow several minutes.

- [ ] **Step 8: Commit**

```bash
git add gin/cmd/api/importcmd.go gin/cmd/api/importcmd_test.go gin/cmd/api/main.go gin/README.md
git commit -m "feat(api): import-osticket CLI command with exit codes and docs

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```
