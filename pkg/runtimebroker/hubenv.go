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

package runtimebroker

import "github.com/ptone/scion-agent/pkg/config"

const redactedEnvValue = "<redacted>"

var safeEnvLogKeys = map[string]struct{}{
	"SCION_AGENT_ID":          {},
	"SCION_AGENT_SLUG":        {},
	"SCION_BROKER_ID":         {},
	"SCION_BROKER_NAME":       {},
	"SCION_CREATOR":           {},
	"SCION_DEBUG":             {},
	"SCION_GROVE_ID":          {},
	"SCION_TELEMETRY_ENABLED": {},
}

func resolveHubEndpointForCreate(reqHubEndpoint, brokerHubEndpoint string, resolvedEnv map[string]string, grovePath, containerHubEndpoint, runtimeName string) string {
	hubEndpoint := reqHubEndpoint
	if hubEndpoint == "" {
		hubEndpoint = brokerHubEndpoint
	}
	if hubEndpoint == "" {
		hubEndpoint = hubEndpointFromResolvedEnv(resolvedEnv)
	}
	if hubEndpoint == "" {
		hubEndpoint = hubEndpointFromGroveSettings(grovePath)
	}
	return applyContainerBridgeOverride(hubEndpoint, containerHubEndpoint, runtimeName)
}

func resolveHubEndpointForStart(brokerHubEndpoint string, resolvedEnv map[string]string, grovePath, containerHubEndpoint, runtimeName string) string {
	hubEndpoint := brokerHubEndpoint
	if hubEndpoint == "" {
		hubEndpoint = hubEndpointFromResolvedEnv(resolvedEnv)
	}
	if hubEndpoint == "" {
		hubEndpoint = hubEndpointFromGroveSettings(grovePath)
	}
	return applyContainerBridgeOverride(hubEndpoint, containerHubEndpoint, runtimeName)
}

func hubEndpointFromResolvedEnv(resolvedEnv map[string]string) string {
	if ep, ok := resolvedEnv["SCION_HUB_ENDPOINT"]; ok && ep != "" {
		return ep
	}
	if ep, ok := resolvedEnv["SCION_HUB_URL"]; ok && ep != "" {
		return ep
	}
	return ""
}

func hubEndpointFromGroveSettings(grovePath string) string {
	if grovePath == "" {
		return ""
	}
	settingsDir := resolveGroveSettingsDir(grovePath)
	groveSettings, err := config.LoadSettingsFromDir(settingsDir)
	if err != nil || groveSettings.IsHubExplicitlyDisabled() {
		return ""
	}
	return groveSettings.GetHubEndpoint()
}

func applyContainerBridgeOverride(endpoint, containerHubEndpoint, runtimeName string) string {
	if containerHubEndpoint == "" || runtimeName == "kubernetes" || !isLocalhostEndpoint(endpoint) {
		return endpoint
	}
	return containerHubEndpoint
}

func redactEnvValueForLog(key, value string) string {
	if _, ok := safeEnvLogKeys[key]; ok {
		return value
	}
	return redactedEnvValue
}
