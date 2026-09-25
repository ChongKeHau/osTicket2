package inbound

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/grandpine/ticket-api/internal/attachment"
	"github.com/grandpine/ticket-api/internal/db"
	"github.com/grandpine/ticket-api/internal/mail"
	"github.com/grandpine/ticket-api/internal/ticket"
	"github.com/jackc/pgx/v5"
)

// Options tune the processor.
type Options struct {
	OwnAddress    string
	DefaultDeptID int64 // 0: the first (seed) department
	MaxAttachment int64
	AllowedMIME   []string
}

// Outcome is what happened to one message.
type Outcome struct {
	Outcome  db.InboundOutcome
	Reason   string
	TicketID *int64
	EntryID  *int64
}

// Processor turns raw messages into tickets and replies.
type Processor struct {
	db       db.Beginner
	store    attachment.Storage
	notifier mail.Notifier
	opts     Options
	log      *slog.Logger
}

func NewProcessor(b db.Beginner, store attachment.Storage, notifier mail.Notifier, opts Options) *Processor {
	return &Processor{db: b, store: store, notifier: notifier, opts: opts, log: slog.Default()}
}

// Process handles one message. A non-nil error is transient: the caller must
// leave the message unseen and retry later.
func (p *Processor) Process(ctx context.Context, raw []byte) (Outcome, error) {
	parsed, perr := Parse(raw, p.opts.MaxAttachment)
	var messageID string
	if perr == nil {
		messageID = parsed.MessageID
	} else {
		sum := sha256.Sum256(raw)
		messageID = "<sha256-" + hex.EncodeToString(sum[:]) + "@inbound.local>"
	}
	var out Outcome
	var stored []string
	err := db.WithTx(ctx, p.db, func(q *db.Queries) error {
		// Dedupe before any other branch: a redelivery (commit succeeded,
		// marking seen did not) returns the recorded outcome, never a second row.
		if existing, err := q.GetInboundByMessageID(ctx, messageID); err == nil {
			out = Outcome{Outcome: existing.Outcome, Reason: existing.Reason, TicketID: existing.TicketID, EntryID: existing.EntryID}
			return nil
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if perr != nil {
			return p.record(ctx, q, &out, messageID, "", "", "", db.InboundOutcomeIgnored, "unparseable: "+dbSafe(perr.Error()), nil, nil)
		}
		if reason := IgnoreReason(parsed, p.opts.OwnAddress); reason != "" {
			return p.record(ctx, q, &out, parsed.MessageID, parsed.FromAddress, parsed.FromName, parsed.Subject, db.InboundOutcomeIgnored, reason, nil, nil)
		}
		match, err := p.matchTicket(ctx, q, parsed)
		if err != nil {
			return err
		}
		body, format := parsed.Text, "text"
		if strings.TrimSpace(body) == "" && parsed.HTML != "" {
			body, format = parsed.HTML, "html"
		}
		if strings.TrimSpace(body) == "" {
			body = "(empty message)"
		}
		fileIDs, notes, keys, err := p.storeAttachments(ctx, q, parsed.Attachments)
		stored = append(stored, keys...)
		if err != nil {
			return err
		}
		beginner, ok := q.DB().(db.Beginner)
		if !ok {
			return errors.New("inbound: query connection cannot begin a transaction")
		}
		svc := ticket.NewService(beginner, ticket.WithNotifier(p.notifier))
		poster := parsed.FromName
		if poster == "" {
			poster = parsed.FromAddress
		}
		reason := strings.Join(notes, "; ")
		if match != nil && strings.EqualFold(strings.TrimSpace(match.RequesterEmail), strings.TrimSpace(parsed.FromAddress)) {
			entry, err := svc.AppendMessage(ctx, match.ID, ticket.MessageInput{Poster: poster, Body: body, Format: format, FileIDs: fileIDs})
			if err != nil {
				return err
			}
			return p.record(ctx, q, &out, parsed.MessageID, parsed.FromAddress, parsed.FromName, parsed.Subject, db.InboundOutcomeReplied, reason, &match.ID, &entry.ID)
		}
		dept := p.opts.DefaultDeptID
		if dept == 0 {
			d, err := q.FirstDepartment(ctx)
			if err != nil {
				return err
			}
			dept = d.ID
		}
		tk, err := svc.CreateExternal(ctx, ticket.ExternalCreateInput{
			Subject: CleanSubject(parsed.Subject), Body: body, Format: format,
			RequesterName: parsed.FromName, RequesterEmail: parsed.FromAddress, DeptID: dept, FileIDs: fileIDs,
			AutoSubmitted: parsed.AutoSubmitted != "" && !strings.EqualFold(parsed.AutoSubmitted, "no"),
		})
		if err != nil {
			return err
		}
		return p.record(ctx, q, &out, parsed.MessageID, parsed.FromAddress, parsed.FromName, parsed.Subject, db.InboundOutcomeCreated, reason, &tk.ID, nil)
	})
	if err != nil {
		for _, k := range stored {
			if derr := p.store.Delete(ctx, k); derr != nil {
				p.log.Warn("inbound: could not remove stored attachment after failure", "key", k, "err", derr)
			}
		}
		return Outcome{}, err
	}
	return out, nil
}

func (p *Processor) record(ctx context.Context, q *db.Queries, out *Outcome, messageID, from, fromName, subject string, outcome db.InboundOutcome, reason string, ticketID, entryID *int64) error {
	rec, err := q.CreateInbound(ctx, db.CreateInboundParams{
		MessageID: messageID, FromAddress: from, FromName: fromName, Subject: subject,
		TicketID: ticketID, EntryID: entryID, Outcome: outcome, Reason: reason,
	})
	if err != nil {
		return err
	}
	*out = Outcome{Outcome: rec.Outcome, Reason: rec.Reason, TicketID: rec.TicketID, EntryID: rec.EntryID}
	return nil
}

// matchTicket finds the ticket a message refers to, by our message ids first, then by subject tag.
func (p *Processor) matchTicket(ctx context.Context, q *db.Queries, parsed *Parsed) (*db.GetTicketRow, error) {
	refs := append(append([]string{}, parsed.InReplyTo...), parsed.References...)
	for _, id := range TicketIDsFromRefs(refs) {
		row, err := q.GetTicket(ctx, id)
		if err == nil {
			return &row, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
	}
	if num := NumberFromSubject(parsed.Subject); num != "" {
		id, err := q.GetTicketByNumber(ctx, num)
		if err == nil {
			row, err := q.GetTicket(ctx, id)
			if err == nil {
				return &row, nil
			}
			if !errors.Is(err, pgx.ErrNoRows) {
				return nil, err
			}
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
	}
	return nil, nil
}

// storeAttachments writes acceptable attachments to storage and the file table.
func (p *Processor) storeAttachments(ctx context.Context, q *db.Queries, atts []Attachment) (ids []int64, notes []string, keys []string, err error) {
	for _, a := range atts {
		if a.Oversized {
			notes = append(notes, "attachment "+a.Filename+" skipped: too large")
			continue
		}
		if !p.mimeAllowed(a.MIME) {
			notes = append(notes, "attachment "+a.Filename+" skipped: type "+a.MIME+" not allowed")
			continue
		}
		var kb [32]byte
		if _, err := rand.Read(kb[:]); err != nil {
			return nil, nil, keys, err
		}
		key := hex.EncodeToString(kb[:])
		size, sum, err := p.store.Put(ctx, key, strings.NewReader(string(a.Data)))
		if err != nil {
			return nil, nil, keys, fmt.Errorf("store attachment %s: %w", a.Filename, err)
		}
		keys = append(keys, key)
		f, err := q.CreateFile(ctx, db.CreateFileParams{Key: key, Name: a.Filename, Mime: a.MIME, Size: size, Sha256: sum, Backend: "local"})
		if err != nil {
			return nil, nil, keys, err
		}
		ids = append(ids, f.ID)
	}
	return ids, notes, keys, nil
}

func (p *Processor) mimeAllowed(m string) bool {
	if len(p.opts.AllowedMIME) == 0 {
		return true
	}
	for _, a := range p.opts.AllowedMIME {
		if strings.EqualFold(a, m) {
			return true
		}
	}
	return false
}
