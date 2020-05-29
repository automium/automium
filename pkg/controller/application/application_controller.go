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

package application

import (
	"context"
	"fmt"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

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

// Add creates a new Application Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileApplication{Client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {

	// Check if the environment variables are set
	if err := doEnvCheck(); err != nil {
		return fmt.Errorf("cannot validate controller environment: %s", err.Error())
	}

	// Create a new controller
	c, err := controller.New("application-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Add custom predicate func for avoiding reconciliation on status update
	cp := predicate.Funcs{
		UpdateFunc: func(ev event.UpdateEvent) bool {
			oldObj := ev.ObjectOld.(*extrasv1beta1.Application)
			newObj := ev.ObjectNew.(*extrasv1beta1.Application)
			if oldObj.Status != newObj.Status {
				return false
			}
			return true
		},
	}

	// Watch for changes to Application
	err = c.Watch(&source.Kind{Type: &extrasv1beta1.Application{}}, &handler.EnqueueRequestForObject{}, cp)
	if err != nil {
		return err
	}

	glog.Infoln("application controller initialized")

	return nil
}

var _ reconcile.Reconciler = &ReconcileApplication{}

// ReconcileApplication reconciles a Application object
type ReconcileApplication struct {
	client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a Application object and makes changes based on the state read
// and what is in the Application.Spec
// +kubebuilder:rbac:groups=extras.automium.io,resources=applications,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=extras.automium.io,resources=applications/status,verbs=get;update;patch
func (r *ReconcileApplication) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the Application instance
	instance := &extrasv1beta1.Application{}
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

	glog.V(2).Infof("starting reconciliation for application %s on cluster %s", instance.Spec.Name, instance.Spec.Cluster)

	// Check if we have an active cluster with the specified name
	clusterID, err := getRancherClusterIDFromName(instance.Spec.Cluster)
	if err != nil {
		glog.Errorf("cannot get ID for cluster: %s\n", err.Error())
		r.updateStatus(instance, "Failed", fmt.Sprintf("cannot get ID for cluster: %s", err.Error()))
		return reconcile.Result{}, nil
	}

	glog.V(5).Infof("got cluster ID: %s\n", clusterID)

	// Retrieve the selected project ID
	applicationProjectID, err := getRancherProjectIDFromName(clusterID, instance.Spec.Project)
	if err != nil {
		glog.Errorf("cannot get ID for project %s: %s\n", instance.Spec.Project, err.Error())
		r.updateStatus(instance, "Failed", fmt.Sprintf("cannot get ID for project %s: %s", instance.Spec.Project, err.Error()))
		return reconcile.Result{}, nil
	}

	glog.V(5).Infof("got %s project ID: %s\n", instance.Spec.Project, applicationProjectID)

	// Retrieve the apps inside the selected project
	projectApps, err := getRancherAppsFromProject(applicationProjectID)
	if err != nil {
		glog.Errorf("cannot get apps in project %s: %s\n", instance.Spec.Project, err.Error())
		r.updateStatus(instance, "Failed", fmt.Sprintf("cannot get apps in project %s: %s", instance.Spec.Project, err.Error()))
		return reconcile.Result{}, nil
	}

	glog.V(5).Infof("found apps: %+v\n", projectApps)

	// Check if the app has been deployed
	targetApp := appItem{Name: "does-not-exist"}
	for _, app := range projectApps {
		if app.Name == instance.Spec.Name {
			targetApp = app
			break
		}
	}

	if targetApp.Name == instance.Spec.Name {
		// Check the version and the answers; if necessary, upgrade the application
		answersMatch, err := answersEquals(targetApp.Answers, *instance)
		if err != nil {
			glog.Errorf("cannot validate answers for app %s in project %s: %s\n", instance.Spec.Name, instance.Spec.Project, err.Error())
			r.updateStatus(instance, "Failed", fmt.Sprintf("cannot validate answers for app %s in project %s: %s", instance.Spec.Name, instance.Spec.Project, err.Error()))
			return reconcile.Result{}, nil
		}
		if targetApp.Version != instance.Spec.Version || !answersMatch {
			// Upgrade
			glog.V(2).Infof("upgrade needed for app %s on cluster %s -- executing\n", instance.Spec.Name, instance.Spec.Cluster)
			r.updateStatus(instance, "Upgrading", "")
			err := deployApplication(clusterID, applicationProjectID, *instance, false)
			if err != nil {
				glog.Errorf("cannot upgrade app %s in project %s: %s\n", instance.Spec.Name, instance.Spec.Project, err.Error())
				r.updateStatus(instance, "Failed", fmt.Sprintf("cannot upgrade app %s in project %s: %s", instance.Spec.Name, instance.Spec.Project, err.Error()))
				return reconcile.Result{}, nil
			}
		} else {
			glog.V(2).Infof("app %s on cluster %s is up-to-date\n", instance.Spec.Name, instance.Spec.Cluster)
		}
	} else {
		// New installation
		glog.V(2).Infof("new installation of app %s requested on cluster %s -- executing\n", instance.Spec.Name, instance.Spec.Cluster)
		r.updateStatus(instance, "Deploying", "")
		err := deployApplication(clusterID, applicationProjectID, *instance, true)
		if err != nil {
			glog.Errorf("cannot install app %s in System project: %s\n", instance.Spec.Name, err.Error())
			r.updateStatus(instance, "Failed", fmt.Sprintf("cannot install app %s in System project: %s", instance.Spec.Name, err.Error()))
			return reconcile.Result{}, nil
		}
	}

	glog.V(2).Infof("completeted app %s reconciliation for cluster %s", instance.Spec.Name, instance.Spec.Cluster)
	r.updateStatus(instance, "Deployed", "")
	return reconcile.Result{RequeueAfter: 5 * time.Minute}, nil
}

func (r *ReconcileApplication) updateStatus(item *extrasv1beta1.Application, phase, errorMsg string) error {
	// If the phase is the same, skip
	if item.Status.Phase == phase {
		return nil
	}

	// Update the error message if needed
	if errorMsg != item.Status.Error {
		item.Status.Error = errorMsg
	}

	// Update the item
	item.Status.Phase = phase

	glog.V(5).Infof("Updating application status for %s/%s\n", item.Namespace, item.Name)
	return r.Status().Update(context.Background(), item)
}
