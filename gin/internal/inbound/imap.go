package inbound

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
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
type IMAP struct{ o IMAPOptions }

func NewIMAP(o IMAPOptions) *IMAP { return &IMAP{o: o} }

func (c *IMAP) connect() (*imapclient.Client, error) {
	addr := net.JoinHostPort(c.o.Host, fmt.Sprint(c.o.Port))
	opts := &imapclient.Options{TLSConfig: &tls.Config{ServerName: c.o.Host, InsecureSkipVerify: c.o.InsecureSkipVerify}} //nolint:gosec // opt-in for tests
	if c.o.TLS == "implicit" {
		return imapclient.DialTLS(addr, opts)
	}
	return imapclient.DialInsecure(addr, opts)
}

// Cycle fetches unseen messages one by one and marks the handled ones seen.
func (c *IMAP) Cycle(ctx context.Context, max int, handle func(raw []byte) (bool, error)) error {
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
			return fmt.Errorf("imap fetch uid %d: %w", uid, err)
		}
		mark, err := handle(raw)
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

// fetchRaw downloads the full message without setting \Seen (BODY.PEEK[]).
func fetchRaw(cl *imapclient.Client, uid imap.UID) ([]byte, error) {
	cmd := cl.Fetch(imap.UIDSetNum(uid), &imap.FetchOptions{UID: true, BodySection: []*imap.FetchItemBodySection{{Peek: true}}})
	defer cmd.Close()
	var raw []byte
	for msg := cmd.Next(); msg != nil; msg = cmd.Next() {
		for item := msg.Next(); item != nil; item = msg.Next() {
			if section, ok := item.(imapclient.FetchItemDataBodySection); ok {
				b, err := io.ReadAll(section.Literal)
				if err != nil {
					return nil, err
				}
				raw = b
			}
		}
	}
	if raw == nil {
		return nil, fmt.Errorf("no body returned")
	}
	return raw, nil
}
