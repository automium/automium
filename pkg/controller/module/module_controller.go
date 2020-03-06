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
	"time"

	"k8s.io/client-go/tools/record"

	goerrors "errors"

	"github.com/automium/automium/pkg/apis/core/v1beta1"
	corev1beta1 "github.com/automium/automium/pkg/apis/core/v1beta1"
	"github.com/automium/automium/pkg/utils"
	"github.com/golang/glog"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
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
	return &ReconcileModule{Client: mgr.GetClient(), scheme: mgr.GetScheme(), recorder: mgr.GetRecorder("module-controller")}
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
	scheme   *runtime.Scheme
	recorder record.EventRecorder
}

// Reconcile reads that state of the cluster for a Module object and makes changes based on the state read
// and what is in the Module.Spec
// Automatically generate RBAC rules to allow the Controller to read and write Jobs
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;delete
// +kubebuilder:rbac:groups=core.automium.io,resources=modules;modules/status,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
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

	// Add extra variables for the Job
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

	// Get the correct action for the module
	action, err := r.chooseModuleAction(instance)
	if err != nil {
		glog.V(2).Infof("cannot evalutate action for module %s: %s\n", instance.Name, err.Error())
		r.recorder.Eventf(instance, "Warning", "ModuleEvaluationFailed", "Module action evaluation failed: %s", err.Error())
		return reconcile.Result{RequeueAfter: 30 * time.Second}, nil
	}
	glog.V(5).Infof("Action selected for module: %s\n", action)

	// Define the command to be executed by the Job
	jobCommand := []string{}
	switch action {
	case "Deploy":
		jobCommand = append(jobCommand, "./deploy")
	case "Upgrade":
		jobCommand = append(jobCommand, "./upgrade")
	case "DeployAndUpgrade":
		jobCommand = append(jobCommand, "/bin/sh", "-c", "./deploy && ./upgrade")
	case "Destroy":
		jobCommand = append(jobCommand, "./remove")
	default:
		glog.Errorf("unknown action for module %s: %s", instance.Name, action)
		r.recorder.Eventf(instance, "Warning", "InvalidData", "unknown action specified: %s", action)
		return reconcile.Result{}, fmt.Errorf("unknown action for module %s: %s", instance.Name, action)
	}

	var retryCount int32 = 1
	var ndots = "1"
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
							Image:           fmt.Sprintf("automium/service-%s:%s", instance.Spec.Source, instance.Spec.Image),
							ImagePullPolicy: "Always",
							Command:         jobCommand,
							EnvFrom:         []corev1.EnvFromSource{provisioner},
							Env:             batchEnvVars,
						},
					},
					RestartPolicy: corev1.RestartPolicyNever,
					DNSConfig: &v1.PodDNSConfig{
						Options: []v1.PodDNSConfigOption{
							{
								Name:  "ndots",
								Value: &ndots,
							},
						},
					},
				},
			},
			BackoffLimit: &retryCount,
		},
	}
	if err := controllerutil.SetControllerReference(instance, deploy, r.scheme); err != nil {
		return reconcile.Result{}, err
	}

	// Try to retrieve an existing Job object
	found := &batchv1.Job{}
	foundJobErr := r.Get(context.TODO(), types.NamespacedName{Name: fmt.Sprintf("%s-job", instance.Name), Namespace: instance.Namespace}, found)
	if foundJobErr != nil && errors.IsNotFound(foundJobErr) {
		glog.Infof("creating Job %s/%s\n", deploy.Namespace, deploy.Name)
		err = r.Create(context.TODO(), deploy)
		if err != nil {
			glog.V(5).Infof("cannot create job: %s\n", err.Error())
			return reconcile.Result{}, err
		}
		r.recorder.Event(instance, "Normal", "Created", "Module created")
	}

	// While the pod is starting, delay the reconcile
	if len(found.Spec.Template.Spec.Containers) == 0 {
		glog.Infof("no containers found for the job %s/%s -- next check in 5s\n", deploy.Namespace, deploy.Name)
		return reconcile.Result{RequeueAfter: 5 * time.Second}, nil
	}

	// Check if the job needs to be recreated
	if !reflect.DeepEqual(deploy.Spec.Template.Spec.Containers[0].Env, found.Spec.Template.Spec.Containers[0].Env) || !reflect.DeepEqual(deploy.Spec.Template.Spec.Containers[0].Command, found.Spec.Template.Spec.Containers[0].Command) {
		// Let's be sure there is no job running for the module
		if found.Status.Active >= 1 {
			glog.V(2).Infof("waiting to operate on job %s: it still has a job running\n", fmt.Sprintf("%s-job", instance.Name))
			r.recorder.Event(instance, "Warning", "JobStillRunning", "Module has still running jobs")
			return reconcile.Result{RequeueAfter: 30 * time.Second}, nil
		}

		// Cleanup Job pods
		glog.Infof("deleting old Job %s/%s and its pods\n", deploy.Namespace, deploy.Name)
		jobPodList := &corev1.PodList{}
		err = r.List(context.TODO(), &client.ListOptions{Namespace: deploy.Namespace}, jobPodList)
		if err != nil {
			glog.Warningf("cannot list pods: %s -- skipping job pods cleanup\n", err.Error())
			r.recorder.Eventf(instance, "Warning", "PodListFailed", "Failed to list module pods: %s -- skipping cleanup", err.Error())
		} else {
			for _, pod := range jobPodList.Items {
				if strings.HasPrefix(pod.Name, fmt.Sprintf("%s-job-", instance.Name)) {
					err = r.Delete(context.TODO(), &pod)
					if err != nil {
						glog.Warningf("cannot delete pod %s for job %s: %s\n", pod.Name, fmt.Sprintf("%s-job-", instance.Name), err.Error())
						r.recorder.Eventf(instance, "Warning", "PodCleanupFailed", "Failed to delete module pod %s: %s -- skipping cleanup", pod.Name, err.Error())
					}
				}
			}
		}

		// Delete the Job
		err = r.Delete(context.TODO(), found)
		if err != nil {
			return reconcile.Result{}, err
		}

		// Update status
		err = r.updateModuleStatus(instance, action, corev1beta1.StatusPhasePending)
		if err != nil {
			glog.Errorf("cannot update status for module %s: %s\n", instance.Name, err.Error())
			r.recorder.Eventf(instance, "Warning", "UpdateFailed", "Failed to update module status: %s", err.Error())
			return reconcile.Result{}, err
		}

		r.recorder.Eventf(instance, "Normal", "Updated", "Module updated")
		// Requeue module update
		return reconcile.Result{Requeue: true}, nil
	}

	err = r.manageNodesForModule(instance)
	if err != nil {
		glog.Errorf("cannot manage node for module %s: %s -- retrying in 5s.\n", instance.Name, err.Error())
		r.recorder.Eventf(instance, "Warning", "NodeManagementFailed", "Failed to manage nodes: %s", err.Error())
		return reconcile.Result{RequeueAfter: 5 * time.Second}, nil
	}

	// Retrieve job status
	status, err := r.retrieveJobStatus(found)
	if err != nil {
		glog.Errorf("cannot get status for job %s: %s\n", found.Name, err.Error())
		r.recorder.Eventf(instance, "Warning", "RetrieveJobStatusFailed", "Failed to retrieve job status: %s", err.Error())
		return reconcile.Result{}, err
	}

	// Manage module status
	glog.V(5).Infof("Updating module %s status...\n", instance.Name)

	// If updated replicas are not equal to the desired replicas, and the module is completed, return Pending
	if instance.Status.UpdatedReplicas != instance.Status.Replicas && status == corev1beta1.StatusPhaseCompleted {
		status = corev1beta1.StatusPhasePending
	}

	// Update the module status
	err = r.updateModuleStatus(instance, action, status)
	if err != nil {
		glog.Errorf("cannot update module %s status: %s\n", instance.Name, err.Error())
		r.recorder.Eventf(instance, "Warning", "ModuleStatusUpdateFailed", "Failed to update module status: %s", err.Error())
		return reconcile.Result{}, err
	}

	// Trigger a requeue if the module is running or pending
	if status == corev1beta1.StatusPhasePending || status == corev1beta1.StatusPhaseRunning {
		glog.V(2).Infof("module %s is in Pending or Running -- reschedule update in 15s.\n", instance.Name)
		return reconcile.Result{RequeueAfter: 15 * time.Second}, nil
	}

	r.recorder.Eventf(instance, "Normal", "Completed", "Module execution completed")
	glog.V(5).Infof("ops on module %s completed.\n", instance.Name)
	return reconcile.Result{}, nil
}

