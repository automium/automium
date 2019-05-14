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
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	goerrors "errors"

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

	glog.Infoln("module controller initialized")

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
// +kubebuilder:rbac:groups=core.automium.io,resources=modules;modules/status,verbs=get;list;watch;create;update;patch;delete
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
			glog.V(5).Infof("cannot create job: %s\n", err.Error())
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

	appName := instance.ObjectMeta.Annotations["module.automium.io/appName"]
	serviceName := instance.ObjectMeta.Annotations["service.automium.io/name"]

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
		if node.ObjectMeta.Annotations["service.automium.io/name"] == serviceName {
			glog.V(2).Infof("found node %s for app %s\n", node.Spec.Hostname, serviceName)
			appNodes = append(appNodes, node)
		}
	}

	// Special cases (nonexistent, delete all)
	if instance.Spec.Replicas == 0 && len(appNodes) > 0 {
		// Delete all nodes
		glog.V(2).Infof("service %s has no replicas - removing all nodes.\n", serviceName)
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
				specHostname = fmt.Sprintf("%s-%s-%d", serviceName, serviceName, i)
			case "kubernetes-nodepool":
				var clusterName string

				for _, val := range instance.Spec.Env {
					if val.Name == "CLUSTER_NAME" {
						clusterName = val.Value
					}
				}

				if clusterName == "" {
					glog.Warningln("nodes for Kubenernetes nodepool requested but empty cluster_name provided -- marking as 'nocluster' nodepool")
					clusterName = "nocluster"
				}

				specHostname = fmt.Sprintf("%s-%s-%d", clusterName, serviceName, i)
			default:
				specHostname = fmt.Sprintf("%s-%d", appName, i)
			}
			err := r.Create(context.TODO(), &corev1beta1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-node-%d", serviceName, i),
					Namespace: instance.Namespace,
					Annotations: map[string]string{
						"service.automium.io/name": serviceName,
					},
				},
				Spec: corev1beta1.NodeSpec{
					Hostname:     specHostname,
					DeletionDate: "",
				},
			})
			if err != nil {
				glog.Errorf("cannot create node %s: %s\n", fmt.Sprintf("%s-node-%d", serviceName, i), err.Error())
			}
		}

		// Retrieve job status
		status, err := r.retrieveJobStatus(request, found.Name, found.Namespace)
		if err != nil {
			glog.Errorf("cannot get status for job %s: %s\n", found.Name, err.Error())
			return reconcile.Result{}, err
		}

		// Manage module status
		glog.V(5).Infof("Updating module %s status...\n", instance.Name)
		err = r.updateModuleStatus(request, status, instance.Spec.Replicas, 1, 1)
		if err != nil {
			glog.Errorf("cannot update module %s status: %s\n", instance.Name, err.Error())
			return reconcile.Result{}, err
		}
		glog.V(5).Infof("Module %s updated\n", instance.Name)
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

	glog.V(5).Infof("ops on module %s completed.\n", instance.Name)
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

func (r *ReconcileModule) updateModuleStatus(request reconcile.Request, phase string, replicas, currentReplicas, updatedReplicas int) error {
	currentModule := &corev1beta1.Module{}
	err := r.Get(context.TODO(), request.NamespacedName, currentModule)
	if err != nil {
		return err
	}

	currentModule.Status.Phase = phase
	currentModule.Status.Replicas = replicas
	currentModule.Status.CurrentReplicas = currentReplicas
	currentModule.Status.UpdatedReplicas = updatedReplicas

	err = r.Status().Update(context.Background(), currentModule)
	if err != nil {
		return err
	}
	return nil
}

func (r *ReconcileModule) retrieveJobStatus(request reconcile.Request, jobName, jobNamespace string) (string, error) {
	job := &batchv1.Job{}
	err := r.Get(context.TODO(), types.NamespacedName{Name: jobName, Namespace: jobNamespace}, job)
	if err != nil {
		return "", err
	}

	glog.V(5).Infof("Job status: %+v\n", job.Status)

	if job.Status.StartTime == nil {
		return corev1beta1.StatusPhasePending, nil
	}

	if job.Status.Active > 0 {
		return corev1beta1.StatusPhaseRunning, nil
	}

	if len(job.Status.Conditions) > 0 {
		switch job.Status.Conditions[0].Type {
		case "Complete":
			return corev1beta1.StatusPhaseCompleted, nil
		case "Failed":
			return corev1beta1.StatusPhaseFailed, nil
		default:
			return "", errors.NewBadRequest(fmt.Sprintf("unknown condition: %s", job.Status.Conditions[0].Type))
		}
	}

	return "", goerrors.New(fmt.Sprintf("cannot detect job %s status", jobName))
}
