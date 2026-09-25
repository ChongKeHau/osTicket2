package inbound

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
)

// IMAPOptions configure the mailbox connection.
type IMAPOptions struct {
	Host               string
	Port               int
	TLS                string // implicit or none
	User, Password     string
	Folder             string
	InsecureSkipVerify bool
}

// IMAP is a Source over one mailbox folder; each Cycle opens and closes a connection.
type IMAP struct {
	o   IMAPOptions
	log *slog.Logger
}

func NewIMAP(o IMAPOptions) *IMAP { return &IMAP{o: o, log: slog.Default()} }

// fetchDataErr marks a per-UID error reading one message's literal (as
// opposed to a failure of the FETCH command itself). Cycle logs it and skips
// just that UID for this cycle rather than ending the cycle.
type fetchDataErr struct{ err error }

func (e *fetchDataErr) Error() string { return e.err.Error() }
func (e *fetchDataErr) Unwrap() error { return e.err }

func (c *IMAP) connect() (*imapclient.Client, error) {
	addr := net.JoinHostPort(c.o.Host, fmt.Sprint(c.o.Port))
	opts := &imapclient.Options{TLSConfig: &tls.Config{ServerName: c.o.Host, InsecureSkipVerify: c.o.InsecureSkipVerify}} //nolint:gosec // opt-in for tests
	if c.o.TLS == "implicit" {
		return imapclient.DialTLS(addr, opts)
	}
	return imapclient.DialInsecure(addr, opts)
}

// Cycle fetches unseen messages one by one and marks the handled ones seen.
func (c *IMAP) Cycle(ctx context.Context, max int, handle func(uid uint32, raw []byte) (bool, error)) error {
	cl, err := c.connect()
	if err != nil {
		return fmt.Errorf("imap dial %s: %w", c.o.Host, err)
	}
	defer cl.Close()
	if err := cl.Login(c.o.User, c.o.Password).Wait(); err != nil {
		return fmt.Errorf("imap login as %s: %w", c.o.User, err)
	}
	defer func() { _ = cl.Logout().Wait() }()
	if _, err := cl.Select(c.o.Folder, nil).Wait(); err != nil {
		return fmt.Errorf("imap select %s: %w", c.o.Folder, err)
	}
	search, err := cl.UIDSearch(&imap.SearchCriteria{NotFlag: []imap.Flag{imap.FlagSeen}}, nil).Wait()
	if err != nil {
		return fmt.Errorf("imap search: %w", err)
	}
	uids := search.AllUIDs()
	if len(uids) > max {
		uids = uids[:max]
	}
	for _, uid := range uids {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		raw, err := fetchRaw(cl, uid)
		if err != nil {
			var derr *fetchDataErr
			if errors.As(err, &derr) {
				c.log.Warn("imap fetch data error", "uid", uint32(uid), "err", derr.err)
				continue
			}
			return fmt.Errorf("imap fetch uid %d: %w", uid, err)
		}
		mark, err := handle(uint32(uid), raw)
		if err != nil {
			return err
		}
		if mark {
			store := cl.Store(imap.UIDSetNum(uid), &imap.StoreFlags{Op: imap.StoreFlagsAdd, Flags: []imap.Flag{imap.FlagSeen}, Silent: true}, nil)
			if err := store.Close(); err != nil {
				return fmt.Errorf("imap mark seen uid %d: %w", uid, err)
			}
		}
	}
	return nil
}

// fetchRaw downloads the full message without setting \Seen (BODY.PEEK[]). A
// missing or NIL body section (the message was expunged between SEARCH and
// FETCH, or the server answered BODY[] NIL) is not an error: it yields empty
// raw bytes, which the processor records as unparseable and the poller marks
// seen like any other handled message. A read error on the literal is
// returned as a *fetchDataErr so the caller can skip just that UID; only a
// failure of the FETCH command itself (a transport-level problem) is
// returned as a plain error.
func fetchRaw(cl *imapclient.Client, uid imap.UID) ([]byte, error) {
	cmd := cl.Fetch(imap.UIDSetNum(uid), &imap.FetchOptions{UID: true, BodySection: []*imap.FetchItemBodySection{{Peek: true}}})
	var raw []byte
	var dataErr error
	for msg := cmd.Next(); msg != nil; msg = cmd.Next() {
		for item := msg.Next(); item != nil; item = msg.Next() {
			section, ok := item.(imapclient.FetchItemDataBodySection)
			if !ok || section.Literal == nil {
				continue
			}
			b, err := io.ReadAll(section.Literal)
			if err != nil {
				if dataErr == nil {
					dataErr = err
				}
				continue
			}
			raw = b
		}
	}
	if err := cmd.Close(); err != nil {
		return nil, err
	}
	if dataErr != nil {
		return nil, &fetchDataErr{err: dataErr}
	}
	return raw, nil
}