func (r *ReconcileModule) updateModuleStatus(module *corev1beta1.Module, action, phase string) error {
	cur, upd, err := r.getUpdatedNodes(module.Namespace, module.ObjectMeta.Annotations["service.automium.io/name"], action, module.Spec.Image, module.Spec.Flavor)
	if err != nil {
		return err
	}

	// Build updated module status and compare with the actual one
	updModuleStatus := module.Status
	updModuleStatus.Phase = phase
	updModuleStatus.Replicas = module.Spec.Replicas
	updModuleStatus.CurrentReplicas = cur
	updModuleStatus.UpdatedReplicas = upd

	if !reflect.DeepEqual(module.Status, updModuleStatus) {
		glog.V(5).Infof("Module status is not deep equal - updating")
		module.Status = updModuleStatus
		err = r.Status().Update(context.Background(), module)
		if err != nil {
			return err
		}
		glog.V(5).Infof("Module %s updated\n", module.Name)
	}

	return nil
}

func (r *ReconcileModule) retrieveJobStatus(job *batchv1.Job) (string, error) {

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

	return "", goerrors.New(fmt.Sprintf("cannot detect job %s status", job.Name))
}

func (r *ReconcileModule) getUpdatedNodes(namespace, serviceName, action, newImageVersion, newFlavor string) (int, int, error) {
	var currentReplicas int
	var updatedReplicas int

	if namespace == "" && serviceName == "" && newImageVersion == "" {
		return -1, -1, goerrors.New("invalid parameters for retrieving application data from nodes")
	}

	nodesList := &corev1beta1.NodeList{}
	err := r.List(context.TODO(), &client.ListOptions{Namespace: namespace}, nodesList)
	if err != nil {
		return -1, -1, err
	}

	for _, node := range nodesList.Items {
		if node.ObjectMeta.Annotations["service.automium.io/name"] == serviceName {
			if utils.NodeIsHealthy(node) {
				if node.Status.NodeProperties.Image == newImageVersion && node.Status.NodeProperties.Flavor == newFlavor {
					glog.V(5).Infof("getUpdatedNodes: node: %s - is an updated replica\n", node.Name)
					updatedReplicas++
					if action == "Deploy" {
						currentReplicas++
					}
				} else {
					glog.V(5).Infof("getUpdatedNodes: node: %s - is a current replica\n", node.Name)
					currentReplicas++
				}
			} else {
				glog.V(5).Infof("getUpdatedNodes: node: %s is not healty - skipping\n", node.Name)
			}
		}
	}

	return currentReplicas, updatedReplicas, nil

}

