package hubclient

import (
	"context"
	"net/url"

	"github.com/ptone/scion-agent/pkg/apiclient"
)

// RuntimeBrokerService handles runtime host operations.
type RuntimeBrokerService interface {
	// Create creates a new host registration and returns a join token.
	// The join token must be used with Join() to complete registration.
	Create(ctx context.Context, req *CreateBrokerRequest) (*CreateBrokerResponse, error)

	// Join completes host registration using a join token.
	// Returns the HMAC secret key for future authentication.
	Join(ctx context.Context, req *JoinBrokerRequest) (*JoinBrokerResponse, error)

	// List returns runtime hosts matching the filter criteria.
	List(ctx context.Context, opts *ListBrokersOptions) (*ListBrokersResponse, error)

	// Get returns a single runtime host by ID.
	Get(ctx context.Context, brokerID string) (*RuntimeBroker, error)

	// Update updates host metadata.
	Update(ctx context.Context, brokerID string, req *UpdateBrokerRequest) (*RuntimeBroker, error)

	// Delete removes a host from all groves.
	Delete(ctx context.Context, brokerID string) error

	// ListGroves returns groves this host contributes to.
	ListGroves(ctx context.Context, brokerID string) (*ListBrokerGrovesResponse, error)

	// Heartbeat sends a heartbeat for a host.
	Heartbeat(ctx context.Context, brokerID string, status *BrokerHeartbeat) error
}

// runtimeBrokerService is the implementation of RuntimeBrokerService.
type runtimeBrokerService struct {
	c *client
}

// ListBrokersOptions configures runtime host list filtering.
type ListBrokersOptions struct {
	Status  string // Filter by status (online, offline)
	Mode    string // Filter by mode (connected, read-only)
	GroveID string // Filter by grove contribution
	Page    apiclient.PageOptions
}

// ListBrokersResponse is the response from listing runtime hosts.
type ListBrokersResponse struct {
	Hosts []RuntimeBroker
	Page  apiclient.PageResult
}

