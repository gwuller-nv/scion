package hubsync

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ptone/scion-agent/pkg/apiclient"
	"github.com/ptone/scion-agent/pkg/config"
	"github.com/ptone/scion-agent/pkg/credentials"
	"github.com/ptone/scion-agent/pkg/hubclient"
	"github.com/ptone/scion-agent/pkg/util"
)

// debugf prints a debug message if debug mode is enabled.
func debugf(format string, args ...interface{}) {
	util.DebugfTagged("hubsync", format, args...)
}

// AgentRef holds both name and ID for an agent.
// Name is used for display, ID is used for API calls.
type AgentRef struct {
	Name string
	ID   string
}

// SyncResult represents the result of comparing local and Hub agents.
type SyncResult struct {
	ToRegister []string   // Local agents to register on Hub
	ToRemove   []AgentRef // Hub agents (for this host) to remove (with IDs for API)
	InSync     []string   // Agents already in sync
	Pending    []AgentRef // Hub agents in pending status (not yet started, no local artifacts expected)
}

// IsInSync returns true if there are no agents to sync.
func (r *SyncResult) IsInSync() bool {
	return len(r.ToRegister) == 0 && len(r.ToRemove) == 0
}

// ExcludeAgent returns a new SyncResult with the specified agent excluded from
// ToRegister, ToRemove, and Pending lists. This is used when operating on a specific agent
// so that the sync check doesn't require syncing the target of the operation.
func (r *SyncResult) ExcludeAgent(agentName string) *SyncResult {
	if agentName == "" {
		return r
	}

	result := &SyncResult{
		InSync: r.InSync,
	}

	for _, name := range r.ToRegister {
		if name != agentName {
			result.ToRegister = append(result.ToRegister, name)
		}
	}

	for _, ref := range r.ToRemove {
		if ref.Name != agentName {
			result.ToRemove = append(result.ToRemove, ref)
		}
	}

	for _, ref := range r.Pending {
		if ref.Name != agentName {
			result.Pending = append(result.Pending, ref)
		}
	}

	return result
}

// HubContext holds the context for Hub operations.
type HubContext struct {
	Client     hubclient.Client
	Endpoint   string
	Settings   *config.Settings
	GroveID    string
	HostID     string
	GrovePath  string
	IsGlobal   bool
}

// EnsureHubReadyOptions configures the behavior of EnsureHubReady.
type EnsureHubReadyOptions struct {
	// AutoConfirm auto-confirms all prompts.
	AutoConfirm bool
	// NoHub disables Hub integration for this invocation.
	NoHub bool
	// SkipSync skips agent synchronization check.
	SkipSync bool
	// TargetAgent is the agent being operated on. If set, this agent is excluded
	// from sync requirements since the current operation will change its state.
	// For delete: the agent won't be required to be registered on Hub first.
	// For create: the agent won't be required to be removed from Hub first.
	TargetAgent string
}