func (r *ReconcileModule) manageNodesForModule(module *corev1beta1.Module) error {
	appName := module.ObjectMeta.Annotations["module.automium.io/appName"]
	serviceName := module.ObjectMeta.Annotations["service.automium.io/name"]

	if appName == "" || serviceName == "" {
		return goerrors.New("missing application and or service annotations in module, cannot continue")
	}

	// Search for existent nodes for service
	appNodes, err := r.retrieveNodesForService(serviceName, module.Namespace)
	if err != nil {
		return err
	}

	// Add all items or the missing items
	if len(appNodes) < module.Spec.Replicas {
		for i := 0; i < module.Spec.Replicas; i++ {
			// Prepare the hostname
			var specHostname string
			switch appName {
			case "kubernetes-cluster":
				specHostname = fmt.Sprintf("%s-%s-%d", serviceName, serviceName, i)
			case "kubernetes-nodepool":
				var clusterName string

				for _, val := range module.Spec.Env {
					if val.Name == "CLUSTER_NAME" {
						clusterName = val.Value
					}
				}

				if clusterName == "" {
					return goerrors.New("requested a Kubernetes nodepool but no cluster_name env variable provided, cannot continue")
				}

				glog.V(5).Infof("Adding a new Node for service %s (hostname: %s)\n", serviceName, fmt.Sprintf("%s-%s-%d", clusterName, serviceName, i))

				specHostname = fmt.Sprintf("%s-%s-%d", clusterName, serviceName, i)
			default:
				specHostname = fmt.Sprintf("%s-%d", appName, i)
			}

			// Create the node item
			err := r.Create(context.TODO(), &corev1beta1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-node-%d", serviceName, i),
					Namespace: module.Namespace,
					Annotations: map[string]string{
						"service.automium.io/name": serviceName,
					},
				},
				Spec: corev1beta1.NodeSpec{
					Hostname:     specHostname,
					DeletionDate: "",
				},
			})
			if err != nil && !errors.IsAlreadyExists(err) {
				return err
			}
		}
	} else if len(appNodes) > module.Spec.Replicas {
		// Delete extra nodes
		err := utils.SortNodesByNumber(appNodes)
		if err != nil {
			return err
		}

		glog.V(2).Infoln("sorted nodes:")
		for idx, item := range appNodes {
			glog.V(2).Infof("[%d] %s\n", idx, item.Spec.Hostname)
		}

		arrToDelete := appNodes[module.Spec.Replicas:len(appNodes)]
		glog.V(2).Infof("service %s - node replicas: %d -> %d\n", appName, len(appNodes), module.Spec.Replicas)

		for _, item := range arrToDelete {
			if item.Spec.DeletionDate == "" {
				glog.V(2).Infof("service: %s - marking for deletion node %s\n", appName, item.Spec.Hostname)
				item.Spec.DeletionDate = time.Now().String()
				r.Update(context.TODO(), &item)
			} else {
				glog.V(2).Infof("service: %s -node %s already marked for deletion\n", appName, item.Spec.Hostname)
			}
		}
	}

	return nil

}

