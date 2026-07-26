// Package api wires HTTP handlers to the store and business-logic layers.
// Handlers stay thin: decode request, call down a layer, encode response.
package api

import (
	"net/http"

	"github.com/pborges/aiinventory/internal/auth"
	"github.com/pborges/aiinventory/internal/store"
	"github.com/pborges/aiinventory/internal/web"
)

type Server struct {
	store *store.Store
	codec *auth.Codec
}

func New(s *store.Store, codec *auth.Codec) http.Handler {
	srv := &Server{store: s, codec: codec}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/auth/bootstrap", srv.handleBootstrapStatus)
	mux.HandleFunc("POST /api/auth/bootstrap", srv.handleBootstrap)
	mux.HandleFunc("POST /api/auth/login", srv.handleLogin)
	mux.HandleFunc("POST /api/auth/logout", srv.handleLogout)
	mux.Handle("GET /api/auth/me", srv.requireAuth(srv.handleMe))

	mux.Handle("/", web.Handler())

	return mux
}

func (s *Server) requireAuth(h http.HandlerFunc) http.Handler {
	return auth.RequireAuth(s.codec, s.store)(h)
}
