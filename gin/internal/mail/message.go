package mail

import (
	"bytes"
	"fmt"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/textproto"
	"strings"
	"time"
)

// Address is a display name plus mailbox.
type Address struct{ Name, Address string }

// String renders the address for a header, encoding the name when needed.
func (a Address) String() string {
	if a.Name == "" {
		return a.Address
	}
	return (&mail.Address{Name: a.Name, Address: a.Address}).String()
}

// Outgoing is everything needed to build one message.
type Outgoing struct {
	From, To      Address
	Subject       string
	MessageID     string
	InReplyTo     string
	AutoSubmitted bool
	Text, HTML    string
	Date          time.Time
}

// Build renders an RFC 5322 multipart/alternative message.
func Build(o Outgoing) ([]byte, error) {
	if err := validateHeaders(o); err != nil {
		return nil, err
	}
	if o.Date.IsZero() {
		o.Date = time.Now()
	}
	var buf bytes.Buffer
	mp := multipart.NewWriter(&buf)
	h := func(k, v string) { fmt.Fprintf(&buf, "%s: %s\r\n", k, v) }
	h("From", o.From.String())
	h("To", o.To.String())
	h("Subject", mime.QEncoding.Encode("utf-8", o.Subject))
	h("Date", o.Date.UTC().Format(time.RFC1123Z))
	h("Message-ID", o.MessageID)
	if o.InReplyTo != "" {
		h("In-Reply-To", o.InReplyTo)
		h("References", o.InReplyTo)
	}
	if o.AutoSubmitted {
		h("Auto-Submitted", "auto-replied")
	}
	h("MIME-Version", "1.0")
	h("Content-Type", fmt.Sprintf("multipart/alternative; boundary=%q", mp.Boundary()))
	buf.WriteString("\r\n")
	for _, part := range []struct{ ctype, body string }{{"text/plain", o.Text}, {"text/html", o.HTML}} {
		hdr := textproto.MIMEHeader{}
		hdr.Set("Content-Type", part.ctype+"; charset=utf-8")
		hdr.Set("Content-Transfer-Encoding", "quoted-printable")
		w, err := mp.CreatePart(hdr)
		if err != nil {
			return nil, err
		}
		qp := quotedprintable.NewWriter(w)
		if _, err := qp.Write([]byte(part.body)); err != nil {
			return nil, err
		}
		if err := qp.Close(); err != nil {
			return nil, err
		}
	}
	if err := mp.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// validateHeaders rejects values that would let a caller inject extra header
// lines (CRLF) into the message, or leave an address empty.
func validateHeaders(o Outgoing) error {
	if o.From.Address == "" || o.To.Address == "" {
		return fmt.Errorf("invalid header value")
	}
	for _, v := range []string{o.From.Address, o.To.Address, o.From.Name, o.To.Name, o.Subject} {
		if strings.ContainsAny(v, "\r\n") {
			return fmt.Errorf("invalid header value")
		}
	}
	return nil
}