func (r *ReconcileModule) retrieveNodesForService(svcName, namespace string) ([]v1beta1.Node, error) {
	// Retrieve all nodes for this namespace

	if svcName == "" || namespace == "" {
		return nil, goerrors.New("cannot retrieve nodes: invalid service and/or namspace")
	}

	nsNodes := &corev1beta1.NodeList{}
	err := r.List(context.TODO(), &client.ListOptions{Namespace: namespace}, nsNodes)
	if err != nil {
		return nil, err
	}

	// Search for existent nodes for service
	appNodes := make([]corev1beta1.Node, 0)
	for _, node := range nsNodes.Items {
		if node.ObjectMeta.Annotations["service.automium.io/name"] == svcName {
			glog.V(2).Infof("found node %s for app %s\n", node.Spec.Hostname, svcName)
			appNodes = append(appNodes, node)
		}
	}

	return appNodes, nil
}

func (r *ReconcileModule) chooseModuleAction(module *v1beta1.Module) (string, error) {

	// If the user wants no replicas, destroy everything
	if module.Spec.Replicas == 0 {
		return "Destroy", nil
	}

	// Retrieve current nodes
	moduleNodes, err := r.retrieveNodesForService(module.ObjectMeta.Annotations["service.automium.io/name"], module.Namespace)
	if err != nil {
		return "", err
	}

	// If there are no nodes or we are scaling down them, deploy
	if utils.GetNodesCount(moduleNodes) == 0 || module.Spec.Replicas < utils.GetNodesCount(moduleNodes) {
		return "Deploy", nil
	}

	// Check if all nodes are consistent (same image and flavor); if not, force an upgrade
	nodesConsistent, healthyNode := utils.NodesAreConsistent(moduleNodes)
	if !nodesConsistent {
		return "Upgrade", nil
	}

	// If we have an healthy node, and its flavor or image are different from the Module ones, do an upgrade
	if healthyNode.Status.NodeProperties.Node != "" {
		if !utils.EqNodeFlavor(healthyNode, module.Spec.Flavor) || !utils.EqNodeImage(healthyNode, module.Spec.Image) {
			return "Upgrade", nil
		}
	}

	return "Deploy", nil
}
