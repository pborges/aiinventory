// Package config loads aiinventory's server configuration from environment variables.
package config

import (
	"os"
	"strconv"
)

type Config struct {
	Port          string
	DBPath        string
	GeminiAPIKey  string
	TLSEnabled    bool
	SessionSecret string
}

func Load() Config {
	return Config{
		Port:          getEnv("PORT", "8080"),
		DBPath:        getEnv("DB_PATH", "./aiinventory.db"),
		GeminiAPIKey:  os.Getenv("GEMINI_API_KEY"),
		TLSEnabled:    getEnvBool("TLS_ENABLED", false),
		SessionSecret: os.Getenv("SESSION_SECRET"),
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