// EnsureHubReady performs all Hub pre-flight checks before agent operations.
// Returns HubContext if Hub is ready, nil if Hub should not be used.
// This function will:
// 1. Check --no-hub flag
// 2. Load settings
// 3. Check hub.local_only setting
// 4. Check hub.enabled setting
// 5. Ensure grove_id exists (generate if missing)
// 6. Check Hub connectivity
// 7. Check grove registration (prompt to register if not)
// 8. Compare and sync agents (unless SkipSync is true)
func EnsureHubReady(grovePath string, opts EnsureHubReadyOptions) (*HubContext, error) {
	debugf("EnsureHubReady: grovePath=%s, opts=%+v", grovePath, opts)

	// Check if --no-hub flag is set
	if opts.NoHub {
		debugf("NoHub flag set, returning nil")
		return nil, nil
	}

	// Resolve grove path
	resolvedPath, isGlobal, err := config.ResolveGrovePath(grovePath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve grove path: %w", err)
	}

	// If no explicit grove path was given and we fell back to global,
	// that means no project grove was found. In this case, skip Hub sync
	// to avoid trying to register a non-existent grove. The user should
	// either run 'scion init' or use --global explicitly.
	if grovePath == "" && isGlobal {
		debugf("No project grove found (fell back to global), skipping Hub sync")
		return nil, nil
	}

	settings, err := config.LoadSettings(resolvedPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load settings: %w", err)
	}

	// Check if hub.local_only is set
	if settings.IsHubLocalOnly() {
		return nil, fmt.Errorf("this grove is configured for local-only mode (hub.local_only=true)\n\n" +
			"To perform this operation:\n" +
			"  - Use --no-hub flag to skip Hub integration\n" +
			"  - Or set hub.local_only=false to enable Hub sync checks")
	}

	// Check if hub is explicitly enabled
	if !settings.IsHubEnabled() {
		return nil, nil
	}

	// Hub is enabled - from here on, any failure is an error (no silent fallback)
	endpoint := getEndpoint(settings)
	if endpoint == "" {
		return nil, wrapHubError(fmt.Errorf("Hub is enabled but no endpoint configured.\n\nConfigure via: scion config set hub.endpoint <url>"))
	}

	// Ensure grove_id exists
	groveID := settings.GroveID
	if groveID == "" {
		// Generate grove_id for groves that don't have one
		groveID = config.GenerateGroveIDForDir(filepath.Dir(resolvedPath))
		if err := config.UpdateSetting(resolvedPath, "grove_id", groveID, isGlobal); err != nil {
			return nil, fmt.Errorf("failed to save grove_id: %w", err)
		}
		// Reload settings to get the updated grove_id
		settings, err = config.LoadSettings(resolvedPath)
		if err != nil {
			return nil, fmt.Errorf("failed to reload settings: %w", err)
		}
	}

	// Create Hub client
	client, err := createHubClient(settings, endpoint)
	if err != nil {
		return nil, wrapHubError(fmt.Errorf("failed to create Hub client: %w", err))
	}

	// Check health
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := client.Health(ctx); err != nil {
		return nil, wrapHubError(fmt.Errorf("Hub at %s is not responding: %w", endpoint, err))
	}

	// Get host ID
	hostID := ""
	if settings.Hub != nil {
		hostID = settings.Hub.HostID
	}

	hubCtx := &HubContext{
		Client:    client,
		Endpoint:  endpoint,
		Settings:  settings,
		GroveID:   groveID,
		HostID:    hostID,
		GrovePath: resolvedPath,
		IsGlobal:  isGlobal,
	}

	debugf("HubContext created: endpoint=%s, groveID=%s, hostID=%s, grovePath=%s, isGlobal=%v",
		endpoint, groveID, hostID, resolvedPath, isGlobal)

	// Check grove registration
	registered, err := isGroveRegistered(ctx, hubCtx)
	if err != nil {
		return nil, wrapHubError(err)
	}

	if !registered {
		// Get grove name for the prompt
		groveName := getGroveName(resolvedPath, isGlobal)

		// Check for existing groves with the same name
		matches, err := findMatchingGroves(ctx, hubCtx, groveName)
		if err != nil {
			debugf("Warning: failed to search for matching groves: %v", err)
			// Continue with registration - the hub will handle matching
		}

		if len(matches) > 0 {
			// Found matching groves - ask user what to do
			choice, selectedID := ShowMatchingGrovesPrompt(groveName, matches, opts.AutoConfirm)
			switch choice {
			case GroveChoiceCancel:
				return nil, fmt.Errorf("registration cancelled")
			case GroveChoiceLink:
				// Update local grove_id to the selected grove
				if err := config.UpdateSetting(resolvedPath, "grove_id", selectedID, isGlobal); err != nil {
					return nil, fmt.Errorf("failed to update local grove_id: %w", err)
				}
				hubCtx.GroveID = selectedID
				debugf("Updated local grove_id to: %s", selectedID)
			case GroveChoiceRegisterNew:
				// Generate a new grove ID to avoid linking to existing grove
				newID := config.GenerateGroveIDForDir(filepath.Dir(resolvedPath))
				if err := config.UpdateSetting(resolvedPath, "grove_id", newID, isGlobal); err != nil {
					return nil, fmt.Errorf("failed to update local grove_id: %w", err)
				}
				hubCtx.GroveID = newID
				debugf("Generated new grove_id: %s", newID)
			}
		} else {
			// No matching groves - ask for confirmation
			if !ShowRegistrationPrompt(groveName, opts.AutoConfirm) {
				return nil, fmt.Errorf("grove must be registered with Hub to perform this operation\n\n" +
					"Register the grove: scion hub register\n" +
					"Or use local-only mode: scion --no-hub <command>")
			}
		}

		// Register the grove
		if err := registerGrove(context.Background(), hubCtx, groveName, isGlobal); err != nil {
			return nil, wrapHubError(fmt.Errorf("failed to register grove: %w", err))
		}
		// Reload settings to get updated host ID and grove_id
		settings, err = config.LoadSettings(resolvedPath)
		if err != nil {
			return nil, fmt.Errorf("failed to reload settings: %w", err)
		}
		hubCtx.Settings = settings
		hubCtx.GroveID = settings.GroveID
		if settings.Hub != nil {
			hubCtx.HostID = settings.Hub.HostID
		}
	}

	// Skip sync if requested
	if opts.SkipSync {
		return hubCtx, nil
	}

	// Compare and sync agents
	syncResult, err := CompareAgents(context.Background(), hubCtx)
	if err != nil {
		return nil, wrapHubError(fmt.Errorf("failed to compare agents: %w", err))
	}

	// If we're operating on a specific agent, exclude it from sync requirements.
	// This allows operations like delete to proceed without first syncing the
	// target agent (e.g., you can delete a local-only agent without registering it).
	effectiveSyncResult := syncResult
	if opts.TargetAgent != "" {
		effectiveSyncResult = syncResult.ExcludeAgent(opts.TargetAgent)
	}

	if !effectiveSyncResult.IsInSync() {
		if ShowSyncPlan(effectiveSyncResult, opts.AutoConfirm) {
			if err := ExecuteSync(context.Background(), hubCtx, effectiveSyncResult); err != nil {
				return nil, wrapHubError(fmt.Errorf("failed to sync agents: %w", err))
			}
		} else {
			return nil, fmt.Errorf("agents must be synchronized with Hub to perform this operation\n\n" +
				"Sync agents: scion hub sync\n" +
				"Or use local-only mode: scion --no-hub <command>")
		}
	}

	return hubCtx, nil
}

