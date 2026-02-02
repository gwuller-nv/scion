package hub

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ptone/scion-agent/pkg/store"
	"github.com/ptone/scion-agent/pkg/store/sqlite"
)

func setupTestHostAuthService(t *testing.T) (*HostAuthService, store.Store) {
	t.Helper()

	s, err := sqlite.New(":memory:")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	if err := s.Migrate(context.Background()); err != nil {
		t.Fatalf("failed to migrate store: %v", err)
	}

	config := DefaultHostAuthConfig()
	svc := NewHostAuthService(config, s)

	return svc, s
}

func TestHostRegistrationAndJoin(t *testing.T) {
	svc, _ := setupTestHostAuthService(t)
	ctx := context.Background()

	// Create a host registration
	req := CreateHostRegistrationRequest{
		Name: "test-host",
		Labels: map[string]string{
			"env": "test",
		},
	}

	resp, err := svc.CreateHostRegistration(ctx, req, "admin-user-id")
	if err != nil {
		t.Fatalf("CreateHostRegistration failed: %v", err)
	}

	if resp.HostID == "" {
		t.Error("HostID should not be empty")
	}
	if resp.JoinToken == "" {
		t.Error("JoinToken should not be empty")
	}
	if !strings.HasPrefix(resp.JoinToken, JoinTokenPrefix) {
		t.Errorf("JoinToken should have prefix %s, got: %s", JoinTokenPrefix, resp.JoinToken)
	}
	if resp.ExpiresAt.IsZero() {
		t.Error("ExpiresAt should be set")
	}
	if resp.ExpiresAt.Before(time.Now()) {
		t.Error("ExpiresAt should be in the future")
	}

	// Complete the join
	joinReq := HostJoinRequest{
		HostID:    resp.HostID,
		JoinToken: resp.JoinToken,
		Hostname:  "test-hostname",
		Version:   "1.0.0",
	}

	joinResp, err := svc.CompleteHostJoin(ctx, joinReq, "http://localhost:9810")
	if err != nil {
		t.Fatalf("CompleteHostJoin failed: %v", err)
	}

	if joinResp.HostID != resp.HostID {
		t.Errorf("HostID mismatch: got %s, want %s", joinResp.HostID, resp.HostID)
	}
	if joinResp.SecretKey == "" {
		t.Error("SecretKey should not be empty")
	}
	if joinResp.HubEndpoint != "http://localhost:9810" {
		t.Errorf("HubEndpoint mismatch: got %s, want http://localhost:9810", joinResp.HubEndpoint)
	}

	// Verify the secret key is valid base64
	secretBytes, err := base64.StdEncoding.DecodeString(joinResp.SecretKey)
	if err != nil {
		t.Errorf("SecretKey should be valid base64: %v", err)
	}
	if len(secretBytes) != 32 {
		t.Errorf("SecretKey should be 32 bytes, got %d", len(secretBytes))
	}
}

func TestJoinWithInvalidToken(t *testing.T) {
	svc, _ := setupTestHostAuthService(t)
	ctx := context.Background()

	// Create a host registration
	req := CreateHostRegistrationRequest{Name: "test-host"}
	resp, err := svc.CreateHostRegistration(ctx, req, "admin")
	if err != nil {
		t.Fatalf("CreateHostRegistration failed: %v", err)
	}

	// Try to join with wrong token
	joinReq := HostJoinRequest{
		HostID:    resp.HostID,
		JoinToken: JoinTokenPrefix + "invalid-token",
		Hostname:  "test",
		Version:   "1.0.0",
	}

	_, err = svc.CompleteHostJoin(ctx, joinReq, "http://localhost:9810")
	if err == nil {
		t.Error("Expected error for invalid token")
	}
	if !strings.Contains(err.Error(), "invalid join token") {
		t.Errorf("Expected 'invalid join token' error, got: %v", err)
	}
}

