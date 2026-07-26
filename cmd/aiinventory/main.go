// Command aiinventory runs the aiinventory server.
package main

import (
	"context"
	"log"
	"net/http"

	"github.com/pborges/aiinventory/internal/api"
	"github.com/pborges/aiinventory/internal/auth"
	"github.com/pborges/aiinventory/internal/config"
	"github.com/pborges/aiinventory/internal/store"
)

func main() {
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

	handler := api.New(db, codec)

	log.Printf("aiinventory listening on :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, handler); err != nil {
		log.Fatal(err)
	}
}
