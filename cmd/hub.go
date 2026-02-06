package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ptone/scion-agent/pkg/apiclient"
	"github.com/ptone/scion-agent/pkg/config"
	"github.com/ptone/scion-agent/pkg/credentials"
	"github.com/ptone/scion-agent/pkg/brokercredentials"
	"github.com/ptone/scion-agent/pkg/hubclient"
	"github.com/ptone/scion-agent/pkg/util"
	"github.com/ptone/scion-agent/pkg/version"
	"github.com/spf13/cobra"
)

var (
	hubRegisterMode   string
	hubForceRegister  bool
	hubOutputJSON     bool
	hubDeregisterHost bool
)

// hubCmd represents the hub command
var hubCmd = &cobra.Command{
	Use:   "hub",
	Short: "Interact with the Scion Hub",
	Long: `Commands for interacting with a remote Scion Hub.

The Hub provides centralized coordination for groves, agents, and templates
across multiple runtime hosts.

Configure the Hub endpoint via:
  - SCION_HUB_ENDPOINT environment variable
  - hub.endpoint in settings.yaml
  - --hub flag on any command`,
}

// hubStatusCmd shows Hub connection status
var hubStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show Hub connection status",
	Long:  `Show the current Hub connection status and configuration.`,
	RunE:  runHubStatus,
}

// hubRegisterCmd registers this host with the Hub
var hubRegisterCmd = &cobra.Command{
	Use:   "register [grove-path]",
	Short: "Register this host with the Hub",
	Long: `Register this host as a runtime contributor for a grove.

If grove-path is not specified, uses the current project grove or global grove.
The host is identified by its hostname to prevent duplicate registrations.

This command will:
1. Create or update the grove in the Hub (matched by git remote or name)
2. Register this host as a contributor to the grove (using hostname as identifier)
3. Save the returned host token for future authentication

Examples:
  # Register the current project grove
  scion hub register

  # Register the global grove
  scion hub register --global`,
	RunE: runHubRegister,
}

// hubDeregisterCmd removes this host from the Hub
var hubDeregisterCmd = &cobra.Command{
	Use:   "deregister",
	Short: "Remove this host from the Hub",
	Long: `Remove this host from the Hub.

This command will:
1. Remove this host from all groves it contributes to
2. Clear the stored host token

Use --host-only to only remove the host record without affecting grove contributions.`,
	RunE: runHubDeregister,
}

// hubGrovesCmd lists groves on the Hub
var hubGrovesCmd = &cobra.Command{
	Use:   "groves",
	Short: "List groves on the Hub",
	Long:  `List groves registered on the Hub that you have access to.`,
	RunE:  runHubGroves,
}

// hubHostsCmd lists runtime hosts on the Hub
var hubHostsCmd = &cobra.Command{
	Use:   "hosts",
	Short: "List runtime hosts on the Hub",
	Long:  `List runtime hosts registered on the Hub.`,
	RunE:  runHubHosts,
}

// hubEnableCmd enables Hub integration
var hubEnableCmd = &cobra.Command{
	Use:   "enable",
	Short: "Enable Hub integration",
	Long: `Enable Hub integration for agent operations.

When enabled, agent operations (create, start, delete) will be routed through
the Hub API instead of being performed locally. This allows centralized
coordination of agents across multiple runtime hosts.

The Hub endpoint must be configured before enabling:
  - SCION_HUB_ENDPOINT environment variable
  - hub.endpoint in settings.yaml
  - --hub flag on any command`,
	RunE: runHubEnable,
}

// hubDisableCmd disables Hub integration
var hubDisableCmd = &cobra.Command{
	Use:   "disable",
	Short: "Disable Hub integration",
	Long: `Disable Hub integration for agent operations.

When disabled, agent operations are performed locally on this host.
The Hub configuration is preserved and can be re-enabled later.`,
	RunE: runHubDisable,
}

func init() {
	rootCmd.AddCommand(hubCmd)
	hubCmd.AddCommand(hubStatusCmd)
	hubCmd.AddCommand(hubRegisterCmd)
	hubCmd.AddCommand(hubDeregisterCmd)
	hubCmd.AddCommand(hubGrovesCmd)
	hubCmd.AddCommand(hubHostsCmd)
	hubCmd.AddCommand(hubEnableCmd)
	hubCmd.AddCommand(hubDisableCmd)

	// Register flags
	hubRegisterCmd.Flags().StringVar(&hubRegisterMode, "mode", "connected", "Registration mode (connected, read-only)")
	hubRegisterCmd.Flags().BoolVar(&hubForceRegister, "force", false, "Force re-registration even if already registered")

	// Deregister flags
	hubDeregisterCmd.Flags().BoolVar(&hubDeregisterHost, "host-only", false, "Only remove host record, not grove contributions")

	// Common flags
	hubStatusCmd.Flags().BoolVar(&hubOutputJSON, "json", false, "Output in JSON format")
	hubGrovesCmd.Flags().BoolVar(&hubOutputJSON, "json", false, "Output in JSON format")
	hubHostsCmd.Flags().BoolVar(&hubOutputJSON, "json", false, "Output in JSON format")
}

