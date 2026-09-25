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
