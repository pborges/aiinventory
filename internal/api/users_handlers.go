package api

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/pborges/aiinventory/internal/auth"
	"github.com/pborges/aiinventory/internal/store"
)

type userListItemResponse struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Enabled   bool   `json:"enabled"`
	CreatedAt string `json:"created_at"`
}

// handleListUsers powers the Settings page's user management section
// (README flow #7). No admin/non-admin distinction — any logged-in,
// enabled user can see and manage every account.
func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.store.ListUsers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]userListItemResponse, 0, len(users))
	for _, u := range users {
		out = append(out, userListItemResponse{ID: u.ID, Username: u.Username, Enabled: u.Enabled, CreatedAt: u.CreatedAt.Format(time.RFC3339)})
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": out})
}

// handleCreateUser creates another account. Reuses the same credentials
// validation as bootstrap/login (internal/api/auth_handlers.go).
func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var creds credentials
	if err := decodeJSON(r, &creds); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if msg := creds.validate(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	hash, err := auth.HashPassword(creds.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	user, err := s.store.CreateUser(r.Context(), creds.Username, hash)
	if err != nil {
		if errors.Is(err, store.ErrUsernameTaken) {
			writeError(w, http.StatusConflict, "username taken")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]userResponse{"user": toUserResponse(user)})
}

type setUserEnabledRequest struct {
	Enabled bool `json:"enabled"`
}

// handleSetUserEnabled toggles an account's enabled flag. Disabling takes
// effect immediately, even for the account's own active session — see
// auth.RequireAuth, which re-checks `enabled` on every request.
func (s *Server) handleSetUserEnabled(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	var req setUserEnabledRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := s.store.SetUserEnabled(r.Context(), id, req.Enabled); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
