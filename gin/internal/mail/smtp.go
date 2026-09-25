package mail

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"time"
)

// Transport opens one session per batch.
type Transport interface {
	Open(ctx context.Context) (Session, error)
}

// Session sends messages over one connection.
type Session interface {
	Send(ctx context.Context, from, to string, raw []byte) error
	Close() error
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

// SMTP opens sessions over one connection per batch.
type SMTP struct{ o SMTPOptions }

func NewSMTP(o SMTPOptions) *SMTP {
	if o.Timeout == 0 {
		o.Timeout = 30 * time.Second
	}
	return &SMTP{o: o}
}

// Open dials, greets, negotiates TLS, and authenticates, returning a Session
// that can send multiple messages over the resulting connection.
func (s *SMTP) Open(ctx context.Context) (Session, error) {
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
		return nil, fmt.Errorf("smtp greeting %s: %w", addr, err)
	}
	if s.o.TLS == "starttls" {
		if err := c.StartTLS(tlsCfg); err != nil {
			c.Close()
			return nil, fmt.Errorf("smtp starttls %s: %w", addr, err)
		}
	}
	if s.o.User != "" {
		if err := c.Auth(smtp.PlainAuth("", s.o.User, s.o.Password, s.o.Host)); err != nil {
			c.Close()
			return nil, fmt.Errorf("smtp auth %s: %w", addr, err)
		}
	}
	return &smtpSession{c: c, conn: conn, timeout: s.o.Timeout}, nil
}

// smtpSession sends messages over one *smtp.Client connection. The deadline
// set in Open covers the handshake only; each Send and Close sets a fresh one
// so a long batch never trips a session-wide deadline.
type smtpSession struct {
	c       *smtp.Client
	conn    net.Conn
	timeout time.Duration
}

func (s *smtpSession) refreshDeadline() { _ = s.conn.SetDeadline(time.Now().Add(s.timeout)) }

// Send delivers raw to one recipient. On any error the connection is reset so
// the next message on the same session starts clean.
func (s *smtpSession) Send(_ context.Context, from, to string, raw []byte) error {
	s.refreshDeadline()
	if err := s.send(from, to, raw); err != nil {
		_ = s.c.Reset()
		return err
	}
	return nil
}

func (s *smtpSession) send(from, to string, raw []byte) error {
	if err := s.c.Mail(from); err != nil {
		return fmt.Errorf("smtp mail from: %w", err)
	}
	if err := s.c.Rcpt(to); err != nil {
		return fmt.Errorf("smtp rcpt to: %w", err)
	}
	w, err := s.c.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err := w.Write(raw); err != nil {
		return fmt.Errorf("smtp write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp data close: %w", err)
	}
	return nil
}

// Close ends the session, falling back to a hard close if Quit fails.
func (s *smtpSession) Close() error {
	s.refreshDeadline()
	if err := s.c.Quit(); err != nil {
		return s.c.Close()
	}
	return nil
}
