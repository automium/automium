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

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// NodeHealthCheckStatusSpec contains the healthchecks for the node obtained from Consul
type NodeHealthCheckStatusSpec struct {
	CheckID     string `json:"checkID"`
	Name        string `json:"name"`
	Status      string `json:"status"`
	Output      string `json:"output"`
	ServiceID   string `json:"serviceID"`
	ServiceName string `json:"serviceName"`
}

// NodePropertiesStatusSpec contains the basic data of node obtained from Consul
type NodePropertiesStatusSpec struct {
	ID            string `json:"id"`
	Node          string `json:"node"`
	Address       string `json:"address"`
	PublicAddress string `json:"publicAddress"`
}

// NodeServiceStatusSpec contains the basic data of service on a node obtained from Consul
// type NodeServiceStatusSpec struct {
// 	ID      string `json:"id"`
// 	Service string `json:"service"`
// 	Port    int    `json:"port"`
// 	Address string `json:"address"`
// }

// NodeServicesStatusSpec is a struct which contains the service name and the service data obtained from Consul
// type NodeServicesStatusSpec struct {
// 	ServiceName string                `json:"serviceName"`
// 	ServiceData NodeServiceStatusSpec `json:"serviceData"`
// }

// NodeSpec defines the desired state of Node
type NodeSpec struct {
	Hostname     string `json:"hostname"`
	DeletionDate string `json:"deletionDate"`
}

// NodeStatus defines the observed state of Node
type NodeStatus struct {
	NodeProperties NodePropertiesStatusSpec `json:"nodeProperties,omitempty"`
	//NodeServices     []NodeServicesStatusSpec    `json:"nodeServices,omitempty"`
	NodeHealthChecks []NodeHealthCheckStatusSpec `json:"nodeHealthChecks,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Node is the Schema for the nodes API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
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
