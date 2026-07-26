// Package api wires HTTP handlers to the store and business-logic layers.
// Handlers stay thin: decode request, call down a layer, encode response.
package api

import (
	"net/http"

	"github.com/pborges/aiinventory/internal/auth"
	"github.com/pborges/aiinventory/internal/gemini"
	"github.com/pborges/aiinventory/internal/store"
	"github.com/pborges/aiinventory/internal/web"
)

type Server struct {
	store  *store.Store
	codec  *auth.Codec
	gemini gemini.Client // nil if GEMINI_API_KEY wasn't configured — AI-dependent routes handle that
}

// New assembles the HTTP handler. geminiClient may be nil if no
// GEMINI_API_KEY was configured; routes that need it (capture, reconcile,
// description regeneration, duplicate detection — added in later phases)
// are responsible for returning a clear error when it's nil.
func New(s *store.Store, codec *auth.Codec, geminiClient gemini.Client) http.Handler {
	srv := &Server{store: s, codec: codec, gemini: geminiClient}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/auth/bootstrap", srv.handleBootstrapStatus)
	mux.HandleFunc("POST /api/auth/bootstrap", srv.handleBootstrap)
	mux.HandleFunc("POST /api/auth/login", srv.handleLogin)
	mux.HandleFunc("POST /api/auth/logout", srv.handleLogout)
	mux.Handle("GET /api/auth/me", srv.requireAuth(srv.handleMe))

	mux.Handle("GET /api/settings", srv.requireAuth(srv.handleGetSettings))
	mux.Handle("PUT /api/settings", srv.requireAuth(srv.handleUpdateSettings))

	mux.Handle("POST /api/capture", srv.requireAuth(srv.handleCapture))

	mux.Handle("/", web.Handler())

	return mux
}

func (s *Server) requireAuth(h http.HandlerFunc) http.Handler {
	return auth.RequireAuth(s.codec, s.store)(h)
}
