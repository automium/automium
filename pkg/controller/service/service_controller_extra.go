package service

import (
	"context"
	"fmt"
	"reflect"

	"github.com/automium/automium/pkg/apis/core/v1beta1"
	extrasv1beta1 "github.com/automium/automium/pkg/apis/extras/v1beta1"
	"github.com/golang/glog"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// ManageExtras manage the Extra resources for a cluster
func (r *ReconcileService) ManageExtras(service *v1beta1.Service) error {

	for _, extraVar := range service.Spec.Extra {
		switch extraVar.Name {
		case "monitoring":
			err := r.manageMonitoringExtra(service, extraVar.Version, extraVar.Parameters)
			if err != nil {
				return nil
			}
		default:
			return fmt.Errorf("unknown extras specified: %s", extraVar.Name)
		}
	}

	return nil
}

func (r *ReconcileService) manageMonitoringExtra(service *v1beta1.Service, version string, parameters map[string]string) error {

	// Prepare the manifest
	monitoringManifest := &extrasv1beta1.Monitoring{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-monitoring", service.Name),
			Namespace: service.Namespace,
			Annotations: map[string]string{
				"service.automium.io/name": service.Name,
			},
		},
		Spec: extrasv1beta1.MonitoringSpec{
			Cluster:    service.Name,
			Version:    version,
			Parameters: parameters,
		},
	}

	// If service replicas is 0, remove the Monitoring object
	if service.Spec.Replicas == 0 {
		glog.Infof("deleting Monitoring %s/%s-monitoring\n", service.Namespace, service.Name)
		err := r.Delete(context.Background(), monitoringManifest)
		if err != nil && errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	// Create or update the manifest
	found := &extrasv1beta1.Monitoring{}
	err := r.Get(context.TODO(), types.NamespacedName{Name: fmt.Sprintf("%s-monitoring", service.Name), Namespace: service.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		glog.Infof("creating Monitoring %s/%s-monitoring\n", service.Namespace, service.Name)
		err = r.Create(context.TODO(), monitoringManifest)
		if err != nil {
			glog.V(5).Infof("cannot create Monitoring object: %s\n", err.Error())
			return err
		}
		r.recorder.Event(service, "Normal", "Created", "Monitoring created")
	} else {
		// Check if the version or the paramers are changed
		if found.Spec.Version != monitoringManifest.Spec.Version || !reflect.DeepEqual(found.Spec.Parameters, monitoringManifest.Spec.Parameters) {
			found.Spec.Version = monitoringManifest.Spec.Version
			found.Spec.Parameters = monitoringManifest.Spec.Parameters
			glog.Infof("updating Monitoring %s/%s-monitoring\n", service.Namespace, service.Name)
			err = r.Update(context.TODO(), found)
			if err != nil {
				glog.V(5).Infof("cannot update Monitoring object: %s\n", err.Error())
				return err
			}
			r.recorder.Event(service, "Normal", "Created", "Monitoring updated")
		}

	}
	return nil
}
