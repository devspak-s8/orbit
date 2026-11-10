// Package config loads server configuration from environment variables.
// Every setting has a sensible default so `go run ./cmd/api` works against
// the default docker compose database with zero configuration.
package config

import (
	"os"
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
}

func Load() Config {
	return Config{
		HTTPAddr:    envOr("ORBIT_HTTP_ADDR", ":8080"),
		DatabaseURL: envOr("ORBIT_DATABASE_URL", "postgres://orbit:orbit@localhost:5434/orbit?sslmode=disable"),
		CORSOrigins: splitList(envOr("ORBIT_CORS_ORIGINS", "http://localhost:5173")),
		SessionTTL:  30 * 24 * time.Hour,
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
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
