package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
)

// well-known settings keys
const (
	SettingGeminiModel                   = "gemini_model"
	SettingSessionSecret                 = "session_secret"
	SettingPromptTagCapture              = "prompt.tag_capture"
	SettingPromptLocationReconciliation  = "prompt.location_reconciliation"
	SettingPromptDescriptionRegeneration = "prompt.description_regeneration"
	SettingPromptDuplicateDetection      = "prompt.duplicate_detection"
)

// GetSetting returns (value, true, nil) if key is set, ("", false, nil) if absent.
func (s *Store) GetSetting(ctx context.Context, key string) (string, bool, error) {
	var value string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("get setting %q: %w", key, err)
	}
	return value, true, nil
}

func (s *Store) SetSetting(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	if err != nil {
		return fmt.Errorf("set setting %q: %w", key, err)
	}
	return nil
}

// GetOrCreateSessionSecret returns the persisted session secret, generating
// and persisting a new random one on first use. Only called when the
// SESSION_SECRET env var isn't set — see internal/config.
func (s *Store) GetOrCreateSessionSecret(ctx context.Context) (string, error) {
	if v, ok, err := s.GetSetting(ctx, SettingSessionSecret); err != nil {
		return "", err
	} else if ok {
		return v, nil
	}

	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate session secret: %w", err)
	}
	secret := base64.RawURLEncoding.EncodeToString(buf)

	if err := s.SetSetting(ctx, SettingSessionSecret, secret); err != nil {
		return "", err
	}
	return secret, nil
}
