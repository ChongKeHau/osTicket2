package mail

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"time"
)

// Transport delivers one raw message.
type Transport interface {
	Send(ctx context.Context, from, to string, raw []byte) error
}

// SMTPOptions configure the SMTP transport.
type SMTPOptions struct {
	Host               string
	Port               int
	TLS                string // implicit, starttls, none
	User, Password     string
	Timeout            time.Duration
	InsecureSkipVerify bool
}

// SMTP sends over one connection per message.
type SMTP struct{ o SMTPOptions }

func NewSMTP(o SMTPOptions) *SMTP {
	if o.Timeout == 0 {
		o.Timeout = 30 * time.Second
	}
	return &SMTP{o: o}
}

func (s *SMTP) dial(ctx context.Context) (*smtp.Client, error) {
	addr := net.JoinHostPort(s.o.Host, fmt.Sprint(s.o.Port))
	d := &net.Dialer{Timeout: s.o.Timeout}
	tlsCfg := &tls.Config{ServerName: s.o.Host, InsecureSkipVerify: s.o.InsecureSkipVerify} //nolint:gosec // opt-in for tests
	var conn net.Conn
	var err error
	if s.o.TLS == "implicit" {
		conn, err = tls.DialWithDialer(d, "tcp", addr, tlsCfg)
	} else {
		conn, err = d.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return nil, fmt.Errorf("smtp dial %s: %w", addr, err)
	}
	_ = conn.SetDeadline(time.Now().Add(s.o.Timeout))
	c, err := smtp.NewClient(conn, s.o.Host)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("smtp greeting: %w", err)
	}
	if s.o.TLS == "starttls" {
		if err := c.StartTLS(tlsCfg); err != nil {
			c.Close()
			return nil, fmt.Errorf("smtp starttls: %w", err)
		}
	}
	if s.o.User != "" {
		if err := c.Auth(smtp.PlainAuth("", s.o.User, s.o.Password, s.o.Host)); err != nil {
			c.Close()
			return nil, fmt.Errorf("smtp auth: %w", err)
		}
	}
	return c, nil
}

// Send delivers raw to one recipient.
func (s *SMTP) Send(ctx context.Context, from, to string, raw []byte) error {
	c, err := s.dial(ctx)
	if err != nil {
		return err
	}
	defer c.Close()
	if err := c.Mail(from); err != nil {
		return fmt.Errorf("smtp mail from: %w", err)
	}
	if err := c.Rcpt(to); err != nil {
		return fmt.Errorf("smtp rcpt to: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err := w.Write(raw); err != nil {
		return fmt.Errorf("smtp write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp data close: %w", err)
	}
	return c.Quit()
}
