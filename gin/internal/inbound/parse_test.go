package inbound

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "eml", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestParsePlain(t *testing.T) {
	p, err := Parse(fixture(t, "plain.eml"), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if p.MessageID != "<plain-1@example.test>" || p.FromAddress != "pat@example.test" || p.FromName != "Pat Requester" || p.Subject != "Printer on fire" || !strings.Contains(p.Text, "smoke everywhere") || p.HTML != "" || len(p.Attachments) != 0 {
		t.Fatalf("parsed = %+v", p)
	}
	if p.Date.IsZero() || p.ContentType != "text/plain" {
		t.Fatalf("date/type = %v %q", p.Date, p.ContentType)
	}
}

func TestParseHTMLOnlySanitised(t *testing.T) {
	p, err := Parse(fixture(t, "html_only.eml"), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if p.Subject != "Café machine" || p.Text != "" || !strings.Contains(p.HTML, "<b>desk</b>") || strings.Contains(p.HTML, "<script") {
		t.Fatalf("parsed = %+v", p)
	}
}

func TestParseAlternativeAndRefs(t *testing.T) {
	p, err := Parse(fixture(t, "alternative.eml"), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(p.Text) != "Still burning." || !strings.Contains(p.HTML, "<i>burning</i>") {
		t.Fatalf("bodies = %q %q", p.Text, p.HTML)
	}
	if len(p.InReplyTo) != 1 || p.InReplyTo[0] != "<ticket-1-abcdef123456@example.test>" || len(p.References) != 1 {
		t.Fatalf("refs = %v %v", p.InReplyTo, p.References)
	}
}

func TestParseAttachments(t *testing.T) {
	p, err := Parse(fixture(t, "attachment.eml"), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Attachments) != 2 {
		t.Fatalf("attachments = %+v", p.Attachments)
	}
	a, b := p.Attachments[0], p.Attachments[1]
	if a.Filename != "pixel.png" || a.MIME != "image/png" || string(a.Data) != "PNG!" || a.Oversized {
		t.Fatalf("a = %+v", a)
	}
	if b.Filename != "big.pdf" || !b.Oversized || len(b.Data) != 0 {
		t.Fatalf("b = %+v", b)
	}
}

func TestParseHeadersAndMissingID(t *testing.T) {
	p, err := Parse(fixture(t, "autoreply.eml"), 1<<20)
	if err != nil || p.AutoSubmitted != "auto-replied" {
		t.Fatalf("auto = %+v %v", p, err)
	}
	p, err = Parse(fixture(t, "bounce.eml"), 1<<20)
	if err != nil || !strings.HasPrefix(p.ContentType, "multipart/report") || !strings.EqualFold(p.FromAddress, "MAILER-DAEMON@mx.example.test") {
		t.Fatalf("bounce = %+v %v", p, err)
	}
	p, err = Parse(fixture(t, "no_message_id.eml"), 1<<20)
	if err != nil || !strings.HasPrefix(p.MessageID, "<sha256-") || !strings.HasSuffix(p.MessageID, "@inbound.local>") {
		t.Fatalf("missing id = %+v %v", p, err)
	}
	p2, _ := Parse(fixture(t, "no_message_id.eml"), 1<<20)
	if p2.MessageID != p.MessageID {
		t.Fatal("synthesised id must be stable")
	}
	if _, err := Parse([]byte("To: x@y.test\r\nSubject: no from\r\n\r\nbody"), 1<<20); err == nil {
		t.Fatal("missing From must error")
	}
	if _, err := Parse([]byte("garbage"), 1<<20); err == nil {
		t.Fatal("garbage must error")
	}
}
