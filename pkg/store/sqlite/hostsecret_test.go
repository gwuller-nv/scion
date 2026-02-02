package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ptone/scion-agent/pkg/store"
)

func TestHostSecretCRUD(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	// First create a runtime host to satisfy FK constraint
	hostID := uuid.New().String()
	host := &store.RuntimeHost{
		ID:      hostID,
		Name:    "test-host",
		Slug:    "test-host",
		Mode:    store.HostModeConnected,
		Status:  store.HostStatusOnline,
		Created: time.Now(),
		Updated: time.Now(),
	}
	if err := s.CreateRuntimeHost(ctx, host); err != nil {
		t.Fatalf("failed to create runtime host: %v", err)
	}

	// Test CreateHostSecret
	secret := &store.HostSecret{
		HostID:    hostID,
		SecretKey: []byte("test-secret-key-32-bytes-long!!"),
		Algorithm: store.HostSecretAlgorithmHMACSHA256,
		Status:    store.HostSecretStatusActive,
	}
	if err := s.CreateHostSecret(ctx, secret); err != nil {
		t.Fatalf("CreateHostSecret failed: %v", err)
	}

	// Verify timestamps were set
	if secret.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set automatically")
	}

	// Test GetHostSecret
	retrieved, err := s.GetHostSecret(ctx, hostID)
	if err != nil {
		t.Fatalf("GetHostSecret failed: %v", err)
	}
	if retrieved.HostID != hostID {
		t.Errorf("HostID mismatch: got %s, want %s", retrieved.HostID, hostID)
	}
	if string(retrieved.SecretKey) != string(secret.SecretKey) {
		t.Error("SecretKey mismatch")
	}
	if retrieved.Algorithm != store.HostSecretAlgorithmHMACSHA256 {
		t.Errorf("Algorithm mismatch: got %s, want %s", retrieved.Algorithm, store.HostSecretAlgorithmHMACSHA256)
	}
	if retrieved.Status != store.HostSecretStatusActive {
		t.Errorf("Status mismatch: got %s, want %s", retrieved.Status, store.HostSecretStatusActive)
	}

	// Test duplicate create returns error
	if err := s.CreateHostSecret(ctx, secret); err != store.ErrAlreadyExists {
		t.Errorf("Expected ErrAlreadyExists, got: %v", err)
	}

	// Test UpdateHostSecret
	newKey := []byte("new-secret-key-32-bytes-long!!!")
	retrieved.SecretKey = newKey
	retrieved.RotatedAt = time.Now()
	retrieved.Status = store.HostSecretStatusDeprecated

	if err := s.UpdateHostSecret(ctx, retrieved); err != nil {
		t.Fatalf("UpdateHostSecret failed: %v", err)
	}

	// Verify update
	updated, err := s.GetHostSecret(ctx, hostID)
	if err != nil {
		t.Fatalf("GetHostSecret after update failed: %v", err)
	}
	if string(updated.SecretKey) != string(newKey) {
		t.Error("SecretKey not updated")
	}
	if updated.Status != store.HostSecretStatusDeprecated {
		t.Errorf("Status not updated: got %s, want %s", updated.Status, store.HostSecretStatusDeprecated)
	}
	if updated.RotatedAt.IsZero() {
		t.Error("RotatedAt should be set")
	}

	// Test DeleteHostSecret
	if err := s.DeleteHostSecret(ctx, hostID); err != nil {
		t.Fatalf("DeleteHostSecret failed: %v", err)
	}

	// Verify deletion
	_, err = s.GetHostSecret(ctx, hostID)
	if err != store.ErrNotFound {
		t.Errorf("Expected ErrNotFound after delete, got: %v", err)
	}

	// Test delete non-existent returns error
	if err := s.DeleteHostSecret(ctx, "non-existent"); err != store.ErrNotFound {
		t.Errorf("Expected ErrNotFound for non-existent delete, got: %v", err)
	}
}

func TestHostSecretForeignKey(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	// Try to create secret for non-existent host
	secret := &store.HostSecret{
		HostID:    "non-existent-host",
		SecretKey: []byte("test-secret"),
		Algorithm: store.HostSecretAlgorithmHMACSHA256,
		Status:    store.HostSecretStatusActive,
	}

	err := s.CreateHostSecret(ctx, secret)
	if err == nil {
		t.Error("Expected error when creating secret for non-existent host")
	}
}

