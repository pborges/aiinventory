// Package api wires HTTP handlers to the store and business-logic layers.
// Handlers stay thin: decode request, call down a layer, encode response.
package api

import (
	"net/http"
	"sync"

	"github.com/pborges/aiinventory/internal/auth"
	"github.com/pborges/aiinventory/internal/gemini"
	"github.com/pborges/aiinventory/internal/inventory"
	"github.com/pborges/aiinventory/internal/store"
	"github.com/pborges/aiinventory/internal/web"
)

type Server struct {
	store           *store.Store
	codec           *auth.Codec
	geminiMu        sync.RWMutex
	gemini          gemini.Client // nil if no Gemini API key is configured (Settings) — AI-dependent routes handle that
	duplicateRunner *inventory.Runner
	scanStoreDir    string // empty disables saving scan images — see saveScan
}

// New assembles the HTTP handler. geminiClient may be nil if no Gemini API
// key was configured yet; routes that need it (capture, reconcile,
// description regeneration, duplicate detection) are responsible for
// returning a clear error when it's nil. The Settings page can swap it out
// for a new client at runtime — see setGeminiClient. scanStoreDir may be
// empty (the common case) to disable saving scan images entirely.
func New(s *store.Store, codec *auth.Codec, geminiClient gemini.Client, scanStoreDir string) http.Handler {
	srv := &Server{
		store:           s,
		codec:           codec,
		gemini:          geminiClient,
		duplicateRunner: &inventory.Runner{},
		scanStoreDir:    scanStoreDir,
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/version", srv.handleVersion)

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
	mux.Handle("POST /api/reconcile/diff", srv.requireAuth(srv.handleReconcileDiff))
	mux.Handle("POST /api/reconcile/apply", srv.requireAuth(srv.handleReconcileApply))

	mux.Handle("GET /api/search", srv.requireAuth(srv.handleSearch))
	mux.Handle("POST /api/items/bulk-delete", srv.requireAuth(srv.handleBulkDelete))

	mux.Handle("GET /api/images/{id}", srv.requireAuth(srv.handleGetImage))

	mux.Handle("GET /api/items/{id}", srv.requireAuth(srv.handleGetItem))
	mux.Handle("PUT /api/items/{id}", srv.requireAuth(srv.handleUpdateItem))
	mux.Handle("POST /api/items/{id}/regenerate-description", srv.requireAuth(srv.handleRegenerateItemDescription))
	mux.Handle("PUT /api/items/{id}/images/order", srv.requireAuth(srv.handleReorderImages))
	mux.Handle("DELETE /api/items/{id}/images/{imageId}", srv.requireAuth(srv.handleDeleteImage))
	mux.Handle("PUT /api/items/{id}/labels", srv.requireAuth(srv.handleSetItemLabels))

	mux.Handle("GET /api/locations", srv.requireAuth(srv.handleListLocations))
	mux.Handle("PUT /api/locations/{id}", srv.requireAuth(srv.handleUpdateLocation))
	mux.Handle("GET /api/locations/{id}/items", srv.requireAuth(srv.handleGetLocationItems))
	mux.Handle("GET /api/locations/{id}/activity", srv.requireAuth(srv.handleGetLocationActivity))
	mux.Handle("POST /api/locations/{id}/move-item", srv.requireAuth(srv.handleMoveItem))
	mux.Handle("PUT /api/locations/{id}/labels", srv.requireAuth(srv.handleSetLocationLabels))

	mux.Handle("GET /api/duplicates/status", srv.requireAuth(srv.handleDuplicatesStatus))
	mux.Handle("POST /api/duplicates/run", srv.requireAuth(srv.handleStartDuplicateRun))
	mux.Handle("GET /api/duplicates/groups", srv.requireAuth(srv.handleListDuplicateGroups))
	mux.Handle("POST /api/duplicates/groups/{id}/dismiss", srv.requireAuth(srv.handleDismissDuplicateGroup))
	mux.Handle("POST /api/duplicates/groups/{id}/merge", srv.requireAuth(srv.handleMergeDuplicateGroup))

	mux.Handle("GET /api/users", srv.requireAuth(srv.handleListUsers))
	mux.Handle("POST /api/users", srv.requireAuth(srv.handleCreateUser))
	mux.Handle("PUT /api/users/{id}", srv.requireAuth(srv.handleSetUserEnabled))

	mux.Handle("GET /api/labels", srv.requireAuth(srv.handleListLabels))
	mux.Handle("POST /api/labels", srv.requireAuth(srv.handleCreateLabel))
	mux.Handle("PUT /api/labels/{id}", srv.requireAuth(srv.handleUpdateLabel))
	mux.Handle("DELETE /api/labels/{id}", srv.requireAuth(srv.handleDeleteLabel))

	mux.Handle("GET /api/location-labels", srv.requireAuth(srv.handleListLocationLabels))
	mux.Handle("POST /api/location-labels", srv.requireAuth(srv.handleCreateLocationLabel))
	mux.Handle("PUT /api/location-labels/{id}", srv.requireAuth(srv.handleUpdateLocationLabel))
	mux.Handle("DELETE /api/location-labels/{id}", srv.requireAuth(srv.handleDeleteLocationLabel))

	mux.Handle("GET /api/tags", srv.requireAuth(srv.handleListRegisteredAssetTags))
	mux.Handle("POST /api/tags", srv.requireAuth(srv.handleCreateRegisteredAssetTag))
	mux.Handle("DELETE /api/tags/{id}", srv.requireAuth(srv.handleDeleteRegisteredAssetTag))
	mux.Handle("POST /api/tags/upload", srv.requireAuth(srv.handleUploadRegisteredAssetTags))
	mux.Handle("POST /api/tags/sheet", srv.requireAuth(srv.handleGenerateAssetTagSheet))
	mux.Handle("POST /api/tags/sheet/register", srv.requireAuth(srv.handleRegisterAssetTagSheet))
	mux.Handle("GET /api/tags/sheet/settings", srv.requireAuth(srv.handleGetAssetTagSheetSettings))
	mux.Handle("PUT /api/tags/sheet/settings", srv.requireAuth(srv.handleSaveAssetTagSheetSettings))
	mux.Handle("DELETE /api/tags/sheet/settings", srv.requireAuth(srv.handleResetAssetTagSheetSettings))

	mux.Handle("GET /api/location-tags", srv.requireAuth(srv.handleListRegisteredLocationTags))
	mux.Handle("POST /api/location-tags", srv.requireAuth(srv.handleCreateRegisteredLocationTag))
	mux.Handle("DELETE /api/location-tags/{id}", srv.requireAuth(srv.handleDeleteRegisteredLocationTag))
	mux.Handle("POST /api/location-tags/upload", srv.requireAuth(srv.handleUploadRegisteredLocationTags))
	mux.Handle("POST /api/location-tags/sheet", srv.requireAuth(srv.handleGenerateLocationTagSheet))
	mux.Handle("POST /api/location-tags/sheet/register", srv.requireAuth(srv.handleRegisterLocationTagSheet))
	mux.Handle("GET /api/location-tags/sheet/settings", srv.requireAuth(srv.handleGetLocationTagSheetSettings))
	mux.Handle("PUT /api/location-tags/sheet/settings", srv.requireAuth(srv.handleSaveLocationTagSheetSettings))
	mux.Handle("DELETE /api/location-tags/sheet/settings", srv.requireAuth(srv.handleResetLocationTagSheetSettings))

	mux.Handle("/", web.Handler())

	return mux
}

func (s *Server) requireAuth(h http.HandlerFunc) http.Handler {
	return auth.RequireAuth(s.codec, s.store)(h)
}

// geminiClient returns the currently configured Gemini client (nil if AI
// features are disabled). Safe to call concurrently with setGeminiClient.
func (s *Server) geminiClient() gemini.Client {
	s.geminiMu.RLock()
	defer s.geminiMu.RUnlock()
	return s.gemini
}

// setGeminiClient swaps in a new Gemini client (or nil to disable AI
// features), taking effect immediately for any request that hasn't already
// captured the old one. Called by the Settings handler when the API key
// changes.
func (s *Server) setGeminiClient(c gemini.Client) {
	s.geminiMu.Lock()
	s.gemini = c
	s.geminiMu.Unlock()
}
