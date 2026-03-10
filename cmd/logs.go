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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ptone/scion-agent/pkg/agent"
	"github.com/ptone/scion-agent/pkg/api"
	"github.com/ptone/scion-agent/pkg/hubclient"
	"github.com/ptone/scion-agent/pkg/runtime"
	"github.com/spf13/cobra"
)

var (
	logsTail     int
	logsSince    string
	logsFollow   bool
	logsSeverity string
	logsBroker   string
	logsJSON     bool
)

// logsCmd represents the logs command
var logsCmd = &cobra.Command{
	Use:               "logs <agent>",
	Short:             "Get logs of an agent",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: getAgentNames,
	RunE: func(cmd *cobra.Command, args []string) error {
		agentName := api.Slugify(args[0])

		// Check if Hub is enabled
		hubCtx, err := CheckHubAvailabilityWithOptions(grovePath, true)
		if err != nil {
			return err
		}
		if hubCtx != nil {
			return getHubCloudLogs(cmd.Context(), hubCtx, agentName)
		}

		// Local mode: read from filesystem
		effectiveProfile := profile
		if effectiveProfile == "" {
			effectiveProfile = agent.GetSavedRuntime(agentName, grovePath)
		}

		rt := runtime.GetRuntime(grovePath, effectiveProfile)

		// Find the agent to get its grove path
		agents, err := rt.List(context.Background(), map[string]string{
			"scion.agent": "true",
			"scion.name":  agentName,
		})
		if err != nil {
			return fmt.Errorf("failed to find agent %s: %w", agentName, err)
		}
		if len(agents) == 0 {
			return fmt.Errorf("agent %s not found", agentName)
		}

		a := agents[0]
		if a.GrovePath == "" {
			return fmt.Errorf("agent %s has no grove path configured", agentName)
		}

		agentLogPath := filepath.Join(a.GrovePath, "agents", agentName, "home", "agent.log")
		if _, err := os.Stat(agentLogPath); os.IsNotExist(err) {
			return fmt.Errorf("log file not found: %s\n\nThe agent may not have started yet or does not produce logs", agentLogPath)
		}

		data, err := os.ReadFile(agentLogPath)
		if err != nil {
			return fmt.Errorf("failed to read log file: %w", err)
		}

		fmt.Print(string(data))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(logsCmd)
	logsCmd.Flags().IntVarP(&logsTail, "tail", "n", 100, "Number of lines from end")
	logsCmd.Flags().StringVar(&logsSince, "since", "", "Show logs since timestamp/duration (e.g., 1h, 2026-03-07T10:00:00Z)")
	logsCmd.Flags().BoolVarP(&logsFollow, "follow", "f", false, "Stream logs in real-time")
	logsCmd.Flags().StringVar(&logsSeverity, "severity", "", "Minimum severity level (DEBUG, INFO, WARNING, ERROR, CRITICAL)")
	logsCmd.Flags().StringVar(&logsBroker, "broker", "", "Filter by runtime broker ID")
	logsCmd.Flags().BoolVar(&logsJSON, "json", false, "Output full JSON entries")
}

// getHubCloudLogs retrieves cloud logs from the hub.
func getHubCloudLogs(ctx context.Context, hubCtx *HubContext, agentName string) error {
	PrintUsingHub(hubCtx.Endpoint)

	opts := &hubclient.GetCloudLogsOptions{
		Tail:     logsTail,
		Severity: logsSeverity,
		BrokerID: logsBroker,
	}

	// Resolve --since flag: supports both RFC3339 and duration formats
	if logsSince != "" {
		since, err := parseSinceFlag(logsSince)
		if err != nil {
			return fmt.Errorf("invalid --since value: %w", err)
		}
		opts.Since = since
	}

	client := hubCtx.Client.GroveAgents(hubCtx.GroveID)

	// Streaming mode
	if logsFollow {
		return client.StreamCloudLogs(ctx, agentName, opts, func(entry hubclient.CloudLogEntry) {
			printLogEntry(entry)
		})
	}

	// Polling mode
	result, err := client.GetCloudLogs(ctx, agentName, opts)
	if err != nil {
		return err
	}

	// Print entries in chronological order (API returns newest-first)
	for i := len(result.Entries) - 1; i >= 0; i-- {
		printLogEntry(result.Entries[i])
	}

	return nil
}

// printLogEntry formats and prints a single log entry.
func printLogEntry(entry hubclient.CloudLogEntry) {
	if logsJSON {
		data, err := json.Marshal(entry)
		if err == nil {
			fmt.Fprintln(os.Stdout, string(data))
		}
		return
	}

	// Compact format: TIMESTAMP  SEVERITY  MESSAGE
	ts := entry.Timestamp.Format(time.RFC3339Nano)
	severity := padRight(entry.Severity, 8)
	fmt.Fprintf(os.Stdout, "%s  %s  %s\n", ts, severity, entry.Message)
}

// parseSinceFlag parses a --since value as either an RFC3339 timestamp or a
// Go duration string (e.g., "1h", "30m", "2h30m").
func parseSinceFlag(value string) (string, error) {
	// Try RFC3339 first
	if _, err := time.Parse(time.RFC3339, value); err == nil {
		return value, nil
	}
	if _, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return value, nil
	}

	// Try as a Go duration (e.g., "1h", "30m")
	d, err := time.ParseDuration(value)
	if err != nil {
		return "", fmt.Errorf("expected RFC3339 timestamp or duration (e.g., 1h, 30m): %s", value)
	}
	return time.Now().Add(-d).Format(time.RFC3339Nano), nil
}

// padRight pads a string to the given width with spaces.
func padRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}