func TestHostJoinTokenCRUD(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	// First create a runtime host to satisfy FK constraint
	hostID := uuid.New().String()
	host := &store.RuntimeHost{
		ID:      hostID,
		Name:    "test-host-for-token",
		Slug:    "test-host-for-token",
		Mode:    store.HostModeConnected,
		Status:  store.HostStatusOffline,
		Created: time.Now(),
		Updated: time.Now(),
	}
	if err := s.CreateRuntimeHost(ctx, host); err != nil {
		t.Fatalf("failed to create runtime host: %v", err)
	}

	// Test CreateJoinToken
	token := &store.HostJoinToken{
		HostID:    hostID,
		TokenHash: "test-token-hash-abc123",
		ExpiresAt: time.Now().Add(1 * time.Hour),
		CreatedBy: "admin-user-id",
	}
	if err := s.CreateJoinToken(ctx, token); err != nil {
		t.Fatalf("CreateJoinToken failed: %v", err)
	}

	// Verify timestamps were set
	if token.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set automatically")
	}

	// Test GetJoinToken by hash
	retrieved, err := s.GetJoinToken(ctx, "test-token-hash-abc123")
	if err != nil {
		t.Fatalf("GetJoinToken failed: %v", err)
	}
	if retrieved.HostID != hostID {
		t.Errorf("HostID mismatch: got %s, want %s", retrieved.HostID, hostID)
	}
	if retrieved.TokenHash != "test-token-hash-abc123" {
		t.Errorf("TokenHash mismatch: got %s, want %s", retrieved.TokenHash, "test-token-hash-abc123")
	}
	if retrieved.CreatedBy != "admin-user-id" {
		t.Errorf("CreatedBy mismatch: got %s, want %s", retrieved.CreatedBy, "admin-user-id")
	}

	// Test GetJoinTokenByHostID
	byHost, err := s.GetJoinTokenByHostID(ctx, hostID)
	if err != nil {
		t.Fatalf("GetJoinTokenByHostID failed: %v", err)
	}
	if byHost.TokenHash != "test-token-hash-abc123" {
		t.Errorf("TokenHash mismatch: got %s, want %s", byHost.TokenHash, "test-token-hash-abc123")
	}

	// Test duplicate create returns error
	if err := s.CreateJoinToken(ctx, token); err != store.ErrAlreadyExists {
		t.Errorf("Expected ErrAlreadyExists, got: %v", err)
	}

	// Test DeleteJoinToken
	if err := s.DeleteJoinToken(ctx, hostID); err != nil {
		t.Fatalf("DeleteJoinToken failed: %v", err)
	}

	// Verify deletion
	_, err = s.GetJoinToken(ctx, "test-token-hash-abc123")
	if err != store.ErrNotFound {
		t.Errorf("Expected ErrNotFound after delete, got: %v", err)
	}

	// Test delete non-existent returns error
	if err := s.DeleteJoinToken(ctx, "non-existent"); err != store.ErrNotFound {
		t.Errorf("Expected ErrNotFound for non-existent delete, got: %v", err)
	}
}

func TestCleanExpiredJoinTokens(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	// Create two hosts
	host1ID := uuid.New().String()
	host2ID := uuid.New().String()
	for i, id := range []string{host1ID, host2ID} {
		host := &store.RuntimeHost{
			ID:      id,
			Name:    "test-host-" + string(rune('a'+i)),
			Slug:    "test-host-" + string(rune('a'+i)),
			Mode:    store.HostModeConnected,
			Status:  store.HostStatusOffline,
			Created: time.Now(),
			Updated: time.Now(),
		}
		if err := s.CreateRuntimeHost(ctx, host); err != nil {
			t.Fatalf("failed to create runtime host: %v", err)
		}
	}

	// Create an expired token and a valid token
	expiredToken := &store.HostJoinToken{
		HostID:    host1ID,
		TokenHash: "expired-token-hash",
		ExpiresAt: time.Now().Add(-1 * time.Hour), // Already expired
		CreatedBy: "admin",
	}
	validToken := &store.HostJoinToken{
		HostID:    host2ID,
		TokenHash: "valid-token-hash",
		ExpiresAt: time.Now().Add(1 * time.Hour), // Still valid
		CreatedBy: "admin",
	}

	if err := s.CreateJoinToken(ctx, expiredToken); err != nil {
		t.Fatalf("CreateJoinToken (expired) failed: %v", err)
	}
	if err := s.CreateJoinToken(ctx, validToken); err != nil {
		t.Fatalf("CreateJoinToken (valid) failed: %v", err)
	}

	// Clean expired tokens
	if err := s.CleanExpiredJoinTokens(ctx); err != nil {
		t.Fatalf("CleanExpiredJoinTokens failed: %v", err)
	}

	// Verify expired token is gone
	_, err := s.GetJoinToken(ctx, "expired-token-hash")
	if err != store.ErrNotFound {
		t.Errorf("Expected expired token to be deleted, got: %v", err)
	}

	// Verify valid token still exists
	_, err = s.GetJoinToken(ctx, "valid-token-hash")
	if err != nil {
		t.Errorf("Expected valid token to still exist, got: %v", err)
	}
}

func TestHostSecretCascadeDelete(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	// Create a runtime host
	hostID := uuid.New().String()
	host := &store.RuntimeHost{
		ID:      hostID,
		Name:    "cascade-test-host",
		Slug:    "cascade-test-host",
		Mode:    store.HostModeConnected,
		Status:  store.HostStatusOnline,
		Created: time.Now(),
		Updated: time.Now(),
	}
	if err := s.CreateRuntimeHost(ctx, host); err != nil {
		t.Fatalf("failed to create runtime host: %v", err)
	}

	// Create a secret for the host
	secret := &store.HostSecret{
		HostID:    hostID,
		SecretKey: []byte("test-secret"),
		Algorithm: store.HostSecretAlgorithmHMACSHA256,
		Status:    store.HostSecretStatusActive,
	}
	if err := s.CreateHostSecret(ctx, secret); err != nil {
		t.Fatalf("CreateHostSecret failed: %v", err)
	}

	// Verify secret exists
	_, err := s.GetHostSecret(ctx, hostID)
	if err != nil {
		t.Fatalf("GetHostSecret failed: %v", err)
	}

	// Delete the runtime host
	if err := s.DeleteRuntimeHost(ctx, hostID); err != nil {
		t.Fatalf("DeleteRuntimeHost failed: %v", err)
	}

	// Verify secret was cascade deleted
	_, err = s.GetHostSecret(ctx, hostID)
	if err != store.ErrNotFound {
		t.Errorf("Expected secret to be cascade deleted, got: %v", err)
	}
}
