// Package config loads service configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	DatabaseURL    string
	JWTSecret      string
	Port           int
	StorageDir     string
	CORSOrigins    []string
	MaxUploadBytes int64
	AllowedMIME    []string
	// TrustedProxies are the IPs/CIDRs allowed to set X-Forwarded-For /
	// X-Real-IP and have gin.Context.ClientIP() honor them. Empty (the
	// default) trusts no proxy, so ClientIP() is always the socket address
	// - see server.New, which passes this to (*gin.Engine).SetTrustedProxies.
	TrustedProxies []string
	Mail           MailConfig
}

// MailConfig holds outbound SMTP and inbound IMAP settings. Zero when mail is disabled.
type MailConfig struct {
	Enabled       bool
	FromName      string
	FromAddress   string
	Domain        string // domain part of FromAddress, used in Message-IDs
	SMTPHost      string
	SMTPPort      int
	SMTPTLS       string // implicit, starttls, none
	SMTPUser      string
	SMTPPassword  string
	IMAPHost      string
	IMAPPort      int
	IMAPTLS       string // implicit, none
	IMAPUser      string
	IMAPPassword  string
	IMAPFolder    string
	PollInterval  time.Duration
	SendInterval  time.Duration
	BaseURL       string
	SiteName      string
	DefaultDeptID int64 // 0 means the seed department
}

var defaultMIME = []string{
	"image/png", "image/jpeg", "image/gif", "application/pdf", "text/plain", "text/csv",
	"application/zip", "application/msword",
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
}

// Load reads configuration through getenv (usually os.Getenv) and validates it.
func Load(getenv func(string) string) (Config, error) {
	cfg := Config{
		DatabaseURL:    getenv("DATABASE_URL"),
		JWTSecret:      getenv("JWT_SECRET"),
		Port:           8080,
		StorageDir:     "./storage",
		MaxUploadBytes: 10 << 20,
		AllowedMIME:    defaultMIME,
	}
	var errs []error
	if cfg.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL is required"))
	}
	if len(cfg.JWTSecret) < 32 {
		errs = append(errs, errors.New("JWT_SECRET must be at least 32 bytes"))
	}
	if v := getenv("PORT"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 || n > 65535 {
			errs = append(errs, fmt.Errorf("PORT %q is not a valid port", v))
		} else {
			cfg.Port = n
		}
	}
	if v := getenv("STORAGE_DIR"); v != "" {
		cfg.StorageDir = v
	}
	if v := getenv("CORS_ORIGINS"); v != "" {
		cfg.CORSOrigins = splitList(v)
	}
	if v := getenv("MAX_UPLOAD_BYTES"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n <= 0 {
			errs = append(errs, fmt.Errorf("MAX_UPLOAD_BYTES %q must be a positive integer", v))
		} else {
			cfg.MaxUploadBytes = n
		}
	}
	if v := getenv("ALLOWED_MIME"); v != "" {
		cfg.AllowedMIME = splitList(v)
	}
	if v := getenv("TRUSTED_PROXIES"); v != "" {
		cfg.TrustedProxies = splitList(v)
	}
	errs = append(errs, loadMail(&cfg.Mail, getenv)...)
	return cfg, errors.Join(errs...)
}

func loadMail(m *MailConfig, getenv func(string) string) []error {
	m.Enabled = strings.EqualFold(getenv("MAIL_ENABLED"), "true")
	if !m.Enabled {
		return nil
	}
	var errs []error
	fail := func(format string, a ...any) { errs = append(errs, fmt.Errorf(format, a...)) }
	str := func(key, def string) string {
		if v := getenv(key); v != "" {
			return v
		}
		return def
	}
	from := getenv("MAIL_FROM")
	if from == "" {
		fail("MAIL_FROM is required when MAIL_ENABLED=true")
	} else if addr, err := mail.ParseAddress(from); err != nil {
		fail("MAIL_FROM %q is not a valid address: %v", from, err)
	} else {
		m.FromName, m.FromAddress = addr.Name, addr.Address
		if at := strings.LastIndex(addr.Address, "@"); at > 0 {
			m.Domain = addr.Address[at+1:]
		}
	}
	m.SMTPHost = str("SMTP_HOST", "smtppro.zoho.com")
	m.SMTPPort = 465
	if v := getenv("SMTP_PORT"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 || n > 65535 {
			fail("SMTP_PORT %q is not a valid port", v)
		} else {
			m.SMTPPort = n
		}
	}
	m.SMTPTLS = getenv("SMTP_TLS")
	if m.SMTPTLS == "" {
		if m.SMTPPort == 465 {
			m.SMTPTLS = "implicit"
		} else {
			m.SMTPTLS = "starttls"
		}
	}
	if m.SMTPTLS != "implicit" && m.SMTPTLS != "starttls" && m.SMTPTLS != "none" {
		fail("SMTP_TLS %q must be implicit, starttls or none", m.SMTPTLS)
	}
	m.SMTPUser = str("SMTP_USER", m.FromAddress)
	m.SMTPPassword = str("SMTP_PASSWORD", getenv("ZOHO_APP_TOKEN"))
	if m.SMTPPassword == "" {
		fail("SMTP_PASSWORD or ZOHO_APP_TOKEN is required when MAIL_ENABLED=true")
	}
	m.IMAPHost = str("IMAP_HOST", "imappro.zoho.com")
	m.IMAPPort = 993
	if v := getenv("IMAP_PORT"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 || n > 65535 {
			fail("IMAP_PORT %q is not a valid port", v)
		} else {
			m.IMAPPort = n
		}
	}
	m.IMAPTLS = str("IMAP_TLS", "implicit")
	if m.IMAPTLS != "implicit" && m.IMAPTLS != "none" {
		fail("IMAP_TLS %q must be implicit or none", m.IMAPTLS)
	}
	m.IMAPUser = str("IMAP_USER", m.FromAddress)
	m.IMAPPassword = str("IMAP_PASSWORD", getenv("ZOHO_APP_TOKEN"))
	m.IMAPFolder = str("IMAP_FOLDER", "INBOX")
	m.PollInterval = 60 * time.Second
	if v := getenv("MAIL_POLL_INTERVAL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d < 10*time.Second {
			fail("MAIL_POLL_INTERVAL %q must be a duration of at least 10s", v)
		} else {
			m.PollInterval = d
		}
	}
	m.SendInterval = 5 * time.Second
	if v := getenv("MAIL_SEND_INTERVAL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d < time.Second {
			fail("MAIL_SEND_INTERVAL %q must be a duration of at least 1s", v)
		} else {
			m.SendInterval = d
		}
	}
	base := strings.TrimRight(getenv("APP_BASE_URL"), "/")
	if base == "" {
		fail("APP_BASE_URL is required when MAIL_ENABLED=true")
	} else if u, err := url.Parse(base); err != nil || u.Scheme == "" || u.Host == "" {
		fail("APP_BASE_URL %q must be an absolute URL", base)
	}
	m.BaseURL = base
	m.SiteName = str("MAIL_SITE_NAME", "Ticket Desk")
	if v := getenv("MAIL_DEFAULT_DEPT_ID"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n <= 0 {
			fail("MAIL_DEFAULT_DEPT_ID %q must be a positive integer", v)
		} else {
			m.DefaultDeptID = n
		}
	}
	return errs
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
