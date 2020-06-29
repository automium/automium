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

// NodeHealthCheckStatusSpec contains the healthchecks for the node obtained from Consul
type NodeHealthCheckStatusSpec struct {
	// CheckID is the ID of the Consul health check.
	CheckID string `json:"checkID"`
	// Name is the name of the Consul health check.
	Name string `json:"name"`
	// Status is the status of the Consul health check.
	// Can be "passing", "warning" or "critical"
	Status string `json:"status"`
	// Output is the output of the Consul health check command.
	// 64 chars maximum
	Output string `json:"output"`
	// ServiceID is the ID of the Consul service this check belongs.
	ServiceID string `json:"serviceID"`
	// ServiceName is the name of the Consul service this check belongs.
	ServiceName string `json:"serviceName"`
}

// NodePropertiesStatusSpec contains the basic data of node obtained from Consul
type NodePropertiesStatusSpec struct {
	// ID is the node ID on Consul.
	// If the node is not registered, this field will be populated with "non-existent-machine".
	ID string `json:"id"`
	// Node is the node name on Consul.
	// If the node is not registered, this field will be populated with "non-existent-machine".
	Node string `json:"node"`
	// Address is the node private address.
	Address string `json:"address"`
	// PublicAddress is the node public address. Empty if the node have only the private IP.
	PublicAddress string `json:"publicAddress"`
	// Flavor is the node flavor. Can be a cloud-provider flavor or a string combination of vCPU-Memory-DiskSize.
	Flavor string `json:"flavor"`
	// Image is the base image from which the node has been launched.
	Image string `json:"image"`
}

// NodeSpec defines the desired state of Node
type NodeSpec struct {
	// Hostname specifies the node which will be tracked by this resource.
	Hostname string `json:"hostname"`
	// DeletionDate is the date timestamp on which the Node resource has been marked for deletion.
	// This field is automatically populated by the controller when needed.
	DeletionDate string `json:"deletionDate"`
}

// NodeStatus defines the observed state of Node
type NodeStatus struct {
	// NodeProperties contains the node information.
	NodeProperties NodePropertiesStatusSpec `json:"nodeProperties,omitempty"`
	// NodeHealthChecks contains the Consul health checks related to this node.
	NodeHealthChecks []NodeHealthCheckStatusSpec `json:"nodeHealthChecks,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Node is the Schema for the nodes API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Hostname",type="string",JSONPath=".spec.hostname",description="the node hostname"
// +kubebuilder:printcolumn:name="Internal-IP",type="string",JSONPath=".status.nodeProperties.address",description="the node private IP"
// +kubebuilder:printcolumn:name="External-IP",type="string",JSONPath=".status.nodeProperties.publicAddress",description="the node public IP"
// +kubebuilder:printcolumn:name="Image",type="string",JSONPath=".status.nodeProperties.image",description="the image deployed on node"
// +kubebuilder:printcolumn:name="Flavor",type="string",JSONPath=".status.nodeProperties.flavor",description="the flavor deployed on node"
// +kubebuilder:printcolumn:name="Service",type="string",JSONPath=".metadata.annotations['service\.automium\.io/name']",description="Service which the node belongs to"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
type Node struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NodeSpec   `json:"spec,omitempty"`
	Status NodeStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// NodeList contains a list of Node
type NodeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Node `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Node{}, &NodeList{})
}
