package mail

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/grandpine/ticket-api/internal/db"
	"github.com/grandpine/ticket-api/internal/db/testutil"
)

func TestParseAndValidateTemplate(t *testing.T) {
	if err := ValidateTemplate("[#{{.Number}}]", "<p>{{.MessageHTML}}</p>", "{{.Message}}"); err != nil {
		t.Fatal(err)
	}
	err := ValidateTemplate("{{.Number", "<p></p>", "x")
	if err == nil || !strings.HasPrefix(err.Error(), "subject:") {
		t.Fatalf("subject parse error = %v", err)
	}
	err = ValidateTemplate("ok", "<p>{{.Nope}}</p>", "x")
	if err == nil || !strings.HasPrefix(err.Error(), "body_html:") {
		t.Fatalf("unknown variable error = %v", err)
	}
	err = ValidateTemplate("ok", "<p></p>", "{{.Missing}}")
	if err == nil || !strings.HasPrefix(err.Error(), "body_text:") {
		t.Fatalf("text unknown variable error = %v", err)
	}
}

func TestRenderEscapesAndCaches(t *testing.T) {
	tx := testutil.Tx(t)
	ctx := context.Background()
	q := db.New(tx)
	r := NewRenderer(time.Minute)
	v := SampleVars()
	v.Subject = "Bold <b>subject</b>"
	v.Message, v.MessageHTML = BodyVars("<p>Hi <script>x</script></p>", "html")
	out, err := r.Render(ctx, q, "ticket_autoresp", v)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out.Subject, "[#"+v.Number+"] Bold <b>subject</b>") {
		t.Fatalf("subject = %q", out.Subject)
	}
	if !strings.Contains(out.HTML, "Bold &lt;b&gt;subject&lt;/b&gt;") || strings.Contains(out.HTML, "<script") || !strings.Contains(out.HTML, "<p>Hi </p>") {
		t.Fatalf("html = %s", out.HTML)
	}
	if !strings.Contains(out.Text, "Hi") || strings.Contains(out.Text, "<p>") {
		t.Fatalf("text = %s", out.Text)
	}
	// A database edit is not visible until the cache entry expires or is invalidated.
	if _, err := q.UpdateEmailTemplate(ctx, db.UpdateEmailTemplateParams{Key: "ticket_autoresp", Subject: strPtr("changed {{.Number}}")}); err != nil {
		t.Fatal(err)
	}
	out, _ = r.Render(ctx, q, "ticket_autoresp", v)
	if strings.HasPrefix(out.Subject, "changed") {
		t.Fatal("cache should still serve the old template")
	}
	r.Invalidate("ticket_autoresp")
	out, _ = r.Render(ctx, q, "ticket_autoresp", v)
	if !strings.HasPrefix(out.Subject, "changed") {
		t.Fatalf("after invalidate subject = %q", out.Subject)
	}
	if _, err := r.Render(ctx, q, "no_such_key", v); err == nil {
		t.Fatal("unknown key must error")
	}
}

func strPtr(s string) *string { return &s }