func TestJoinWithExpiredToken(t *testing.T) {
	// Create service with short token expiry
	s, err := sqlite.New(":memory:")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	if err := s.Migrate(context.Background()); err != nil {
		t.Fatalf("failed to migrate store: %v", err)
	}

	config := DefaultHostAuthConfig()
	config.JoinTokenExpiry = -1 * time.Hour // Already expired
	svc := NewHostAuthService(config, s)
	ctx := context.Background()

	// Create a host registration (token will already be expired)
	req := CreateHostRegistrationRequest{Name: "test-host"}
	resp, err := svc.CreateHostRegistration(ctx, req, "admin")
	if err != nil {
		t.Fatalf("CreateHostRegistration failed: %v", err)
	}

	// Try to join
	joinReq := HostJoinRequest{
		HostID:    resp.HostID,
		JoinToken: resp.JoinToken,
		Hostname:  "test",
		Version:   "1.0.0",
	}

	_, err = svc.CompleteHostJoin(ctx, joinReq, "http://localhost:9810")
	if err == nil {
		t.Error("Expected error for expired token")
	}
	if !strings.Contains(err.Error(), "expired") {
		t.Errorf("Expected 'expired' error, got: %v", err)
	}
}

func TestJoinTokenSingleUse(t *testing.T) {
	svc, _ := setupTestHostAuthService(t)
	ctx := context.Background()

	// Create and complete a host registration
	req := CreateHostRegistrationRequest{Name: "test-host"}
	resp, err := svc.CreateHostRegistration(ctx, req, "admin")
	if err != nil {
		t.Fatalf("CreateHostRegistration failed: %v", err)
	}

	joinReq := HostJoinRequest{
		HostID:    resp.HostID,
		JoinToken: resp.JoinToken,
		Hostname:  "test",
		Version:   "1.0.0",
	}

	// First join should succeed
	_, err = svc.CompleteHostJoin(ctx, joinReq, "http://localhost:9810")
	if err != nil {
		t.Fatalf("First CompleteHostJoin failed: %v", err)
	}

	// Second join with same token should fail
	_, err = svc.CompleteHostJoin(ctx, joinReq, "http://localhost:9810")
	if err == nil {
		t.Error("Expected error for reused token")
	}
}

func TestValidateHostSignature(t *testing.T) {
	svc, s := setupTestHostAuthService(t)
	ctx := context.Background()

	// Create a host and set up its secret
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

	secretKey := []byte("test-secret-key-32-bytes-long!!")
	secret := &store.HostSecret{
		HostID:    hostID,
		SecretKey: secretKey,
		Algorithm: store.HostSecretAlgorithmHMACSHA256,
		Status:    store.HostSecretStatusActive,
	}
	if err := s.CreateHostSecret(ctx, secret); err != nil {
		t.Fatalf("failed to create host secret: %v", err)
	}

	// Create a signed request
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	nonce := "test-nonce-123"
	body := []byte(`{"test": "data"}`)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/test", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(HeaderHostID, hostID)
	req.Header.Set(HeaderTimestamp, timestamp)
	req.Header.Set(HeaderNonce, nonce)

	// Build canonical string and compute signature
	canonicalString := svc.buildCanonicalString(req, timestamp, nonce)

	// Reset body for validation
	req.Body = io.NopCloser(bytes.NewReader(body))

	h := hmac.New(sha256.New, secretKey)
	h.Write(canonicalString)
	signature := base64.StdEncoding.EncodeToString(h.Sum(nil))
	req.Header.Set(HeaderSignature, signature)

	// Validate the signature
	identity, err := svc.ValidateHostSignature(ctx, req)
	if err != nil {
		t.Fatalf("ValidateHostSignature failed: %v", err)
	}

	if identity.HostID() != hostID {
		t.Errorf("HostID mismatch: got %s, want %s", identity.HostID(), hostID)
	}
	if identity.Type() != "host" {
		t.Errorf("Type mismatch: got %s, want host", identity.Type())
	}
}

