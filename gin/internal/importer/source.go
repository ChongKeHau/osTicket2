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
	ID, DeptID                                   int64
	Username, Email, Passwd, FirstName, LastName string
	IsAdmin, IsActive                            bool
	Created, Updated                             time.Time
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
	ID                                                                              int64
	Number                                                                          string
	UserID, UserEmailID, StatusID, DeptID, TopicID, StaffID, SLAID, TeamID, EmailID int64
	Flags                                                                           uint32
	IPAddress, Source, SourceExtra                                                  string
	IsAnswered                                                                      bool
	DueDate, Closed, LastUpdate, Created, Updated                                   time.Time
}

type SrcEntry struct {
	ID, ThreadID, PID, StaffID        int64
	Type, Poster, Title, Body, Format string
	Created, Updated                  time.Time
}

type SrcFile struct {
	ID                       int64
	Backend, Type, Key, Name string
	Size                     int64
	Created                  time.Time
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
	q := "SELECT a.object_id, a.file_id, COALESCE(a.name,''), a.inline, f.bk, f.type, f.`key`, f.name, f.size, f.created FROM " + s.t("attachment") + " a JOIN " + s.t("file") + " f ON f.id = a.file_id WHERE a.type = 'H' ORDER BY a.id"
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
	q := "SELECT te.id, te.thread_id, te.staff_id, ev.name, COALESCE(te.data,''), te.annulled, te.timestamp FROM " + s.t("thread_event") + " te JOIN " + s.t("event") + " ev ON ev.id = te.event_id WHERE te.thread_type = 'T' ORDER BY te.id"
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