// UpdateBrokerRequest is the request for updating a runtime host.
type UpdateBrokerRequest struct {
	Name        string            `json:"name,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// ListBrokerGrovesResponse is the response from listing host groves.
type ListBrokerGrovesResponse struct {
	Groves []BrokerGroveInfo `json:"groves"`
}

// BrokerHeartbeat is the heartbeat payload.
type BrokerHeartbeat struct {
	Status string           `json:"status"`
	Groves []GroveHeartbeat `json:"groves,omitempty"`
}

// GroveHeartbeat is per-grove status in a heartbeat.
type GroveHeartbeat struct {
	GroveID    string           `json:"groveId"`
	AgentCount int              `json:"agentCount"`
	Agents     []AgentHeartbeat `json:"agents,omitempty"`
}

// AgentHeartbeat is per-agent status in a heartbeat.
type AgentHeartbeat struct {
	AgentID         string `json:"agentId"`
	Status          string `json:"status"`
	ContainerStatus string `json:"containerStatus,omitempty"`
}

// CreateBrokerRequest is the request to create a new host registration.
type CreateBrokerRequest struct {
	Name         string            `json:"name"`
	Capabilities []string          `json:"capabilities,omitempty"`
	Labels       map[string]string `json:"labels,omitempty"`
}

// CreateBrokerResponse is returned when creating a new host.
type CreateBrokerResponse struct {
	BrokerID string `json:"brokerId"`
	JoinToken string `json:"joinToken"`
	ExpiresAt string `json:"expiresAt"`
}

// JoinBrokerRequest is the request to complete host registration.
type JoinBrokerRequest struct {
	BrokerID string   `json:"brokerId"`
	JoinToken    string   `json:"joinToken"`
	Hostname     string   `json:"hostname"`
	Version      string   `json:"version"`
	Capabilities []string `json:"capabilities,omitempty"`
}

// JoinBrokerResponse is returned after completing host registration.
type JoinBrokerResponse struct {
	SecretKey   string `json:"secretKey"` // Base64-encoded HMAC secret
	HubEndpoint string `json:"hubEndpoint"`
	BrokerID string `json:"brokerId"`
}

// Create creates a new host registration and returns a join token.
func (s *runtimeBrokerService) Create(ctx context.Context, req *CreateBrokerRequest) (*CreateBrokerResponse, error) {
	resp, err := s.c.transport.Post(ctx, "/api/v1/hosts", req, nil)
	if err != nil {
		return nil, err
	}
	return apiclient.DecodeResponse[CreateBrokerResponse](resp)
}

// Join completes host registration using a join token.
func (s *runtimeBrokerService) Join(ctx context.Context, req *JoinBrokerRequest) (*JoinBrokerResponse, error) {
	resp, err := s.c.transport.Post(ctx, "/api/v1/hosts/join", req, nil)
	if err != nil {
		return nil, err
	}
	return apiclient.DecodeResponse[JoinBrokerResponse](resp)
}

// List returns runtime hosts matching the filter criteria.
func (s *runtimeBrokerService) List(ctx context.Context, opts *ListBrokersOptions) (*ListBrokersResponse, error) {
	query := url.Values{}
	if opts != nil {
		if opts.Status != "" {
			query.Set("status", opts.Status)
		}
		if opts.Mode != "" {
			query.Set("mode", opts.Mode)
		}
		if opts.GroveID != "" {
			query.Set("groveId", opts.GroveID)
		}
		opts.Page.ToQuery(query)
	}

	resp, err := s.c.transport.GetWithQuery(ctx, "/api/v1/runtime-hosts", query, nil)
	if err != nil {
		return nil, err
	}

	type listResponse struct {
		Hosts      []RuntimeBroker `json:"hosts"`
		NextCursor string        `json:"nextCursor,omitempty"`
		TotalCount int           `json:"totalCount,omitempty"`
	}

	result, err := apiclient.DecodeResponse[listResponse](resp)
	if err != nil {
		return nil, err
	}

	return &ListBrokersResponse{
		Hosts: result.Hosts,
		Page: apiclient.PageResult{
			NextCursor: result.NextCursor,
			TotalCount: result.TotalCount,
		},
	}, nil
}

// Get returns a single runtime host by ID.
func (s *runtimeBrokerService) Get(ctx context.Context, brokerID string) (*RuntimeBroker, error) {
	resp, err := s.c.transport.Get(ctx, "/api/v1/runtime-hosts/"+brokerID, nil)
	if err != nil {
		return nil, err
	}
	return apiclient.DecodeResponse[RuntimeBroker](resp)
}

// Update updates host metadata.
func (s *runtimeBrokerService) Update(ctx context.Context, brokerID string, req *UpdateBrokerRequest) (*RuntimeBroker, error) {
	resp, err := s.c.transport.Patch(ctx, "/api/v1/runtime-hosts/"+brokerID, req, nil)
	if err != nil {
		return nil, err
	}
	return apiclient.DecodeResponse[RuntimeBroker](resp)
}

// Delete removes a host from all groves.
func (s *runtimeBrokerService) Delete(ctx context.Context, brokerID string) error {
	resp, err := s.c.transport.Delete(ctx, "/api/v1/runtime-hosts/"+brokerID, nil)
	if err != nil {
		return err
	}
	return apiclient.CheckResponse(resp)
}

// ListGroves returns groves this host contributes to.
func (s *runtimeBrokerService) ListGroves(ctx context.Context, brokerID string) (*ListBrokerGrovesResponse, error) {
	resp, err := s.c.transport.Get(ctx, "/api/v1/runtime-hosts/"+brokerID+"/groves", nil)
	if err != nil {
		return nil, err
	}
	return apiclient.DecodeResponse[ListBrokerGrovesResponse](resp)
}

// Heartbeat sends a heartbeat for a host.
func (s *runtimeBrokerService) Heartbeat(ctx context.Context, brokerID string, status *BrokerHeartbeat) error {
	resp, err := s.c.transport.Post(ctx, "/api/v1/runtime-hosts/"+brokerID+"/heartbeat", status, nil)
	if err != nil {
		return err
	}
	return apiclient.CheckResponse(resp)
}
