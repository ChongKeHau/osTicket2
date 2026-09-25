package inbound

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// Source yields raw messages from a mailbox. handle receives each message's
// UID (stable while the folder's UIDVALIDITY is) with its raw bytes.
type Source interface {
	Cycle(ctx context.Context, max int, handle func(uid uint32, raw []byte) (markSeen bool, err error)) error
}

// MessageProcessor handles one raw message; *Processor implements it.
type MessageProcessor interface {
	Process(ctx context.Context, raw []byte) (Outcome, error)
}

// maxPerCycle bounds one poll.
const maxPerCycle = 100

// poisonAfter is how many consecutive failing cycles make a UID poison.
const poisonAfter = 3

// poisonRetry is how long a poison UID is skipped before it is offered to the
// processor again. A database outage of a few minutes poisons every message at
// the head of the mailbox; without an expiry they would wait for a restart.
const poisonRetry = 30 * time.Minute

// Poller feeds a Source into a Processor.
type Poller struct {
	src  Source
	proc MessageProcessor
	log  *slog.Logger

	mu       sync.Mutex
	failures map[uint32]int       // consecutive Process failures per UID
	poison   map[uint32]time.Time // UIDs skipped until the recorded time
	now      func() time.Time
}

func NewPoller(src Source, proc MessageProcessor) *Poller {
	return &Poller{src: src, proc: proc, log: slog.Default(), failures: map[uint32]int{}, poison: map[uint32]time.Time{}, now: time.Now}
}

// RunOnce processes up to maxPerCycle unseen messages. Permanent problems are
// recorded by the processor and the message is marked seen; a transient error
// stops the cycle and leaves the message unseen. A UID whose processing fails
// on poisonAfter consecutive cycles is logged at error level and skipped for
// poisonRetry (left unseen, so a restart or the expiry retries it), so it
// cannot block the messages behind it.
func (p *Poller) RunOnce(ctx context.Context) (int, error) {
	n := 0
	err := p.src.Cycle(ctx, maxPerCycle, func(uid uint32, raw []byte) (bool, error) {
		if p.poisoned(uid) {
			return false, nil
		}
		out, err := p.proc.Process(ctx, raw)
		if err != nil {
			if ctx.Err() != nil {
				return false, err
			}
			if p.fail(uid) {
				p.log.Error("inbound message keeps failing; skipping it for a while", "uid", uid, "failures", poisonAfter, "retry_after", poisonRetry, "err", err)
				return false, nil
			}
			return false, err
		}
		p.succeed(uid)
		n++
		if out.Outcome != "" {
			var ticketID int64
			if out.TicketID != nil {
				ticketID = *out.TicketID
			}
			p.log.Info("inbound mail", "outcome", out.Outcome, "ticket_id", ticketID, "reason", out.Reason)
		}
		return true, nil
	})
	return n, err
}

// poisoned reports whether uid is still on the skip list; an expired entry is
// dropped so the message gets another poisonAfter cycles.
func (p *Poller) poisoned(uid uint32) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	until, ok := p.poison[uid]
	if !ok {
		return false
	}
	if p.now().Before(until) {
		return true
	}
	delete(p.poison, uid)
	return false
}

// fail counts one more consecutive failure and reports whether uid is now poison.
func (p *Poller) fail(uid uint32) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.failures[uid]++
	if p.failures[uid] < poisonAfter {
		return false
	}
	delete(p.failures, uid)
	p.poison[uid] = p.now().Add(poisonRetry)
	return true
}

func (p *Poller) succeed(uid uint32) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.failures, uid)
}

// Run loops RunOnce every interval until ctx is done.
func (p *Poller) Run(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		p.cycle(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (p *Poller) cycle(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			p.log.Error("inbound poller panic", "panic", fmt.Sprint(r))
		}
	}()
	if _, err := p.RunOnce(ctx); err != nil && ctx.Err() == nil {
		p.log.Warn("inbound poll failed", "err", err)
	}
}
