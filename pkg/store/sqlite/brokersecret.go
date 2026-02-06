// Package sqlite provides a SQLite implementation of the Store interface.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/ptone/scion-agent/pkg/store"
)

// ============================================================================
// Host Secret Operations
// ============================================================================

// CreateBrokerSecret creates a new host secret record.
func (s *SQLiteStore) CreateBrokerSecret(ctx context.Context, secret *store.BrokerSecret) error {
	if secret.BrokerID == "" {
		return store.ErrInvalidInput
	}

	now := time.Now()
	if secret.CreatedAt.IsZero() {
		secret.CreatedAt = now
	}
	if secret.Algorithm == "" {
		secret.Algorithm = store.BrokerSecretAlgorithmHMACSHA256
	}
	if secret.Status == "" {
		secret.Status = store.BrokerSecretStatusActive
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO broker_secrets (
			broker_id, secret_key, algorithm,
			created_at, rotated_at, expires_at, status
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`,
		secret.BrokerID, secret.SecretKey, secret.Algorithm,
		secret.CreatedAt, nullableTime(secret.RotatedAt), nullableTime(secret.ExpiresAt), secret.Status,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") ||
			strings.Contains(err.Error(), "FOREIGN KEY constraint failed") {
			return store.ErrAlreadyExists
		}
		return err
	}
	return nil
}

// GetBrokerSecret retrieves a host secret by host ID.
func (s *SQLiteStore) GetBrokerSecret(ctx context.Context, brokerID string) (*store.BrokerSecret, error) {
	secret := &store.BrokerSecret{}
	var rotatedAt, expiresAt sql.NullTime

	err := s.db.QueryRowContext(ctx, `
		SELECT broker_id, secret_key, algorithm,
			created_at, rotated_at, expires_at, status
		FROM broker_secrets WHERE broker_id = ?
	`, brokerID).Scan(
		&secret.BrokerID, &secret.SecretKey, &secret.Algorithm,
		&secret.CreatedAt, &rotatedAt, &expiresAt, &secret.Status,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrNotFound
		}
		return nil, err
	}

	if rotatedAt.Valid {
		secret.RotatedAt = rotatedAt.Time
	}
	if expiresAt.Valid {
		secret.ExpiresAt = expiresAt.Time
	}

	return secret, nil
}

// GetActiveSecrets retrieves all active and deprecated secrets for a host.
// This supports dual-secret validation during rotation grace periods.
func (s *SQLiteStore) GetActiveSecrets(ctx context.Context, brokerID string) ([]*store.BrokerSecret, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT broker_id, secret_key, algorithm,
			created_at, rotated_at, expires_at, status
		FROM broker_secrets
		WHERE broker_id = ? AND status IN (?, ?)
		ORDER BY created_at DESC
	`, brokerID, store.BrokerSecretStatusActive, store.BrokerSecretStatusDeprecated)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var secrets []*store.BrokerSecret
	for rows.Next() {
		secret := &store.BrokerSecret{}
		var rotatedAt, expiresAt sql.NullTime

		if err := rows.Scan(
			&secret.BrokerID, &secret.SecretKey, &secret.Algorithm,
			&secret.CreatedAt, &rotatedAt, &expiresAt, &secret.Status,
		); err != nil {
			return nil, err
		}

		if rotatedAt.Valid {
			secret.RotatedAt = rotatedAt.Time
		}
		if expiresAt.Valid {
			secret.ExpiresAt = expiresAt.Time
		}

		secrets = append(secrets, secret)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return secrets, nil
}

// UpdateBrokerSecret updates an existing host secret.
func (s *SQLiteStore) UpdateBrokerSecret(ctx context.Context, secret *store.BrokerSecret) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE broker_secrets SET
			secret_key = ?,
			algorithm = ?,
			rotated_at = ?,
			expires_at = ?,
			status = ?
		WHERE broker_id = ?
	`,
		secret.SecretKey, secret.Algorithm,
		nullableTime(secret.RotatedAt), nullableTime(secret.ExpiresAt), secret.Status,
		secret.BrokerID,
	)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return store.ErrNotFound
	}
	return nil
}

// DeleteBrokerSecret removes a host secret.
func (s *SQLiteStore) DeleteBrokerSecret(ctx context.Context, brokerID string) error {
	result, err := s.db.ExecContext(ctx, `
		DELETE FROM broker_secrets WHERE broker_id = ?
	`, brokerID)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return store.ErrNotFound
	}
	return nil
}

// ============================================================================
// Host Join Token Operations
// ============================================================================

// CreateJoinToken creates a new join token for host registration.
func (s *SQLiteStore) CreateJoinToken(ctx context.Context, token *store.BrokerJoinToken) error {
	if token.BrokerID == "" || token.TokenHash == "" {
		return store.ErrInvalidInput
	}

	now := time.Now()
	if token.CreatedAt.IsZero() {
		token.CreatedAt = now
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO broker_join_tokens (
			broker_id, token_hash, expires_at, created_at, created_by
		) VALUES (?, ?, ?, ?, ?)
	`,
		token.BrokerID, token.TokenHash, token.ExpiresAt, token.CreatedAt, token.CreatedBy,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return store.ErrAlreadyExists
		}
		if strings.Contains(err.Error(), "FOREIGN KEY constraint failed") {
			return store.ErrNotFound
		}
		return err
	}
	return nil
}

// GetJoinToken retrieves a join token by token hash.
func (s *SQLiteStore) GetJoinToken(ctx context.Context, tokenHash string) (*store.BrokerJoinToken, error) {
	token := &store.BrokerJoinToken{}

	err := s.db.QueryRowContext(ctx, `
		SELECT broker_id, token_hash, expires_at, created_at, created_by
		FROM broker_join_tokens WHERE token_hash = ?
	`, tokenHash).Scan(
		&token.BrokerID, &token.TokenHash, &token.ExpiresAt, &token.CreatedAt, &token.CreatedBy,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrNotFound
		}
		return nil, err
	}
	return token, nil
}

// GetJoinTokenByBrokerID retrieves a join token by host ID.
func (s *SQLiteStore) GetJoinTokenByBrokerID(ctx context.Context, brokerID string) (*store.BrokerJoinToken, error) {
	token := &store.BrokerJoinToken{}

	err := s.db.QueryRowContext(ctx, `
		SELECT broker_id, token_hash, expires_at, created_at, created_by
		FROM broker_join_tokens WHERE broker_id = ?
	`, brokerID).Scan(
		&token.BrokerID, &token.TokenHash, &token.ExpiresAt, &token.CreatedAt, &token.CreatedBy,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrNotFound
		}
		return nil, err
	}
	return token, nil
}

// DeleteJoinToken removes a join token by host ID.
func (s *SQLiteStore) DeleteJoinToken(ctx context.Context, brokerID string) error {
	result, err := s.db.ExecContext(ctx, `
		DELETE FROM broker_join_tokens WHERE broker_id = ?
	`, brokerID)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return store.ErrNotFound
	}
	return nil
}

// CleanExpiredJoinTokens removes all expired join tokens.
func (s *SQLiteStore) CleanExpiredJoinTokens(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM broker_join_tokens WHERE expires_at < ?
	`, time.Now())
	return err
}
