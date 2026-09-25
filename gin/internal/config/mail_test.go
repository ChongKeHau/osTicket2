package config

import (
	"strings"
	"testing"
	"time"
)

// env is defined in config_test.go.

func TestMailDisabledByDefault(t *testing.T) {
	cfg, err := Load(env(map[string]string{"DATABASE_URL": "x", "JWT_SECRET": strings.Repeat("s", 32)}))
	if err != nil || cfg.Mail.Enabled {
		t.Fatalf("cfg.Mail = %+v, err %v", cfg.Mail, err)
	}
}

func TestMailZohoDefaults(t *testing.T) {
	cfg, err := Load(env(map[string]string{
		"DATABASE_URL": "x", "JWT_SECRET": strings.Repeat("s", 32),
		"MAIL_ENABLED": "true", "MAIL_FROM": "Support <support@example.com>", "ZOHO_APP_TOKEN": "tok",
		"APP_BASE_URL": "https://desk.example.com/",
	}))
	if err != nil {
		t.Fatal(err)
	}
	m := cfg.Mail
	if !m.Enabled || m.FromName != "Support" || m.FromAddress != "support@example.com" || m.Domain != "example.com" {
		t.Fatalf("from = %+v", m)
	}
	if m.SMTPHost != "smtppro.zoho.com" || m.SMTPPort != 465 || m.SMTPTLS != "implicit" || m.SMTPUser != "support@example.com" || m.SMTPPassword != "tok" {
		t.Fatalf("smtp = %+v", m)
	}
	if m.IMAPHost != "imappro.zoho.com" || m.IMAPPort != 993 || m.IMAPTLS != "implicit" || m.IMAPUser != "support@example.com" || m.IMAPPassword != "tok" || m.IMAPFolder != "INBOX" {
		t.Fatalf("imap = %+v", m)
	}
	if m.PollInterval != 60*time.Second || m.SendInterval != 5*time.Second || m.BaseURL != "https://desk.example.com" || m.SiteName != "Ticket Desk" || m.DefaultDeptID != 0 {
		t.Fatalf("misc = %+v", m)
	}
}

func TestMailPort587DefaultsToStartTLS(t *testing.T) {
	cfg, err := Load(env(map[string]string{
		"DATABASE_URL": "x", "JWT_SECRET": strings.Repeat("s", 32),
		"MAIL_ENABLED": "true", "MAIL_FROM": "support@example.com", "SMTP_PASSWORD": "p",
		"APP_BASE_URL": "https://desk.example.com", "SMTP_PORT": "587", "MAIL_POLL_INTERVAL": "2m", "MAIL_DEFAULT_DEPT_ID": "7",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Mail.SMTPTLS != "starttls" || cfg.Mail.FromName != "" || cfg.Mail.PollInterval != 2*time.Minute || cfg.Mail.DefaultDeptID != 7 {
		t.Fatalf("mail = %+v", cfg.Mail)
	}
}

func TestMailValidationErrors(t *testing.T) {
	base := map[string]string{"DATABASE_URL": "x", "JWT_SECRET": strings.Repeat("s", 32), "MAIL_ENABLED": "true"}
	cases := []struct {
		name string
		add  map[string]string
		want string
	}{
		{"no from", map[string]string{"APP_BASE_URL": "https://d", "ZOHO_APP_TOKEN": "t"}, "MAIL_FROM"},
		{"bad from", map[string]string{"MAIL_FROM": "not an address", "APP_BASE_URL": "https://d", "ZOHO_APP_TOKEN": "t"}, "MAIL_FROM"},
		{"no base url", map[string]string{"MAIL_FROM": "a@b.c", "ZOHO_APP_TOKEN": "t"}, "APP_BASE_URL"},
		{"no password", map[string]string{"MAIL_FROM": "a@b.c", "APP_BASE_URL": "https://d"}, "SMTP_PASSWORD"},
		{"bad tls", map[string]string{"MAIL_FROM": "a@b.c", "APP_BASE_URL": "https://d", "ZOHO_APP_TOKEN": "t", "SMTP_TLS": "maybe"}, "SMTP_TLS"},
		{"short poll", map[string]string{"MAIL_FROM": "a@b.c", "APP_BASE_URL": "https://d", "ZOHO_APP_TOKEN": "t", "MAIL_POLL_INTERVAL": "2s"}, "MAIL_POLL_INTERVAL"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := map[string]string{}
			for k, v := range base {
				m[k] = v
			}
			for k, v := range c.add {
				m[k] = v
			}
			_, err := Load(env(m))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("got %v, want containing %q", err, c.want)
			}
		})
	}
}
