// Package dashboard aggregates ticket events for the staff dashboard.
package dashboard

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/grandpine/ticket-api/internal/apperr"
	"github.com/grandpine/ticket-api/internal/auth"
	"github.com/grandpine/ticket-api/internal/db"
)

// Periods lists the accepted window lengths in days.
var Periods = map[int]bool{7: true, 14: true, 30: true, 90: true}

const DefaultPeriod = 30

// Point is one day of the activity series.
type Point struct {
	Date     string `json:"date"`
	Opened   int64  `json:"opened"`
	Assigned int64  `json:"assigned"`
	Closed   int64  `json:"closed"`
	Reopened int64  `json:"reopened"`
}

// Row is one line of a breakdown table. ID is nil for the "— none —" and
// "— system —" rows.
type Row struct {
	ID       *int64 `json:"id"`
	Name     string `json:"name"`
	Opened   int64  `json:"opened"`
	Assigned int64  `json:"assigned"`
	Closed   int64  `json:"closed"`
	Reopened int64  `json:"reopened"`
}

// Stats is the dashboard response.
type Stats struct {
	Start        string  `json:"start"`
	Period       int     `json:"period"`
	Series       []Point `json:"series"`
	ByDepartment []Row   `json:"by_department"`
	ByTopic      []Row   `json:"by_topic"`
	ByStaff      []Row   `json:"by_staff"`
}

type Service struct{ b db.Beginner }

func NewService(b db.Beginner) *Service { return &Service{b: b} }

// Window returns the UTC [from, to) window for start and period.
func Window(start time.Time, period int) (time.Time, time.Time) {
	from := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
	return from, from.AddDate(0, 0, period)
}

// Stats aggregates events in [start, start+period) for the departments p can see.
func (s *Service) Stats(ctx context.Context, p auth.Principal, start time.Time, period int) (*Stats, error) {
	if !Periods[period] {
		return nil, apperr.Validation("period", "must be one of 7, 14, 30, 90")
	}
	from, to := Window(start, period)
	deptIDs := p.DeptIDs
	if deptIDs == nil {
		deptIDs = []int64{}
	}
	var out *Stats
	err := db.WithTx(ctx, s.b, func(q *db.Queries) error {
		series, err := q.DashboardSeries(ctx, db.DashboardSeriesParams{FromAt: from, ToAt: to, AllDepts: p.IsAdmin, DeptIds: deptIDs})
		if err != nil {
			return fmt.Errorf("series: %w", err)
		}
		byDay := map[string]Point{}
		for _, r := range series {
			d := r.Day.Time.Format("2006-01-02")
			byDay[d] = Point{Date: d, Opened: r.Opened, Assigned: r.Assigned, Closed: r.Closed, Reopened: r.Reopened}
		}
		points := make([]Point, 0, period)
		for i := 0; i < period; i++ {
			d := from.AddDate(0, 0, i).Format("2006-01-02")
			if p, ok := byDay[d]; ok {
				points = append(points, p)
			} else {
				points = append(points, Point{Date: d})
			}
		}
		depts, err := q.DashboardByDepartment(ctx, db.DashboardByDepartmentParams{FromAt: from, ToAt: to, AllDepts: p.IsAdmin, DeptIds: deptIDs})
		if err != nil {
			return fmt.Errorf("by department: %w", err)
		}
		topics, err := q.DashboardByTopic(ctx, db.DashboardByTopicParams{FromAt: from, ToAt: to, AllDepts: p.IsAdmin, DeptIds: deptIDs})
		if err != nil {
			return fmt.Errorf("by topic: %w", err)
		}
		staff, err := q.DashboardByStaff(ctx, db.DashboardByStaffParams{FromAt: from, ToAt: to, AllDepts: p.IsAdmin, DeptIds: deptIDs})
		if err != nil {
			return fmt.Errorf("by staff: %w", err)
		}
		out = &Stats{Start: from.Format("2006-01-02"), Period: period, Series: points,
			ByDepartment: make([]Row, 0, len(depts)), ByTopic: make([]Row, 0, len(topics)), ByStaff: make([]Row, 0, len(staff))}
		for _, r := range depts {
			id := r.ID
			out.ByDepartment = appendRow(out.ByDepartment, &id, r.Name, r.Opened, r.Assigned, r.Closed, r.Reopened)
		}
		for _, r := range topics {
			out.ByTopic = appendRow(out.ByTopic, r.ID, nullableName(r.Name, "— none —"), r.Opened, r.Assigned, r.Closed, r.Reopened)
		}
		for _, r := range staff {
			out.ByStaff = appendRow(out.ByStaff, r.ID, staffName(r), r.Opened, r.Assigned, r.Closed, r.Reopened)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func appendRow(rows []Row, id *int64, name string, opened, assigned, closed, reopened int64) []Row {
	if opened+assigned+closed+reopened == 0 {
		return rows
	}
	return append(rows, Row{ID: id, Name: name, Opened: opened, Assigned: assigned, Closed: closed, Reopened: reopened})
}

// nullableName returns fallback when v is nil or empty, matching what sqlc
// generates for a LEFT JOIN's nullable text column with
// emit_pointers_for_null_types (*string, not pgtype.Text).
func nullableName(v *string, fallback string) string {
	if v == nil || *v == "" {
		return fallback
	}
	return *v
}

func staffName(r db.DashboardByStaffRow) string {
	if r.ID == nil {
		return "— system —"
	}
	full := strings.TrimSpace(nullableName(r.FirstName, "") + " " + nullableName(r.LastName, ""))
	if full == "" {
		return nullableName(r.Username, "— system —")
	}
	return full
}