func TestValidateHostSignature_InvalidSignature(t *testing.T) {
	svc, s := setupTestHostAuthService(t)
	ctx := context.Background()

	// Create a host with secret
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

	secret := &store.HostSecret{
		HostID:    hostID,
		SecretKey: []byte("correct-secret-key-32-bytes-ok!"),
		Algorithm: store.HostSecretAlgorithmHMACSHA256,
		Status:    store.HostSecretStatusActive,
	}
	if err := s.CreateHostSecret(ctx, secret); err != nil {
		t.Fatalf("failed to create host secret: %v", err)
	}

	// Create a request with wrong signature
	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	req.Header.Set(HeaderHostID, hostID)
	req.Header.Set(HeaderTimestamp, strconv.FormatInt(time.Now().Unix(), 10))
	req.Header.Set(HeaderNonce, "test-nonce")
	req.Header.Set(HeaderSignature, "invalid-signature")

	_, err := svc.ValidateHostSignature(ctx, req)
	if err == nil {
		t.Error("Expected error for invalid signature")
	}
	if !strings.Contains(err.Error(), "invalid signature") {
		t.Errorf("Expected 'invalid signature' error, got: %v", err)
	}
}

func TestValidateHostSignature_ClockSkew(t *testing.T) {
	// Create service with short clock skew tolerance
	s, err := sqlite.New(":memory:")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	if err := s.Migrate(context.Background()); err != nil {
		t.Fatalf("failed to migrate store: %v", err)
	}

	config := DefaultHostAuthConfig()
	config.MaxClockSkew = 1 * time.Second
	svc := NewHostAuthService(config, s)
	ctx := context.Background()

	// Create a host with secret
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

	secret := &store.HostSecret{
		HostID:    hostID,
		SecretKey: []byte("test-secret-key-32-bytes-long!!"),
		Algorithm: store.HostSecretAlgorithmHMACSHA256,
		Status:    store.HostSecretStatusActive,
	}
	if err := s.CreateHostSecret(ctx, secret); err != nil {
		t.Fatalf("failed to create host secret: %v", err)
	}

	// Create a request with old timestamp
	oldTimestamp := strconv.FormatInt(time.Now().Add(-10*time.Minute).Unix(), 10)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	req.Header.Set(HeaderHostID, hostID)
	req.Header.Set(HeaderTimestamp, oldTimestamp)
	req.Header.Set(HeaderNonce, "test-nonce")
	req.Header.Set(HeaderSignature, "some-signature")

	_, err = svc.ValidateHostSignature(ctx, req)
	if err == nil {
		t.Error("Expected error for clock skew")
	}
	if !strings.Contains(err.Error(), "timestamp") {
		t.Errorf("Expected timestamp error, got: %v", err)
	}
}

func TestValidateHostSignature_MissingHeaders(t *testing.T) {
	svc, _ := setupTestHostAuthService(t)
	ctx := context.Background()

	tests := []struct {
		name        string
		setupReq    func(*http.Request)
		expectedErr string
	}{
		{
			name: "missing host ID",
			setupReq: func(r *http.Request) {
				r.Header.Set(HeaderTimestamp, strconv.FormatInt(time.Now().Unix(), 10))
				r.Header.Set(HeaderSignature, "sig")
			},
			expectedErr: "missing X-Scion-Host-ID",
		},
		{
			name: "missing timestamp",
			setupReq: func(r *http.Request) {
				r.Header.Set(HeaderHostID, "host-id")
				r.Header.Set(HeaderSignature, "sig")
			},
			expectedErr: "missing X-Scion-Timestamp",
		},
		{
			name: "missing signature",
			setupReq: func(r *http.Request) {
				r.Header.Set(HeaderHostID, "host-id")
				r.Header.Set(HeaderTimestamp, strconv.FormatInt(time.Now().Unix(), 10))
			},
			expectedErr: "missing X-Scion-Signature",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
			tc.setupReq(req)

			_, err := svc.ValidateHostSignature(ctx, req)
			if err == nil {
				t.Error("Expected error")
			}
			if !strings.Contains(err.Error(), tc.expectedErr) {
				t.Errorf("Expected error containing '%s', got: %v", tc.expectedErr, err)
			}
		})
	}
}

