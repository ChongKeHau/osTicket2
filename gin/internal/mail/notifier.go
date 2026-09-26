package mail

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"

	"github.com/grandpine/ticket-api/internal/config"
	"github.com/grandpine/ticket-api/internal/db"
	"github.com/jackc/pgx/v5"
)

// Recipient is one To address.
type Recipient struct{ Name, Address string }

// Notification is a request to mail one template to one or more recipients.
// TicketID is nil for account mail (portal sign-up, sign-in, reset), which
// neither threads onto earlier mail nor gets a ticket link by default.
type Notification struct {
	TemplateKey   string
	TicketID      *int64
	EntryID       *int64
	To            []Recipient
	Vars          Vars
	AutoSubmitted bool
}

// Notifier queues mail inside the caller's transaction.
type Notifier interface {
	Enqueue(ctx context.Context, q *db.Queries, n Notification) error
}

// Disabled is the Notifier used when mail is off.
type Disabled struct{}

func (Disabled) Enqueue(context.Context, *db.Queries, Notification) error { return nil }

// DBNotifier renders templates and inserts outbox rows.
type DBNotifier struct {
	r        *Renderer
	domain   string
	baseURL  string
	siteName string
	log      *slog.Logger
}

func NewNotifier(cfg config.MailConfig, r *Renderer) *DBNotifier {
	return &DBNotifier{r: r, domain: cfg.Domain, baseURL: cfg.BaseURL, siteName: cfg.SiteName, log: slog.Default()}
}

var messageIDRe = regexp.MustCompile(`^<?ticket-(\d+)-[0-9a-f]+@[^>]+>?$`)

// NewMessageID returns <ticket-<id>-<random>@domain>, or <client-<random>@domain>
// for mail that belongs to no ticket.
func NewMessageID(ticketID *int64, domain string) (string, error) {
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	if ticketID == nil {
		return fmt.Sprintf("<client-%s@%s>", hex.EncodeToString(b[:]), domain), nil
	}
	return fmt.Sprintf("<ticket-%d-%s@%s>", *ticketID, hex.EncodeToString(b[:]), domain), nil
}

// TicketIDFromMessageID recovers the ticket id from one of our message ids;
// client- ids (ticket-less mail) report false.
func TicketIDFromMessageID(id string) (int64, bool) {
	m := messageIDRe.FindStringSubmatch(id)
	if m == nil {
		return 0, false
	}
	n, err := strconv.ParseInt(m[1], 10, 64)
	return n, err == nil
}

// Enqueue renders once and inserts one outbox row per recipient. A render
// failure is logged and dropped so the surrounding ticket change still commits.
// A caller-supplied Vars.Link is kept; otherwise ticket mail links to the ticket.
func (n *DBNotifier) Enqueue(ctx context.Context, q *db.Queries, nt Notification) error {
	nt.Vars.SiteName = n.siteName
	if nt.Vars.Link == "" && nt.TicketID != nil {
		nt.Vars.Link = fmt.Sprintf("%s/tickets/%d", n.baseURL, *nt.TicketID)
	}
	rendered, err := n.r.Render(ctx, q, nt.TemplateKey, nt.Vars)
	if err != nil {
		attrs := []any{"template", nt.TemplateKey, "err", err}
		if nt.TicketID != nil {
			attrs = append(attrs, "ticket_id", *nt.TicketID)
		}
		n.log.Error("email render failed", attrs...)
		return nil
	}
	for _, to := range nt.To {
		if to.Address == "" {
			continue
		}
		var inReplyTo *string
		if nt.TicketID != nil {
			prev, err := q.LastSentMessageID(ctx, db.LastSentMessageIDParams{TicketID: nt.TicketID, ToAddress: to.Address})
			switch {
			case err == nil:
				inReplyTo = &prev
			case errors.Is(err, pgx.ErrNoRows):
			default:
				return err
			}
		}
		mid, err := NewMessageID(nt.TicketID, n.domain)
		if err != nil {
			return err
		}
		if _, err := q.CreateOutbox(ctx, db.CreateOutboxParams{
			TicketID: nt.TicketID, EntryID: nt.EntryID, TemplateKey: nt.TemplateKey,
			ToAddress: to.Address, ToName: to.Name, Subject: rendered.Subject,
			BodyHtml: rendered.HTML, BodyText: rendered.Text, MessageID: mid, InReplyTo: inReplyTo,
			AutoSubmitted: nt.AutoSubmitted,
		}); err != nil {
			return err
		}
	}
	return nil
}
