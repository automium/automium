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

package node

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	corev1beta1 "github.com/automium/automium/pkg/apis/core/v1beta1"
	"github.com/golang/glog"
	consulapi "github.com/hashicorp/consul/api"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new Node Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
// USER ACTION REQUIRED: update cmd/manager/main.go to call this core.Add(mgr) to install this Controller
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileNode{Client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("node-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to Node
	err = c.Watch(&source.Kind{Type: &corev1beta1.Node{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	glog.Infoln("node controller initialized")

	return nil
}

var _ reconcile.Reconciler = &ReconcileNode{}

// ReconcileNode reconciles a Node object
type ReconcileNode struct {
	client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a Node object and makes changes based on the state read
// and what is in the Node.Spec
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=core.automium.io,resources=nodes;nodes/status,verbs=get;list;watch;create;update;patch;delete
func (r *ReconcileNode) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the Node instance
	instance := &corev1beta1.Node{}
	err := r.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	// Ask the reconciler to requery every 30 seconds
	refreshReconcilier := reconcile.Result{
		Requeue:      true,
		RequeueAfter: 30 * time.Second,
	}

	// Check if the node can be safely deleted
	if instance.Spec.DeletionDate != "" && instance.Status.NodeProperties.ID == "non-existent-machine" {
		glog.V(2).Infof("node %s has no ID and deletion date set (%s) - deleting\n", instance.Spec.Hostname, instance.Spec.Hostname)
		err := r.Delete(context.TODO(), instance)
		if err != nil {
			glog.Errorf("cannot delete node %s - %s\n", instance.Spec.Hostname, err.Error())
		} else {
			glog.V(2).Infof("node %s has no ID and deletion date set (%s) - deleting\n", instance.Spec.Hostname, instance.Spec.Hostname)
		}
		return refreshReconcilier, err
	}

	// Create a Consul client
	consulClient, err := consulapi.NewClient(&consulapi.Config{
		Address:    "consul.service.automium.consul:8500",
		Datacenter: "automium",
	})

	if err != nil {
		glog.Errorf("cannot connect to Consul -  %s\n", err.Error())
		// Send an errored status
		instance.Status.NodeProperties = corev1beta1.NodePropertiesStatusSpec{
			Address: "",
			ID:      "consul-connection-error",
			Node:    err.Error(),
		}
		instance.Status.NodeHealthChecks = make([]corev1beta1.NodeHealthCheckStatusSpec, 0)
		err = r.UpdateNodeStatusIfNeeded(instance, request.NamespacedName)
		if err != nil {
			glog.Errorf("cannot update node object: %s\n", err.Error())
		}
		return refreshReconcilier, err
	}

	// Retrieve data about the node
	nodeInfo, _, err := consulClient.Catalog().Node(instance.Spec.Hostname, &consulapi.QueryOptions{})
	if err != nil {
		glog.Errorf("cannot retrieve node info for node %s: %s\n", instance.Spec.Hostname, err.Error())
		return refreshReconcilier, err
	}

	if nodeInfo == nil {
		// Send an empty status
		instance.Status.NodeProperties = corev1beta1.NodePropertiesStatusSpec{
			Address: "",
			ID:      "non-existent-machine",
			Node:    "non-existent-machine",
		}
		instance.Status.NodeHealthChecks = make([]corev1beta1.NodeHealthCheckStatusSpec, 0)
		err = r.UpdateNodeStatusIfNeeded(instance, request.NamespacedName)
		if err != nil {
			glog.Errorf("cannot update node object: %s\n", err.Error())
			return refreshReconcilier, err
		}
		glog.V(2).Infof("instance %s not ready.\n", instance.Spec.Hostname)
		return refreshReconcilier, nil
	}

	// Retrieve health checks for the node
	nodeHC, _, err := consulClient.Health().Node(instance.Spec.Hostname, &consulapi.QueryOptions{})
	if err != nil {
		glog.Errorf("cannot retrieve node health checks for instance %s: %s\n", instance.Spec.Hostname, err.Error())
		return refreshReconcilier, err
	}

	nodeFIP := ""
	// Check if there is a node which indicates the floating ip
	fipNodeInfo, _, err := consulClient.Catalog().Node(fmt.Sprintf("external%s", instance.Spec.Hostname), &consulapi.QueryOptions{})
	if err != nil {
		glog.Warningf("cannot check the floating ip for instance %s: %s\n", instance.Spec.Hostname, err.Error())
	} else {
		if fipNodeInfo != nil {
			nodeFIP = fipNodeInfo.Node.Address
		}
	}

	// Populate node properties
	instance.Status.NodeProperties = corev1beta1.NodePropertiesStatusSpec{
		ID:            nodeInfo.Node.ID,
		Node:          nodeInfo.Node.Node,
		Address:       nodeInfo.Node.Address,
		PublicAddress: nodeFIP,
	}

	// Populate node health checks
	if len(nodeHC) > 0 {
		hcSpec := make([]corev1beta1.NodeHealthCheckStatusSpec, 0)
		for _, hc := range nodeHC {
			HCOutput := ""

			if len(hc.Output) >= 64 {
				HCOutput = hc.Output[0:60] + "..."
			} else {
				HCOutput = hc.Output
			}

			hcSpec = append(hcSpec, corev1beta1.NodeHealthCheckStatusSpec{
				CheckID:     hc.CheckID,
				Name:        hc.Name,
				Status:      hc.Status,
				Output:      HCOutput,
				ServiceID:   hc.ServiceID,
				ServiceName: hc.ServiceName,
			})
		}
		instance.Status.NodeHealthChecks = hcSpec
	}

	actualNode := &corev1beta1.Node{}
	err = r.Get(context.TODO(), request.NamespacedName, actualNode)
	// Update if is necessary
	err = r.UpdateNodeStatusIfNeeded(instance, request.NamespacedName)
	if err != nil {
		instance.ObjectMeta.ResourceVersion = ""
		err = r.Create(context.TODO(), instance)
		if err != nil {
			glog.Errorf("cannot create or update node object for instance %s: %s\n", instance.Spec.Hostname, err.Error())
			return refreshReconcilier, err
		}
		glog.V(2).Infof("node %s created\n", instance.Spec.Hostname)
	}

	return refreshReconcilier, nil
}

// UpdateNodeStatusIfNeeded updates a Node status only if there are differencies between the existing object and the modified object
func (r *ReconcileNode) UpdateNodeStatusIfNeeded(newNode *corev1beta1.Node, nameSpacedName types.NamespacedName) error {
	actualNode := &corev1beta1.Node{}
	err := r.Get(context.TODO(), nameSpacedName, actualNode)
	if err != nil {
		return err
	}
	if !CompareStatuses(actualNode.Status, newNode.Status) {
		err = r.Status().Update(context.Background(), newNode)
		if err != nil {
			return err
		}
	}
	return nil
}

// CompareStatuses compares two Node status. If there are equals, returns true
func CompareStatuses(old, new corev1beta1.NodeStatus) bool {

	oldStatusJSON, err := json.Marshal(old)
	if err != nil {
		glog.Errorf("cannot marshal old status: %s\n", err.Error())
		return false
	}

	newStatusJSON, err := json.Marshal(new)
	if err != nil {
		glog.Errorf("cannot marshal new status: %s\n", err.Error())
		return false
	}

	if string(oldStatusJSON) == string(newStatusJSON) {
		return true
	} else {
		return false
	}
}
