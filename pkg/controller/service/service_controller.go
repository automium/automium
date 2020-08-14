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
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"

	corev1beta1 "github.com/automium/automium/pkg/apis/core/v1beta1"
	"github.com/automium/automium/pkg/slack"
	"github.com/automium/automium/pkg/utils"
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
	return &ReconcileService{Client: mgr.GetClient(), scheme: mgr.GetScheme(), recorder: mgr.GetRecorder("service-controller")}
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

	glog.Infoln("service controller initialized")

	return nil
}

var _ reconcile.Reconciler = &ReconcileService{}

// ReconcileService reconciles a Service object
type ReconcileService struct {
	client.Client
	scheme   *runtime.Scheme
	recorder record.EventRecorder
}

// Reconcile reads that state of the cluster for a Service object and makes changes based on the state read
// and what is in the Service.Spec
// Automatically generate RBAC rules
// +kubebuilder:rbac:groups=core.automium.io,resources=services;services/status,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
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
			Name:  strings.ToUpper(item.Name),
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
		}

		// Check if a custom CLUSTER_NAME is defined in the service env
		customClusterNameFound := false
		for _, itm := range tfEnvVars {
			if itm.Name == "CLUSTER_NAME" && itm.Value != "" {
				customClusterNameFound = true
				break
			}
		}

		if !customClusterNameFound {
			specificEnvVars = append(specificEnvVars, corev1.EnvVar{
				Name:  "CLUSTER_NAME",
				Value: instance.Name,
			})
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
			Annotations: map[string]string{
				"service.automium.io/name":      instance.Name,
				"module.automium.io/appName":    instance.ObjectMeta.Labels["app"],
				"module.automium.io/appVersion": instance.Spec.Version,
			},
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
		err = r.Create(context.TODO(), deploy)
		if err != nil {
			glog.Infof("cannot create module %s: %s\n", deploy.Name, err.Error())
			return reconcile.Result{}, err
		}
		r.recorder.Event(instance, "Normal", "Created", "Service created")
		return reconcile.Result{}, nil
	} else if err != nil {
		glog.Infof("cannot get module %s: %s\n", deploy.Name, err.Error())
		return reconcile.Result{}, err
	}

	// Update the found object and write the result back if there are any changes
	if utils.IsModuleDifferent(found, deploy) {
		found.Spec = deploy.Spec
		glog.V(2).Infof("updating module %s/%s\n", deploy.Namespace, deploy.Name)
		err = r.Update(context.TODO(), found)
		r.recorder.Event(instance, "Normal", "Updated", "Service updated")
		if err != nil {
			return reconcile.Result{}, err
		}

		// Refresh the instance
		err := r.Get(context.TODO(), request.NamespacedName, instance)
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	// Update the status from the module if needed
	if instance.Status.Phase != found.Status.Phase {
		instance.Status.Phase = found.Status.Phase
		instance.Status.ModuleRef = found.Name

		err = r.Status().Update(context.Background(), instance)
		if err != nil {
			glog.Errorf("cannot update service status: %s", err.Error())
			r.recorder.Eventf(instance, "Warning", "StatusUpdateFailed", "Cannot update service status: %s", err.Error())
			return reconcile.Result{RequeueAfter: 5 * time.Second}, nil
		}
		glog.V(5).Infof("Service phase is not equal - send notification if enabled")
		err := utils.NotifyStatusViaSlack(instance.Name, instance.Status.Phase)
		if err != nil && err != slack.ErrWebhookNotEnabled {
			glog.Warningf("cannot send message via Slack webhook: %s", err)
			r.recorder.Eventf(instance, "Warning", "SlackWebhookNotificationFailed", "Failed to send notification to Slack: %s", err.Error())
		}
	}

	if found.Status.Phase == corev1beta1.StatusPhasePending || found.Status.Phase == corev1beta1.StatusPhaseRunning {
		glog.V(2).Infof("service module %s is in Pending or Running -- reschedule update in 15s.\n", instance.Name)
		return reconcile.Result{RequeueAfter: 15 * time.Second}, nil
	}

	// If it's a master deployment and it is in Completed phase, then manage the extra resources
	if instance.Status.Phase == corev1beta1.StatusPhaseCompleted && instance.ObjectMeta.Labels["app"] == "kubernetes-cluster" {
		glog.V(5).Infof("Managing extras for service %s/%s\n", instance.Namespace, instance.Name)
		err := r.ManageExtras(instance)
		if err != nil {
			glog.Errorf("cannot manage Extra resource for cluster %s: %s", instance.Name, err.Error())
			return reconcile.Result{}, err
		}
	}

	return reconcile.Result{RequeueAfter: 30 * time.Second}, nil
}
