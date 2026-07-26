package api

import (
	"net/http"

	"github.com/pborges/aiinventory/internal/version"
)

type versionResponse struct {
	Version string `json:"version"`
}

// handleVersion is intentionally unauthenticated — the webui footer shows
// the running version on the login screen too.
func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, versionResponse{Version: version.Version})
}
