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
	if len(list.Items) != 8 || list.Items[0]["key"] != "assigned_alert" {
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
	if !strings.Contains(w.Body.String(), `"ticket_id":`+itoa(tid)) {
		t.Fatalf("outbox ticket_id missing: %s", w.Body)
	}
	// Account mail has no ticket: its ticket_id is JSON null, not 0.
	mid, _ := NewMessageID(nil, "example.test")
	ticketless, err := q.CreateOutbox(ctx, db.CreateOutboxParams{TemplateKey: "client_confirm", ToAddress: "pat@example.test", Subject: "s", BodyHtml: "<p>h</p>", BodyText: "t", MessageID: mid})
	if err != nil {
		t.Fatal(err)
	}
	w = do(e, http.MethodGet, "/api/v1/email/outbox?status=pending", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"ticket_id":null`) {
		t.Fatalf("ticketless outbox = %d %s", w.Code, w.Body)
	}
	// Detail: the full row with both bodies; the ticketless row's ticket_id is null.
	w = do(e, http.MethodGet, "/api/v1/email/outbox/"+itoa(id), "")
	var detail struct {
		ID       int64  `json:"id"`
		TicketID *int64 `json:"ticket_id"`
		Subject  string `json:"subject"`
		Status   string `json:"status"`
		BodyText string `json:"body_text"`
		BodyHTML string `json:"body_html"`
	}
	if w.Code != 200 {
		t.Fatalf("outbox detail = %d %s", w.Code, w.Body)
	}
	if err := json.Unmarshal(w.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	want, _ := q.GetOutbox(ctx, id)
	if detail.ID != id || detail.TicketID == nil || *detail.TicketID != tid || detail.Status != "failed" || detail.Subject != want.Subject || detail.BodyText != want.BodyText || detail.BodyHTML != want.BodyHtml || detail.BodyText == "" {
		t.Fatalf("outbox detail = %+v", detail)
	}
	w = do(e, http.MethodGet, "/api/v1/email/outbox/"+itoa(ticketless), "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"ticket_id":null`) || !strings.Contains(w.Body.String(), `"body_text":"t"`) || !strings.Contains(w.Body.String(), `"body_html":"\u003cp\u003eh\u003c/p\u003e"`) {
		t.Fatalf("ticketless detail = %d %s", w.Code, w.Body)
	}
	if w := do(e, http.MethodGet, "/api/v1/email/outbox/999999", ""); w.Code != 404 {
		t.Fatalf("detail missing = %d", w.Code)
	}
	if w := do(e, http.MethodGet, "/api/v1/email/outbox/abc", ""); w.Code != 400 {
		t.Fatalf("detail bad id = %d", w.Code)
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
	if w := do(e, http.MethodGet, "/api/v1/email/outbox/"+itoa(id), ""); w.Code != 403 {
		t.Fatalf("non-admin detail = %d", w.Code)
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
