package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/pborges/aiinventory/internal/auth"
	"github.com/pborges/aiinventory/internal/domain"
	"github.com/pborges/aiinventory/internal/store"
)

type userResponse struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Enabled  bool   `json:"enabled"`
}

func toUserResponse(u domain.User) userResponse {
	return userResponse{ID: u.ID, Username: u.Username, Enabled: u.Enabled}
}

type credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (c credentials) validate() string {
	if strings.TrimSpace(c.Username) == "" {
		return "username is required"
	}
	if len(c.Password) < 8 {
		return "password must be at least 8 characters"
	}
	return ""
}

// handleBootstrapStatus tells the frontend whether to show "create the
// first account" instead of the login screen.
func (s *Server) handleBootstrapStatus(w http.ResponseWriter, r *http.Request) {
	n, err := s.store.CountUsers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"needed": n == 0})
}

func (s *Server) handleBootstrap(w http.ResponseWriter, r *http.Request) {
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

	user, err := s.store.CreateFirstUser(r.Context(), creds.Username, hash)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrBootstrapNotAllowed):
			writeError(w, http.StatusConflict, "an account already exists")
		case errors.Is(err, store.ErrUsernameTaken):
			writeError(w, http.StatusConflict, "username taken")
		default:
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	if err := s.codec.SetCookie(w, r, user.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]userResponse{"user": toUserResponse(user)})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var creds credentials
	if err := decodeJSON(r, &creds); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	const invalidMsg = "invalid username or password"

	user, err := s.store.GetUserByUsername(r.Context(), creds.Username)
	if err != nil {
		writeError(w, http.StatusUnauthorized, invalidMsg)
		return
	}
	if !user.Enabled || !auth.VerifyPassword(user.PasswordHash, creds.Password) {
		writeError(w, http.StatusUnauthorized, invalidMsg)
		return
	}

	if err := s.codec.SetCookie(w, r, user.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]userResponse{"user": toUserResponse(user)})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	auth.ClearCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.CurrentUser(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	writeJSON(w, http.StatusOK, map[string]userResponse{"user": toUserResponse(user)})
}