// authInfo describes the authentication method being used
type authInfo struct {
	Method      string // Human-readable description
	MethodType  string // Short type: "oauth", "bearer", "apikey", "devauth", "none"
	Source      string // Where the credentials came from
	IsDevAuth   bool   // Whether dev-auth is being used
	HasOAuth    bool   // Whether OAuth credentials are present
	OAuthCreds  *credentials.HubCredentials
}

// getAuthInfo determines what authentication method will be used for a given endpoint
func getAuthInfo(settings *config.Settings, endpoint string) authInfo {
	info := authInfo{
		Method:     "none",
		MethodType: "none",
	}

	// Check settings-based auth first
	if settings.Hub != nil {
		if settings.Hub.Token != "" {
			info.Method = "Bearer token"
			info.MethodType = "bearer"
			info.Source = "settings"
			return info
		}
		if settings.Hub.APIKey != "" {
			info.Method = "API key"
			info.MethodType = "apikey"
			info.Source = "settings"
			return info
		}
	}

	// Check for OAuth credentials from scion hub auth login
	if endpoint != "" {
		if creds, err := credentials.Load(endpoint); err == nil && creds.AccessToken != "" {
			info.Method = "OAuth"
			info.MethodType = "oauth"
			info.Source = "scion hub auth login"
			info.HasOAuth = true
			info.OAuthCreds = creds
			return info
		}
	}

	// Check for dev auth
	token, source := apiclient.ResolveDevTokenWithSource()
	if token != "" {
		info.Method = "Dev auth"
		info.MethodType = "devauth"
		info.Source = source
		info.IsDevAuth = true
		return info
	}

	return info
}

func getHubClient(settings *config.Settings) (hubclient.Client, error) {
	endpoint := GetHubEndpoint(settings)
	if endpoint == "" {
		return nil, fmt.Errorf("Hub endpoint not configured. Set SCION_HUB_ENDPOINT or use --hub flag")
	}

	var opts []hubclient.Option

	// Get auth info for logging
	info := getAuthInfo(settings, endpoint)

	// Add authentication - check in priority order
	// Note: HostToken is intentionally NOT used here. HostTokens are for host-level
	// operations (registration, heartbeats) and are NOT user authentication tokens.
	// For user operations (listing groves, agents, etc.), we use user tokens, API keys,
	// OAuth credentials, or dev auth.
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

	// Fallback to auto dev auth if no explicit auth configured
	// This checks SCION_DEV_TOKEN env var and ~/.scion/dev-token file
	if !authConfigured {
		opts = append(opts, hubclient.WithAutoDevAuth())
	}

	util.Debugf("Hub client auth: %s (source: %s)", info.Method, info.Source)
	util.Debugf("Hub endpoint: %s", endpoint)

	opts = append(opts, hubclient.WithTimeout(30*time.Second))

	return hubclient.New(endpoint, opts...)
}

