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
	"reflect"

	"github.com/golang/glog"

	"github.com/automium/automium/pkg/apis/extras/v1beta1"
	extrasv1beta1 "github.com/automium/automium/pkg/apis/extras/v1beta1"
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

	// Watch the Applications created for the Monitoring
	err = c.Watch(&source.Kind{Type: &extrasv1beta1.Application{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &extrasv1beta1.Monitoring{},
	})
	if err != nil {
		return err
	}

	glog.Infoln("monitoring controller initialized")

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

	// Define the desired monitoring Application object
	deploy := &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name + "-app",
			Namespace: instance.Namespace,
		},
		Spec: v1beta1.ApplicationSpec{
			Name:       "automium-monitoring",
			Version:    instance.Spec.Version,
			Cluster:    instance.Spec.Cluster,
			Project:    "System",
			Namespace:  "automium-prometheus",
			Parameters: instance.Spec.Parameters,
		},
	}
	if err := controllerutil.SetControllerReference(instance, deploy, r.scheme); err != nil {
		return reconcile.Result{}, err
	}

	// Check if the Application already exists
	found := &v1beta1.Application{}
	err = r.Get(context.TODO(), types.NamespacedName{Name: deploy.Name, Namespace: deploy.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		glog.Infof("Creating new monitoring application %s/%s\n", deploy.ObjectMeta.Namespace, deploy.ObjectMeta.Name)
		err = r.Create(context.TODO(), deploy)
		return reconcile.Result{}, err
	} else if err != nil {
		return reconcile.Result{}, err
	}

	// Update the found object and write the result back if there are any changes
	if !reflect.DeepEqual(deploy.Spec, found.Spec) {
		found.Spec = deploy.Spec
		glog.Infof("Updating monitoring application %s/%s\n", deploy.ObjectMeta.Namespace, deploy.ObjectMeta.Name)
		err = r.Update(context.TODO(), found)
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	return reconcile.Result{}, nil
}
