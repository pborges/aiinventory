// Command aiinventory runs the aiinventory server.
package main

import (
	"context"
	"crypto/tls"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/pborges/aiinventory/internal/api"
	"github.com/pborges/aiinventory/internal/auth"
	"github.com/pborges/aiinventory/internal/config"
	"github.com/pborges/aiinventory/internal/gemini"
	"github.com/pborges/aiinventory/internal/store"
	"github.com/pborges/aiinventory/internal/tlscert"
	"github.com/pborges/aiinventory/internal/version"
)

func main() {
	log.Printf("aiinventory %s", version.Version)

	cfg := config.Load()
	ctx := context.Background()

	db, err := store.Open(ctx, cfg.DBPath)
	if err != nil {
		log.Fatalf("open database %s: %v", cfg.DBPath, err)
	}
	defer db.Close()
	log.Printf("database ready at %s", cfg.DBPath)

	sessionSecret := cfg.SessionSecret
	if sessionSecret == "" {
		sessionSecret, err = db.GetOrCreateSessionSecret(ctx)
		if err != nil {
			log.Fatalf("resolve session secret: %v", err)
		}
		log.Printf("SESSION_SECRET not set; using auto-generated secret persisted in settings")
	}
	codec := auth.NewCodec(sessionSecret)

	geminiAPIKey, _, err := db.GetSetting(ctx, store.SettingGeminiAPIKey)
	if err != nil {
		log.Fatalf("resolve gemini api key: %v", err)
	}

	var geminiClient gemini.Client
	if geminiAPIKey == "" {
		log.Printf("no Gemini API key configured; AI-dependent features are disabled until one is set in Settings")
	} else if gc, err := gemini.NewGenAIClient(ctx, geminiAPIKey); err != nil {
		log.Printf("gemini client unavailable, AI-dependent features are disabled: %v", err)
	} else {
		geminiClient = gc
	}

	if cfg.ScanStoreDir != "" {
		if err := os.MkdirAll(cfg.ScanStoreDir, 0o755); err != nil {
			log.Fatalf("create scan store directory %s: %v", cfg.ScanStoreDir, err)
		}
		log.Printf("saving scan images into %s", cfg.ScanStoreDir)
	}

	handler := api.NewWithOptions(db, codec, geminiClient, cfg.ScanStoreDir, api.Options{
		TrustProxyHeaders: cfg.TrustProxyHeaders,
	})
	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       2 * time.Minute,
		WriteTimeout:      6 * time.Minute, // duplicate scans have a 5-minute deadline
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    1 << 20,
	}

	if !cfg.TLSEnabled {
		log.Printf("aiinventory listening on :%s", cfg.Port)
		if err := server.ListenAndServe(); err != nil {
			log.Fatal(err)
		}
		return
	}

	cert, err := tlscert.LoadOrGenerate(ctx, db)
	if err != nil {
		log.Fatalf("prepare TLS certificate: %v", err)
	}
	server.TLSConfig = &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
	log.Printf("aiinventory listening on :%s (HTTPS, self-signed — browsers will warn until you accept the certificate)", cfg.Port)
	if err := server.ListenAndServeTLS("", ""); err != nil {
		log.Fatal(err)
	}
}
