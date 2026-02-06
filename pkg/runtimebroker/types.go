package runtimebroker

import (
	"strings"
	"time"

	"github.com/ptone/scion-agent/pkg/api"
)

// ============================================================================
// Health & Info Types
// ============================================================================

// HealthResponse is the response for health check endpoints.
type HealthResponse struct {
	Status  string            `json:"status"`
	Version string            `json:"version"`
	Mode    string            `json:"mode,omitempty"`
	Uptime  string            `json:"uptime"`
	Checks  map[string]string `json:"checks,omitempty"`
}

// BrokerInfoResponse is the response for the /api/v1/info endpoint.
type BrokerInfoResponse struct {
	BrokerID string            `json:"brokerId"`
	Name         string            `json:"name,omitempty"`
	Version      string            `json:"version"`
	Mode         string            `json:"mode"`
	Capabilities *BrokerCapabilities `json:"capabilities,omitempty"`
	Profiles     []BrokerProfile     `json:"profiles,omitempty"`
	Groves       []GroveInfo       `json:"groves,omitempty"`
}

// BrokerProfile describes a runtime profile available on a host.
type BrokerProfile struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	Available bool   `json:"available"`
	Context   string `json:"context,omitempty"`
	Namespace string `json:"namespace,omitempty"`
}

// BrokerCapabilities describes what this runtime host can do.
type BrokerCapabilities struct {
	WebPTY bool `json:"webPty"`
	Sync   bool `json:"sync"`
	Attach bool `json:"attach"`
	Exec   bool `json:"exec"`
}

// GroveInfo is a summary of a grove registered on this host.
type GroveInfo struct {
	GroveID    string `json:"groveId"`
	GroveName  string `json:"groveName"`
	GitRemote  string `json:"gitRemote,omitempty"`
	AgentCount int    `json:"agentCount"`
}

// ============================================================================
// Agent Types
// ============================================================================

// Agent status values matching the API specification.
const (
	AgentStatusPending      = "pending"
	AgentStatusProvisioning = "provisioning"
	AgentStatusStarting     = "starting"
	AgentStatusRunning      = "running"
	AgentStatusStopping     = "stopping"
	AgentStatusStopped      = "stopped"
	AgentStatusError        = "error"
)

// AgentResponse represents an agent in API responses.
type AgentResponse struct {
	ID              string            `json:"id,omitempty"`
	AgentID         string            `json:"agentId"`
	Name            string            `json:"name"`
	GroveID         string            `json:"groveId,omitempty"`
	UserID          string            `json:"userId,omitempty"`
	Status          string            `json:"status"`
	StatusReason    string            `json:"statusReason,omitempty"`
	Ready           bool              `json:"ready,omitempty"`
	ContainerStatus string            `json:"containerStatus,omitempty"`
	Config          *AgentConfig      `json:"config,omitempty"`
	Runtime         *AgentRuntime     `json:"runtime,omitempty"`
	Labels          map[string]string `json:"labels,omitempty"`
	CreatedAt       time.Time         `json:"createdAt,omitempty"`
	UpdatedAt       time.Time         `json:"updatedAt,omitempty"`
}

// AgentConfig contains agent configuration details.
type AgentConfig struct {
	Template  string                 `json:"template,omitempty"`
	Image     string                 `json:"image,omitempty"`
	HomeDir   string                 `json:"homeDir,omitempty"`
	Workspace string                 `json:"workspace,omitempty"`
	RepoRoot  string                 `json:"repoRoot,omitempty"`
	Harness   string                 `json:"harness,omitempty"`
	UseTmux   bool                   `json:"useTmux,omitempty"`
	Env       []string               `json:"env,omitempty"`
	Volumes   []api.VolumeMount      `json:"volumes,omitempty"`
	Resources *api.K8sResources      `json:"resources,omitempty"`
	K8s       *api.KubernetesConfig  `json:"kubernetes,omitempty"`
}

// AgentRuntime contains runtime information about the agent.
type AgentRuntime struct {
	ContainerID string    `json:"containerId,omitempty"`
	Node        string    `json:"node,omitempty"`
	StartedAt   time.Time `json:"startedAt,omitempty"`
	IPAddress   string    `json:"ipAddress,omitempty"`
}

// ListAgentsResponse is the response for listing agents.
type ListAgentsResponse struct {
	Agents     []AgentResponse `json:"agents"`
	NextCursor string          `json:"nextCursor,omitempty"`
	TotalCount int             `json:"totalCount"`
}

