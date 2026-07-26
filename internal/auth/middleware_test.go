package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pborges/aiinventory/internal/domain"
	"github.com/pborges/aiinventory/internal/store"
)

type fakeUserStore struct {
	users map[int64]domain.User
}

func (f *fakeUserStore) GetUserByID(ctx context.Context, id int64) (domain.User, error) {
	u, ok := f.users[id]
	if !ok {
		return domain.User{}, store.ErrNotFound
	}
	return u, nil
}

func newProtectedHandler(codec *Codec, users UserStore) http.Handler {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := CurrentUser(r.Context())
		if !ok {
			http.Error(w, "no user in context", http.StatusInternalServerError)
			return
		}
		w.Write([]byte(u.Username))
	})
	return RequireAuth(codec, users)(inner)
}

func TestRequireAuth(t *testing.T) {
	codec := NewCodec("test-secret")
	users := &fakeUserStore{users: map[int64]domain.User{
		1: {ID: 1, Username: "alice", Enabled: true},
		2: {ID: 2, Username: "bob", Enabled: false},
	}}
	handler := newProtectedHandler(codec, users)

	t.Run("no cookie", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", w.Code)
		}
	})

	t.Run("valid cookie, enabled user", func(t *testing.T) {
		value, _ := codec.Encode(1)
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.AddCookie(&http.Cookie{Name: CookieName, Value: value})
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		if w.Body.String() != "alice" {
			t.Fatalf("body = %q, want alice", w.Body.String())
		}
	})

	t.Run("valid cookie, disabled user rejected immediately", func(t *testing.T) {
		value, _ := codec.Encode(2)
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.AddCookie(&http.Cookie{Name: CookieName, Value: value})
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401 (disabled user's existing cookie must stop working)", w.Code)
		}
	})

	t.Run("valid cookie, deleted user", func(t *testing.T) {
		value, _ := codec.Encode(999)
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.AddCookie(&http.Cookie{Name: CookieName, Value: value})
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", w.Code)
		}
	})

	t.Run("garbage cookie", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.AddCookie(&http.Cookie{Name: CookieName, Value: "garbage"})
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", w.Code)
		}
	})
}
