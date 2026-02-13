/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"

	"github.com/ptone/scion-agent/pkg/util"
	"github.com/ptone/scion-agent/pkg/version"
	"github.com/spf13/cobra"
)

// versionCmd represents the version command
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number of scion",
	Long:  `All software has versions. This is scion's`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if isJSONOutput() {
			return outputJSON(map[string]string{
				"version":   version.Version,
				"commit":    version.Commit,
				"buildTime": version.BuildTime,
				"short":     version.Short(),
			})
		}
		fmt.Println(util.GetBanner())
		fmt.Println(version.Get())
		return nil
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
