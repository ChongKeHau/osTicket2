package inbound

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"
)

// fakeSource serves msgs in order; message i has UID i+1.
type fakeSource struct {
	msgs   [][]byte
	seen   []bool
	cycles int
}

func (f *fakeSource) Cycle(ctx context.Context, max int, handle func(uint32, []byte) (bool, error)) error {
	f.cycles++
	n := 0
	for i, m := range f.msgs {
		if f.seen[i] || n >= max {
			continue
		}
		mark, err := handle(uint32(i+1), m)
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

// stubProcessor fails every message equal to poison and succeeds otherwise.
type stubProcessor struct {
	poison []byte
	calls  map[string]int
}

func (s *stubProcessor) Process(_ context.Context, raw []byte) (Outcome, error) {
	s.calls[string(raw)]++
	if bytes.Equal(raw, s.poison) {
		return Outcome{}, errors.New("boom")
	}
	return Outcome{}, nil
}

func TestPollerSkipsPoisonUIDAfterThreeCycles(t *testing.T) {
	src := &fakeSource{msgs: [][]byte{[]byte("poison"), []byte("a"), []byte("b")}, seen: []bool{false, false, false}}
	proc := &stubProcessor{poison: []byte("poison"), calls: map[string]int{}}
	p := NewPoller(src, proc)
	ctx := context.Background()
	// Cycles 1 and 2: the failure stops the cycle, as any transient error does.
	for i := 1; i < poisonAfter; i++ {
		if _, err := p.RunOnce(ctx); err == nil {
			t.Fatalf("cycle %d: expected the transient error", i)
		}
		if src.seen[1] || src.seen[2] {
			t.Fatalf("cycle %d: later messages processed early", i)
		}
	}
	// Cycle 3: the third consecutive failure poisons UID 1 and the cycle goes on.
	n, err := p.RunOnce(ctx)
	if err != nil || n != 2 {
		t.Fatalf("cycle 3 = %d %v", n, err)
	}
	if src.seen[0] || !src.seen[1] || !src.seen[2] {
		t.Fatalf("seen = %v, want poison unseen and the rest seen", src.seen)
	}
	// Later cycles skip it without calling Process.
	if n, err := p.RunOnce(ctx); err != nil || n != 0 {
		t.Fatalf("cycle 4 = %d %v", n, err)
	}
	if proc.calls["poison"] != poisonAfter || proc.calls["a"] != 1 || proc.calls["b"] != 1 {
		t.Fatalf("calls = %v", proc.calls)
	}
	if src.seen[0] {
		t.Fatal("poison message must stay unseen so a restart retries it")
	}
}

func TestPollerFailureCountResetsOnSuccess(t *testing.T) {
	src := &fakeSource{msgs: [][]byte{[]byte("flaky")}, seen: []bool{false}}
	proc := &stubProcessor{poison: []byte("flaky"), calls: map[string]int{}}
	p := NewPoller(src, proc)
	ctx := context.Background()
	for i := 1; i < poisonAfter; i++ {
		_, _ = p.RunOnce(ctx)
	}
	proc.poison = nil // it recovers
	if n, err := p.RunOnce(ctx); err != nil || n != 1 || !src.seen[0] {
		t.Fatalf("recovered = %d %v %v", n, err, src.seen)
	}
	if len(p.failures) != 0 || len(p.poison) != 0 {
		t.Fatalf("failures = %v poison = %v", p.failures, p.poison)
	}
}

func TestPollerRetriesPoisonUIDAfterExpiry(t *testing.T) {
	src := &fakeSource{msgs: [][]byte{[]byte("poison"), []byte("a")}, seen: []bool{false, false}}
	proc := &stubProcessor{poison: []byte("poison"), calls: map[string]int{}}
	p := NewPoller(src, proc)
	clock := time.Unix(1_700_000_000, 0)
	p.now = func() time.Time { return clock }

	for i := 0; i < poisonAfter; i++ {
		_, _ = p.RunOnce(context.Background())
	}
	if !p.poisoned(1) {
		t.Fatal("uid 1 should be poison after three failing cycles")
	}
	src.seen[1] = false
	if _, err := p.RunOnce(context.Background()); err != nil {
		t.Fatalf("cycle while poisoned: %v", err)
	}
	if proc.calls["poison"] != poisonAfter {
		t.Fatalf("poison processed %d times while skipped, want %d", proc.calls["poison"], poisonAfter)
	}

	// Once the retry window passes the message is offered again. It fails
	// once more, which starts a fresh count rather than skipping it for good.
	clock = clock.Add(poisonRetry + time.Second)
	if p.poisoned(1) {
		t.Fatal("uid 1 should be retried after the expiry")
	}
	src.seen[1] = false
	_, err := p.RunOnce(context.Background())
	if err == nil {
		t.Fatal("expected the retried poison message to fail the cycle again")
	}
	if proc.calls["poison"] != poisonAfter+1 {
		t.Fatalf("poison processed %d times after expiry, want %d", proc.calls["poison"], poisonAfter+1)
	}
}
