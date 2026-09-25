package inbound

import (
	"context"
	"testing"
)

type fakeSource struct {
	msgs   [][]byte
	seen   []bool
	cycles int
}

func (f *fakeSource) Cycle(ctx context.Context, max int, handle func([]byte) (bool, error)) error {
	f.cycles++
	n := 0
	for i, m := range f.msgs {
		if f.seen[i] || n >= max {
			continue
		}
		mark, err := handle(m)
		if err != nil {
			return err
		}
		if mark {
			f.seen[i] = true
		}
		n++
	}
	return nil
}

func TestPollerProcessesUnseenAndMarks(t *testing.T) {
	f := newProcFixture(t)
	src := &fakeSource{msgs: [][]byte{fixture(t, "plain.eml"), fixture(t, "autoreply.eml"), []byte("garbage")}, seen: []bool{false, false, false}}
	p := NewPoller(src, f.proc)
	n, err := p.RunOnce(f.ctx)
	if err != nil || n != 3 {
		t.Fatalf("run = %d %v", n, err)
	}
	for i, s := range src.seen {
		if !s {
			t.Fatalf("message %d not marked seen", i)
		}
	}
	var tickets int
	_ = f.tx.QueryRow(f.ctx, "SELECT count(*) FROM ticket").Scan(&tickets)
	if tickets != 1 {
		t.Fatalf("tickets = %d", tickets)
	}
	// A second cycle finds nothing new.
	if n, _ := p.RunOnce(f.ctx); n != 0 {
		t.Fatalf("second run = %d", n)
	}
}

// TestPollerMarksSeenOnEmptyMessage covers what the IMAP adapter hands the
// poller for a missing or NIL body section: empty raw bytes rather than an
// error. Process records that as unparseable (nil error), so the poller must
// still mark it seen instead of retrying it forever.
func TestPollerMarksSeenOnEmptyMessage(t *testing.T) {
	f := newProcFixture(t)
	src := &fakeSource{msgs: [][]byte{{}}, seen: []bool{false}}
	p := NewPoller(src, f.proc)
	n, err := p.RunOnce(f.ctx)
	if err != nil || n != 1 {
		t.Fatalf("run = %d %v", n, err)
	}
	if !src.seen[0] {
		t.Fatal("empty message must be marked seen")
	}
}

func TestPollerLeavesMessageUnseenOnTransientError(t *testing.T) {
	f := newProcFixture(t)
	// Close the transaction so every database call fails.
	_ = f.tx.Rollback(f.ctx)
	src := &fakeSource{msgs: [][]byte{fixture(t, "plain.eml")}, seen: []bool{false}}
	p := NewPoller(src, f.proc)
	if _, err := p.RunOnce(f.ctx); err == nil {
		t.Fatal("expected a transient error")
	}
	if src.seen[0] {
		t.Fatal("message must stay unseen after a transient failure")
	}
}
