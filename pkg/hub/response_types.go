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

package hub

import (
	"github.com/ptone/scion-agent/pkg/store"
)

// AgentWithCapabilities wraps a store.Agent with capability annotations.
type AgentWithCapabilities struct {
	store.Agent
	Cap *Capabilities `json:"_capabilities,omitempty"`
}

// GroveWithCapabilities wraps a store.Grove with capability annotations.
type GroveWithCapabilities struct {
	store.Grove
	Cap *Capabilities `json:"_capabilities,omitempty"`
}

// TemplateWithCapabilities wraps a store.Template with capability annotations.
type TemplateWithCapabilities struct {
	store.Template
	Cap *Capabilities `json:"_capabilities,omitempty"`
}

// GroupWithCapabilities wraps a store.Group with capability annotations.
type GroupWithCapabilities struct {
	store.Group
	Cap *Capabilities `json:"_capabilities,omitempty"`
}

// UserWithCapabilities wraps a store.User with capability annotations.
type UserWithCapabilities struct {
	store.User
	Cap *Capabilities `json:"_capabilities,omitempty"`
}

// PolicyWithCapabilities wraps a store.Policy with capability annotations.
type PolicyWithCapabilities struct {
	store.Policy
	Cap *Capabilities `json:"_capabilities,omitempty"`
}
