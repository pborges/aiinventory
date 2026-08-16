package api

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// rateLimiter is a deliberately small per-client fixed-window limiter. It
// protects bcrypt and paid AI calls without introducing a deployment
// dependency. Entries are discarded when their next request arrives after
// the window, keeping the map bounded by recently active clients.
type rateLimiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	seen   map[string]rateWindow
	lastGC time.Time
}

type rateWindow struct {
	started time.Time
	count   int
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	return &rateLimiter{limit: limit, window: window, seen: make(map[string]rateWindow)}
}

func (l *rateLimiter) allow(key string, now time.Time) bool {
	return l.allowN(key, now, 1)
}

func (l *rateLimiter) allowN(key string, now time.Time, count int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.lastGC.IsZero() || now.Sub(l.lastGC) >= l.window {
		for existingKey, existing := range l.seen {
			if now.Sub(existing.started) >= l.window {
				delete(l.seen, existingKey)
			}
		}
		l.lastGC = now
	}
	w := l.seen[key]
	if w.started.IsZero() || now.Sub(w.started) >= l.window {
		if count > l.limit {
			return false
		}
		l.seen[key] = rateWindow{started: now, count: count}
		return true
	}
	if count > l.limit-w.count {
		return false
	}
	w.count += count
	l.seen[key] = w
	return true
}

func writeRateLimitError(w http.ResponseWriter) {
	w.Header().Set("Retry-After", "60")
	writeError(w, http.StatusTooManyRequests, "too many requests")
}

func remoteHost(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func (s *Server) clientKey(r *http.Request) string {
	if s.trustProxyHeaders {
		if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
			candidate := strings.TrimSpace(strings.Split(forwarded, ",")[0])
			if net.ParseIP(candidate) != nil {
				return candidate
			}
		}
		if candidate := strings.TrimSpace(r.Header.Get("X-Real-IP")); net.ParseIP(candidate) != nil {
			return candidate
		}
	}
	return remoteHost(r)
}

func (s *Server) rateLimit(l *rateLimiter, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !l.allow(s.clientKey(r), time.Now()) {
			writeRateLimitError(w)
			return
		}
		next(w, r)
	}
}
