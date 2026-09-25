package mail

import (
	"bytes"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"strings"
	"testing"
	"time"
)

func TestBuildMessage(t *testing.T) {
	raw, err := Build(Outgoing{
		From: Address{Name: "Desk", Address: "desk@example.test"}, To: Address{Name: "Pät", Address: "pat@example.test"},
		Subject: "[#000001] Prïnter", MessageID: "<ticket-1-abc@example.test>", InReplyTo: "<ticket-1-000@example.test>",
		AutoSubmitted: true, Text: "Hello Pät\nline 2", HTML: "<p>Hello Pät</p>", Date: time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("not a valid message: %v\n%s", err, raw)
	}
	h := msg.Header
	dec := new(mime.WordDecoder)
	subj, _ := dec.DecodeHeader(h.Get("Subject"))
	to, _ := dec.DecodeHeader(h.Get("To"))
	if subj != "[#000001] Prïnter" || !strings.Contains(to, "Pät") || !strings.Contains(to, "pat@example.test") {
		t.Fatalf("subject %q to %q", subj, to)
	}
	if h.Get("Message-Id") != "<ticket-1-abc@example.test>" && h.Get("Message-ID") != "<ticket-1-abc@example.test>" {
		t.Fatalf("message id = %q", h.Get("Message-Id"))
	}
	if h.Get("In-Reply-To") != "<ticket-1-000@example.test>" || h.Get("References") != "<ticket-1-000@example.test>" || h.Get("Auto-Submitted") != "auto-replied" || h.Get("MIME-Version") != "1.0" || h.Get("Date") == "" {
		t.Fatalf("headers = %v", h)
	}
	mt, params, err := mime.ParseMediaType(h.Get("Content-Type"))
	if err != nil || mt != "multipart/alternative" {
		t.Fatalf("content type = %s %v", mt, err)
	}
	mr := multipart.NewReader(msg.Body, params["boundary"])
	var types []string
	var bodies []string
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(p)
		types = append(types, p.Header.Get("Content-Type"))
		bodies = append(bodies, string(b))
	}
	if len(types) != 2 || !strings.HasPrefix(types[0], "text/plain") || !strings.HasPrefix(types[1], "text/html") {
		t.Fatalf("parts = %v", types)
	}
	if !strings.Contains(bodies[0], "Hello Pät") || !strings.Contains(bodies[1], "<p>Hello Pät</p>") {
		t.Fatalf("bodies = %q", bodies)
	}
}

func TestBuildWithoutThreadingOrAuto(t *testing.T) {
	raw, err := Build(Outgoing{From: Address{Address: "a@b.test"}, To: Address{Address: "c@d.test"}, Subject: "s", MessageID: "<x@b.test>", Text: "t", HTML: "<p>t</p>"})
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	if strings.Contains(s, "In-Reply-To") || strings.Contains(s, "Auto-Submitted") || !strings.Contains(s, "From: a@b.test") {
		t.Fatalf("raw = %s", s)
	}
}

func TestBuildRejectsHeaderInjection(t *testing.T) {
	base := Outgoing{From: Address{Address: "a@b.test"}, To: Address{Address: "c@d.test"}, Subject: "s", MessageID: "<x@b.test>", Text: "t", HTML: "<p>t</p>"}

	withInjectedTo := base
	withInjectedTo.To.Address = "x@y.test\r\nBcc: z@y.test"
	if _, err := Build(withInjectedTo); err == nil {
		t.Fatal("expected error for CRLF in To address")
	}

	withInjectedSubject := base
	withInjectedSubject.Subject = "a\nb"
	if _, err := Build(withInjectedSubject); err == nil {
		t.Fatal("expected error for newline in Subject")
	}

	withEmptyTo := base
	withEmptyTo.To.Address = ""
	if _, err := Build(withEmptyTo); err == nil {
		t.Fatal("expected error for empty To address")
	}
}
