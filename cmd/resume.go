package cmd

import (
	"github.com/spf13/cobra"
)

// resumeCmd represents the resume command
var resumeCmd = &cobra.Command{
	Use:   "resume <agent-name>",
	Short: "Resume a stopped scion agent",
	Long: `Resume an existing stopped LLM agent. 
The agent will be re-launched with the harness-specific resume flag, 
preserving its previous state.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return RunAgent(cmd, args, true)
	},
}

func init() {
	rootCmd.AddCommand(resumeCmd)
	resumeCmd.Flags().StringVarP(&templateName, "type", "t", "", "Template to use")
	resumeCmd.Flags().StringVarP(&agentImage, "image", "i", "", "Container image to use (overrides template)")
	resumeCmd.Flags().BoolVar(&noAuth, "no-auth", false, "Disable authentication propagation")
	resumeCmd.Flags().BoolVarP(&attach, "attach", "a", false, "Attach to the agent TTY after starting")
}