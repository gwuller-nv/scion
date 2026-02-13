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
	"encoding/json"
	"fmt"
	"os"
)

// isJSONOutput returns true if the output format is set to JSON.
func isJSONOutput() bool {
	return outputFormat == "json"
}

// outputJSON pretty-prints a value as JSON to stdout.
func outputJSON(v interface{}) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// ActionResult is a standard JSON shape for action command results.
type ActionResult struct {
	Status   string                 `json:"status"`
	Command  string                 `json:"command"`
	Agent    string                 `json:"agent,omitempty"`
	Message  string                 `json:"message,omitempty"`
	Warnings []string               `json:"warnings,omitempty"`
	Details  map[string]interface{} `json:"details,omitempty"`
}

// outputActionResult outputs an ActionResult as JSON or plain text depending on format.
func outputActionResult(r ActionResult) error {
	if isJSONOutput() {
		return outputJSON(r)
	}
	if r.Message != "" {
		fmt.Println(r.Message)
	}
	for _, w := range r.Warnings {
		fmt.Printf("Warning: %s\n", w)
	}
	return nil
}

// interactiveOnlyCommands maps command paths to the reason they cannot support --format json.
var interactiveOnlyCommands = map[string]string{
	"scion attach":              "it requires an interactive terminal session",
	"scion logs":                "it produces streaming output",
	"scion broker start":        "it runs a long-running server process",
	"scion broker stop":         "it manages a daemon process",
	"scion broker register":     "it requires interactive prompts",
	"scion broker deregister":   "it requires interactive prompts",
	"scion broker provide":      "it requires interactive prompts",
	"scion broker withdraw":     "it requires interactive prompts",
	"scion server start":        "it runs a long-running server process",
	"scion server stop":         "it manages a server process",
	"scion server status":       "it manages a server process",
	"scion message":             "it is used for internal agent messaging",
	"scion msg":                 "it is used for internal agent messaging",
	"scion cdw":                 "it is a shell integration command",
	"scion hub auth login":      "it requires interactive browser authentication",
	"scion hub auth logout":     "it manages authentication state",
}