// CompareAgents compares local agents with Hub agents for the current host.
func CompareAgents(ctx context.Context, hubCtx *HubContext) (*SyncResult, error) {
	result := &SyncResult{}

	debugf("CompareAgents starting: groveID=%s, hostID=%s, grovePath=%s",
		hubCtx.GroveID, hubCtx.HostID, hubCtx.GrovePath)

	// Get local agents
	localAgents, err := GetLocalAgents(hubCtx.GrovePath)
	if err != nil {
		return nil, fmt.Errorf("failed to get local agents: %w", err)
	}
	debugf("Local agents found: %v", localAgents)

	// Get Hub agents for this grove and host
	ctxTimeout, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	opts := &hubclient.ListAgentsOptions{
		GroveID:       hubCtx.GroveID,
		RuntimeHostID: hubCtx.HostID,
	}

	resp, err := hubCtx.Client.GroveAgents(hubCtx.GroveID).List(ctxTimeout, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to list Hub agents: %w", err)
	}

	debugf("Hub agents found: %d total", len(resp.Agents))
	for _, a := range resp.Agents {
		debugf("  - Hub agent: name=%s, id=%s, status=%s, hostID=%s",
			a.Name, a.ID, a.Status, a.RuntimeHostID)
	}

	// Build map of Hub agents
	hubAgentMap := make(map[string]bool)
	for _, a := range resp.Agents {
		hubAgentMap[a.Name] = true
	}

	// Build map of local agents
	localAgentMap := make(map[string]bool)
	for _, name := range localAgents {
		localAgentMap[name] = true
	}

	// Find agents to register (local but not on Hub)
	for _, name := range localAgents {
		if hubAgentMap[name] {
			result.InSync = append(result.InSync, name)
		} else {
			result.ToRegister = append(result.ToRegister, name)
		}
	}

	// Find agents to remove (on Hub for this host but not local)
	// Skip agents in "pending" status - these are created on Hub but not yet started,
	// so they're expected to not have local representation until the container is started.
	for _, a := range resp.Agents {
		if !localAgentMap[a.Name] {
			if a.Status == "pending" {
				// Track pending agents separately - they don't require sync
				result.Pending = append(result.Pending, AgentRef{Name: a.Name, ID: a.ID})
				debugf("Agent %s (id=%s) is pending, not requiring sync", a.Name, a.ID)
			} else {
				result.ToRemove = append(result.ToRemove, AgentRef{Name: a.Name, ID: a.ID})
				debugf("Agent %s (id=%s) on Hub but not local, marking for removal", a.Name, a.ID)
			}
		}
	}

	debugf("Sync result: toRegister=%v, toRemove=%d, pending=%d, inSync=%d",
		result.ToRegister, len(result.ToRemove), len(result.Pending), len(result.InSync))

	return result, nil
}

