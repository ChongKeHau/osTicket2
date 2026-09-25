// Package inbound turns mailbox messages into tickets and replies.
package inbound

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/emersion/go-message"
	_ "github.com/emersion/go-message/charset" // register legacy charsets
	gomail "github.com/emersion/go-message/mail"
	"github.com/grandpine/ticket-api/internal/mail"
)

// Attachment is a non-inline part with a filename.
type Attachment struct {
	Filename  string
	MIME      string
	Data      []byte
	Oversized bool
}

// Parsed is the subset of a message the processor needs.
type Parsed struct {
	MessageID            string
	FromAddress          string
	FromName             string
	Subject              string
	Date                 time.Time
	InReplyTo            []string
	References           []string
	AutoSubmitted        string
	Precedence           string
	AutoResponseSuppress string
	ContentType          string
	Text                 string
	HTML                 string
	Attachments          []Attachment
}

// Parse reads headers, the first text/plain and text/html parts, and attachments.
func Parse(raw []byte, maxAttachment int64) (*Parsed, error) {
	mr, err := gomail.CreateReader(bytes.NewReader(raw))
	if err != nil && !message.IsUnknownCharset(err) {
		return nil, fmt.Errorf("parse message: %w", err)
	}
	if mr == nil {
		return nil, errors.New("parse message: no reader")
	}
	p := &Parsed{}
	h := mr.Header
	if from, err := h.AddressList("From"); err == nil && len(from) > 0 && from[0].Address != "" {
		p.FromAddress, p.FromName = from[0].Address, from[0].Name
	} else {
		return nil, errors.New("parse message: missing From")
	}
	p.Subject, _ = h.Subject()
	p.Date, _ = h.Date()
	if id, err := h.MessageID(); err == nil && id != "" {
		p.MessageID = "<" + id + ">"
	} else {
		sum := sha256.Sum256(raw)
		p.MessageID = "<sha256-" + hex.EncodeToString(sum[:]) + "@inbound.local>"
	}
	p.InReplyTo = angled(h.MsgIDList("In-Reply-To"))
	p.References = angled(h.MsgIDList("References"))
	p.AutoSubmitted = strings.TrimSpace(h.Get("Auto-Submitted"))
	p.Precedence = strings.TrimSpace(h.Get("Precedence"))
	p.AutoResponseSuppress = strings.TrimSpace(h.Get("X-Auto-Response-Suppress"))
	p.ContentType = strings.TrimSpace(h.Get("Content-Type"))
	if ct, _, err := h.ContentType(); err == nil {
		p.ContentType = ct
		if raw := strings.TrimSpace(h.Get("Content-Type")); strings.HasPrefix(ct, "multipart/report") {
			p.ContentType = raw
		}
	}
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			if message.IsUnknownCharset(err) {
				continue
			}
			break // a broken part ends the body scan; what was read stays
		}
		switch ph := part.Header.(type) {
		case *gomail.InlineHeader:
			ct, _, _ := ph.ContentType()
			switch {
			case ct == "text/plain" && p.Text == "":
				b, _ := io.ReadAll(part.Body)
				p.Text = string(b)
			case ct == "text/html" && p.HTML == "":
				b, _ := io.ReadAll(part.Body)
				p.HTML = mail.SanitizeHTML(string(b))
			}
		case *gomail.AttachmentHeader:
			name, _ := ph.Filename()
			if name == "" {
				continue
			}
			ct, _, _ := ph.ContentType()
			if ct == "" {
				ct = "application/octet-stream"
			}
			a := Attachment{Filename: name, MIME: ct}
			data, err := io.ReadAll(io.LimitReader(part.Body, maxAttachment+1))
			if err == nil && int64(len(data)) > maxAttachment {
				a.Oversized = true
			} else if err == nil {
				a.Data = data
			}
			p.Attachments = append(p.Attachments, a)
		}
	}
	return p, nil
}

// angled ensures each id carries angle brackets, as our outbox stores them.
func angled(ids []string, _ error) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if !strings.HasPrefix(id, "<") {
			id = "<" + id + ">"
		}
		out = append(out, id)
	}
	return out
}
