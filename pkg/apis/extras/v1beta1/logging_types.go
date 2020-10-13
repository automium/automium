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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LoggingSpec defines the desired state of Logging
type LoggingSpec struct {
	// Cluster is the Rancher name for the target cluster.
	Cluster string `json:"cluster"`
	// Version is the version of the Logging application to deploy.
	Version string `json:"version"`
	// Parameters are custom answers for the Logging application.
	Parameters map[string]string `json:"parameters,omitempty"`
}

// LoggingStatus defines the observed state of Logging
type LoggingStatus struct {
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Logging is the Schema for the loggings API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Cluster",type="string",JSONPath=".spec.cluster",description="Target cluster"
// +kubebuilder:printcolumn:name="Version",type="string",JSONPath=".spec.version",description="Logging stack version"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
type Logging struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LoggingSpec   `json:"spec,omitempty"`
	Status LoggingStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// LoggingList contains a list of Logging
type LoggingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Logging `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Logging{}, &LoggingList{})
}
