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

package harness

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ptone/scion-agent/pkg/api"
)

func (c *Codex) reconcileConfig(agentHome string, telemetry *api.TelemetryConfig, env map[string]string) error {
	codexDir := filepath.Join(agentHome, ".codex")
	if err := os.MkdirAll(codexDir, 0755); err != nil {
		return fmt.Errorf("failed to create .codex directory: %w", err)
	}

	configPath := filepath.Join(codexDir, "config.toml")
	content := ""
	if data, err := os.ReadFile(configPath); err == nil {
		content = string(data)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to read codex config: %w", err)
	}

	// Remove existing [otel] section — it will be rebuilt only if telemetry is enabled.
	content = removeTOMLSection(content, "otel")

	// Reconcile [otel] only when telemetry is enabled.
	if telemetry != nil && (telemetry.Enabled == nil || *telemetry.Enabled) {
		endpoint := resolveCodexOTELEndpoint(telemetry, env)
		protocol := resolveCodexOTELProtocol(telemetry, env)

		logUserPrompt := false
		if telemetry.Filter != nil && telemetry.Filter.Events != nil {
			if listContains(telemetry.Filter.Events.Include, "agent.user.prompt") {
				logUserPrompt = true
			}
			if listContains(telemetry.Filter.Events.Exclude, "agent.user.prompt") {
				logUserPrompt = false
			}
		}

		// Build exporter key based on protocol.
		exporterKey := "otlp-grpc"
		if protocol == "http" || protocol == "http/protobuf" {
			exporterKey = "otlp-http"
		}

		// Build headers inline table.
		headers := ""
		if telemetry.Cloud != nil && len(telemetry.Cloud.Headers) > 0 {
			parts := make([]string, 0, len(telemetry.Cloud.Headers))
			for k, v := range telemetry.Cloud.Headers {
				parts = append(parts, fmt.Sprintf(`"%s" = "%s"`, k, v))
			}
			sort.Strings(parts)
			headers = fmt.Sprintf(",\n  headers = { %s }", strings.Join(parts, ", "))
		}

		otelSection := fmt.Sprintf("[otel]\nenabled = true\nlog_user_prompt = %v\nexporter = { %s = {\n  endpoint = \"%s\"%s\n}}\n",
			logUserPrompt, exporterKey, endpoint, headers)

		content = strings.TrimRight(content, "\n\t ") + "\n\n" + otelSection
	}

	return os.WriteFile(configPath, []byte(strings.TrimSpace(content)+"\n"), 0644)
}

func resolveCodexOTELEndpoint(telemetry *api.TelemetryConfig, env map[string]string) string {
	if v := firstNonEmpty(
		resolveEnv("SCION_CODEX_OTEL_ENDPOINT", env),
		resolveEnv("SCION_OTEL_ENDPOINT", env),
	); v != "" {
		return v
	}
	if telemetry != nil && telemetry.Cloud != nil && telemetry.Cloud.Endpoint != "" {
		return telemetry.Cloud.Endpoint
	}
	return "localhost:4317"
}

func resolveCodexOTELProtocol(telemetry *api.TelemetryConfig, env map[string]string) string {
	if v := firstNonEmpty(
		resolveEnv("SCION_CODEX_OTEL_PROTOCOL", env),
		resolveEnv("SCION_OTEL_PROTOCOL", env),
	); v != "" {
		return v
	}
	if telemetry != nil && telemetry.Cloud != nil && telemetry.Cloud.Protocol != "" {
		return telemetry.Cloud.Protocol
	}
	return "grpc"
}

func resolveEnv(key string, env map[string]string) string {
	if env != nil {
		if v := strings.TrimSpace(env[key]); v != "" {
			return v
		}
	}
	return strings.TrimSpace(os.Getenv(key))
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func listContains(items []string, target string) bool {
	for _, item := range items {
		if strings.TrimSpace(item) == target {
			return true
		}
	}
	return false
}

func removeTOMLSection(content, section string) string {
	lines := strings.Split(content, "\n")
	target := "[" + section + "]"

	sectionStart := -1
	sectionEnd := len(lines)

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == target {
			sectionStart = i
			for j := i + 1; j < len(lines); j++ {
				t := strings.TrimSpace(lines[j])
				if strings.HasPrefix(t, "[") && strings.HasSuffix(t, "]") {
					sectionEnd = j
					break
				}
			}
			break
		}
	}

	if sectionStart == -1 {
		return content
	}

	// Also consume blank lines immediately before the section header.
	for sectionStart > 0 && strings.TrimSpace(lines[sectionStart-1]) == "" {
		sectionStart--
	}

	result := append(lines[:sectionStart], lines[sectionEnd:]...)
	return strings.Join(result, "\n")
}

func upsertTOMLKey(content, section, key, value string) string {
	lines := strings.Split(content, "\n")
	targetSection := strings.TrimSpace(section)

	sectionStart := 0
	sectionEnd := len(lines)
	currentSection := ""
	foundSection := targetSection == ""

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			sectionName := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(trimmed, "["), "]"))
			if currentSection == targetSection {
				sectionEnd = i
				break
			}
			currentSection = sectionName
			if sectionName == targetSection {
				foundSection = true
				sectionStart = i + 1
				sectionEnd = len(lines)
			}
		}
	}

	if targetSection == "" {
		sectionStart = 0
		for i, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
				sectionEnd = i
				break
			}
		}
	}

	if !foundSection && targetSection != "" {
		if strings.TrimSpace(content) != "" && !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		if strings.TrimSpace(content) != "" {
			content += "\n"
		}
		content += "[" + targetSection + "]\n" + key + " = " + value + "\n"
		return content
	}

	for i := sectionStart; i < sectionEnd; i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, key+" ") || strings.HasPrefix(line, key+"=") {
			lines[i] = key + " = " + value
			return strings.Join(lines, "\n")
		}
	}

	insertAt := sectionEnd
	newLine := key + " = " + value
	lines = append(lines[:insertAt], append([]string{newLine}, lines[insertAt:]...)...)
	return strings.Join(lines, "\n")
}
