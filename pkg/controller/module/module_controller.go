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

package module

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	corev1beta1 "github.com/automium/automium/pkg/apis/core/v1beta1"
	"github.com/golang/glog"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
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

// Add creates a new Module Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
// USER ACTION REQUIRED: update cmd/manager/main.go to call this core.Add(mgr) to install this Controller
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileModule{Client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("module-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to Module
	err = c.Watch(&source.Kind{Type: &corev1beta1.Module{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch the Job created by Module
	err = c.Watch(&source.Kind{Type: &batchv1.Job{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &corev1beta1.Module{},
	})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileModule{}

// ReconcileModule reconciles a Module object
type ReconcileModule struct {
	client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a Module object and makes changes based on the state read
// and what is in the Module.Spec
// Automatically generate RBAC rules to allow the Controller to read and write Jobs
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;delete
// +kubebuilder:rbac:groups=core.automium.io,resources=modules,verbs=get;list;watch;create;update;patch;delete
func (r *ReconcileModule) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the Module instance
	instance := &corev1beta1.Module{}
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

	// Define the reference to the provisioning configuration
	provisioner := corev1.EnvFromSource{
		ConfigMapRef: &corev1.ConfigMapEnvSource{
			LocalObjectReference: corev1.LocalObjectReference{Name: "provisioner-config"},
		},
	}

	// Define the command to be executed by the Job
	jobCommand := []string{}
	switch instance.Spec.Action {
	case "Deploy":
		jobCommand = append(jobCommand, "./deploy")
	case "Upgrade":
		jobCommand = append(jobCommand, "./upgrade")
	case "DeployAndUpgrade":
		jobCommand = append(jobCommand, "/bin/sh", "-c", "./deploy && ./upgrade")
	case "Destroy":
		jobCommand = append(jobCommand, "./remove")
	default:
		glog.Errorf("unknown action for module %s: %s", instance.Name, instance.Spec.Action)
		return reconcile.Result{}, fmt.Errorf("unknown action for module %s: %s", instance.Name, instance.Spec.Action)
	}

	batchEnvVars := append(instance.Spec.Env,
		corev1.EnvVar{
			Name:  "FLAVOR",
			Value: instance.Spec.Flavor,
		},
		corev1.EnvVar{
			Name:  "QUANTITY",
			Value: fmt.Sprintf("%d", instance.Spec.Replicas),
		},
		corev1.EnvVar{
			Name:  "IMAGE",
			Value: instance.Spec.Image,
		},
	)

	var retryCount int32 = 1
	// Define the desired Job object
	deploy := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-job", instance.Name),
			Namespace: instance.Namespace,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"job": fmt.Sprintf("%s-job", instance.Name),
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "provisioner",
							Image:           fmt.Sprintf("automium/service-%s:latest", instance.Spec.Source),
							ImagePullPolicy: "Always",
							Command:         jobCommand,
							EnvFrom:         []corev1.EnvFromSource{provisioner},
							Env:             batchEnvVars,
						},
					},
					RestartPolicy: corev1.RestartPolicyNever,
				},
			},
			BackoffLimit: &retryCount,
		},
	}
	if err := controllerutil.SetControllerReference(instance, deploy, r.scheme); err != nil {
		return reconcile.Result{}, err
	}

	// Check if the Job already exists
	found := &batchv1.Job{}
	err = r.Get(context.TODO(), types.NamespacedName{Name: deploy.Name, Namespace: deploy.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		glog.Infof("creating Job %s/%s\n", deploy.Namespace, deploy.Name)
		err = r.Create(context.TODO(), deploy)
		if err != nil {
			return reconcile.Result{}, err
		}
	} else if err != nil {
		return reconcile.Result{}, err
	}

	// The Job already exists.
	// TODO Wait until the job (pod) is Completed, returning a specific error.
	if len(found.Spec.Template.Spec.Containers) == 0 {
		glog.Infof("no containers found for the job %s/%s\n", deploy.Namespace, deploy.Name)
		return reconcile.Result{}, nil
	}

	// Check if the job needs to be recreated by comparing the new env and the found job env
	if !reflect.DeepEqual(deploy.Spec.Template.Spec.Containers[0].Env, found.Spec.Template.Spec.Containers[0].Env) {

		// Check if the job is still running
		if found.Status.Active >= 1 {
			glog.V(2).Infof("waiting for operate on job %s: it still has running pods\n", fmt.Sprintf("%s-job", instance.Name))
			return reconcile.Result{}, fmt.Errorf("job %s is still running", fmt.Sprintf("%s-job", instance.Name))
		}

		// Cleanup Job pods
		glog.Infof("deleting old Job %s/%s and its pods\n", deploy.Namespace, deploy.Name)
		jobPodList := &corev1.PodList{}
		err = r.List(context.TODO(), &client.ListOptions{Namespace: deploy.Namespace}, jobPodList)
		if err != nil {
			glog.Warningf("cannot list pods: %s -- skipping job pods cleanup\n", err.Error())
		} else {
			for _, pod := range jobPodList.Items {
				if strings.HasPrefix(pod.Name, fmt.Sprintf("%s-job-", instance.Name)) {
					err = r.Delete(context.TODO(), &pod)
					if err != nil {
						glog.Warningf("cannot delete pod %s for job %s: %s\n", pod.Name, fmt.Sprintf("%s-job-", instance.Name), err.Error())
					}
				}
			}
		}

		// Delete the Job
		err = r.Delete(context.TODO(), found)
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	return reconcile.Result{}, nil
}
