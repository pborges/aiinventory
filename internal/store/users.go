package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/pborges/aiinventory/internal/domain"
)

var ErrNotFound = errors.New("not found")
var ErrUsernameTaken = errors.New("username taken")
var ErrBootstrapNotAllowed = errors.New("users already exist")
var ErrAssetTagTaken = errors.New("asset tag taken")

func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n); err != nil {
		return 0, fmt.Errorf("count users: %w", err)
	}
	return n, nil
}

// CreateFirstUser creates the very first account, atomically refusing if any
// user already exists. This is the only way to create an account before
// anyone is logged in (see README's Auth section — first-boot bootstrap).
func (s *Store) CreateFirstUser(ctx context.Context, username, passwordHash string) (domain.User, error) {
	var user domain.User
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n); err != nil {
			return fmt.Errorf("count users: %w", err)
		}
		if n > 0 {
			return ErrBootstrapNotAllowed
		}

		res, err := tx.ExecContext(ctx, `
			INSERT INTO users (username, password_hash, enabled) VALUES (?, ?, 1)`,
			username, passwordHash)
		if err != nil {
			if isUniqueConstraintErr(err) {
				return ErrUsernameTaken
			}
			return fmt.Errorf("create first user: %w", err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			return err
		}

		user, err = s.scanUser(tx.QueryRowContext(ctx, `
			SELECT id, username, password_hash, enabled, created_at FROM users WHERE id = ?`, id))
		return err
	})
	return user, err
}

// CreateUser inserts a new, enabled user with the given (already-hashed) password.
func (s *Store) CreateUser(ctx context.Context, username, passwordHash string) (domain.User, error) {
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO users (username, password_hash, enabled) VALUES (?, ?, 1)`,
		username, passwordHash)
	if err != nil {
		if isUniqueConstraintErr(err) {
			return domain.User{}, ErrUsernameTaken
		}
		return domain.User{}, fmt.Errorf("create user: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return domain.User{}, fmt.Errorf("create user: %w", err)
	}
	return s.GetUserByID(ctx, id)
}

func (s *Store) GetUserByID(ctx context.Context, id int64) (domain.User, error) {
	return s.scanUser(s.db.QueryRowContext(ctx, `
		SELECT id, username, password_hash, enabled, created_at FROM users WHERE id = ?`, id))
}

func (s *Store) GetUserByUsername(ctx context.Context, username string) (domain.User, error) {
	return s.scanUser(s.db.QueryRowContext(ctx, `
		SELECT id, username, password_hash, enabled, created_at FROM users WHERE username = ?`, username))
}

func (s *Store) scanUser(row *sql.Row) (domain.User, error) {
	var u domain.User
	var enabled int
	var createdAt string
	if err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &enabled, &createdAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.User{}, ErrNotFound
		}
		return domain.User{}, fmt.Errorf("scan user: %w", err)
	}
	u.Enabled = enabled != 0
	u.CreatedAt, _ = time.Parse(time.DateTime, createdAt)
	return u, nil
}

func (s *Store) ListUsers(ctx context.Context) ([]domain.User, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, username, password_hash, enabled, created_at FROM users ORDER BY username`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	var users []domain.User
	for rows.Next() {
		var u domain.User
		var enabled int
		var createdAt string
		if err := rows.Scan(&u.ID, &u.Username, &u.PasswordHash, &enabled, &createdAt); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		u.Enabled = enabled != 0
		u.CreatedAt, _ = time.Parse(time.DateTime, createdAt)
		users = append(users, u)
	}
	return users, rows.Err()
}

func (s *Store) SetUserEnabled(ctx context.Context, id int64, enabled bool) error {
	res, err := s.db.ExecContext(ctx, `UPDATE users SET enabled = ? WHERE id = ?`, boolToInt(enabled), id)
	if err != nil {
		return fmt.Errorf("set user enabled: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) SetUserPassword(ctx context.Context, id int64, passwordHash string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE users SET password_hash = ? WHERE id = ?`, passwordHash, id)
	if err != nil {
		return fmt.Errorf("set user password: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// isUniqueConstraintErr reports whether err came from a UNIQUE constraint
// violation. modernc.org/sqlite wraps sqlite's error text rather than
// exposing typed error codes for this, so we match on the message.
func isUniqueConstraintErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}