// CreateAgentRequest is the request body for creating an agent.
type CreateAgentRequest struct {
	RequestID   string             `json:"requestId,omitempty"`
	AgentID     string             `json:"agentId,omitempty"`
	Name        string             `json:"name"`
	GroveID     string             `json:"groveId,omitempty"`
	UserID      string             `json:"userId,omitempty"`
	Config      *CreateAgentConfig `json:"config,omitempty"`
	HubEndpoint string             `json:"hubEndpoint,omitempty"`
	AgentToken  string             `json:"agentToken,omitempty"`

	// ResolvedEnv contains the fully merged environment variables and secrets
	// from all applicable scopes (user, grove, runtime host). These are resolved
	// by the Hub before dispatching the agent creation request.
	// The Runtime Host should merge these with config.Env, with config.Env
	// taking precedence over ResolvedEnv.
	ResolvedEnv map[string]string `json:"resolvedEnv,omitempty"`

	// GrovePath is the local filesystem path to the grove on this runtime host.
	// This is provided by the Hub from the grove contributor record.
	GrovePath string `json:"grovePath,omitempty"`
}

// CreateAgentConfig contains configuration for agent creation.
type CreateAgentConfig struct {
	Template    string                `json:"template,omitempty"`
	Image       string                `json:"image,omitempty"`
	HomeDir     string                `json:"homeDir,omitempty"`
	Workspace   string                `json:"workspace,omitempty"`
	RepoRoot    string                `json:"repoRoot,omitempty"`
	Env         []string              `json:"env,omitempty"`
	Volumes     []api.VolumeMount     `json:"volumes,omitempty"`
	Labels      map[string]string     `json:"labels,omitempty"`
	Annotations map[string]string     `json:"annotations,omitempty"`
	Harness     string                `json:"harness,omitempty"`
	UseTmux     bool                  `json:"useTmux,omitempty"`
	Task        string                `json:"task,omitempty"`
	CommandArgs []string              `json:"commandArgs,omitempty"`
	Kubernetes  *api.KubernetesConfig `json:"kubernetes,omitempty"`

	// TemplateID is the Hub template ID for cache lookup.
	// When provided, the Runtime Host can use this to look up or fetch
	// the template from the Hub and cache it locally.
	TemplateID string `json:"templateId,omitempty"`

	// TemplateHash is the content hash of the template for cache validation.
	// If the cached template's hash matches, it can be used without re-downloading.
	TemplateHash string `json:"templateHash,omitempty"`
}

// CreateAgentResponse is the response for creating an agent.
type CreateAgentResponse struct {
	Agent   *AgentResponse `json:"agent"`
	Created bool           `json:"created"`
}

// ============================================================================
// Interaction Types
// ============================================================================

// MessageRequest is the request body for sending a message to an agent.
type MessageRequest struct {
	Message   string `json:"message"`
	Interrupt bool   `json:"interrupt,omitempty"`
}

// ExecRequest is the request body for executing a command in an agent.
type ExecRequest struct {
	Command []string `json:"command"`
	Timeout int      `json:"timeout,omitempty"` // Timeout in seconds
}

// ExecResponse is the response for command execution.
type ExecResponse struct {
	Output   string `json:"output"`
	ExitCode int    `json:"exitCode"`
}

// StatsResponse contains resource usage statistics for an agent.
type StatsResponse struct {
	CPUUsagePercent    float64 `json:"cpuUsagePercent"`
	MemoryUsageBytes   int64   `json:"memoryUsageBytes"`
	MemoryLimitBytes   int64   `json:"memoryLimitBytes,omitempty"`
	NetworkRxBytes     int64   `json:"networkRxBytes,omitempty"`
	NetworkTxBytes     int64   `json:"networkTxBytes,omitempty"`
}

// ============================================================================
// Conversion Functions
// ============================================================================

// AgentInfoToResponse converts an api.AgentInfo to an AgentResponse.
func AgentInfoToResponse(info api.AgentInfo) AgentResponse {
	status := info.Status
	if status == "" {
		// Map container status to agent status
		switch {
		case info.ContainerStatus == "":
			status = AgentStatusPending
		case containsAny(info.ContainerStatus, "up", "running"):
			status = AgentStatusRunning
		case containsAny(info.ContainerStatus, "created"):
			status = AgentStatusProvisioning
		case containsAny(info.ContainerStatus, "exited", "stopped"):
			status = AgentStatusStopped
		default:
			status = info.ContainerStatus
		}
	}

	resp := AgentResponse{
		ID:              info.ID,
		AgentID:         info.AgentID,
		Name:            info.Name,
		GroveID:         info.GroveID,
		Status:          status,
		ContainerStatus: info.ContainerStatus,
		Labels:          info.Labels,
		CreatedAt:       info.Created,
		Ready:           status == AgentStatusRunning,
	}

	if info.Template != "" || info.Image != "" {
		resp.Config = &AgentConfig{
			Template: info.Template,
			Image:    info.Image,
		}
	}

	if info.ID != "" {
		resp.Runtime = &AgentRuntime{
			ContainerID: info.ID,
		}
	}

	return resp
}

// containsAny checks if s contains any of the substrings (case-insensitive).
func containsAny(s string, substrs ...string) bool {
	s = strings.ToLower(s)
	for _, sub := range substrs {
		if strings.Contains(s, strings.ToLower(sub)) {
			return true
		}
	}
	return false
}
