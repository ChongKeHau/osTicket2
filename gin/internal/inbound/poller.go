package inbound

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// Source yields raw messages from a mailbox.
type Source interface {
	Cycle(ctx context.Context, max int, handle func(raw []byte) (markSeen bool, err error)) error
}

// maxPerCycle bounds one poll.
const maxPerCycle = 100

// Poller feeds a Source into a Processor.
type Poller struct {
	src  Source
	proc *Processor
	log  *slog.Logger
}

func NewPoller(src Source, proc *Processor) *Poller {
	return &Poller{src: src, proc: proc, log: slog.Default()}
}

// RunOnce processes up to maxPerCycle unseen messages. Permanent problems are
// recorded by the processor and the message is marked seen; a transient error
// stops the cycle and leaves the message unseen.
func (p *Poller) RunOnce(ctx context.Context) (int, error) {
	n := 0
	err := p.src.Cycle(ctx, maxPerCycle, func(raw []byte) (bool, error) {
		out, err := p.proc.Process(ctx, raw)
		if err != nil {
			return false, err
		}
		n++
		if out.Outcome != "" {
			p.log.Info("inbound mail", "outcome", out.Outcome, "ticket_id", out.TicketID, "reason", out.Reason)
		}
		return true, nil
	})
	return n, err
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
