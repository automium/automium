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

// ApplicationSpec defines the desired state of Application
type ApplicationSpec struct {
	// Name is the name of the extra application to deploy.
	Name string `json:"name"`
	// Version is the version of the extra application to deploy.
	Version string `json:"version"`
	// Cluster is the cluster name in Rancher.
	Cluster string `json:"cluster"`
	// Project is the Rancher project where the extra applicaton will be deployed.
	Project string `json:"project"`
	// Namespace is the Kubernetes target namespace. If it exists, it will be moved into the Project defined in this resource.
	Namespace string `json:"namespace"`
	// Parameters are the custom answers that will be provided when deploying the extra application.
	Parameters map[string]string `json:"parameters,omitempty"`
}

// ApplicationStatus defines the observed state of Application
type ApplicationStatus struct {
	// Phase is the deploy status as reported from Rancher.
	Phase string `json:"phase"`
	// Error is the error string populated if something wrong happend during the deploy.
	Error string `json:"error,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Application is the Schema for the applications API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Application",type="string",JSONPath=".spec.name",description="Application name"
// +kubebuilder:printcolumn:name="Cluster",type="string",JSONPath=".spec.cluster",description="Target cluster"
// +kubebuilder:printcolumn:name="Version",type="string",JSONPath=".spec.version",description="Monitoring stack version"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.phase",description="The execution phase"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
type Application struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ApplicationSpec   `json:"spec,omitempty"`
	Status ApplicationStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ApplicationList contains a list of Application
type ApplicationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Application `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Application{}, &ApplicationList{})
}
