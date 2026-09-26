package dashboard

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/grandpine/ticket-api/internal/apperr"
	"github.com/grandpine/ticket-api/internal/auth"
)

type fakeStats struct {
	got    []any
	result *Stats
	err    error
}

func (f *fakeStats) Stats(_ context.Context, p auth.Principal, start time.Time, period int) (*Stats, error) {
	f.got = []any{p, start, period}
	if f.err != nil {
		return nil, f.err
	}
	return f.result, nil
}

func router(f *fakeStats) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	private := r.Group("/api/v1", func(c *gin.Context) {
		auth.WithPrincipal(c, auth.Principal{StaffID: 7, DeptIDs: []int64{1}})
	})
	NewHandler(f).Mount(private)
	return r
}

func get(r *gin.Engine, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	return w
}

func TestStatsRouteParsesParams(t *testing.T) {
	f := &fakeStats{result: &Stats{Start: "2026-09-01", Period: 7, Series: []Point{}, ByDepartment: []Row{}, ByTopic: []Row{}, ByStaff: []Row{}}}
	w := get(router(f), "/api/v1/dashboard/stats?start=2026-09-01&period=7")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"by_department":[]`) {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if f.got[2] != 7 || f.got[1].(time.Time).Format("2006-01-02") != "2026-09-01" {
		t.Fatalf("params %+v", f.got)
	}
}

func TestStatsRouteDefaults(t *testing.T) {
	f := &fakeStats{result: &Stats{}}
	get(router(f), "/api/v1/dashboard/stats")
	if f.got[2] != DefaultPeriod {
		t.Fatalf("period %v", f.got[2])
	}
	want := time.Now().UTC().AddDate(0, 0, -(DefaultPeriod - 1)).Format("2006-01-02")
	if f.got[1].(time.Time).Format("2006-01-02") != want {
		t.Fatalf("start %v want %s", f.got[1], want)
	}
}

func TestStatsRouteRejectsBadInput(t *testing.T) {
	f := &fakeStats{err: apperr.Validation("period", "must be one of 7, 14, 30, 90")}
	if w := get(router(f), "/api/v1/dashboard/stats?period=10"); w.Code != 400 {
		t.Fatalf("period: %d %s", w.Code, w.Body.String())
	}
	if w := get(router(f), "/api/v1/dashboard/stats?start=yesterday"); w.Code != 400 || !strings.Contains(w.Body.String(), "start") {
		t.Fatalf("start: %d %s", w.Code, w.Body.String())
	}
	if w := get(router(f), "/api/v1/dashboard/stats?period=abc"); w.Code != 400 {
		t.Fatalf("period text: %d", w.Code)
	}
}
