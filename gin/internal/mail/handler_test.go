package mail

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/grandpine/ticket-api/internal/auth"
	"github.com/grandpine/ticket-api/internal/db"
	"github.com/grandpine/ticket-api/internal/db/testutil"
)

func adminRouter(h *Handler, p auth.Principal) *gin.Engine {
	gin.SetMode(gin.TestMode)
	e := gin.New()
	g := e.Group("/api/v1", func(c *gin.Context) { auth.WithPrincipal(c, p); c.Next() })
	h.Mount(g)
	return e
}

func do(e *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)
	return w
}

func TestTemplatesListAndPatch(t *testing.T) {
	tx := testutil.Tx(t)
	r := NewRenderer(time.Minute)
	e := adminRouter(NewHandler(tx, r), auth.Principal{StaffID: 1, IsAdmin: true})
	w := do(e, http.MethodGet, "/api/v1/email/templates", "")
	if w.Code != 200 {
		t.Fatalf("list = %d %s", w.Code, w.Body)
	}
	var list struct {
		Items []map[string]any `json:"items"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &list)
	if len(list.Items) != 4 || list.Items[0]["key"] != "assigned_alert" {
		t.Fatalf("items = %v", list.Items)
	}
	w = do(e, http.MethodPatch, "/api/v1/email/templates/ticket_reply", `{"subject":"New {{.Number}}"}`)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"subject":"New {{.Number}}"`) {
		t.Fatalf("patch = %d %s", w.Code, w.Body)
	}
	out, err := r.Render(context.Background(), db.New(tx), "ticket_reply", SampleVars())
	if err != nil || !strings.HasPrefix(out.Subject, "New ") {
		t.Fatalf("cache not invalidated: %q %v", out.Subject, err)
	}
	w = do(e, http.MethodPatch, "/api/v1/email/templates/ticket_reply", `{"body_html":"<p>{{.Nope}}</p>"}`)
	if w.Code != 400 || !strings.Contains(w.Body.String(), `"body_html"`) {
		t.Fatalf("bad template = %d %s", w.Code, w.Body)
	}
	w = do(e, http.MethodPatch, "/api/v1/email/templates/nope", `{"subject":"x"}`)
	if w.Code != 404 {
		t.Fatalf("unknown key = %d", w.Code)
	}
	w = do(e, http.MethodPatch, "/api/v1/email/templates/ticket_reply", `{}`)
	if w.Code != 400 {
		t.Fatalf("empty patch = %d", w.Code)
	}
}

func TestOutboxListRetryAndInbound(t *testing.T) {
	tx := testutil.Tx(t)
	ctx := context.Background()
	q := db.New(tx)
	tid := newTicket(t, q)
	id := queueOne(t, q, tid, "pat@example.test")
	if _, err := tx.Exec(ctx, "UPDATE email_outbox SET status = 'failed', attempts = 10 WHERE id = $1", id); err != nil {
		t.Fatal(err)
	}
	if _, err := q.CreateInbound(ctx, db.CreateInboundParams{MessageID: "<m@x>", FromAddress: "a@b.test", Subject: "s", Outcome: db.InboundOutcomeIgnored, Reason: "loop"}); err != nil {
		t.Fatal(err)
	}
	e := adminRouter(NewHandler(tx, NewRenderer(time.Minute)), auth.Principal{StaffID: 1, IsAdmin: true})
	w := do(e, http.MethodGet, "/api/v1/email/outbox?status=failed", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"total":1`) || strings.Contains(w.Body.String(), "body_html") {
		t.Fatalf("outbox = %d %s", w.Code, w.Body)
	}
	if w := do(e, http.MethodGet, "/api/v1/email/outbox?status=bogus", ""); w.Code != 400 {
		t.Fatalf("bad status = %d", w.Code)
	}
	w = do(e, http.MethodPost, "/api/v1/email/outbox/"+itoa(id)+"/retry", "")
	if w.Code != 200 {
		t.Fatalf("retry = %d %s", w.Code, w.Body)
	}
	row, _ := q.GetOutbox(ctx, id)
	if row.Status != db.EmailStatusPending || row.Attempts != 0 {
		t.Fatalf("row = %+v", row)
	}
	if err := q.MarkOutboxSent(ctx, id); err != nil {
		t.Fatal(err)
	}
	if w := do(e, http.MethodPost, "/api/v1/email/outbox/"+itoa(id)+"/retry", ""); w.Code != 409 {
		t.Fatalf("retry sent = %d", w.Code)
	}
	if w := do(e, http.MethodPost, "/api/v1/email/outbox/999999/retry", ""); w.Code != 404 {
		t.Fatalf("retry missing = %d", w.Code)
	}
	w = do(e, http.MethodGet, "/api/v1/email/inbound", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"reason":"loop"`) {
		t.Fatalf("inbound = %d %s", w.Code, w.Body)
	}
	// Non-admin is forbidden.
	e = adminRouter(NewHandler(tx, NewRenderer(time.Minute)), auth.Principal{StaffID: 2})
	if w := do(e, http.MethodGet, "/api/v1/email/templates", ""); w.Code != 403 {
		t.Fatalf("non-admin = %d", w.Code)
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