func TestHostAuthMiddleware(t *testing.T) {
	svc, s := setupTestHostAuthService(t)
	ctx := context.Background()

	// Create a host with secret
	hostID := uuid.New().String()
	host := &store.RuntimeHost{
		ID:      hostID,
		Name:    "middleware-test-host",
		Slug:    "middleware-test-host",
		Mode:    store.HostModeConnected,
		Status:  store.HostStatusOnline,
		Created: time.Now(),
		Updated: time.Now(),
	}
	if err := s.CreateRuntimeHost(ctx, host); err != nil {
		t.Fatalf("failed to create runtime host: %v", err)
	}

	secretKey := []byte("middleware-secret-key-32-bytes!!")
	secret := &store.HostSecret{
		HostID:    hostID,
		SecretKey: secretKey,
		Algorithm: store.HostSecretAlgorithmHMACSHA256,
		Status:    store.HostSecretStatusActive,
	}
	if err := s.CreateHostSecret(ctx, secret); err != nil {
		t.Fatalf("failed to create host secret: %v", err)
	}

	// Create a handler that checks for host identity
	var gotIdentity HostIdentity
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotIdentity = GetHostIdentityFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	// Wrap with middleware
	wrapped := HostAuthMiddleware(svc)(handler)

	// Test 1: Request without host ID header should pass through
	t.Run("no host header passes through", func(t *testing.T) {
		gotIdentity = nil
		req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
		w := httptest.NewRecorder()
		wrapped.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected 200, got %d", w.Code)
		}
		if gotIdentity != nil {
			t.Error("Expected no identity for unauthenticated request")
		}
	})

	// Test 2: Request with valid signature should set identity
	t.Run("valid signature sets identity", func(t *testing.T) {
		gotIdentity = nil
		timestamp := strconv.FormatInt(time.Now().Unix(), 10)
		nonce := "test-nonce"

		req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
		req.Header.Set(HeaderHostID, hostID)
		req.Header.Set(HeaderTimestamp, timestamp)
		req.Header.Set(HeaderNonce, nonce)

		// Compute signature
		canonicalString := svc.buildCanonicalString(req, timestamp, nonce)
		h := hmac.New(sha256.New, secretKey)
		h.Write(canonicalString)
		signature := base64.StdEncoding.EncodeToString(h.Sum(nil))
		req.Header.Set(HeaderSignature, signature)

		w := httptest.NewRecorder()
		wrapped.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected 200, got %d", w.Code)
		}
		if gotIdentity == nil {
			t.Fatal("Expected identity to be set")
		}
		if gotIdentity.HostID() != hostID {
			t.Errorf("HostID mismatch: got %s, want %s", gotIdentity.HostID(), hostID)
		}
	})

	// Test 3: Request with invalid signature should return 401
	t.Run("invalid signature returns 401", func(t *testing.T) {
		gotIdentity = nil
		req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
		req.Header.Set(HeaderHostID, hostID)
		req.Header.Set(HeaderTimestamp, strconv.FormatInt(time.Now().Unix(), 10))
		req.Header.Set(HeaderNonce, "nonce")
		req.Header.Set(HeaderSignature, "invalid-signature")

		w := httptest.NewRecorder()
		wrapped.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("Expected 401, got %d", w.Code)
		}
	})
}

func TestSlugify(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Test Host", "test-host"},
		{"My-Host-Name", "my-host-name"},
		{"host123", "host123"},
		{"Host With   Spaces", "host-with---spaces"},
		{"Special!@#$Characters", "specialcharacters"},
		{"UPPERCASE", "uppercase"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result := slugify(tc.input)
			if result != tc.expected {
				t.Errorf("slugify(%q) = %q, want %q", tc.input, result, tc.expected)
			}
		})
	}
}
