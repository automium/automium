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

package monitoring

import (
	"context"
	"fmt"
	"time"

	extrasv1beta1 "github.com/automium/automium/pkg/apis/extras/v1beta1"
	"github.com/golang/glog"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller")

// Add creates a new Monitoring Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileMonitoring{Client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {

	// Check if the environment variables are set
	if err := doEnvCheck(); err != nil {
		return fmt.Errorf("cannot validate controller environment: %s", err.Error())
	}

	// Create a new controller
	c, err := controller.New("monitoring-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to Monitoring
	err = c.Watch(&source.Kind{Type: &extrasv1beta1.Monitoring{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileMonitoring{}

// ReconcileMonitoring reconciles a Monitoring object
type ReconcileMonitoring struct {
	client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a Monitoring object and makes changes based on the state read
// and what is in the Monitoring.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  The scaffolding writes
// a Deployment as an example
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=extras.automium.io,resources=monitorings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=extras.automium.io,resources=monitorings/status,verbs=get;update;patch
func (r *ReconcileMonitoring) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the Monitoring instance
	instance := &extrasv1beta1.Monitoring{}
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

	glog.V(2).Infof("starting monitoring reconciliation for cluster %s", instance.Spec.Cluster)

	// Check if we have an active cluster with the specified name
	clusterID, err := getRancherClusterIDFromName(instance.Spec.Cluster)
	if err != nil {
		glog.Errorf("cannot get ID for cluster: %s -- requery in 10s\n", err.Error())
		return reconcile.Result{RequeueAfter: 10 * time.Second}, nil
	}

	glog.V(5).Infof("got cluster ID: %s\n", clusterID)

	// Retrieve the System project ID
	systemProjectID, err := getRancherProjectIDFromName(clusterID, "System")
	if err != nil {
		glog.Errorf("cannot get ID for System project: %s -- requery in 10s\n", err.Error())
		return reconcile.Result{RequeueAfter: 10 * time.Second}, nil
	}

	glog.V(5).Infof("got System project ID: %s\n", systemProjectID)

	// Retrieve the apps inside the System project
	systemApps, err := getRancherAppsFromProject(systemProjectID)
	if err != nil {
		glog.Errorf("cannot get apps for System project: %s -- requery in 10s\n", err.Error())
		return reconcile.Result{RequeueAfter: 10 * time.Second}, nil
	}

	glog.V(5).Infof("found apps: %+v\n", systemApps)

	// Check if the monitoring app has been deployed
	monitApp := appItem{Name: "does-not-exists"}
	for _, app := range systemApps {
		if app.Name == "automium-monitoring" {
			monitApp = app
			break
		}
	}

	if monitApp.Name == "automium-monitoring" {
		// Check the version and if necessary update
		if monitApp.Version != instance.Spec.Version {
			// Upgrade
			glog.V(2).Infof("upgrade needed for monitoring app on cluster %s -- executing\n", instance.Spec.Cluster)
			if err := r.updateStatus(instance, "Upgrading"); err != nil {
				return reconcile.Result{}, err
			}
			err := deployMonitoringApp(clusterID, systemProjectID, instance.Spec.Version, false)
			if err != nil {
				glog.Errorf("cannot install monitoring app in System project: %s -- requery in 10s\n", err.Error())
				return reconcile.Result{RequeueAfter: 10 * time.Second}, nil
			}
		} else {
			glog.V(2).Infof("monitoring app on cluster %s is up-to-date\n", instance.Spec.Cluster)
		}
	} else {
		// New installation
		glog.V(2).Infof("new installation of monitoring app requested on cluster %s -- executing\n", instance.Spec.Cluster)
		if err := r.updateStatus(instance, "Deploying"); err != nil {
			return reconcile.Result{}, err
		}
		err := deployMonitoringApp(clusterID, systemProjectID, instance.Spec.Version, true)
		if err != nil {
			glog.Errorf("cannot install monitoring app in System project: %s -- requery in 10s\n", err.Error())
			return reconcile.Result{RequeueAfter: 10 * time.Second}, nil
		}
	}

	glog.V(2).Infof("completeted monitoring reconciliation for cluster %s", instance.Spec.Cluster)
	if err := r.updateStatus(instance, "Deployed"); err != nil {
		return reconcile.Result{}, err
	}
	return reconcile.Result{RequeueAfter: 5 * time.Minute}, nil
}

func (r *ReconcileMonitoring) updateStatus(item *extrasv1beta1.Monitoring, phase string) error {
	// If the phase is the same, skip
	if item.Status.Phase == phase {
		return nil
	}

	// Update the item
	item.Status.Phase = phase
	return r.Status().Update(context.Background(), item)
}
