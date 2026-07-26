package auth

import (
	"context"
	"errors"
	"net/http"

	"github.com/pborges/aiinventory/internal/domain"
	"github.com/pborges/aiinventory/internal/store"
)

// UserStore is the narrow slice of store.Store this package needs.
type UserStore interface {
	GetUserByID(ctx context.Context, id int64) (domain.User, error)
}

type contextKey int

const userContextKey contextKey = 0

// RequireAuth rejects requests without a valid, unexpired session cookie
// belonging to a currently-enabled user. Because sessions are stateless
// signed cookies (see Codec), the user is re-fetched from the store on
// every request: this is what makes disabling an account (users.enabled)
// take effect immediately, rather than only blocking new logins.
func RequireAuth(codec *Codec, users UserStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(CookieName)
			if err != nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			userID, err := codec.Decode(cookie.Value)
			if err != nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			user, err := users.GetUserByID(r.Context(), userID)
			if err != nil {
				if !errors.Is(err, store.ErrNotFound) {
					http.Error(w, "internal error", http.StatusInternalServerError)
					return
				}
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			if !user.Enabled {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), userContextKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// CurrentUser retrieves the user set by RequireAuth on the request context.
func CurrentUser(ctx context.Context) (domain.User, bool) {
	u, ok := ctx.Value(userContextKey).(domain.User)
	return u, ok
}
