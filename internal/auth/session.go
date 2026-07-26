package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

const (
	CookieName = "aiinventory_session"
	sessionTTL = 30 * 24 * time.Hour
)

var (
	ErrInvalidSession = errors.New("invalid session")
	ErrSessionExpired = errors.New("session expired")
)

type sessionPayload struct {
	UserID   int64     `json:"user_id"`
	IssuedAt time.Time `json:"issued_at"`
}

// Codec signs and verifies stateless session cookies with HMAC-SHA256.
// There is deliberately no server-side session table — see README's Auth
// section — so the cookie itself carries the session, and RequireAuth
// re-checks the user's `enabled` flag on every request.
type Codec struct {
	secret []byte
}

func NewCodec(secret string) *Codec {
	return &Codec{secret: []byte(secret)}
}

func (c *Codec) Encode(userID int64) (string, error) {
	payload, err := json.Marshal(sessionPayload{UserID: userID, IssuedAt: time.Now().UTC()})
	if err != nil {
		return "", err
	}
	sig := c.sign(payload)
	return b64(payload) + "." + b64(sig), nil
}

func (c *Codec) Decode(value string) (int64, error) {
	parts := strings.SplitN(value, ".", 2)
	if len(parts) != 2 {
		return 0, ErrInvalidSession
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return 0, ErrInvalidSession
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return 0, ErrInvalidSession
	}
	if !hmac.Equal(sig, c.sign(payload)) {
		return 0, ErrInvalidSession
	}

	var p sessionPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return 0, ErrInvalidSession
	}
	if time.Since(p.IssuedAt) > sessionTTL {
		return 0, ErrSessionExpired
	}
	return p.UserID, nil
}

func (c *Codec) sign(payload []byte) []byte {
	mac := hmac.New(sha256.New, c.secret)
	mac.Write(payload)
	return mac.Sum(nil)
}

func b64(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

// SetCookie issues a signed session cookie for userID.
func (c *Codec) SetCookie(w http.ResponseWriter, r *http.Request, userID int64) error {
	value, err := c.Encode(userID)
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    value,
		Path:     "/",
		MaxAge:   int(sessionTTL.Seconds()),
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}

func ClearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}
