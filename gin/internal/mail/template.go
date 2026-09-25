package mail

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	htmltpl "html/template"
	"strings"
	"sync"
	texttpl "text/template"
	"time"

	"github.com/grandpine/ticket-api/internal/db"
	"github.com/jackc/pgx/v5"
)

// Vars is the complete variable set available to templates.
type Vars struct {
	SiteName       string
	Number         string
	Subject        string
	RequesterName  string
	RequesterEmail string
	AgentName      string
	Link           string
	Message        string
	MessageHTML    htmltpl.HTML
}

// SampleVars gives every field a value, for validation renders.
func SampleVars() Vars {
	return Vars{
		SiteName: "Ticket Desk", Number: "000123", Subject: "Sample subject", RequesterName: "Pat Requester",
		RequesterEmail: "pat@example.test", AgentName: "Ada Agent", Link: "https://desk.example.test/tickets/1",
		Message: "Sample message", MessageHTML: "<p>Sample message</p>",
	}
}

// Rendered is a template executed against a Vars.
type Rendered struct{ Subject, HTML, Text string }

type parsed struct {
	subject *texttpl.Template
	html    *htmltpl.Template
	text    *texttpl.Template
}

// ParseTemplate parses the three parts; errors name the part.
func ParseTemplate(subject, bodyHTML, bodyText string) (*parsed, error) {
	s, err := texttpl.New("subject").Option("missingkey=error").Parse(subject)
	if err != nil {
		return nil, fmt.Errorf("subject: %w", err)
	}
	h, err := htmltpl.New("html").Option("missingkey=error").Parse(bodyHTML)
	if err != nil {
		return nil, fmt.Errorf("body_html: %w", err)
	}
	t, err := texttpl.New("text").Option("missingkey=error").Parse(bodyText)
	if err != nil {
		return nil, fmt.Errorf("body_text: %w", err)
	}
	return &parsed{subject: s, html: h, text: t}, nil
}

func (p *parsed) execute(v Vars) (Rendered, error) {
	var out Rendered
	var buf bytes.Buffer
	if err := p.subject.Execute(&buf, v); err != nil {
		return out, fmt.Errorf("subject: %w", err)
	}
	out.Subject = strings.TrimSpace(buf.String())
	buf.Reset()
	if err := p.html.Execute(&buf, v); err != nil {
		return out, fmt.Errorf("body_html: %w", err)
	}
	out.HTML = buf.String()
	buf.Reset()
	if err := p.text.Execute(&buf, v); err != nil {
		return out, fmt.Errorf("body_text: %w", err)
	}
	out.Text = buf.String()
	return out, nil
}

// ValidateTemplate parses and test-renders a template edit; unknown variables fail.
func ValidateTemplate(subject, bodyHTML, bodyText string) error {
	p, err := ParseTemplate(subject, bodyHTML, bodyText)
	if err != nil {
		return err
	}
	_, err = p.execute(SampleVars())
	return err
}

// Renderer loads templates from the database with a short cache.
type Renderer struct {
	ttl   time.Duration
	mu    sync.Mutex
	cache map[string]cached
}

type cached struct {
	p       *parsed
	expires time.Time
}

func NewRenderer(ttl time.Duration) *Renderer {
	return &Renderer{ttl: ttl, cache: map[string]cached{}}
}

// Invalidate drops the cached copy of key (after an edit).
func (r *Renderer) Invalidate(key string) {
	r.mu.Lock()
	delete(r.cache, key)
	r.mu.Unlock()
}

// Render executes the template stored under key.
func (r *Renderer) Render(ctx context.Context, q *db.Queries, key string, v Vars) (Rendered, error) {
	r.mu.Lock()
	c, ok := r.cache[key]
	r.mu.Unlock()
	if !ok || time.Now().After(c.expires) {
		row, err := q.GetEmailTemplate(ctx, key)
		if errors.Is(err, pgx.ErrNoRows) {
			return Rendered{}, fmt.Errorf("email template %q not found", key)
		}
		if err != nil {
			return Rendered{}, err
		}
		p, err := ParseTemplate(row.Subject, row.BodyHtml, row.BodyText)
		if err != nil {
			return Rendered{}, fmt.Errorf("email template %q: %w", key, err)
		}
		c = cached{p: p, expires: time.Now().Add(r.ttl)}
		r.mu.Lock()
		r.cache[key] = c
		r.mu.Unlock()
	}
	out, err := c.p.execute(v)
	if err != nil {
		return Rendered{}, fmt.Errorf("email template %q: %w", key, err)
	}
	return out, nil
}
