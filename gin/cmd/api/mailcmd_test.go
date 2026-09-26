package main

import (
	"strings"
	"testing"

	"github.com/grandpine/ticket-api/internal/config"
)

func TestTestMessageContents(t *testing.T) {
	cfg := config.MailConfig{FromName: "Desk", FromAddress: "desk@example.test", Domain: "example.test", SiteName: "Desk"}
	raw, err := testMessage(cfg, "you@example.test")
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	for _, want := range []string{"To: you@example.test", "Subject: ", "Message-ID: <client-", "@example.test>", "Desk"} {
		if !strings.Contains(s, want) {
			t.Fatalf("test message missing %q:\n%s", want, s)
		}
	}
}

func TestMailTestFlags(t *testing.T) {
	if _, err := parseMailTestFlags([]string{}); err == nil {
		t.Fatal("--to is required")
	}
	to, err := parseMailTestFlags([]string{"--to", "a@b.test"})
	if err != nil || to != "a@b.test" {
		t.Fatalf("to = %q %v", to, err)
	}
}
