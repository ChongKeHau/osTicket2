package mail

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/grandpine/ticket-api/internal/db"
)

// MaxAttempts is the number of sends before a row is marked failed.
const MaxAttempts = 10

// Backoff returns the delay before the next attempt and whether attempts has
// reached the cutoff (the row is marked failed instead of rescheduled).
func Backoff(attempts int) (time.Duration, bool) {
	if attempts >= MaxAttempts {
		return 0, true
	}
	switch attempts {
	case 1:
		return time.Minute, false
	case 2:
		return 5 * time.Minute, false
	case 3:
		return 15 * time.Minute, false
	default:
		return time.Hour, false
	}
}

// Sender drains the outbox.
type Sender struct {
	db    db.Beginner
	t     Transport
	from  Address
	batch int
	log   *slog.Logger
}

func NewSender(b db.Beginner, t Transport, from Address, batch int) *Sender {
	if batch < 1 {
		batch = 20
	}
	return &Sender{db: b, t: t, from: from, batch: batch, log: slog.Default()}
}

// RunOnce claims one batch and sends it over one connection. Rows stay locked
// for the duration, so a second worker skips them.
func (s *Sender) RunOnce(ctx context.Context) (sent, failed int, err error) {
	err = db.WithTx(ctx, s.db, func(q *db.Queries) error {
		rows, err := q.ClaimOutbox(ctx, int32(s.batch))
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		sess, openErr := s.t.Open(ctx)
		if openErr != nil {
			// A dial failure isn't any one row's fault: back off every row we
			// claimed rather than leaving them pending with a burned attempt
			// but no record of why.
			for _, r := range rows {
				if err := s.markFailed(ctx, q, r, openErr); err != nil {
					return err
				}
				failed++
			}
			return nil
		}
		defer sess.Close()
		for _, r := range rows {
			raw, buildErr := Build(Outgoing{
				From: s.from, To: Address{Name: r.ToName, Address: r.ToAddress}, Subject: r.Subject,
				MessageID: r.MessageID, InReplyTo: deref(r.InReplyTo), AutoSubmitted: r.AutoSubmitted,
				Text: r.BodyText, HTML: r.BodyHtml,
			})
			sendErr := buildErr
			if sendErr == nil {
				sendErr = sess.Send(ctx, s.from.Address, r.ToAddress, raw)
			}
			if sendErr == nil {
				if err := q.MarkOutboxSent(ctx, r.ID); err != nil {
					return err
				}
				sent++
				continue
			}
			if err := s.markFailed(ctx, q, r, sendErr); err != nil {
				return err
			}
			failed++
		}
		return nil
	})
	return sent, failed, err
}

// markFailed records sendErr against r via the backoff schedule.
func (s *Sender) markFailed(ctx context.Context, q *db.Queries, r db.EmailOutbox, sendErr error) error {
	delay, dead := Backoff(int(r.Attempts))
	status := db.EmailStatusPending
	if dead {
		status = db.EmailStatusFailed
	}
	msg := sendErr.Error()
	if err := q.MarkOutboxFailed(ctx, db.MarkOutboxFailedParams{ID: r.ID, LastError: &msg, NextAttemptAt: time.Now().Add(delay), Status: status}); err != nil {
		return err
	}
	s.log.Warn("email send failed", "outbox_id", r.ID, "to", r.ToAddress, "attempt", r.Attempts, "status", status, "err", msg)
	return nil
}

// Run loops RunOnce every interval until ctx is done.
func (s *Sender) Run(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		s.cycle(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// cycleTimeout bounds one batch once it is detached from the loop context.
const cycleTimeout = 2 * time.Minute

// cycle runs one batch on a context detached from the loop's: cancelling the
// loop (shutdown) must not roll back MarkOutboxSent for mail SMTP has already
// accepted, which would send it again on the next start. The loop context only
// decides whether another cycle starts.
func (s *Sender) cycle(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			s.log.Error("email sender panic", "panic", fmt.Sprint(r))
		}
	}()
	work, cancel := context.WithTimeout(context.WithoutCancel(ctx), cycleTimeout)
	defer cancel()
	if _, _, err := s.RunOnce(work); err != nil {
		s.log.Warn("email sender cycle failed", "err", err)
	}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
