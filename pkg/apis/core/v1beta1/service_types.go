/*
Copyright 2018 The Automium Authors.

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

package v1beta1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ServiceSpec defines the desired state of Service
type ServiceSpec struct {
	// Replicas is the number of service replicas.
	// +kubebuilder:validation:Minimum=0
	Replicas int `json:"replicas"`
	// Flavor is the flavor which will be used for node instances.
	Flavor string `json:"flavor"`
	// Version is the image version which will be used for the nodes.
	Version string `json:"version"`
	// Tags is an optional field for service tagging. Not used at the moment.
	Tags []string `json:"tags,omitempty"`
	// Env is an array of EnvVar used for configuring the service.
	Env []corev1.EnvVar `json:"env,omitempty"`
	// Extra is an array of ExtraSpec used for configuring the extra components, such as monitoring or backups, for this service.
	Extra []ExtraSpec `json:"extra,omitempty"`
}

// ExtraSpec defines the Extra field format
type ExtraSpec struct {
	// Name is the extra component name.
	Name string `json:"name"`
	// Version is the extra component version.
	Version string `json:"version"`
	// Parameters is a map which can be used to further configure the extra component.
	Parameters map[string]string `json:"parameters,omitempty"`
}

// ServiceStatus defines the observed state of Service
type ServiceStatus struct {
	// Phase is the service phase. It can be "Running", "Completed", "Pending" or "Failed".
	Phase string `json:"phase"`
	// ModuleRef contains the reference to the module used by this service.
	ModuleRef string `json:"moduleRef"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Service is the Schema for the services API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
// +kubebuilder:subresource:scale:specpath=.spec.replicas,statuspath=.status.replicas
// +kubebuilder:printcolumn:name="Application",type="string",JSONPath=".metadata.labels['app']",description="The desired application"
// +kubebuilder:printcolumn:name="Version",type="string",JSONPath=".spec.version",description="The application version"
// +kubebuilder:printcolumn:name="Replicas",type="string",JSONPath=".spec.replicas",description="The application replicas"
// +kubebuilder:printcolumn:name="Flavor",type="string",JSONPath=".spec.flavor",description="The instance flavor"
// +kubebuilder:printcolumn:name="Module",type="string",JSONPath=".status.moduleRef",description="module.core.automium.io reference"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.phase",description="The execution phase"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
type Service struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServiceSpec   `json:"spec,omitempty"`
	Status ServiceStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ServiceList contains a list of Service
type ServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Service `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Service{}, &ServiceList{})
}
