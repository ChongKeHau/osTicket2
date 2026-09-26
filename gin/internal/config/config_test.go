package config

import (
	"strings"
	"testing"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load(env(map[string]string{
		"DATABASE_URL": "postgres://x",
		"JWT_SECRET":   strings.Repeat("s", 32),
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Port != 8080 || cfg.StorageDir != "./storage" || cfg.MaxUploadBytes != 10<<20 {
		t.Fatalf("defaults wrong: %+v", cfg)
	}
	if len(cfg.AllowedMIME) == 0 || len(cfg.CORSOrigins) != 0 {
		t.Fatalf("list defaults wrong: %+v", cfg)
	}
}

func TestLoadOverrides(t *testing.T) {
	cfg, err := Load(env(map[string]string{
		"DATABASE_URL":     "postgres://x",
		"JWT_SECRET":       strings.Repeat("s", 40),
		"PORT":             "9090",
		"STORAGE_DIR":      "/data",
		"CORS_ORIGINS":     "http://a.test, http://b.test,",
		"MAX_UPLOAD_BYTES": "1024",
		"ALLOWED_MIME":     "image/png,text/plain",
		"TRUSTED_PROXIES":  "10.0.0.1, 10.0.0.0/8,",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Port != 9090 || cfg.StorageDir != "/data" || cfg.MaxUploadBytes != 1024 {
		t.Fatalf("overrides wrong: %+v", cfg)
	}
	if len(cfg.CORSOrigins) != 2 || cfg.CORSOrigins[1] != "http://b.test" {
		t.Fatalf("cors wrong: %v", cfg.CORSOrigins)
	}
	if len(cfg.AllowedMIME) != 2 {
		t.Fatalf("mime wrong: %v", cfg.AllowedMIME)
	}
	if len(cfg.TrustedProxies) != 2 || cfg.TrustedProxies[0] != "10.0.0.1" || cfg.TrustedProxies[1] != "10.0.0.0/8" {
		t.Fatalf("trusted proxies wrong: %v", cfg.TrustedProxies)
	}
}

// TestLoadDefaultsNoTrustedProxies proves TrustedProxies defaults to empty:
// with none configured, server.New must pass an empty slice to
// SetTrustedProxies, which trusts no proxy.
func TestLoadDefaultsNoTrustedProxies(t *testing.T) {
	cfg, err := Load(env(map[string]string{
		"DATABASE_URL": "postgres://x",
		"JWT_SECRET":   strings.Repeat("s", 32),
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.TrustedProxies) != 0 {
		t.Fatalf("trusted proxies must default to empty: %v", cfg.TrustedProxies)
	}
}

func TestLoadValidation(t *testing.T) {
	cases := map[string]map[string]string{
		"missing db":      {"JWT_SECRET": strings.Repeat("s", 32)},
		"short secret":    {"DATABASE_URL": "x", "JWT_SECRET": "short"},
		"bad port":        {"DATABASE_URL": "x", "JWT_SECRET": strings.Repeat("s", 32), "PORT": "abc"},
		"bad upload size": {"DATABASE_URL": "x", "JWT_SECRET": strings.Repeat("s", 32), "MAX_UPLOAD_BYTES": "-1"},
	}
	for name, m := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Load(env(m)); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestLoadAppBaseURL(t *testing.T) {
	base := map[string]string{"DATABASE_URL": "postgres://x", "JWT_SECRET": strings.Repeat("s", 32)}
	with := func(k, v string) map[string]string {
		m := map[string]string{}
		for a, b := range base {
			m[a] = b
		}
		m[k] = v
		return m
	}
	cfg, err := Load(env(base))
	if err != nil || cfg.AppBaseURL != "" {
		t.Fatalf("unset: %q %v", cfg.AppBaseURL, err)
	}
	// Read even when mail is disabled, trailing slash trimmed.
	cfg, err = Load(env(with("APP_BASE_URL", "http://localhost:5173/")))
	if err != nil || cfg.AppBaseURL != "http://localhost:5173" || cfg.Mail.Enabled || cfg.Mail.BaseURL != "" {
		t.Fatalf("mail off: %+v %v", cfg, err)
	}
	if _, err := Load(env(with("APP_BASE_URL", "not a url"))); err == nil || !strings.Contains(err.Error(), "APP_BASE_URL") {
		t.Fatalf("bad url: %v", err)
	}
	// With mail on, both fields carry the same value and a bad URL is reported once.
	m := with("APP_BASE_URL", "https://desk.example.com/")
	m["MAIL_ENABLED"], m["MAIL_FROM"], m["ZOHO_APP_TOKEN"] = "true", "desk@example.com", "t"
	cfg, err = Load(env(m))
	if err != nil || cfg.AppBaseURL != "https://desk.example.com" || cfg.Mail.BaseURL != cfg.AppBaseURL {
		t.Fatalf("mail on: %+v %v", cfg, err)
	}
	m["APP_BASE_URL"] = "nope"
	if _, err := Load(env(m)); err == nil || strings.Count(err.Error(), "APP_BASE_URL") != 1 {
		t.Fatalf("mail on, bad url: %v", err)
	}
}
