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

package service

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"

	corev1beta1 "github.com/automium/automium/pkg/apis/core/v1beta1"
	"github.com/golang/glog"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new Service Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
// USER ACTION REQUIRED: update cmd/manager/main.go to call this core.Add(mgr) to install this Controller
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileService{Client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("service-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to Service
	err = c.Watch(&source.Kind{Type: &corev1beta1.Service{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch the Module created by Service
	err = c.Watch(&source.Kind{Type: &corev1beta1.Module{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &corev1beta1.Service{},
	})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileService{}

// ReconcileService reconciles a Service object
type ReconcileService struct {
	client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a Service object and makes changes based on the state read
// and what is in the Service.Spec
// Automatically generate RBAC rules
// +kubebuilder:rbac:groups=core.automium.io,resources=services,verbs=get;list;watch;create;update;patch;delete
func (r *ReconcileService) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the Service instance
	instance := &corev1beta1.Service{}
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

	// Change variable name in order to be used by Terraform directly
	tfEnvVars := make([]corev1.EnvVar, 0)
	for _, item := range instance.Spec.Env {
		tfEnvVars = append(tfEnvVars, corev1.EnvVar{
			Name:  fmt.Sprintf("TF_VAR_%s", item.Name),
			Value: item.Value,
		})
	}

	// Select the cloud provider to use (defaults to openstack)
	var cloudPlatform string

	switch os.Getenv("PLATFORM") {
	case "vcd":
		cloudPlatform = "vcd"
	case "vsphere":
		cloudPlatform = "vsphere"
	case "aws":
		cloudPlatform = "aws"
	default:
		cloudPlatform = "openstack"
	}

	// Append the cloud provider and name
	tfEnvVars = append(tfEnvVars,
		corev1.EnvVar{
			Name:  "PROVIDER",
			Value: cloudPlatform,
		},
		corev1.EnvVar{
			Name:  "NAME",
			Value: instance.Name,
		},
	)

	// Prepare provisioner name and specific env variables for specific service
	var appProvisioner string
	var specificEnvVars []corev1.EnvVar

	switch instance.ObjectMeta.Labels["app"] {
	case "kubernetes-cluster":
		specificEnvVars = []corev1.EnvVar{
			{
				Name:  "MASTER",
				Value: "true",
			},
			{
				Name:  "NODE",
				Value: "false",
			},
			{
				Name:  "ETCD",
				Value: "true",
			},
			{
				Name:  "CLUSTER_NAME",
				Value: instance.Name,
			},
		}
		appProvisioner = "kubernetes"
	case "kubernetes-nodepool":
		specificEnvVars = []corev1.EnvVar{
			{
				Name:  "MASTER",
				Value: "false",
			},
			{
				Name:  "NODE",
				Value: "true",
			},
			{
				Name:  "ETCD",
				Value: "false",
			},
		}
		appProvisioner = "kubernetes"
	default:
		appProvisioner = instance.ObjectMeta.Labels["app"]
	}
	// Append new prepared variables
	tfEnvVars = append(tfEnvVars, specificEnvVars...)

	// Define the desired Module object
	deploy := &corev1beta1.Module{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-module", instance.Name),
			Namespace: instance.Namespace,
		},
		Spec: corev1beta1.ModuleSpec{
			Source:   appProvisioner,
			Image:    fmt.Sprintf("%s-%s", appProvisioner, instance.Spec.Version),
			Flavor:   instance.Spec.Flavor,
			Replicas: instance.Spec.Replicas,
			Env:      tfEnvVars,
		},
	}

	if err := controllerutil.SetControllerReference(instance, deploy, r.scheme); err != nil {
		return reconcile.Result{}, err
	}

	// Check if the Module already exists
	found := &corev1beta1.Module{}
	err = r.Get(context.TODO(), types.NamespacedName{Name: deploy.Name, Namespace: deploy.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		glog.Infof("creating module %s/%s\n", deploy.Namespace, deploy.Name)
		deploy.Spec.Action = "Deploy"
		err = r.Create(context.TODO(), deploy)
		if err != nil {
			return reconcile.Result{}, err
		}
	} else if err != nil {
		return reconcile.Result{}, err
	}

	// Update the found object and write the result back if there are any changes
	if isModuleDifferent(found, deploy) {
		glog.V(2).Infof("updating module %s/%s\n", deploy.Namespace, deploy.Name)
		deploy.Spec.Action = deployAction(found.Spec.Replicas, deploy.Spec.Replicas, found.Spec.Image, deploy.Spec.Image, found.Spec.Flavor, deploy.Spec.Flavor)
		found.Spec = deploy.Spec
		err = r.Update(context.TODO(), found)
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	appName := instance.ObjectMeta.Labels["app"] // this will be for example 'haproxy'

	// Retrieve all nodes for this namespace
	nsNodes := &corev1beta1.NodeList{}
	err = r.List(context.TODO(), &client.ListOptions{Namespace: deploy.Namespace}, nsNodes)
	if err != nil {
		glog.Errorf("cannot get nodes for namespace %s: %s\n", deploy.Namespace, err.Error())
		return reconcile.Result{}, err
	}

	// Search for existent nodes for service
	appNodes := make([]corev1beta1.Node, 0)
	for _, node := range nsNodes.Items {
		if node.ObjectMeta.Annotations["service.automium.io/name"] == instance.Name {
			glog.V(2).Infof("found node %s for app %s\n", node.Spec.Hostname, instance.Name)
			appNodes = append(appNodes, node)
		}
	}

	// Special cases (nonexistent, delete all)
	if instance.Spec.Replicas == 0 && len(appNodes) > 0 {
		// Delete all nodes
		glog.V(2).Infof("service %s has no replicas - removing all nodes.\n", instance.Name)
		for _, item := range appNodes {
			r.Delete(context.TODO(), &item)
		}
		return reconcile.Result{}, nil
	}

	if len(appNodes) == 0 || len(appNodes) < instance.Spec.Replicas {
		// Add all items

		replicasCount := instance.Spec.Replicas
		if appName == "orchestrator" {
			replicasCount = 3 * instance.Spec.Replicas // In this case a replica consists in 3 VMs
		}

		for i := 0; i < replicasCount; i++ {
			var specHostname string
			switch appName {
			case "kubernetes-cluster":
				specHostname = fmt.Sprintf("%s-%s-%d", instance.Name, instance.Name, i)
			case "kubernetes-nodepool":
				var clusterName string

				for _, val := range instance.Spec.Env {
					if val.Name == "cluster_name" {
						clusterName = val.Value
					}
				}

				if clusterName == "" {
					glog.Warningln("nodes for Kubenernetes nodepool requested but empty cluster_name provided -- marking as 'nocluster' nodepool")
					clusterName = "nocluster"
				}

				specHostname = fmt.Sprintf("%s-%s-%d", clusterName, instance.Name, i)
			default:
				specHostname = fmt.Sprintf("%s-%d", appName, i)
			}
			r.Create(context.TODO(), &corev1beta1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-node-%d", instance.Name, i),
					Namespace: instance.Namespace,
					Annotations: map[string]string{
						"service.automium.io/name": instance.Name,
					},
				},
				Spec: corev1beta1.NodeSpec{
					Hostname:     specHostname,
					DeletionDate: "",
				},
			})
		}
		return reconcile.Result{}, nil
	}

	if len(appNodes) > instance.Spec.Replicas && appName != "orchestrator" {
		err := sortNodesByNumber(appNodes)
		if err != nil {
			glog.Warningf("cannot sorting nodes for removal: %s -- skipping\n", err.Error())
			return reconcile.Result{}, err
		}
		glog.V(2).Infoln("sorted nodes:")
		for idx, item := range appNodes {
			glog.V(2).Infof("[%d] %s\n", idx, item.Spec.Hostname)
		}
		arrToDelete := appNodes[instance.Spec.Replicas:len(appNodes)]
		glog.V(2).Infof("service %s - node replicas: %d -> %d\n", appName, len(appNodes), instance.Spec.Replicas)

		for _, item := range arrToDelete {
			glog.V(2).Infof("service: %s - marking for deletion node %s\n", appName, item.Spec.Hostname)
			item.Spec.DeletionDate = time.Now().String()
			r.Update(context.TODO(), &item)
		}
	}

	if appName == "orchestrator" && (len(appNodes) > instance.Spec.Replicas*3) {
		err := sortNodesByNumber(appNodes)
		if err != nil {
			glog.Warningf("cannot sorting nodes for removal: %s -- skipping\n", err.Error())
			return reconcile.Result{}, err
		}
		glog.V(2).Infoln("sorted nodes:")
		for idx, item := range appNodes {
			glog.V(2).Infof("[%d] %s\n", idx, item.Spec.Hostname)
		}
		arrToDelete := appNodes[instance.Spec.Replicas:len(appNodes)]
		glog.V(2).Infof("service %s - node replicas: %d -> %d\n", appName, len(appNodes), instance.Spec.Replicas)
		for _, item := range arrToDelete {
			glog.V(2).Infof("service: %s - marking for deletion node %s\n", appName, item.Spec.Hostname)
			item.Spec.DeletionDate = time.Now().String()
			r.Update(context.TODO(), &item)
		}
	}

	return reconcile.Result{}, nil
}

func sortNodesByNumber(nodes []corev1beta1.Node) error {
	// TODO: improve this
	var globalErr error
	re := regexp.MustCompile("[0-9]+")
	sort.Slice(nodes, func(i, j int) bool {
		if globalErr != nil {
			return false
		}
		itm1, err := strconv.Atoi(re.FindAllString(nodes[i].Spec.Hostname, -1)[0])
		if err != nil {
			globalErr = err
		}
		itm2, err := strconv.Atoi(re.FindAllString(nodes[j].Spec.Hostname, -1)[0])
		if err != nil {
			globalErr = err
		}
		return itm1 < itm2
	})
	return globalErr
}

func deployAction(actualReplicas, nextReplicas int, actualVersion, nextVersion, actualFlavor, nextFlavor string) string {
	if nextReplicas == 0 {
		return "Destroy"
	}

	if actualFlavor == nextFlavor && actualVersion == nextVersion {
		return "Deploy"
	}

	if actualReplicas != nextReplicas {
		return "DeployAndUpgrade"
	}

	return "Upgrade"

}

func isModuleDifferent(actual, new *corev1beta1.Module) bool {
	if actual.Spec.Flavor == new.Spec.Flavor && actual.Spec.Image == new.Spec.Image && actual.Spec.Replicas == new.Spec.Replicas {
		return false
	}
	return true
}
