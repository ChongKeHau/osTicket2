package mail

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/grandpine/ticket-api/internal/db"
	"github.com/grandpine/ticket-api/internal/db/testutil"
)

func smtpOpts(addr string) SMTPOptions {
	host, port, _ := net.SplitHostPort(addr)
	p, _ := strconv.Atoi(port)
	return SMTPOptions{Host: host, Port: p, TLS: "none", User: "u", Password: "p", Timeout: 5 * time.Second}
}

func TestSMTPTransportSends(t *testing.T) {
	srv := startFakeSMTP(t)
	tr := NewSMTP(smtpOpts(srv.addr))
	sess, err := tr.Open(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	raw, _ := Build(Outgoing{From: Address{Address: "a@b.test"}, To: Address{Address: "c@d.test"}, Subject: "hi", MessageID: "<m@b.test>", Text: "t", HTML: "<p>t</p>"})
	if err := sess.Send(context.Background(), "a@b.test", "c@d.test", raw); err != nil {
		t.Fatal(err)
	}
	got := srv.received()
	if len(got) != 1 || !strings.Contains(got[0], "Subject: hi") {
		t.Fatalf("received = %q", got)
	}
	srv.rejectTo = "nobody@d.test"
	err = sess.Send(context.Background(), "a@b.test", "nobody@d.test", raw)
	if err == nil || !strings.Contains(err.Error(), "550") {
		t.Fatalf("rejection error = %v", err)
	}
}

// TestSMTPDeadlinePerSend: the timeout applies to each message exchange, not
// to the whole session, so a batch that outlasts one timeout still goes out.
func TestSMTPDeadlinePerSend(t *testing.T) {
	srv := startFakeSMTP(t)
	o := smtpOpts(srv.addr)
	o.Timeout = 500 * time.Millisecond
	sess, err := NewSMTP(o).Open(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := Build(Outgoing{From: Address{Address: "a@b.test"}, To: Address{Address: "c@d.test"}, Subject: "hi", MessageID: "<m@b.test>", Text: "t"})
	for i := 0; i < 2; i++ {
		time.Sleep(300 * time.Millisecond) // the second send starts past Open + Timeout
		if err := sess.Send(context.Background(), "a@b.test", "c@d.test", raw); err != nil {
			t.Fatalf("send %d: %v", i, err)
		}
	}
	time.Sleep(300 * time.Millisecond)
	if err := sess.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if got := srv.received(); len(got) != 2 {
		t.Fatalf("received = %d", len(got))
	}
}

func TestBackoff(t *testing.T) {
	want := map[int]time.Duration{1: time.Minute, 2: 5 * time.Minute, 3: 15 * time.Minute, 4: time.Hour, 9: time.Hour}
	for attempts, d := range want {
		got, failed := Backoff(attempts)
		if got != d || failed {
			t.Fatalf("attempt %d = %v failed=%v", attempts, got, failed)
		}
	}
	if _, failed := Backoff(10); !failed {
		t.Fatal("tenth attempt marks failed")
	}
}

type flakyTransport struct {
	fail  bool
	calls int
	to    []string
}

func (f *flakyTransport) Open(context.Context) (Session, error) { return f, nil }

func (f *flakyTransport) Send(_ context.Context, _, to string, _ []byte) error {
	f.calls++
	f.to = append(f.to, to)
	if f.fail {
		return errors.New("boom")
	}
	return nil
}

func (f *flakyTransport) Close() error { return nil }

func queueOne(t *testing.T, q *db.Queries, tid int64, to string) int64 {
	t.Helper()
	mid, _ := NewMessageID(tid, "example.test")
	id, err := q.CreateOutbox(context.Background(), db.CreateOutboxParams{TicketID: &tid, TemplateKey: "ticket_reply", ToAddress: to, Subject: "s", BodyHtml: "<p>h</p>", BodyText: "t", MessageID: mid})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestSenderMarksSentAndRecordsRejection(t *testing.T) {
	tx := testutil.Tx(t)
	ctx := context.Background()
	q := db.New(tx)
	tid := newTicket(t, q)
	id := queueOne(t, q, tid, "pat@example.test")
	srv := startFakeSMTP(t)
	s := NewSender(tx, NewSMTP(smtpOpts(srv.addr)), Address{Name: "Desk", Address: "desk@example.test"}, 20)
	sent, failed, err := s.RunOnce(ctx)
	if err != nil || sent != 1 || failed != 0 {
		t.Fatalf("run = %d %d %v", sent, failed, err)
	}
	row, _ := q.GetOutbox(ctx, id)
	if row.Status != db.EmailStatusSent || row.SentAt == nil || row.Attempts != 1 {
		t.Fatalf("row = %+v", row)
	}
	if got := srv.received(); len(got) != 1 || !strings.Contains(got[0], "To: pat@example.test") {
		t.Fatalf("received = %q", got)
	}
	// Rejected recipient: pending with error and a later attempt.
	id2 := queueOne(t, q, tid, "nobody@example.test")
	srv.rejectTo = "nobody@example.test"
	before := time.Now()
	sent, failed, err = s.RunOnce(ctx)
	if err != nil || sent != 0 || failed != 1 {
		t.Fatalf("run = %d %d %v", sent, failed, err)
	}
	row, _ = q.GetOutbox(ctx, id2)
	if row.Status != db.EmailStatusPending || row.LastError == nil || !strings.Contains(*row.LastError, "550") || !row.NextAttemptAt.After(before.Add(50*time.Second)) {
		t.Fatalf("rejected row = %+v", row)
	}
	// Nothing due now.
	if sent, failed, _ := s.RunOnce(ctx); sent != 0 || failed != 0 {
		t.Fatal("row scheduled in the future must not be claimed")
	}
}

func TestSenderOpensOneConnectionPerBatch(t *testing.T) {
	tx := testutil.Tx(t)
	ctx := context.Background()
	q := db.New(tx)
	tid := newTicket(t, q)
	queueOne(t, q, tid, "a@example.test")
	queueOne(t, q, tid, "b@example.test")
	queueOne(t, q, tid, "c@example.test")
	srv := startFakeSMTP(t)
	s := NewSender(tx, NewSMTP(smtpOpts(srv.addr)), Address{Name: "Desk", Address: "desk@example.test"}, 20)
	sent, failed, err := s.RunOnce(ctx)
	if err != nil || sent != 3 || failed != 0 {
		t.Fatalf("run = %d %d %v", sent, failed, err)
	}
	if n := srv.connections(); n != 1 {
		t.Fatalf("connections = %d, want 1", n)
	}
	if got := srv.received(); len(got) != 3 {
		t.Fatalf("received = %d messages, want 3", len(got))
	}
}

// downTransport fails every Open, as an unreachable server or a rejected
// login does.
type downTransport struct{ opens int }

func (d *downTransport) Open(context.Context) (Session, error) {
	d.opens++
	return nil, errors.New("smtp auth smtp.example.test:465: 535 authentication failed")
}

// TestSenderOpenFailureLeavesRowsUntouched: an unreachable server is nobody's
// row's fault, so the claim is rolled back (no attempt burned, no backoff, no
// last_error), the cycle logs one warn line and the error is returned.
func TestSenderOpenFailureLeavesRowsUntouched(t *testing.T) {
	tx := testutil.Tx(t)
	ctx := context.Background()
	q := db.New(tx)
	tid := newTicket(t, q)
	ids := []int64{queueOne(t, q, tid, "a@example.test"), queueOne(t, q, tid, "b@example.test")}
	before := map[int64]db.EmailOutbox{}
	for _, id := range ids {
		before[id], _ = q.GetOutbox(ctx, id)
	}
	tr := &downTransport{}
	s := NewSender(tx, tr, Address{Address: "desk@example.test"}, 20)
	var logs bytes.Buffer
	s.log = slog.New(slog.NewTextHandler(&logs, nil))
	sent, failed, err := s.RunOnce(ctx)
	if err == nil || sent != 0 || failed != 0 {
		t.Fatalf("run = %d %d %v", sent, failed, err)
	}
	s.cycle(ctx)
	if tr.opens != 2 {
		t.Fatalf("opens = %d", tr.opens)
	}
	for _, id := range ids {
		row, _ := q.GetOutbox(ctx, id)
		b := before[id]
		if row.Status != db.EmailStatusPending || row.Attempts != b.Attempts || row.LastError != nil || !row.NextAttemptAt.Equal(b.NextAttemptAt) {
			t.Fatalf("row %d = %+v, before %+v", id, row, b)
		}
	}
	lines := strings.Split(strings.TrimSpace(logs.String()), "\n")
	if len(lines) != 1 || !strings.Contains(lines[0], "level=WARN") || !strings.Contains(lines[0], "smtp.example.test") {
		t.Fatalf("cycle logs = %q", logs.String())
	}
}

// TestSenderDialFailureNamesHostNotSecret uses the real transport against a
// closed port: the returned error names the host and never the password.
func TestSenderDialFailureNamesHostNotSecret(t *testing.T) {
	tx := testutil.Tx(t)
	ctx := context.Background()
	q := db.New(tx)
	tid := newTicket(t, q)
	id := queueOne(t, q, tid, "a@example.test")
	// Nothing listens on 127.0.0.1:1, so Open fails to dial.
	tr := NewSMTP(SMTPOptions{Host: "127.0.0.1", Port: 1, TLS: "none", User: "desk", Password: "s3cret-token", Timeout: 2 * time.Second})
	s := NewSender(tx, tr, Address{Address: "desk@example.test"}, 20)
	_, _, err := s.RunOnce(ctx)
	if err == nil || !strings.Contains(err.Error(), "127.0.0.1") || strings.Contains(err.Error(), "s3cret-token") {
		t.Fatalf("err = %v", err)
	}
	if row, _ := q.GetOutbox(ctx, id); row.Attempts != 0 || row.LastError != nil || row.Status != db.EmailStatusPending {
		t.Fatalf("row = %+v", row)
	}
}

func TestSenderFailsAfterMaxAttempts(t *testing.T) {
	tx := testutil.Tx(t)
	ctx := context.Background()
	q := db.New(tx)
	tid := newTicket(t, q)
	id := queueOne(t, q, tid, "pat@example.test")
	if _, err := tx.Exec(ctx, "UPDATE email_outbox SET attempts = $1 WHERE id = $2", MaxAttempts-1, id); err != nil {
		t.Fatal(err)
	}
	s := NewSender(tx, &flakyTransport{fail: true}, Address{Address: "desk@example.test"}, 20)
	if _, failed, err := s.RunOnce(ctx); err != nil || failed != 1 {
		t.Fatalf("run failed=%d err=%v", failed, err)
	}
	row, _ := q.GetOutbox(ctx, id)
	if row.Status != db.EmailStatusFailed || row.Attempts != MaxAttempts {
		t.Fatalf("row = %+v", row)
	}
	if n, err := q.RetryOutbox(ctx, id); err != nil || n != 1 {
		t.Fatalf("retry = %d %v", n, err)
	}
	row, _ = q.GetOutbox(ctx, id)
	if row.Status != db.EmailStatusPending || row.Attempts != 0 {
		t.Fatalf("after retry = %+v", row)
	}
}

func TestClaimOutboxSkipsLockedRows(t *testing.T) {
	// SKIP LOCKED only shows across connections, so this test commits one row to the
	// shared database and claims it from two separate transactions.
	pool := testutil.Pool(t)
	ctx := context.Background()
	setup, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	q := db.New(setup)
	tid := newTicket(t, q)
	queueOne(t, q, tid, "a@example.test")
	if err := setup.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, "DELETE FROM ticket WHERE id = $1", tid) })
	txA, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer txA.Rollback(ctx)
	txB, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer txB.Rollback(ctx)
	a, err := db.New(txA).ClaimOutbox(ctx, 20)
	if err != nil || len(a) != 1 {
		t.Fatalf("first claim = %d %v", len(a), err)
	}
	done := make(chan int, 1)
	go func() {
		b, _ := db.New(txB).ClaimOutbox(ctx, 20)
		done <- len(b)
	}()
	select {
	case n := <-done:
		if n != 0 {
			t.Fatalf("second claim got %d rows", n)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("second claim blocked instead of skipping locked rows")
	}
}

// cancelOnSend cancels the loop context from inside every Send, simulating
// SIGTERM arriving while a batch is in flight.
type cancelOnSend struct {
	flakyTransport
	cancel context.CancelFunc
}

func (c *cancelOnSend) Open(context.Context) (Session, error) { return c, nil }

func (c *cancelOnSend) Send(ctx context.Context, from, to string, raw []byte) error {
	c.cancel()
	return c.flakyTransport.Send(ctx, from, to, raw)
}

func TestSenderRunFinishesBatchWhenCancelledMidSend(t *testing.T) {
	tx := testutil.Tx(t)
	q := db.New(tx)
	tid := newTicket(t, q)
	ids := []int64{queueOne(t, q, tid, "a@example.test"), queueOne(t, q, tid, "b@example.test")}
	loop, cancel := context.WithCancel(context.Background())
	defer cancel()
	tr := &cancelOnSend{cancel: cancel}
	s := NewSender(tx, tr, Address{Name: "Desk", Address: "desk@example.test"}, 20)
	done := make(chan struct{})
	go func() { s.Run(loop, time.Hour); close(done) }()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
	if tr.calls != 2 {
		t.Fatalf("sends = %d, want 2 (the whole batch)", tr.calls)
	}
	for _, id := range ids {
		row, err := q.GetOutbox(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		if row.Status != db.EmailStatusSent || row.SentAt == nil || row.Attempts != 1 {
			t.Fatalf("row %d = %+v, want sent after one attempt", id, row)
		}
	}
}

func TestSenderCycleRunsWithCancelledLoopContext(t *testing.T) {
	tx := testutil.Tx(t)
	q := db.New(tx)
	tid := newTicket(t, q)
	id := queueOne(t, q, tid, "a@example.test")
	loop, cancel := context.WithCancel(context.Background())
	cancel()
	tr := &flakyTransport{}
	s := NewSender(tx, tr, Address{Name: "Desk", Address: "desk@example.test"}, 20)
	s.cycle(loop)
	row, err := q.GetOutbox(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if tr.calls != 1 || row.Status != db.EmailStatusSent || row.Attempts != 1 {
		t.Fatalf("calls = %d, row = %+v, want sent", tr.calls, row)
	}
}
