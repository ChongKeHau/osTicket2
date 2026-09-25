package mail

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
)

// fakeSMTP speaks enough SMTP for the transport: EHLO, AUTH PLAIN, MAIL, RCPT, DATA, QUIT.
type fakeSMTP struct {
	addr      string
	mu        sync.Mutex
	messages  []string
	rcpts     []string
	rejectTo  string // RCPT TO address to reject with 550
	failData  bool   // reply 451 after DATA
	authUsers map[string]string
}

func startFakeSMTP(t *testing.T) *fakeSMTP {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &fakeSMTP{addr: ln.Addr().String(), authUsers: map[string]string{}}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go s.serve(c)
		}
	}()
	t.Cleanup(func() { ln.Close() })
	return s
}

func (s *fakeSMTP) serve(c net.Conn) {
	defer c.Close()
	r := bufio.NewReader(c)
	w := func(line string) { fmt.Fprintf(c, "%s\r\n", line) }
	w("220 fake ESMTP")
	var data strings.Builder
	inData := false
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		if inData {
			if line == "." {
				inData = false
				if s.failData {
					w("451 try again later")
				} else {
					s.mu.Lock()
					s.messages = append(s.messages, data.String())
					s.mu.Unlock()
					w("250 queued")
				}
				data.Reset()
				continue
			}
			data.WriteString(strings.TrimPrefix(line, ".") + "\r\n")
			continue
		}
		up := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(up, "EHLO"), strings.HasPrefix(up, "HELO"):
			w("250-fake")
			w("250-AUTH PLAIN")
			w("250 8BITMIME")
		case strings.HasPrefix(up, "AUTH PLAIN"):
			w("235 ok")
		case strings.HasPrefix(up, "MAIL FROM"):
			w("250 ok")
		case strings.HasPrefix(up, "RCPT TO"):
			addr := strings.Trim(strings.TrimPrefix(line[8:], ":"), "<> ")
			if s.rejectTo != "" && strings.EqualFold(addr, s.rejectTo) {
				w("550 no such user")
				continue
			}
			s.mu.Lock()
			s.rcpts = append(s.rcpts, addr)
			s.mu.Unlock()
			w("250 ok")
		case up == "DATA":
			inData = true
			w("354 go ahead")
		case up == "QUIT":
			w("221 bye")
			return
		case up == "RSET":
			w("250 ok")
		default:
			w("500 unknown")
		}
	}
}

func (s *fakeSMTP) received() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.messages...)
}
