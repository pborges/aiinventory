package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pborges/aiinventory/internal/auth"
	"github.com/pborges/aiinventory/internal/store"
)

func newTestServer(t *testing.T) (http.Handler, *store.Store) {
	t.Helper()
	s := store.NewTestStore(t)
	codec := auth.NewCodec("test-secret")
	return New(s, codec, nil), s
}

func doJSON(t *testing.T, h http.Handler, method, path string, body any, cookies []*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func TestBootstrapFlow(t *testing.T) {
	h, _ := newTestServer(t)

	// initially, bootstrap is needed
	w := doJSON(t, h, http.MethodGet, "/api/auth/bootstrap", nil, nil)
	var status map[string]bool
	json.NewDecoder(w.Body).Decode(&status)
	if !status["needed"] {
		t.Fatalf("bootstrap status = %v, want needed=true", status)
	}

	// create the first account
	w = doJSON(t, h, http.MethodPost, "/api/auth/bootstrap", credentials{Username: "alice", Password: "correcthorse"}, nil)
	if w.Code != http.StatusCreated {
		t.Fatalf("bootstrap status = %d, body = %s", w.Code, w.Body.String())
	}
	cookies := w.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("bootstrap did not set a session cookie")
	}

	// bootstrap is no longer needed
	w = doJSON(t, h, http.MethodGet, "/api/auth/bootstrap", nil, nil)
	json.NewDecoder(w.Body).Decode(&status)
	if status["needed"] {
		t.Fatalf("bootstrap status = %v, want needed=false after first account created", status)
	}

	// a second bootstrap attempt is rejected
	w = doJSON(t, h, http.MethodPost, "/api/auth/bootstrap", credentials{Username: "mallory", Password: "correcthorse"}, nil)
	if w.Code != http.StatusConflict {
		t.Fatalf("second bootstrap status = %d, want 409", w.Code)
	}

	// the session cookie from bootstrap works against a protected route
	w = doJSON(t, h, http.MethodGet, "/api/auth/me", nil, cookies)
	if w.Code != http.StatusOK {
		t.Fatalf("/api/auth/me status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestLoginLogoutFlow(t *testing.T) {
	h, _ := newTestServer(t)
	doJSON(t, h, http.MethodPost, "/api/auth/bootstrap", credentials{Username: "alice", Password: "correcthorse"}, nil)

	// wrong password
	w := doJSON(t, h, http.MethodPost, "/api/auth/login", credentials{Username: "alice", Password: "wrongpassword"}, nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password status = %d, want 401", w.Code)
	}

	// unknown username
	w = doJSON(t, h, http.MethodPost, "/api/auth/login", credentials{Username: "nobody", Password: "correcthorse"}, nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unknown user status = %d, want 401", w.Code)
	}

	// correct login
	w = doJSON(t, h, http.MethodPost, "/api/auth/login", credentials{Username: "alice", Password: "correcthorse"}, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("login status = %d, body = %s", w.Code, w.Body.String())
	}
	cookies := w.Result().Cookies()

	w = doJSON(t, h, http.MethodGet, "/api/auth/me", nil, cookies)
	if w.Code != http.StatusOK {
		t.Fatalf("/api/auth/me after login = %d", w.Code)
	}

	// logout clears the cookie
	w = doJSON(t, h, http.MethodPost, "/api/auth/logout", nil, cookies)
	if w.Code != http.StatusNoContent {
		t.Fatalf("logout status = %d, want 204", w.Code)
	}
}

func TestDisabledUserRejectedImmediately(t *testing.T) {
	h, s := newTestServer(t)
	w := doJSON(t, h, http.MethodPost, "/api/auth/bootstrap", credentials{Username: "alice", Password: "correcthorse"}, nil)
	cookies := w.Result().Cookies()

	users, err := s.ListUsers(t.Context())
	if err != nil || len(users) != 1 {
		t.Fatalf("ListUsers = %v, %v", users, err)
	}
	if err := s.SetUserEnabled(t.Context(), users[0].ID, false); err != nil {
		t.Fatalf("SetUserEnabled: %v", err)
	}

	w = doJSON(t, h, http.MethodGet, "/api/auth/me", nil, cookies)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("/api/auth/me for disabled user = %d, want 401", w.Code)
	}
}
