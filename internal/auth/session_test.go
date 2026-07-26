package auth

import (
	"encoding/json"
	"testing"
	"time"
)

func TestCodecRoundTrip(t *testing.T) {
	c := NewCodec("test-secret")

	value, err := c.Encode(42)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	userID, err := c.Decode(value)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if userID != 42 {
		t.Fatalf("Decode userID = %d, want 42", userID)
	}
}

func TestCodecRejectsTampering(t *testing.T) {
	c := NewCodec("test-secret")
	value, _ := c.Encode(42)

	// flip the last character of the payload segment to simulate tampering
	tampered := value[:len(value)-5] + "AAAAA" + value[len(value):]
	if _, err := c.Decode(tampered); err == nil {
		t.Fatal("Decode accepted a tampered cookie")
	}
}

func TestCodecRejectsWrongSecret(t *testing.T) {
	value, _ := NewCodec("secret-a").Encode(1)
	if _, err := NewCodec("secret-b").Decode(value); err == nil {
		t.Fatal("Decode accepted a cookie signed with a different secret")
	}
}

func TestCodecRejectsExpired(t *testing.T) {
	c := NewCodec("test-secret")
	payload, _ := json.Marshal(sessionPayload{UserID: 1, IssuedAt: time.Now().Add(-31 * 24 * time.Hour)})
	sig := c.sign(payload)
	value := b64(payload) + "." + b64(sig)

	_, err := c.Decode(value)
	if err != ErrSessionExpired {
		t.Fatalf("Decode error = %v, want ErrSessionExpired", err)
	}
}
