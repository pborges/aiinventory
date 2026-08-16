package api

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimiterChargesBatchCostAndPrunesStaleClients(t *testing.T) {
	limiter := newRateLimiter(3, time.Minute)
	now := time.Now()
	if !limiter.allowN("client-a", now, 2) || limiter.allowN("client-a", now, 2) {
		t.Fatal("batch cost was not counted against the limit")
	}
	if !limiter.allow("client-b", now) {
		t.Fatal("independent client was unexpectedly limited")
	}
	if !limiter.allow("client-c", now.Add(2*time.Minute)) {
		t.Fatal("request after window was unexpectedly limited")
	}
	if _, exists := limiter.seen["client-a"]; exists {
		t.Fatal("stale client entry was not pruned")
	}
}

func TestClientKeyOnlyTrustsForwardingHeadersWhenConfigured(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.8:1234"
	req.Header.Set("X-Forwarded-For", "203.0.113.7, 10.0.0.8")

	server := &Server{}
	if got := server.clientKey(req); got != "10.0.0.8" {
		t.Fatalf("untrusted client key = %q, want direct peer", got)
	}
	server.trustProxyHeaders = true
	if got := server.clientKey(req); got != "203.0.113.7" {
		t.Fatalf("trusted proxy client key = %q, want forwarded client", got)
	}
}
