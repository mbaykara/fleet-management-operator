/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package fleetclient

import "time"

// Pipeline represents a Fleet Management pipeline
type Pipeline struct {
	Name       string     `json:"name"`
	Contents   string     `json:"contents"`
	Matchers   []string   `json:"matchers,omitempty"`
	Enabled    bool       `json:"enabled"`
	ID         string     `json:"id,omitempty"`
	ConfigType string     `json:"configType,omitempty"`
	Source     *Source    `json:"source,omitempty"`
	CreatedAt  *time.Time `json:"createdAt,omitempty"`
	UpdatedAt  *time.Time `json:"updatedAt,omitempty"`
}

// Source represents the origin of a pipeline
type Source struct {
	Type      string `json:"type,omitempty"`
	Namespace string `json:"namespace,omitempty"`
}

// UpsertPipelineRequest is the request to create or update a pipeline
type UpsertPipelineRequest struct {
	Pipeline     *Pipeline `json:"pipeline"`
	ValidateOnly bool      `json:"validateOnly,omitempty"`
}

// FleetAPIError represents an error from the Fleet Management API
type FleetAPIError struct {
	StatusCode int
	Operation  string
	Message    string
}

func (e *FleetAPIError) Error() string {
	return e.Message
}
