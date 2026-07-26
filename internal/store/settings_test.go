package store

import (
	"context"
	"testing"
)

func TestSettingsGetSet(t *testing.T) {
	ctx := context.Background()
	s := NewTestStore(t)

	if _, ok, err := s.GetSetting(ctx, "nope"); err != nil || ok {
		t.Fatalf("GetSetting on missing key = (ok=%v, err=%v), want (false, nil)", ok, err)
	}

	if err := s.SetSetting(ctx, SettingGeminiModel, "gemini-2.5-flash"); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}
	if v, ok, err := s.GetSetting(ctx, SettingGeminiModel); err != nil || !ok || v != "gemini-2.5-flash" {
		t.Fatalf("GetSetting = (%q, %v, %v), want (gemini-2.5-flash, true, nil)", v, ok, err)
	}

	// overwrite
	if err := s.SetSetting(ctx, SettingGeminiModel, "gemini-2.5-pro"); err != nil {
		t.Fatalf("SetSetting overwrite: %v", err)
	}
	if v, _, _ := s.GetSetting(ctx, SettingGeminiModel); v != "gemini-2.5-pro" {
		t.Fatalf("GetSetting after overwrite = %q, want gemini-2.5-pro", v)
	}
}

func TestGetOrCreateSessionSecret(t *testing.T) {
	ctx := context.Background()
	s := NewTestStore(t)

	secret1, err := s.GetOrCreateSessionSecret(ctx)
	if err != nil {
		t.Fatalf("GetOrCreateSessionSecret: %v", err)
	}
	if len(secret1) < 32 {
		t.Fatalf("secret looks too short: %q", secret1)
	}

	secret2, err := s.GetOrCreateSessionSecret(ctx)
	if err != nil {
		t.Fatalf("GetOrCreateSessionSecret second call: %v", err)
	}
	if secret1 != secret2 {
		t.Fatalf("secret changed across calls: %q != %q", secret1, secret2)
	}
}
