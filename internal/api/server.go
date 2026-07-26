// Package api wires HTTP handlers to the store and business-logic layers.
// Handlers stay thin: decode request, call down a layer, encode response.
package api

import (
	"net/http"

	"github.com/pborges/aiinventory/internal/auth"
	"github.com/pborges/aiinventory/internal/gemini"
	"github.com/pborges/aiinventory/internal/inventory"
	"github.com/pborges/aiinventory/internal/store"
	"github.com/pborges/aiinventory/internal/web"
)

type Server struct {
	store            *store.Store
	codec            *auth.Codec
	gemini           gemini.Client // nil if GEMINI_API_KEY wasn't configured — AI-dependent routes handle that
	duplicateRunner  *inventory.Runner
	descriptionBatch *inventory.DescriptionBatch
}

// New assembles the HTTP handler. geminiClient may be nil if no
// GEMINI_API_KEY was configured; routes that need it (capture, reconcile,
// description regeneration, duplicate detection) are responsible for
// returning a clear error when it's nil.
func New(s *store.Store, codec *auth.Codec, geminiClient gemini.Client) http.Handler {
	srv := &Server{
		store:            s,
		codec:            codec,
		gemini:           geminiClient,
		duplicateRunner:  &inventory.Runner{},
		descriptionBatch: &inventory.DescriptionBatch{},
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/auth/bootstrap", srv.handleBootstrapStatus)
	mux.HandleFunc("POST /api/auth/bootstrap", srv.handleBootstrap)
	mux.HandleFunc("POST /api/auth/login", srv.handleLogin)
	mux.HandleFunc("POST /api/auth/logout", srv.handleLogout)
	mux.Handle("GET /api/auth/me", srv.requireAuth(srv.handleMe))

	mux.Handle("GET /api/settings", srv.requireAuth(srv.handleGetSettings))
	mux.Handle("PUT /api/settings", srv.requireAuth(srv.handleUpdateSettings))

	mux.Handle("POST /api/capture/preview", srv.requireAuth(srv.handleCapturePreview))
	mux.Handle("POST /api/capture/apply", srv.requireAuth(srv.handleCaptureApply))

	mux.Handle("POST /api/reconcile/preview", srv.requireAuth(srv.handleReconcilePreview))
	mux.Handle("POST /api/reconcile/apply", srv.requireAuth(srv.handleReconcileApply))

	mux.Handle("GET /api/search", srv.requireAuth(srv.handleSearch))
	mux.Handle("POST /api/items/bulk-delete", srv.requireAuth(srv.handleBulkDelete))
	mux.Handle("POST /api/items/bulk-regenerate-description", srv.requireAuth(srv.handleBulkRegenerateDescription))
	mux.Handle("GET /api/items/bulk-regenerate-description/status", srv.requireAuth(srv.handleBulkRegenerateDescriptionStatus))

	mux.Handle("GET /api/images/{id}", srv.requireAuth(srv.handleGetImage))

	mux.Handle("GET /api/items/{id}", srv.requireAuth(srv.handleGetItem))
	mux.Handle("PUT /api/items/{id}", srv.requireAuth(srv.handleUpdateItem))
	mux.Handle("POST /api/items/{id}/regenerate-description", srv.requireAuth(srv.handleRegenerateItemDescription))
	mux.Handle("PUT /api/items/{id}/images/order", srv.requireAuth(srv.handleReorderImages))
	mux.Handle("DELETE /api/items/{id}/images/{imageId}", srv.requireAuth(srv.handleDeleteImage))

	mux.Handle("GET /api/locations", srv.requireAuth(srv.handleListLocations))
	mux.Handle("GET /api/locations/{id}/items", srv.requireAuth(srv.handleGetLocationItems))
	mux.Handle("GET /api/locations/{id}/activity", srv.requireAuth(srv.handleGetLocationActivity))
	mux.Handle("POST /api/locations/{id}/move-item", srv.requireAuth(srv.handleMoveItem))

	mux.Handle("GET /api/duplicates/status", srv.requireAuth(srv.handleDuplicatesStatus))
	mux.Handle("POST /api/duplicates/run", srv.requireAuth(srv.handleStartDuplicateRun))
	mux.Handle("GET /api/duplicates/groups", srv.requireAuth(srv.handleListDuplicateGroups))
	mux.Handle("POST /api/duplicates/groups/{id}/dismiss", srv.requireAuth(srv.handleDismissDuplicateGroup))
	mux.Handle("POST /api/duplicates/groups/{id}/merge", srv.requireAuth(srv.handleMergeDuplicateGroup))

	mux.Handle("GET /api/users", srv.requireAuth(srv.handleListUsers))
	mux.Handle("POST /api/users", srv.requireAuth(srv.handleCreateUser))
	mux.Handle("PUT /api/users/{id}", srv.requireAuth(srv.handleSetUserEnabled))

	mux.Handle("/", web.Handler())

	return mux
}

func (s *Server) requireAuth(h http.HandlerFunc) http.Handler {
	return auth.RequireAuth(s.codec, s.store)(h)
}
