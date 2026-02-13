// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/ptone/scion-agent/pkg/agent"
	"github.com/ptone/scion-agent/pkg/api"
	"github.com/ptone/scion-agent/pkg/hubclient"
	"github.com/ptone/scion-agent/pkg/runtime"
	"github.com/spf13/cobra"
)

// createCmd represents the create command
var createCmd = &cobra.Command{
	Use:   "create <agent-name> [task...]",
	Short: "Provision a new scion agent without starting it",
	Long: `Provision a new isolated LLM agent directory to perform a specific task.
The agent will be created from a template.

The agent-name is required as the first argument. All subsequent arguments
form the task prompt, which will be written to prompt.md. If no task
arguments are provided, an empty prompt.md is created for later editing.`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		agentName := args[0]
		task := strings.TrimSpace(strings.Join(args[1:], " "))

		// Check if Hub should be used, excluding the target agent from sync requirements.
		// This allows creating an agent even if it already exists on Hub (recreate scenario)
		// or if other agents are out of sync.
		hubCtx, err := CheckHubAvailabilityForAgent(grovePath, agentName, false)
		if err != nil {
			return err
		}

		if hubCtx != nil {
			return createAgentViaHub(hubCtx, agentName, task)
		}

		// Local mode
		effectiveProfile := profile
		if effectiveProfile == "" {
			effectiveProfile = agent.GetSavedProfile(agentName, grovePath)
		}

		rt := runtime.GetRuntime(grovePath, effectiveProfile)
		mgr := agent.NewManager(rt)

		opts := api.StartOptions{
			Name:      agentName,
			Task:      task,
			Template:  templateName,
			Profile:   effectiveProfile,
			Image:     agentImage,
			GrovePath: grovePath,
			Branch:    branch,
			Workspace: workspace,
		}

		// Check if container already exists

		agents, err := rt.List(context.Background(), nil)
		if err == nil {
			for _, a := range agents {
				if a.ID == agentName || a.Name == agentName {
					fmt.Printf("Agent container '%s' already exists (Status: %s).\n", agentName, a.Status)
					// We continue to check directory
				}
			}
		}

		_, err = mgr.Provision(context.Background(), opts)
		if err != nil {
			return err
		}

		if isJSONOutput() {
			return outputJSON(ActionResult{
				Status:  "success",
				Command: "create",
				Agent:   agentName,
				Message: fmt.Sprintf("Agent '%s' created successfully.", agentName),
			})
		}
		fmt.Printf("Agent '%s' created successfully.\n", agentName)
		return nil
	},
}

func createAgentViaHub(hubCtx *HubContext, agentName string, task string) error {
	PrintUsingHub(hubCtx.Endpoint)

	// Get the grove ID for this project
	groveID, err := GetGroveID(hubCtx)
	if err != nil {
		return wrapHubError(err)
	}

	// Resolve template if specified
	var resolvedTemplate string
	if templateName != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		result, err := ResolveTemplateForHub(ctx, hubCtx, templateName)
		if err != nil {
			return wrapHubError(fmt.Errorf("template resolution failed: %w", err))
		}

		// Use the template ID if available, otherwise fall back to name
		if result.TemplateID != "" {
			resolvedTemplate = result.TemplateID
		} else {
			resolvedTemplate = result.TemplateName
		}
	}

	// Build create request — always provision-only (create does not start the agent)
	req := &hubclient.CreateAgentRequest{
		Name:            agentName,
		GroveID:         groveID,
		Template:        resolvedTemplate,
		RuntimeBrokerID: runtimeBrokerID,
		Task:            task,
		Branch:          branch,
		ProvisionOnly:   true,
	}

	if agentImage != "" {
		req.Config = &hubclient.AgentConfig{
			Image: agentImage,
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	resp, err := createAgentWithBrokerResolution(ctx, hubCtx, groveID, req)
	if err != nil {
		return wrapHubError(fmt.Errorf("failed to create agent via Hub: %w", err))
	}

	// Print info line when broker was auto-resolved (not explicitly specified)
	if !isJSONOutput() {
		printAutoResolvedBroker(ctx, hubCtx, runtimeBrokerID, req.RuntimeBrokerID, resp)
	}

	if isJSONOutput() {
		result := ActionResult{
			Status:   "success",
			Command:  "create",
			Agent:    agentName,
			Message:  fmt.Sprintf("Agent '%s' created via Hub.", agentName),
			Warnings: resp.Warnings,
			Details:  map[string]interface{}{},
		}
		if resp.Agent != nil {
			result.Details["slug"] = resp.Agent.Slug
			result.Details["status"] = resp.Agent.Status
			if resp.Agent.RuntimeBrokerID != "" {
				result.Details["runtimeBrokerId"] = resp.Agent.RuntimeBrokerID
			}
			if resp.Agent.RuntimeBrokerName != "" {
				result.Details["runtimeBrokerName"] = resp.Agent.RuntimeBrokerName
			}
		}
		return outputJSON(result)
	}

	if resp.Agent != nil {
		brokerInfo := ""
		if resp.Agent.RuntimeBrokerName != "" {
			brokerInfo = fmt.Sprintf(" on broker %s", resp.Agent.RuntimeBrokerName)
		} else if resp.Agent.RuntimeBrokerID != "" {
			brokerInfo = fmt.Sprintf(" on broker %s", resp.Agent.RuntimeBrokerID)
		}
		fmt.Printf("Agent '%s' created via Hub%s.\n", agentName, brokerInfo)
		fmt.Printf("Agent Slug: %s\n", resp.Agent.Slug)
		fmt.Printf("Status: %s\n", resp.Agent.Status)

		// For local broker, print the agent directory path so the user can inspect/tweak files
		if hubCtx.BrokerID != "" && hubCtx.GrovePath != "" {
			agentDir := filepath.Join(hubCtx.GrovePath, "agents", agentName)
			fmt.Printf("Agent directory: %s\n", agentDir)
		}
	} else {
		fmt.Printf("Agent '%s' created via Hub.\n", agentName)
	}
	for _, w := range resp.Warnings {
		fmt.Printf("Warning: %s\n", w)
	}

	return nil
}

func init() {
	rootCmd.AddCommand(createCmd)
	createCmd.Flags().StringVarP(&templateName, "type", "t", "", "Template to use")
	createCmd.Flags().StringVarP(&agentImage, "image", "i", "", "Container image to use (overrides template)")
	createCmd.Flags().StringVarP(&branch, "branch", "b", "", "Git branch to use for the agent workspace")
	createCmd.Flags().StringVarP(&workspace, "workspace", "w", "", "Host path to mount as /workspace")
	createCmd.Flags().StringVar(&runtimeBrokerID, "broker", "", "Preferred runtime broker ID or name")

	// Template resolution flags for Hub mode (Section 9.4)
	createCmd.Flags().BoolVar(&uploadTemplate, "upload-template", false, "Automatically upload local template to Hub if not found")
	createCmd.Flags().BoolVar(&noUpload, "no-upload", false, "Fail if template requires upload (never prompt)")
	createCmd.Flags().StringVar(&templateScope, "template-scope", "grove", "Scope for uploaded template (global, grove, user)")
}
