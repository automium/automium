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

package logging

import (
	"context"
	"reflect"

	"github.com/automium/automium/pkg/apis/extras/v1beta1"
	extrasv1beta1 "github.com/automium/automium/pkg/apis/extras/v1beta1"
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
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller")

// Add creates a new Logging Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileLogging{Client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("logging-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to Logging
	err = c.Watch(&source.Kind{Type: &extrasv1beta1.Logging{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch the Applications created for the Logging
	err = c.Watch(&source.Kind{Type: &extrasv1beta1.Application{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &extrasv1beta1.Logging{},
	})
	if err != nil {
		return err
	}

	glog.Infoln("logging controller initialized")

	return nil
}

var _ reconcile.Reconciler = &ReconcileLogging{}

// ReconcileLogging reconciles a Logging object
type ReconcileLogging struct {
	client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a Logging object and makes changes based on the state read
// and what is in the Logging.Spec
// +kubebuilder:rbac:groups=extras.automium.io,resources=loggings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=extras.automium.io,resources=loggings/status,verbs=get;update;patch
func (r *ReconcileLogging) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the Logging instance
	instance := &extrasv1beta1.Logging{}
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

	// Define the desired logging Application object
	deploy := &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name + "-app",
			Namespace: instance.Namespace,
		},
		Spec: v1beta1.ApplicationSpec{
			Name:       "automium-logging",
			Version:    instance.Spec.Version,
			Cluster:    instance.Spec.Cluster,
			Project:    "System",
			Namespace:  "kube-system",
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
		glog.Infof("Creating new logging application %s/%s\n", deploy.ObjectMeta.Namespace, deploy.ObjectMeta.Name)
		err = r.Create(context.TODO(), deploy)
		return reconcile.Result{}, err
	} else if err != nil {
		return reconcile.Result{}, err
	}

	// Update the found object and write the result back if there are any changes
	if !reflect.DeepEqual(deploy.Spec, found.Spec) {
		found.Spec = deploy.Spec
		glog.Infof("Updating logging application %s/%s\n", deploy.ObjectMeta.Namespace, deploy.ObjectMeta.Name)
		err = r.Update(context.TODO(), found)
		if err != nil {
			return reconcile.Result{}, err
		}
	}
	return reconcile.Result{}, nil
}
