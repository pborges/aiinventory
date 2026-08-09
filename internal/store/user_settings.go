package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// GetUserSetting returns (value, true, nil) if key is set for userID,
// ("", false, nil) if absent — the per-user analog of GetSetting/SetSetting,
// generic enough to back any future per-user preference without a new
// migration: callers layer their own typed shape and default on top via
// GetUserSettingJSON/SetUserSettingJSON below.
func (s *Store) GetUserSetting(ctx context.Context, userID int64, key string) (string, bool, error) {
	var value string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM user_settings WHERE user_id = ? AND key = ?`, userID, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("get user setting %q: %w", key, err)
	}
	return value, true, nil
}

func (s *Store) SetUserSetting(ctx context.Context, userID int64, key, value string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO user_settings (user_id, key, value) VALUES (?, ?, ?)
		ON CONFLICT(user_id, key) DO UPDATE SET value = excluded.value, updated_at = datetime('now')`,
		userID, key, value)
	if err != nil {
		return fmt.Errorf("set user setting %q: %w", key, err)
	}
	return nil
}

// DeleteUserSetting removes userID's override for key, if any — "restore
// defaults" for whatever typed default the caller falls back to via
// GetUserSettingJSON.
func (s *Store) DeleteUserSetting(ctx context.Context, userID int64, key string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM user_settings WHERE user_id = ? AND key = ?`, userID, key)
	if err != nil {
		return fmt.Errorf("delete user setting %q: %w", key, err)
	}
	return nil
}

// GetUserSettingJSON decodes key's JSON-encoded value into a T for userID,
// returning def unchanged (and no error) if the key is unset. A generic
// free function rather than a method since Go methods can't take their own
// type parameters — callers get type inference from def, e.g.
// GetUserSettingJSON(ctx, s, userID, key, myDefaultStruct).
func GetUserSettingJSON[T any](ctx context.Context, s *Store, userID int64, key string, def T) (T, error) {
	raw, ok, err := s.GetUserSetting(ctx, userID, key)
	if err != nil || !ok {
		return def, err
	}
	var v T
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return def, fmt.Errorf("decode user setting %q: %w", key, err)
	}
	return v, nil
}

// SetUserSettingJSON JSON-encodes v and stores it under key for userID.
func SetUserSettingJSON[T any](ctx context.Context, s *Store, userID int64, key string, v T) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("encode user setting %q: %w", key, err)
	}
	return s.SetUserSetting(ctx, userID, key, string(raw))
}
