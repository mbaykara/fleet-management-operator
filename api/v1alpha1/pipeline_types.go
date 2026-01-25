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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ConfigType represents the type of collector configuration
// +kubebuilder:validation:Enum=Alloy;OpenTelemetryCollector
type ConfigType string

const (
	// ConfigTypeAlloy represents Grafana Alloy configuration syntax
	ConfigTypeAlloy ConfigType = "Alloy"

	// ConfigTypeOpenTelemetryCollector represents OpenTelemetry Collector configuration syntax
	ConfigTypeOpenTelemetryCollector ConfigType = "OpenTelemetryCollector"
)

// ToFleetAPI converts CRD ConfigType to Fleet Management API format
func (c ConfigType) ToFleetAPI() string {
	switch c {
	case ConfigTypeAlloy:
		return "CONFIG_TYPE_ALLOY"
	case ConfigTypeOpenTelemetryCollector:
		return "CONFIG_TYPE_OTEL"
	default:
		return "CONFIG_TYPE_ALLOY"
	}
}

// ConfigTypeFromFleetAPI converts Fleet Management API format to CRD ConfigType
func ConfigTypeFromFleetAPI(apiType string) ConfigType {
	switch apiType {
	case "CONFIG_TYPE_OTEL":
		return ConfigTypeOpenTelemetryCollector
	case "CONFIG_TYPE_ALLOY":
		return ConfigTypeAlloy
	default:
		return ConfigTypeAlloy
	}
}

// PipelineSpec defines the desired state of Pipeline
type PipelineSpec struct {
	// Name of the pipeline (unique identifier in Fleet Management)
	// If not specified, uses metadata.name
	// +optional
	Name string `json:"name,omitempty"`

	// Contents of the pipeline configuration (Alloy or OpenTelemetry Collector config)
	// +required
	// +kubebuilder:validation:MinLength=1
	Contents string `json:"contents"`

	// Matchers to assign pipeline to collectors
	// Prometheus Alertmanager syntax: key=value, key!=value, key=~regex, key!~regex
	// +optional
	// +kubebuilder:validation:MaxItems=100
	Matchers []string `json:"matchers,omitempty"`

	// Enabled indicates whether the pipeline is enabled for collectors
	// +optional
	// +kubebuilder:default=true
	Enabled bool `json:"enabled"`

	// ConfigType specifies the type of configuration (Alloy or OpenTelemetryCollector)
	// +optional
	// +kubebuilder:default=Alloy
	ConfigType ConfigType `json:"configType,omitempty"`
}

// PipelineStatus defines the observed state of Pipeline.
type PipelineStatus struct {
	// ID is the server-assigned pipeline ID from Fleet Management
	// +optional
	ID string `json:"id,omitempty"`

	// ObservedGeneration reflects the generation of the most recently observed Pipeline spec
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// CreatedAt is the timestamp when the pipeline was created in Fleet Management
	// +optional
	CreatedAt *metav1.Time `json:"createdAt,omitempty"`

	// UpdatedAt is the timestamp when the pipeline was last updated in Fleet Management
	// +optional
	UpdatedAt *metav1.Time `json:"updatedAt,omitempty"`

	// RevisionID is the current revision ID from Fleet Management
	// +optional
	RevisionID string `json:"revisionId,omitempty"`

	// Conditions represent the current state of the Pipeline resource.
	//
	// Standard condition types:
	// - "Ready": Pipeline is successfully synced to Fleet Management
	// - "Synced": Last reconciliation succeeded
	//
	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=fmp
// +kubebuilder:printcolumn:name="Enabled",type="boolean",JSONPath=".spec.enabled"
// +kubebuilder:printcolumn:name="Config Type",type="string",JSONPath=".spec.configType"
// +kubebuilder:printcolumn:name="Fleet ID",type="string",JSONPath=".status.id"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Pipeline is the Schema for the pipelines API
type Pipeline struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of Pipeline
	// +required
	Spec PipelineSpec `json:"spec"`

	// status defines the observed state of Pipeline
	// +optional
	Status PipelineStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// PipelineList contains a list of Pipeline
type PipelineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []Pipeline `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Pipeline{}, &PipelineList{})
}
