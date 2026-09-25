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
