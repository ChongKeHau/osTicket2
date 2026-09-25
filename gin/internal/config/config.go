// Package config loads service configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
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
	return cfg, errors.Join(errs...)
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
