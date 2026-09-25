package inbound

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
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

// TestParseCleansStringsForPostgres covers bytes Postgres rejects in text
// columns: NUL and invalid UTF-8 (a part with no charset parameter is not
// converted by go-message, so latin-1 bytes arrive raw).
func TestParseCleansStringsForPostgres(t *testing.T) {
	raw := "From: =?utf-8?q?P=E9t=00_R?= <pat@example.test>\r\n" +
		"To: desk@example.test\r\n" +
		"Subject: Caf\xe9\x00 broken\r\n" +
		"Message-ID: <bad\xe9\x00-1@example.test>\r\n" +
		"In-Reply-To: <irt\xe9\x00@example.test>\r\n" +
		"References: <ref\xe9\x00@example.test>\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: multipart/mixed; boundary=\"b1\"\r\n\r\n" +
		"--b1\r\nContent-Type: multipart/alternative; boundary=\"b2\"\r\n\r\n" +
		"--b2\r\nContent-Type: text/plain\r\n\r\nna\xefve\x00 body\r\n" +
		"--b2\r\nContent-Type: text/html\r\n\r\n<p>h\xe9\x00llo</p>\r\n" +
		"--b2--\r\n" +
		"--b1\r\nContent-Type: application/pdf\r\nContent-Disposition: attachment; filename=\"r\xe9\x00sum.pdf\"\r\n\r\n%PDF\r\n" +
		"--b1--\r\n"
	p, err := Parse([]byte(raw), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	strs := map[string]string{
		"subject": p.Subject, "from name": p.FromName, "from address": p.FromAddress,
		"message-id": p.MessageID, "text": p.Text, "html": p.HTML,
	}
	for i, s := range p.InReplyTo {
		strs[fmt.Sprintf("in-reply-to %d", i)] = s
	}
	for i, s := range p.References {
		strs[fmt.Sprintf("references %d", i)] = s
	}
	for i, a := range p.Attachments {
		strs[fmt.Sprintf("attachment %d name", i)] = a.Filename
		strs[fmt.Sprintf("attachment %d mime", i)] = a.MIME
	}
	for k, s := range strs {
		if strings.ContainsRune(s, 0) || !utf8.ValidString(s) {
			t.Errorf("%s not Postgres-safe: %q", k, s)
		}
	}
	if !strings.Contains(p.Text, "na�ve body") || !strings.Contains(p.Subject, "Caf� broken") || !strings.Contains(p.HTML, "h�llo") {
		t.Fatalf("text/subject/html = %q / %q / %q", p.Text, p.Subject, p.HTML)
	}
	// go-message's msg-id parser rejects the 8-bit ids above (the id falls
	// back to the sha256 form and the reference lists come out empty); the
	// loop above still guards them should that parser ever let bytes through.
	if len(p.Attachments) != 1 {
		t.Fatalf("parsed = %+v", p)
	}
}
