package hubclient

import (
	"context"
	"fmt"
	"net/url"

	"github.com/ptone/scion-agent/pkg/apiclient"
)

// GroveService handles grove operations.
type GroveService interface {
	// List returns groves matching the filter criteria.
	List(ctx context.Context, opts *ListGrovesOptions) (*ListGrovesResponse, error)

	// Get returns a single grove by ID.
	Get(ctx context.Context, groveID string) (*Grove, error)

	// Register registers a grove (upsert based on git remote).
	Register(ctx context.Context, req *RegisterGroveRequest) (*RegisterGroveResponse, error)

	// Create creates a grove without a contributing host.
	Create(ctx context.Context, req *CreateGroveRequest) (*Grove, error)

	// Update updates grove metadata.
	Update(ctx context.Context, groveID string, req *UpdateGroveRequest) (*Grove, error)

	// Delete removes a grove.
	Delete(ctx context.Context, groveID string, deleteAgents bool) error

	// ListAgents returns agents in a grove.
	ListAgents(ctx context.Context, groveID string, opts *ListAgentsOptions) (*ListAgentsResponse, error)

	// ListContributors returns runtime hosts contributing to a grove.
	ListContributors(ctx context.Context, groveID string) (*ListContributorsResponse, error)

	// AddContributor adds a host as a contributor to a grove.
	AddContributor(ctx context.Context, groveID string, req *AddContributorRequest) (*AddContributorResponse, error)

	// RemoveContributor removes a host from a grove.
	RemoveContributor(ctx context.Context, groveID, brokerID string) error

	// GetAgent returns an agent by ID or slug within a grove.
	GetAgent(ctx context.Context, groveID, agentID string) (*Agent, error)

	// DeleteAgent removes an agent by ID or slug within a grove.
	DeleteAgent(ctx context.Context, groveID, agentID string, opts *DeleteAgentOptions) error

	// GetSettings retrieves grove settings.
	GetSettings(ctx context.Context, groveID string) (*GroveSettings, error)

	// UpdateSettings updates grove settings.
	UpdateSettings(ctx context.Context, groveID string, settings *GroveSettings) (*GroveSettings, error)
}

// groveService is the implementation of GroveService.
type groveService struct {
	c *client
}

// ListGrovesOptions configures grove list filtering.
type ListGrovesOptions struct {
	Visibility string // Filter by visibility
	GitRemote  string // Filter by git remote (exact or prefix)
	BrokerID string // Filter by contributing host
	Name       string // Filter by exact name (case-insensitive)
	Labels     map[string]string
	Page       apiclient.PageOptions
}

// ListGrovesResponse is the response from listing groves.
type ListGrovesResponse struct {
	Groves []Grove
	Page   apiclient.PageResult
}

// RegisterGroveRequest is the request for registering a grove.
type RegisterGroveRequest struct {
	ID        string            `json:"id,omitempty"`      // Client-provided grove ID (from grove_id setting)
	Name      string            `json:"name"`
	GitRemote string            `json:"gitRemote"`
	Path      string            `json:"path,omitempty"`
	BrokerID string            `json:"brokerId,omitempty"`  // Link to existing host (two-phase flow)
	Host      *HostInfo         `json:"host,omitempty"`    // DEPRECATED: Use HostID with two-phase registration
	Profiles  []string          `json:"profiles,omitempty"`
	Mode      string            `json:"mode,omitempty"`    // connected, read-only
	Labels    map[string]string `json:"labels,omitempty"`
}

// HostInfo describes the registering host.
type HostInfo struct {
	ID           string            `json:"id,omitempty"`
	Name         string            `json:"name"`
	Version      string            `json:"version"`
	Capabilities *BrokerCapabilities `json:"capabilities,omitempty"`
	Profiles     []BrokerProfile     `json:"profiles,omitempty"`
}

// RegisterGroveResponse is the response from registering a grove.
type RegisterGroveResponse struct {
	Grove     *Grove       `json:"grove"`
	Host      *RuntimeBroker `json:"host,omitempty"` // Populated if hostId or host provided
	Created   bool         `json:"created"`        // True if grove was newly created
	BrokerToken string       `json:"brokerToken,omitempty"` // DEPRECATED: use two-phase registration
	SecretKey string       `json:"secretKey,omitempty"` // DEPRECATED: secrets only from /hosts/join
}

