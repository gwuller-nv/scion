package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/ptone/scion-agent/pkg/agent"
	"github.com/ptone/scion-agent/pkg/runtime"
	"github.com/spf13/cobra"
)

// deleteCmd represents the delete command
var deleteCmd = &cobra.Command{
	Use:     "delete <agent>",
	Aliases: []string{"rm"},
	Short:   "Delete an agent",
	Long:    `Stop and remove an agent container and its associated files and worktree.`,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		agentName := args[0]

		effectiveProfile := profile
		if effectiveProfile == "" {
			effectiveProfile = agent.GetSavedRuntime(agentName, grovePath)
		}

		rt := runtime.GetRuntime(grovePath, effectiveProfile)
		mgr := agent.NewManager(rt)


		fmt.Printf("Deleting agent '%s'...\n", agentName)
		
		// Try to stop first, ignore error if already stopped
		_ = mgr.Stop(context.Background(), agentName)

		// We check if it exists in List to provide better feedback
		agents, _ := mgr.List(context.Background(), map[string]string{"scion.name": agentName})
		containerFound := false
		for _, a := range agents {
			if a.Name == agentName || a.ID == agentName || strings.TrimPrefix(a.Name, "/") == agentName {
				containerFound = true
				break
			}
		}

		if !containerFound {
			fmt.Println("No container found, removing agent definition...")
		}

		if err := mgr.Delete(context.Background(), agentName, true, grovePath, removeBranch); err != nil {
			return err
		}

		fmt.Printf("Agent '%s' deleted.\n", agentName)
		return nil
	},
}

var removeBranch bool

func init() {
	rootCmd.AddCommand(deleteCmd)
	deleteCmd.Flags().BoolVarP(&removeBranch, "remove-branch", "b", false, "Remove the git branch associated with the worktree")
}