func runHubStatus(cmd *cobra.Command, args []string) error {
	// Resolve grove path to find project settings
	resolvedPath, _, err := config.ResolveGrovePath(grovePath)
	if err != nil {
		return fmt.Errorf("failed to resolve grove path: %w", err)
	}

	settings, err := config.LoadSettings(resolvedPath)
	if err != nil {
		return fmt.Errorf("failed to load settings: %w", err)
	}

	endpoint := GetHubEndpoint(settings)

	hubEnabled := settings.IsHubEnabled()

	// Get authentication info
	authInfo := getAuthInfo(settings, endpoint)

	if hubOutputJSON {
		status := map[string]interface{}{
			"enabled":       hubEnabled,
			"cliOverride":   noHub,
			"endpoint":      endpoint,
			"configured":    settings.IsHubConfigured(),
			"groveId":       settings.GroveID,
		}
		if settings.Hub != nil {
			status["brokerId"] = settings.Hub.BrokerID
			status["hasToken"] = settings.Hub.Token != ""
			status["hasApiKey"] = settings.Hub.APIKey != ""
			status["hasBrokerToken"] = settings.Hub.BrokerToken != ""
		}

		// Add auth info to JSON output
		status["authMethod"] = authInfo.MethodType
		status["authSource"] = authInfo.Source
		status["isDevAuth"] = authInfo.IsDevAuth
		if authInfo.OAuthCreds != nil && authInfo.OAuthCreds.User != nil {
			status["authUser"] = map[string]string{
				"id":          authInfo.OAuthCreds.User.ID,
				"email":       authInfo.OAuthCreds.User.Email,
				"displayName": authInfo.OAuthCreds.User.DisplayName,
				"role":        authInfo.OAuthCreds.User.Role,
			}
			if !authInfo.OAuthCreds.ExpiresAt.IsZero() {
				status["authExpires"] = authInfo.OAuthCreds.ExpiresAt.Format(time.RFC3339)
			}
		}

		// Try to connect and get health
		if endpoint != "" && !noHub {
			client, err := getHubClient(settings)
			if err == nil {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if health, err := client.Health(ctx); err == nil {
					status["connected"] = true
					status["hubVersion"] = health.Version
					status["hubStatus"] = health.Status
				} else {
					status["connected"] = false
					status["error"] = err.Error()
				}
			}
		}

		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(status)
	}

	// Text output
	fmt.Println("Hub Integration Status")
	fmt.Println("======================")
	fmt.Printf("Enabled:    %v\n", hubEnabled)
	if noHub {
		fmt.Printf("            (overridden by --no-hub flag)\n")
	}
	fmt.Printf("Endpoint:   %s\n", valueOrNone(endpoint))
	fmt.Printf("Configured: %v\n", settings.IsHubConfigured())

	// Show grove_id from top-level setting (where it's now stored)
	fmt.Printf("Grove ID:   %s\n", valueOrNone(settings.GroveID))
	if settings.Hub != nil {
		fmt.Printf("Host ID:    %s\n", valueOrNone(settings.Hub.BrokerID))
	}

	// Authentication status section
	fmt.Println()
	fmt.Println("Authentication")
	fmt.Println("--------------")
	if authInfo.MethodType == "none" {
		fmt.Println("Method:     Not authenticated")
	} else {
		fmt.Printf("Method:     %s\n", authInfo.Method)
		if authInfo.Source != "" {
			fmt.Printf("Source:     %s\n", authInfo.Source)
		}
		if authInfo.IsDevAuth {
			fmt.Println("            (development mode - not for production use)")
		}
		if authInfo.HasOAuth && authInfo.OAuthCreds != nil {
			if authInfo.OAuthCreds.User != nil {
				fmt.Printf("User:       %s (%s)\n", authInfo.OAuthCreds.User.DisplayName, authInfo.OAuthCreds.User.Email)
				if authInfo.OAuthCreds.User.Role != "" {
					fmt.Printf("Role:       %s\n", authInfo.OAuthCreds.User.Role)
				}
			}
			if !authInfo.OAuthCreds.ExpiresAt.IsZero() {
				if time.Now().After(authInfo.OAuthCreds.ExpiresAt) {
					fmt.Printf("Expires:    %s (EXPIRED)\n", authInfo.OAuthCreds.ExpiresAt.Format(time.RFC3339))
				} else {
					fmt.Printf("Expires:    %s\n", authInfo.OAuthCreds.ExpiresAt.Format(time.RFC3339))
				}
			}
		}
	}

	// Try to connect
	if endpoint != "" && !noHub {
		client, err := getHubClient(settings)
		if err != nil {
			fmt.Printf("\nConnection: failed (%s)\n", err)
			return nil
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		health, err := client.Health(ctx)
		if err != nil {
			fmt.Printf("\nConnection: failed (%s)\n", err)
		} else {
			fmt.Printf("\nConnection: ok\n")
			fmt.Printf("Hub Version: %s\n", health.Version)
			fmt.Printf("Hub Status:  %s\n", health.Status)

			// If OAuth, verify auth is actually working by calling /auth/me
			if authInfo.HasOAuth {
				meCtx, meCancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer meCancel()
				if user, err := client.Auth().Me(meCtx); err == nil {
					fmt.Printf("\nAuthenticated as: %s (%s) [%s]\n", user.DisplayName, user.Email, user.Role)
				} else {
					fmt.Printf("\nAuth verification: failed (%s)\n", err)
					fmt.Println("Run 'scion hub auth login' to re-authenticate.")
				}
			}
		}
	}

	return nil
}

func runHubRegister(cmd *cobra.Command, args []string) error {
	// Determine grove path
	gp := grovePath
	if len(args) > 0 {
		gp = args[0]
	}
	if gp == "" && globalMode {
		gp = "global"
	}

	// Resolve grove path
	resolvedPath, isGlobal, err := config.ResolveGrovePath(gp)
	if err != nil {
		return fmt.Errorf("failed to resolve grove path: %w", err)
	}

	settings, err := config.LoadSettings(resolvedPath)
	if err != nil {
		return fmt.Errorf("failed to load settings: %w", err)
	}

	client, err := getHubClient(settings)
	if err != nil {
		return err
	}

	endpoint := GetHubEndpoint(settings)

	// Get grove info
	var groveName string
	var gitRemote string
	var groveID string

	// Get grove_id from settings, or generate if missing (backward compatibility)
	groveID = settings.GroveID
	if groveID == "" {
		// Generate grove_id for older groves that don't have one
		groveID = config.GenerateGroveIDForDir(filepath.Dir(resolvedPath))
		// Save it for future use
		if err := config.UpdateSetting(resolvedPath, "grove_id", groveID, isGlobal); err != nil {
			fmt.Printf("Warning: failed to save generated grove_id: %v\n", err)
		}
	}

	if isGlobal {
		groveName = "global"
	} else {
		// Get git remote (optional - not required for registration)
		gitRemote = util.GetGitRemote()
		if gitRemote != "" {
			// Get project name from git remote
			groveName = util.ExtractRepoName(gitRemote)
		} else {
			// No origin remote - use directory name as grove name
			groveName = filepath.Base(filepath.Dir(resolvedPath))
		}
	}

	// Get hostname (always use system hostname to prevent duplicates)
	brokerName, err := os.Hostname()
	if err != nil {
		brokerName = "local-host"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// ==== TWO-PHASE HOST REGISTRATION ====
	// Phase 1: Check for existing host credentials or create new host
	// Phase 2: Complete host join to get HMAC secret
	// Phase 3: Register grove with host ID

	credStore := brokercredentials.NewStore("")
	existingCreds, credErr := credStore.Load()

	var brokerID string
	var needsJoin bool

	// Check if we already have valid credentials
	if credErr == nil && existingCreds != nil && existingCreds.BrokerID != "" && !hubForceRegister {
		// Existing credentials found - verify they're still valid
		brokerID = existingCreds.BrokerID
		fmt.Printf("Using existing host credentials (hostId: %s)\n", brokerID)

		// Verify the host still exists on the hub
		_, err := client.RuntimeBrokers().Get(ctx, brokerID)
		if err != nil {
			fmt.Printf("Warning: existing host not found on Hub, will re-register\n")
			brokerID = ""
			needsJoin = true
		}
	} else {
		needsJoin = true
	}

	// Phase 1 & 2: Create host and complete join if needed
	if needsJoin || brokerID == "" {
		fmt.Printf("Registering host with Hub...\n")

		// Phase 1: Create host registration
		createReq := &hubclient.CreateBrokerRequest{
			Name: brokerName,
			Capabilities: []string{
				"sync",
				"attach",
			},
		}

		createResp, err := client.RuntimeBrokers().Create(ctx, createReq)
		if err != nil {
			return fmt.Errorf("failed to create host registration: %w", err)
		}

		fmt.Printf("Host created (ID: %s), completing join...\n", createResp.BrokerID)

		// Phase 2: Complete host join with join token
		joinReq := &hubclient.JoinBrokerRequest{
			BrokerID:    createResp.BrokerID,
			JoinToken: createResp.JoinToken,
			Hostname:  brokerName,
			Version:   version.Version,
			Capabilities: []string{
				"sync",
				"attach",
			},
		}

		joinResp, err := client.RuntimeBrokers().Join(ctx, joinReq)
		if err != nil {
			return fmt.Errorf("failed to complete host join: %w", err)
		}

		brokerID = joinResp.BrokerID

		// Save credentials
		if err := credStore.SaveFromJoinResponse(brokerID, joinResp.SecretKey, endpoint); err != nil {
			fmt.Printf("Warning: failed to save host credentials: %v\n", err)
		} else {
			fmt.Printf("Host credentials saved to %s\n", credStore.Path())
		}
	}

	// Phase 3: Register grove with host ID link
	req := &hubclient.RegisterGroveRequest{
		ID:        groveID,
		Name:      groveName,
		GitRemote: util.NormalizeGitRemote(gitRemote),
		Path:      resolvedPath,
		BrokerID:    brokerID, // Link to the registered host
		Mode:      hubRegisterMode,
	}

	resp, err := client.Groves().Register(ctx, req)
	if err != nil {
		return fmt.Errorf("grove registration failed: %w", err)
	}

	// Save hub settings to GLOBAL settings since registration is a host-level operation.
	// The RuntimeBroker server reads from global settings to know which Hub to connect to.
	globalDir, err := config.GetGlobalDir()
	if err != nil {
		fmt.Printf("Warning: failed to get global directory: %v\n", err)
	} else {
		// Save the hub endpoint so RuntimeBroker knows where to connect
		if endpoint != "" {
			if err := config.UpdateSetting(globalDir, "hub.endpoint", endpoint, true); err != nil {
				fmt.Printf("Warning: failed to save hub endpoint to global settings: %v\n", err)
			}
		}

		// Save the host ID to global settings
		if err := config.UpdateSetting(globalDir, "hub.brokerId", brokerID, true); err != nil {
			fmt.Printf("Warning: failed to save host ID: %v\n", err)
		}
	}

	if resp.Created {
		fmt.Printf("Created new grove: %s (ID: %s)\n", resp.Grove.Name, resp.Grove.ID)
	} else {
		fmt.Printf("Linked to existing grove: %s (ID: %s)\n", resp.Grove.Name, resp.Grove.ID)
		// Update local grove_id to match the hub grove's ID
		if resp.Grove.ID != groveID {
			if err := config.UpdateSetting(resolvedPath, "grove_id", resp.Grove.ID, isGlobal); err != nil {
				fmt.Printf("Warning: failed to update local grove_id: %v\n", err)
			}
		}
	}

	if resp.Host != nil {
		fmt.Printf("Host registered: %s (ID: %s)\n", resp.Host.Name, resp.Host.ID)
	} else {
		fmt.Printf("Host linked: %s\n", brokerID)
	}

	return nil
}

func runHubDeregister(cmd *cobra.Command, args []string) error {
	// Resolve grove path to find project settings
	resolvedPath, _, err := config.ResolveGrovePath(grovePath)
	if err != nil {
		return fmt.Errorf("failed to resolve grove path: %w", err)
	}

	settings, err := config.LoadSettings(resolvedPath)
	if err != nil {
		return fmt.Errorf("failed to load settings: %w", err)
	}

	if settings.Hub == nil || settings.Hub.BrokerID == "" {
		return fmt.Errorf("no host registration found")
	}

	client, err := getHubClient(settings)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	brokerID := settings.Hub.BrokerID

	if err := client.RuntimeBrokers().Delete(ctx, brokerID); err != nil {
		return fmt.Errorf("deregistration failed: %w", err)
	}

	// Clear the stored credentials from GLOBAL settings
	// These are host-level credentials, not grove-specific.
	globalDir, globalErr := config.GetGlobalDir()
	if globalErr != nil {
		fmt.Printf("Warning: failed to get global directory: %v\n", globalErr)
	} else {
		_ = config.UpdateSetting(globalDir, "hub.brokerToken", "", true)
		_ = config.UpdateSetting(globalDir, "hub.brokerId", "", true)
	}

	fmt.Printf("Host %s deregistered from Hub\n", brokerID)
	return nil
}

func runHubGroves(cmd *cobra.Command, args []string) error {
	// Resolve grove path to find project settings
	resolvedPath, _, err := config.ResolveGrovePath(grovePath)
	if err != nil {
		return fmt.Errorf("failed to resolve grove path: %w", err)
	}

	settings, err := config.LoadSettings(resolvedPath)
	if err != nil {
		return fmt.Errorf("failed to load settings: %w", err)
	}

	client, err := getHubClient(settings)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := client.Groves().List(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to list groves: %w", err)
	}

	if hubOutputJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(resp.Groves)
	}

	if len(resp.Groves) == 0 {
		fmt.Println("No groves found")
		return nil
	}

	fmt.Printf("%-36s  %-20s  %-10s  %s\n", "ID", "NAME", "AGENTS", "GIT REMOTE")
	fmt.Printf("%-36s  %-20s  %-10s  %s\n", "------------------------------------", "--------------------", "----------", "----------")
	for _, g := range resp.Groves {
		gitRemote := g.GitRemote
		if len(gitRemote) > 40 {
			gitRemote = gitRemote[:37] + "..."
		}
		fmt.Printf("%-36s  %-20s  %-10d  %s\n", g.ID, truncate(g.Name, 20), g.AgentCount, gitRemote)
	}

	return nil
}

func runHubHosts(cmd *cobra.Command, args []string) error {
	// Resolve grove path to find project settings
	resolvedPath, _, err := config.ResolveGrovePath(grovePath)
	if err != nil {
		return fmt.Errorf("failed to resolve grove path: %w", err)
	}

	settings, err := config.LoadSettings(resolvedPath)
	if err != nil {
		return fmt.Errorf("failed to load settings: %w", err)
	}

	client, err := getHubClient(settings)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := client.RuntimeBrokers().List(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to list hosts: %w", err)
	}

	if hubOutputJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(resp.Hosts)
	}

	if len(resp.Hosts) == 0 {
		fmt.Println("No runtime hosts found")
		return nil
	}

	fmt.Printf("%-36s  %-20s  %-10s  %s\n", "ID", "NAME", "STATUS", "MODE")
	fmt.Printf("%-36s  %-20s  %-10s  %s\n", "------------------------------------", "--------------------", "----------", "----------")
	for _, h := range resp.Hosts {
		fmt.Printf("%-36s  %-20s  %-10s  %s\n", h.ID, truncate(h.Name, 20), h.Status, h.Mode)
	}

	return nil
}