// CreateGroveRequest is the request for creating a grove without a host.
type CreateGroveRequest struct {
	Name       string            `json:"name"`
	GitRemote  string            `json:"gitRemote,omitempty"`
	Visibility string            `json:"visibility,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`
}

// UpdateGroveRequest is the request for updating a grove.
type UpdateGroveRequest struct {
	Name        string            `json:"name,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	Visibility  string            `json:"visibility,omitempty"`
}

// ListContributorsResponse is the response from listing grove contributors.
type ListContributorsResponse struct {
	Contributors []GroveContributor `json:"contributors"`
}

// AddContributorRequest is the request for adding a host as a grove contributor.
type AddContributorRequest struct {
	BrokerID string `json:"brokerId"`
	LocalPath string `json:"localPath,omitempty"`
	Mode      string `json:"mode,omitempty"` // "connected" or "read-only"
}

// AddContributorResponse is the response after adding a contributor.
type AddContributorResponse struct {
	Contributor *GroveContributor `json:"contributor"`
}

// List returns groves matching the filter criteria.
func (s *groveService) List(ctx context.Context, opts *ListGrovesOptions) (*ListGrovesResponse, error) {
	query := url.Values{}
	if opts != nil {
		if opts.Visibility != "" {
			query.Set("visibility", opts.Visibility)
		}
		if opts.GitRemote != "" {
			query.Set("gitRemote", opts.GitRemote)
		}
		if opts.BrokerID != "" {
			query.Set("brokerId", opts.BrokerID)
		}
		if opts.Name != "" {
			query.Set("name", opts.Name)
		}
		for k, v := range opts.Labels {
			query.Add("label", fmt.Sprintf("%s=%s", k, v))
		}
		opts.Page.ToQuery(query)
	}

	resp, err := s.c.transport.GetWithQuery(ctx, "/api/v1/groves", query, nil)
	if err != nil {
		return nil, err
	}

	type listResponse struct {
		Groves     []Grove `json:"groves"`
		NextCursor string  `json:"nextCursor,omitempty"`
		TotalCount int     `json:"totalCount,omitempty"`
	}

	result, err := apiclient.DecodeResponse[listResponse](resp)
	if err != nil {
		return nil, err
	}

	return &ListGrovesResponse{
		Groves: result.Groves,
		Page: apiclient.PageResult{
			NextCursor: result.NextCursor,
			TotalCount: result.TotalCount,
		},
	}, nil
}

// Get returns a single grove by ID.
func (s *groveService) Get(ctx context.Context, groveID string) (*Grove, error) {
	resp, err := s.c.transport.Get(ctx, "/api/v1/groves/"+groveID, nil)
	if err != nil {
		return nil, err
	}
	return apiclient.DecodeResponse[Grove](resp)
}

// Register registers a grove (upsert based on git remote).
func (s *groveService) Register(ctx context.Context, req *RegisterGroveRequest) (*RegisterGroveResponse, error) {
	resp, err := s.c.transport.Post(ctx, "/api/v1/groves/register", req, nil)
	if err != nil {
		return nil, err
	}
	return apiclient.DecodeResponse[RegisterGroveResponse](resp)
}

// Create creates a grove without a contributing host.
func (s *groveService) Create(ctx context.Context, req *CreateGroveRequest) (*Grove, error) {
	resp, err := s.c.transport.Post(ctx, "/api/v1/groves", req, nil)
	if err != nil {
		return nil, err
	}
	return apiclient.DecodeResponse[Grove](resp)
}

// Update updates grove metadata.
func (s *groveService) Update(ctx context.Context, groveID string, req *UpdateGroveRequest) (*Grove, error) {
	resp, err := s.c.transport.Patch(ctx, "/api/v1/groves/"+groveID, req, nil)
	if err != nil {
		return nil, err
	}
	return apiclient.DecodeResponse[Grove](resp)
}

// Delete removes a grove.
func (s *groveService) Delete(ctx context.Context, groveID string, deleteAgents bool) error {
	path := "/api/v1/groves/" + groveID
	if deleteAgents {
		path += "?deleteAgents=true"
	}
	resp, err := s.c.transport.Delete(ctx, path, nil)
	if err != nil {
		return err
	}
	return apiclient.CheckResponse(resp)
}

// ListAgents returns agents in a grove.
func (s *groveService) ListAgents(ctx context.Context, groveID string, opts *ListAgentsOptions) (*ListAgentsResponse, error) {
	query := url.Values{}
	if opts != nil {
		if opts.Status != "" {
			query.Set("status", opts.Status)
		}
		if opts.RuntimeBrokerID != "" {
			query.Set("runtimeBrokerId", opts.RuntimeBrokerID)
		}
		for k, v := range opts.Labels {
			query.Add("label", fmt.Sprintf("%s=%s", k, v))
		}
		opts.Page.ToQuery(query)
	}

	resp, err := s.c.transport.GetWithQuery(ctx, "/api/v1/groves/"+groveID+"/agents", query, nil)
	if err != nil {
		return nil, err
	}

	type listResponse struct {
		Agents     []Agent `json:"agents"`
		NextCursor string  `json:"nextCursor,omitempty"`
		TotalCount int     `json:"totalCount,omitempty"`
	}

	result, err := apiclient.DecodeResponse[listResponse](resp)
	if err != nil {
		return nil, err
	}

	return &ListAgentsResponse{
		Agents: result.Agents,
		Page: apiclient.PageResult{
			NextCursor: result.NextCursor,
			TotalCount: result.TotalCount,
		},
	}, nil
}

// ListContributors returns runtime hosts contributing to a grove.
func (s *groveService) ListContributors(ctx context.Context, groveID string) (*ListContributorsResponse, error) {
	resp, err := s.c.transport.Get(ctx, "/api/v1/groves/"+groveID+"/contributors", nil)
	if err != nil {
		return nil, err
	}
	return apiclient.DecodeResponse[ListContributorsResponse](resp)
}

// AddContributor adds a host as a contributor to a grove.
func (s *groveService) AddContributor(ctx context.Context, groveID string, req *AddContributorRequest) (*AddContributorResponse, error) {
	resp, err := s.c.transport.Post(ctx, "/api/v1/groves/"+groveID+"/contributors", req, nil)
	if err != nil {
		return nil, err
	}
	return apiclient.DecodeResponse[AddContributorResponse](resp)
}

// RemoveContributor removes a host from a grove.
func (s *groveService) RemoveContributor(ctx context.Context, groveID, brokerID string) error {
	resp, err := s.c.transport.Delete(ctx, "/api/v1/groves/"+groveID+"/contributors/"+brokerID, nil)
	if err != nil {
		return err
	}
	return apiclient.CheckResponse(resp)
}

// GetSettings retrieves grove settings.
func (s *groveService) GetSettings(ctx context.Context, groveID string) (*GroveSettings, error) {
	resp, err := s.c.transport.Get(ctx, "/api/v1/groves/"+groveID+"/settings", nil)
	if err != nil {
		return nil, err
	}
	return apiclient.DecodeResponse[GroveSettings](resp)
}

// UpdateSettings updates grove settings.
func (s *groveService) UpdateSettings(ctx context.Context, groveID string, settings *GroveSettings) (*GroveSettings, error) {
	resp, err := s.c.transport.Put(ctx, "/api/v1/groves/"+groveID+"/settings", settings, nil)
	if err != nil {
		return nil, err
	}
	return apiclient.DecodeResponse[GroveSettings](resp)
}

// GetAgent returns an agent by ID or slug within a grove.
func (s *groveService) GetAgent(ctx context.Context, groveID, agentID string) (*Agent, error) {
	resp, err := s.c.transport.Get(ctx, "/api/v1/groves/"+groveID+"/agents/"+agentID, nil)
	if err != nil {
		return nil, err
	}
	return apiclient.DecodeResponse[Agent](resp)
}

// DeleteAgent removes an agent by ID or slug within a grove.
func (s *groveService) DeleteAgent(ctx context.Context, groveID, agentID string, opts *DeleteAgentOptions) error {
	path := "/api/v1/groves/" + groveID + "/agents/" + agentID
	if opts != nil {
		query := url.Values{}
		if opts.DeleteFiles {
			query.Set("deleteFiles", "true")
		}
		if opts.RemoveBranch {
			query.Set("removeBranch", "true")
		}
		if len(query) > 0 {
			path += "?" + query.Encode()
		}
	}

	resp, err := s.c.transport.Delete(ctx, path, nil)
	if err != nil {
		return err
	}
	return apiclient.CheckResponse(resp)
}
