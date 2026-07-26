// Command aiinventory runs the aiinventory server.
package main

import (
	"log"
	"net/http"

	"github.com/pborges/aiinventory/internal/config"
	"github.com/pborges/aiinventory/internal/web"
)

func main() {
	cfg := config.Load()

	mux := http.NewServeMux()
	mux.Handle("/", web.Handler())

	log.Printf("aiinventory listening on :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, mux); err != nil {
		log.Fatal(err)
	}
}
