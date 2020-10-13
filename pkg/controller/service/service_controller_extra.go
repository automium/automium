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

	for _, extraSpec := range service.Spec.Extra {
		switch extraSpec.Name {
		case "monitoring":
			err := r.manageMonitoringExtra(service, extraSpec)
			if err != nil {
				return err
			}
			break
		case "backup":
			err := r.manageBackupExtra(service, extraSpec)
			if err != nil {
				return err
			}
			break
		case "logging":
			err := r.manageLoggingExtra(service, extraSpec)
			if err != nil {
				return err
			}
			break
		default:
			return fmt.Errorf("unknown extras specified: %s", extraSpec.Name)
		}
	}

	glog.V(5).Infof("Management of extras for service %s completed\n", service.Name)
	return nil
}

func (r *ReconcileService) manageMonitoringExtra(service *v1beta1.Service, extraSpec v1beta1.ExtraSpec) error {

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
			Version:    extraSpec.Version,
			Parameters: extraSpec.Parameters,
		},
	}

	// If service replicas is 0, remove the Monitoring object
	if service.Spec.Replicas == 0 {
		glog.Infof("deleting Monitoring %s/%s\n", monitoringManifest.ObjectMeta.Namespace, monitoringManifest.ObjectMeta.Name)
		err := r.Delete(context.Background(), monitoringManifest)
		if err != nil && errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	// Create or update the manifest
	found := &extrasv1beta1.Monitoring{}
	err := r.Get(context.TODO(), types.NamespacedName{Name: monitoringManifest.ObjectMeta.Name, Namespace: monitoringManifest.ObjectMeta.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		glog.Infof("creating Monitoring %s/%s\n", monitoringManifest.ObjectMeta.Namespace, monitoringManifest.ObjectMeta.Name)
		err = r.Create(context.TODO(), monitoringManifest)
		if err != nil {
			glog.V(5).Infof("cannot create Monitoring object: %s\n", err.Error())
			return err
		}
		r.recorder.Event(service, "Normal", "Created", "Monitoring created")
		return nil
	}

	// Check if the version or the paramers are changed
	if found.Spec.Version != monitoringManifest.Spec.Version || !reflect.DeepEqual(found.Spec.Parameters, monitoringManifest.Spec.Parameters) {
		found.Spec.Version = monitoringManifest.Spec.Version
		found.Spec.Parameters = monitoringManifest.Spec.Parameters
		glog.Infof("updating Monitoring %s/%s-monitoring\n", found.ObjectMeta.Namespace, found.ObjectMeta.Name)
		err = r.Update(context.TODO(), found)
		if err != nil {
			glog.V(5).Infof("cannot update Monitoring object: %s\n", err.Error())
			return err
		}
		r.recorder.Event(service, "Normal", "Created", "Monitoring updated")

	}
	return nil
}

func (r *ReconcileService) manageBackupExtra(service *v1beta1.Service, extraSpec v1beta1.ExtraSpec) error {

	// Prepare the manifest
	backupManifest := &extrasv1beta1.Backup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-backup", service.Name),
			Namespace: service.Namespace,
			Annotations: map[string]string{
				"service.automium.io/name": service.Name,
			},
		},
		Spec: extrasv1beta1.BackupSpec{
			Cluster:    service.Name,
			Version:    extraSpec.Version,
			Parameters: extraSpec.Parameters,
		},
	}

	// If service replicas is 0, remove the Backup object
	if service.Spec.Replicas == 0 {
		glog.Infof("deleting Backup %s/%s\n", backupManifest.ObjectMeta.Namespace, backupManifest.ObjectMeta.Name)
		err := r.Delete(context.Background(), backupManifest)
		if err != nil && errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	// Create or update the manifest
	found := &extrasv1beta1.Backup{}
	err := r.Get(context.TODO(), types.NamespacedName{Name: backupManifest.ObjectMeta.Name, Namespace: backupManifest.ObjectMeta.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		glog.Infof("creating Backup %s/%s\n", backupManifest.ObjectMeta.Namespace, backupManifest.ObjectMeta.Name)
		err = r.Create(context.TODO(), backupManifest)
		if err != nil {
			glog.V(5).Infof("cannot create Backup object: %s\n", err.Error())
			return err
		}
		r.recorder.Event(service, "Normal", "Created", "Backup created")
		return nil
	}

	// Check if the version or the paramers are changed
	if found.Spec.Version != backupManifest.Spec.Version || !reflect.DeepEqual(found.Spec.Parameters, backupManifest.Spec.Parameters) {
		found.Spec.Version = backupManifest.Spec.Version
		found.Spec.Parameters = backupManifest.Spec.Parameters
		glog.Infof("updating Backup %s/%s-backup\n", found.ObjectMeta.Namespace, found.ObjectMeta.Name)
		err = r.Update(context.TODO(), found)
		if err != nil {
			glog.V(5).Infof("cannot update Backup object: %s\n", err.Error())
			return err
		}
		r.recorder.Event(service, "Normal", "Created", "Backup updated")

	}
	return nil
}

func (r *ReconcileService) manageLoggingExtra(service *v1beta1.Service, extraSpec v1beta1.ExtraSpec) error {

	// Prepare the manifest
	extraSpec.Parameters["clusterName"] = service.Name

	loggingManifest := &extrasv1beta1.Logging{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-logging", service.Name),
			Namespace: service.Namespace,
			Annotations: map[string]string{
				"service.automium.io/name": service.Name,
			},
		},
		Spec: extrasv1beta1.LoggingSpec{
			Cluster:    service.Name,
			Version:    extraSpec.Version,
			Parameters: extraSpec.Parameters,
		},
	}

	// If service replicas is 0, remove the Logging object
	if service.Spec.Replicas == 0 {
		glog.Infof("deleting Logging %s/%s\n", loggingManifest.ObjectMeta.Namespace, loggingManifest.ObjectMeta.Name)
		err := r.Delete(context.Background(), loggingManifest)
		if err != nil && errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	// Create or update the manifest
	found := &extrasv1beta1.Logging{}
	err := r.Get(context.TODO(), types.NamespacedName{Name: loggingManifest.ObjectMeta.Name, Namespace: loggingManifest.ObjectMeta.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		glog.Infof("creating Logging %s/%s\n", loggingManifest.ObjectMeta.Namespace, loggingManifest.ObjectMeta.Name)
		err = r.Create(context.TODO(), loggingManifest)
		if err != nil {
			glog.V(5).Infof("cannot create Logging object: %s\n", err.Error())
			return err
		}
		r.recorder.Event(service, "Normal", "Created", "Logging created")
		return nil
	}

	// Check if the version or the paramers are changed
	if found.Spec.Version != loggingManifest.Spec.Version || !reflect.DeepEqual(found.Spec.Parameters, loggingManifest.Spec.Parameters) {
		found.Spec.Version = loggingManifest.Spec.Version
		found.Spec.Parameters = loggingManifest.Spec.Parameters
		glog.Infof("updating Logging %s/%s-logging\n", found.ObjectMeta.Namespace, found.ObjectMeta.Name)
		err = r.Update(context.TODO(), found)
		if err != nil {
			glog.V(5).Infof("cannot update Logging object: %s\n", err.Error())
			return err
		}
		r.recorder.Event(service, "Normal", "Created", "Logging updated")

	}
	return nil
}