func valueOrNone(s string) string {
	if s == "" {
		return "(not configured)"
	}
	return s
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func runHubEnable(cmd *cobra.Command, args []string) error {
	// Resolve grove path
	resolvedPath, isGlobal, err := config.ResolveGrovePath(grovePath)
	if err != nil {
		return fmt.Errorf("failed to resolve grove path: %w", err)
	}

	settings, err := config.LoadSettings(resolvedPath)
	if err != nil {
		return fmt.Errorf("failed to load settings: %w", err)
	}

	endpoint := GetHubEndpoint(settings)
	if endpoint == "" {
		return fmt.Errorf("Hub endpoint not configured.\n\nConfigure the Hub endpoint via:\n  - SCION_HUB_ENDPOINT environment variable\n  - hub.endpoint in settings.yaml\n  - --hub flag on any command\n\nExample: scion config set hub.endpoint https://hub.scion.dev --global")
	}

	// Try to connect and verify Hub is healthy before enabling
	client, err := getHubClient(settings)
	if err != nil {
		return fmt.Errorf("failed to create Hub client: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	health, err := client.Health(ctx)
	if err != nil {
		return fmt.Errorf("failed to connect to Hub at %s: %w\n\nVerify the Hub endpoint is correct and the Hub is running.", endpoint, err)
	}

	// Save the enabled setting
	if err := config.UpdateSetting(resolvedPath, "hub.enabled", "true", isGlobal); err != nil {
		return fmt.Errorf("failed to save setting: %w", err)
	}

	// If the endpoint was provided via --hub flag, persist it to settings
	if hubEndpoint != "" {
		if err := config.UpdateSetting(resolvedPath, "hub.endpoint", hubEndpoint, isGlobal); err != nil {
			return fmt.Errorf("failed to save endpoint: %w", err)
		}
	}

	fmt.Printf("Hub integration enabled.\n")
	fmt.Printf("Endpoint: %s\n", endpoint)
	fmt.Printf("Hub Status: %s (version %s)\n", health.Status, health.Version)
	fmt.Println("\nAgent operations (create, start, delete) will now be routed through the Hub.")
	fmt.Println("Use 'scion hub disable' to switch back to local-only mode.")

	return nil
}

func runHubDisable(cmd *cobra.Command, args []string) error {
	// Resolve grove path
	resolvedPath, isGlobal, err := config.ResolveGrovePath(grovePath)
	if err != nil {
		return fmt.Errorf("failed to resolve grove path: %w", err)
	}

	settings, err := config.LoadSettings(resolvedPath)
	if err != nil {
		return fmt.Errorf("failed to load settings: %w", err)
	}

	if !settings.IsHubEnabled() {
		fmt.Println("Hub integration is already disabled.")
		return nil
	}

	// Save the disabled setting
	if err := config.UpdateSetting(resolvedPath, "hub.enabled", "false", isGlobal); err != nil {
		return fmt.Errorf("failed to save setting: %w", err)
	}

	fmt.Println("Hub integration disabled.")
	fmt.Println("Agent operations will now be performed locally.")
	fmt.Println("\nHub configuration is preserved. Use 'scion hub enable' to re-enable.")

	return nil
}