// ExecuteSync performs the synchronization based on SyncResult.
func ExecuteSync(ctx context.Context, hubCtx *HubContext, result *SyncResult) error {
	ctxTimeout, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	debugf("ExecuteSync starting: groveID=%s, hostID=%s", hubCtx.GroveID, hubCtx.HostID)

	// Register local agents on Hub
	// Note: We don't specify a runtime host ID - the hub will resolve it based on
	// available grove contributors (single contributor = auto-select, multiple = error)
	for _, name := range result.ToRegister {
		fmt.Printf("Registering agent '%s' on Hub...\n", name)
		debugf("Creating agent: name=%s, groveID=%s (hub will resolve runtime host)", name, hubCtx.GroveID)
		req := &hubclient.CreateAgentRequest{
			Name:    name,
			GroveID: hubCtx.GroveID,
		}
		resp, err := hubCtx.Client.GroveAgents(hubCtx.GroveID).Create(ctxTimeout, req)
		if err != nil {
			debugf("Failed to register agent '%s': %v", name, err)
			return fmt.Errorf("failed to register agent '%s': %w", name, err)
		}
		debugf("Agent '%s' created with ID: %s", name, resp.Agent.ID)
	}

	// Remove Hub agents that are not on this host
	for _, ref := range result.ToRemove {
		fmt.Printf("Removing agent '%s' from Hub...\n", ref.Name)
		debugf("Deleting agent via grove-scoped endpoint: name=%s, id=%s, groveID=%s",
			ref.Name, ref.ID, hubCtx.GroveID)
		// Use grove-scoped endpoint which supports both ID and slug lookup
		if err := hubCtx.Client.Groves().DeleteAgent(ctxTimeout, hubCtx.GroveID, ref.ID, nil); err != nil {
			debugf("Failed to remove agent '%s' (id=%s): %v", ref.Name, ref.ID, err)
			return fmt.Errorf("failed to remove agent '%s': %w", ref.Name, err)
		}
		debugf("Agent '%s' removed successfully", ref.Name)
	}

	if len(result.ToRegister) > 0 || len(result.ToRemove) > 0 {
		fmt.Println("Agent synchronization complete.")
	}

	return nil
}

// GetLocalAgents returns agent names from .scion/agents/.
func GetLocalAgents(grovePath string) ([]string, error) {
	agentsDir := filepath.Join(grovePath, "agents")

	entries, err := os.ReadDir(agentsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}

	var agents []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		// Check if it has a scion-agent config file (YAML or JSON)
		yamlPath := filepath.Join(agentsDir, entry.Name(), "scion-agent.yaml")
		jsonPath := filepath.Join(agentsDir, entry.Name(), "scion-agent.json")
		if _, err := os.Stat(yamlPath); err == nil {
			agents = append(agents, entry.Name())
		} else if _, err := os.Stat(jsonPath); err == nil {
			agents = append(agents, entry.Name())
		}
	}

	return agents, nil
}

// isGroveRegistered checks if the grove is registered with the Hub.
func isGroveRegistered(ctx context.Context, hubCtx *HubContext) (bool, error) {
	ctxTimeout, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Try to get the grove by ID
	_, err := hubCtx.Client.Groves().Get(ctxTimeout, hubCtx.GroveID)
	if err != nil {
		// Check if it's a "not found" error
		errStr := err.Error()
		if containsIgnoreCase(errStr, "404") || containsIgnoreCase(errStr, "not found") {
			return false, nil
		}
		return false, fmt.Errorf("failed to check grove registration: %w", err)
	}

	return true, nil
}

