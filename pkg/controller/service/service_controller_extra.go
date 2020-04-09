package service

import (
	"context"
	"fmt"

	"github.com/automium/automium/pkg/apis/core/v1beta1"
	extrasv1beta1 "github.com/automium/automium/pkg/apis/extras/v1beta1"
	"github.com/golang/glog"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// ManageMonitoringExtra manage the Monitoring resource for a cluster
func (r *ReconcileService) ManageMonitoringExtra(service *v1beta1.Service) error {
	var hasMonitoring = false
	var installVersion = ""

	for _, extraVar := range service.Spec.Extra {
		if extraVar.Name == "monitoring" {
			hasMonitoring = true
			if extraVar.Version == "" {
				installVersion = "1.0.0"
			} else {
				installVersion = extraVar.Version
			}
		}
	}

	if hasMonitoring {
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
				Cluster: service.Name,
				Version: installVersion,
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
			// Only check if the version has changed
			if found.Spec.Version != monitoringManifest.Spec.Version {
				found.Spec.Version = monitoringManifest.Spec.Version
				err = r.Update(context.TODO(), found)
				if err != nil {
					glog.V(5).Infof("cannot update Monitoring object: %s\n", err.Error())
					return err
				}
				r.recorder.Event(service, "Normal", "Created", "Monitoring updated")
			}

		}
	}

	return nil
}
