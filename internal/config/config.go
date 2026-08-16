// Package config loads aiinventory's server configuration from environment
// variables and command-line flags.
package config

import (
	"flag"
	"os"
	"strconv"
)

type Config struct {
	Port          string
	DBPath        string
	TLSEnabled    bool
	SessionSecret string
	// ScanStoreDir, if set, saves every resized scan image (item capture and
	// location reconcile) into this directory for debugging OCR misreads.
	// Empty disables saving entirely.
	ScanStoreDir string
	// TrustProxyHeaders enables client IP extraction from forwarding headers.
	// It must only be used behind a trusted proxy that sanitizes those headers.
	TrustProxyHeaders bool
}

func Load() Config {
	storeDir := flag.String("store", "", "directory to save resized scan images into for debugging (disabled if empty)")
	flag.Parse()

	return Config{
		Port:              getEnv("PORT", "8080"),
		DBPath:            getEnv("DB_PATH", "./aiinventory.db"),
		TLSEnabled:        getEnvBool("TLS_ENABLED", false),
		SessionSecret:     os.Getenv("SESSION_SECRET"),
		ScanStoreDir:      *storeDir,
		TrustProxyHeaders: getEnvBool("TRUST_PROXY_HEADERS", false),
	}
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}