// findMatchingGroves finds groves with the same name on the Hub.
func findMatchingGroves(ctx context.Context, hubCtx *HubContext, groveName string) ([]GroveMatch, error) {
	ctxTimeout, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	resp, err := hubCtx.Client.Groves().List(ctxTimeout, &hubclient.ListGrovesOptions{
		Name: groveName,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to search for matching groves: %w", err)
	}

	var matches []GroveMatch
	for _, g := range resp.Groves {
		matches = append(matches, GroveMatch{
			ID:        g.ID,
			Name:      g.Name,
			GitRemote: g.GitRemote,
		})
	}

	return matches, nil
}

// registerGrove registers the grove with the Hub.
func registerGrove(ctx context.Context, hubCtx *HubContext, groveName string, isGlobal bool) error {
	ctxTimeout, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Get git remote (optional)
	var gitRemote string
	if !isGlobal {
		gitRemote = util.GetGitRemote()
	}

	// Get hostname
	hostName, err := os.Hostname()
	if err != nil {
		hostName = "local-host"
	}

	req := &hubclient.RegisterGroveRequest{
		ID:        hubCtx.GroveID,
		Name:      groveName,
		GitRemote: util.NormalizeGitRemote(gitRemote),
		Path:      hubCtx.GrovePath,
		Mode:      "connected",
		Host: &hubclient.HostInfo{
			ID:   hubCtx.HostID,
			Name: hostName,
		},
	}

	resp, err := hubCtx.Client.Groves().Register(ctxTimeout, req)
	if err != nil {
		return err
	}

	// Save the host token and ID
	if resp.HostToken != "" {
		if err := config.UpdateSetting(hubCtx.GrovePath, "hub.hostToken", resp.HostToken, isGlobal); err != nil {
			fmt.Printf("Warning: failed to save host token: %v\n", err)
		}
	}
	if resp.Host != nil && resp.Host.ID != "" {
		if err := config.UpdateSetting(hubCtx.GrovePath, "hub.hostId", resp.Host.ID, isGlobal); err != nil {
			fmt.Printf("Warning: failed to save host ID: %v\n", err)
		}
	}

	if resp.Created {
		fmt.Printf("Created new grove: %s (ID: %s)\n", resp.Grove.Name, resp.Grove.ID)
	} else {
		fmt.Printf("Linked to existing grove: %s (ID: %s)\n", resp.Grove.Name, resp.Grove.ID)
		// Update local grove_id to match the hub grove's ID
		if resp.Grove.ID != hubCtx.GroveID {
			if err := config.UpdateSetting(hubCtx.GrovePath, "grove_id", resp.Grove.ID, isGlobal); err != nil {
				fmt.Printf("Warning: failed to update local grove_id: %v\n", err)
			} else {
				hubCtx.GroveID = resp.Grove.ID
			}
		}
	}
	if resp.Host != nil {
		fmt.Printf("Host registered: %s (ID: %s)\n", resp.Host.Name, resp.Host.ID)
	}

	return nil
}

// getGroveName returns a human-readable grove name.
func getGroveName(grovePath string, isGlobal bool) string {
	if isGlobal {
		return "global"
	}
	gitRemote := util.GetGitRemote()
	if gitRemote != "" {
		return util.ExtractRepoName(gitRemote)
	}
	return filepath.Base(filepath.Dir(grovePath))
}

// getEndpoint returns the Hub endpoint from settings.
func getEndpoint(settings *config.Settings) string {
	if settings.Hub != nil {
		return settings.Hub.Endpoint
	}
	return ""
}

// createHubClient creates a new Hub client with proper authentication.
func createHubClient(settings *config.Settings, endpoint string) (hubclient.Client, error) {
	var opts []hubclient.Option

	// Add authentication - check in priority order
	authConfigured := false
	if settings.Hub != nil {
		if settings.Hub.Token != "" {
			opts = append(opts, hubclient.WithBearerToken(settings.Hub.Token))
			authConfigured = true
		} else if settings.Hub.APIKey != "" {
			opts = append(opts, hubclient.WithAPIKey(settings.Hub.APIKey))
			authConfigured = true
		}
	}

	// Check for OAuth credentials from scion hub auth login
	if !authConfigured {
		if accessToken := credentials.GetAccessToken(endpoint); accessToken != "" {
			opts = append(opts, hubclient.WithBearerToken(accessToken))
			authConfigured = true
		}
	}

	// Fallback to auto dev auth
	if !authConfigured {
		opts = append(opts, hubclient.WithAutoDevAuth())
	}

	opts = append(opts, hubclient.WithTimeout(30*time.Second))

	return hubclient.New(endpoint, opts...)
}

// wrapHubError wraps a Hub error with guidance to disable Hub integration.
func wrapHubError(err error) error {
	if apiclient.IsUnauthorizedError(err) {
		return fmt.Errorf("authentication failed, login to hub with 'scion hub auth login'")
	}
	return fmt.Errorf("%w\n\nTo use local-only mode, use: scion --no-hub <command>", err)
}

// containsIgnoreCase checks if a string contains a substring (case-insensitive).
func containsIgnoreCase(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
