// Package config loads server configuration from environment variables.
// Every setting has a sensible default so `go run ./cmd/api` works against
// the default docker compose database with zero configuration.
package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	// HTTPAddr is the listen address for the API server.
	HTTPAddr string
	// DatabaseURL is the PostgreSQL connection string.
	DatabaseURL string
	// CORSOrigins lists allowed browser origins for cross-origin API calls.
	CORSOrigins []string
	// SessionTTL is the lifetime of an authenticated session.
	SessionTTL time.Duration
	// SMTP configures outbound email for the automation engine. Server-level
	// by design for the alpha (ADR-0005): the self-hoster configures the
	// relay once; an empty host disables the email node with a clear error.
	SMTP SMTPConfig
	// CookieSecure sets the Secure flag on session cookies. Enable when
	// serving the API over HTTPS (e.g. the public demo behind TLS).
	CookieSecure bool
}

// SMTPConfig is the outbound email relay configuration.
type SMTPConfig struct {
	Host string
	Port int
	User string
	Pass string
	From string
}

func Load() Config {
	return Config{
		HTTPAddr:    envOr("ORBIT_HTTP_ADDR", ":8080"),
		DatabaseURL: envOr("ORBIT_DATABASE_URL", "postgres://orbit:orbit@localhost:5434/orbit?sslmode=disable"),
		CORSOrigins: splitList(envOr("ORBIT_CORS_ORIGINS", "http://localhost:5173")),
		SessionTTL:  30 * 24 * time.Hour,
		SMTP: SMTPConfig{
			Host: envOr("ORBIT_SMTP_HOST", ""),
			Port: envInt("ORBIT_SMTP_PORT", 587),
			User: envOr("ORBIT_SMTP_USER", ""),
			Pass: envOr("ORBIT_SMTP_PASS", ""),
			From: envOr("ORBIT_SMTP_FROM", "orbit@localhost"),
		},
		CookieSecure: os.Getenv("ORBIT_COOKIE_SECURE") == "true",
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func splitList(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
