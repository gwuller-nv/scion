// Package brokercredentials manages Runtime Broker credentials for Hub authentication.
// This package is separate from pkg/credentials which handles CLI user credentials.
package brokercredentials

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	// DefaultFileName is the default name of the host credentials file.
	DefaultFileName = "broker-credentials.json"
	// FileMode is the file permissions for the credentials file (owner read/write only).
	FileMode = 0600
	// DirMode is the directory permissions (owner rwx only).
	DirMode = 0700
)

var (
	// ErrNotFound is returned when no credentials are found.
	ErrNotFound = errors.New("host credentials not found")
	// ErrInvalidCredentials is returned when credentials are malformed.
	ErrInvalidCredentials = errors.New("invalid host credentials")
)

// BrokerCredentials contains the credentials for a Runtime Host.
type BrokerCredentials struct {
	// HostID is the unique identifier for this host.
	BrokerID string `json:"brokerId"`
	// SecretKey is the base64-encoded shared secret for HMAC authentication.
	SecretKey string `json:"secretKey"`
	// HubEndpoint is the URL of the Hub API.
	HubEndpoint string `json:"hubEndpoint"`
	// RegisteredAt is when this host was registered with the Hub.
	RegisteredAt time.Time `json:"registeredAt"`
}

// Store manages host credentials on the local filesystem.
type Store struct {
	path string
	mu   sync.RWMutex
}

// NewStore creates a new credential store at the given path.
// If path is empty, DefaultPath() is used.
func NewStore(path string) *Store {
	if path == "" {
		path = DefaultPath()
	}
	return &Store{path: path}
}

// DefaultPath returns the default path to the host credentials file.
// This is ~/.scion/broker-credentials.json
func DefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		// Fallback to current directory if home is not available
		return DefaultFileName
	}
	return filepath.Join(home, ".scion", DefaultFileName)
}

// Path returns the path to the credentials file.
func (s *Store) Path() string {
	return s.path
}

// Exists checks if the credentials file exists.
func (s *Store) Exists() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	_, err := os.Stat(s.path)
	return err == nil
}

// Load reads and parses the credentials file.
func (s *Store) Load() (*BrokerCredentials, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to read credentials file: %w", err)
	}

	var creds BrokerCredentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidCredentials, err)
	}

	// Validate required fields
	if creds.BrokerID == "" {
		return nil, fmt.Errorf("%w: missing hostId", ErrInvalidCredentials)
	}
	if creds.SecretKey == "" {
		return nil, fmt.Errorf("%w: missing secretKey", ErrInvalidCredentials)
	}

	return &creds, nil
}

// Save writes credentials to the file with proper permissions.
func (s *Store) Save(creds *BrokerCredentials) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if creds == nil {
		return errors.New("credentials cannot be nil")
	}
	if creds.BrokerID == "" {
		return errors.New("hostId is required")
	}
	if creds.SecretKey == "" {
		return errors.New("secretKey is required")
	}

	// Ensure parent directory exists with restricted permissions
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, DirMode); err != nil {
		return fmt.Errorf("failed to create credentials directory: %w", err)
	}

	// Marshal with pretty printing for readability
	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal credentials: %w", err)
	}

	// Write with restricted permissions (owner read/write only)
	if err := os.WriteFile(s.path, data, FileMode); err != nil {
		return fmt.Errorf("failed to write credentials file: %w", err)
	}

	return nil
}

// Delete removes the credentials file.
func (s *Store) Delete() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	err := os.Remove(s.path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete credentials file: %w", err)
	}
	return nil
}

// GetSecretKey loads the credentials and decodes the secret key.
// Returns the decoded secret key bytes.
func (s *Store) GetSecretKey() ([]byte, error) {
	creds, err := s.Load()
	if err != nil {
		return nil, err
	}

	secretKey, err := base64.StdEncoding.DecodeString(creds.SecretKey)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to decode secretKey: %v", ErrInvalidCredentials, err)
	}

	return secretKey, nil
}

// SaveFromJoinResponse creates and saves credentials from a join response.
// This is a convenience method for the common use case of saving credentials
// immediately after completing a host join.
func (s *Store) SaveFromJoinResponse(brokerID, secretKey, hubEndpoint string) error {
	creds := &BrokerCredentials{
		BrokerID:     brokerID,
		SecretKey:    secretKey,
		HubEndpoint:  hubEndpoint,
		RegisteredAt: time.Now(),
	}
	return s.Save(creds)
}

// ModTime returns the modification time of the credentials file.
// Returns zero time if the file doesn't exist or can't be stat'd.
func (s *Store) ModTime() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()

	info, err := os.Stat(s.path)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

// LoadIfChanged loads credentials if the file has been modified since lastModTime.
// Returns the new credentials and mod time if changed, or nil if unchanged.
func (s *Store) LoadIfChanged(lastModTime time.Time) (*BrokerCredentials, time.Time, error) {
	currentModTime := s.ModTime()
	if currentModTime.IsZero() {
		return nil, time.Time{}, nil // File doesn't exist
	}

	// Check if file has been modified (including if lastModTime was zero)
	if !lastModTime.IsZero() && !currentModTime.After(lastModTime) {
		return nil, lastModTime, nil // No change
	}

	creds, err := s.Load()
	if err != nil {
		return nil, time.Time{}, err
	}

	return creds, currentModTime, nil
}
