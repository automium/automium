package application

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"reflect"
	"strings"

	"github.com/automium/automium/pkg/apis/extras/v1beta1"
	extrasv1beta1 "github.com/automium/automium/pkg/apis/extras/v1beta1"
	"github.com/golang/glog"
)

type appItem struct {
	Name      string
	Catalog   string
	Template  string
	Version   string
	Namespace string
	State     string
	Answers   map[string]string
}

type rancherDataItem struct {
	Name  string `json:"name"`
	ID    string `json:"id"`
	State string `json:"state"`
}

type rancherAppItem struct {
	Name            string            `json:"name"`
	ID              string            `json:"id"`
	State           string            `json:"state"`
	ExternalID      string            `json:"externalId"`
	ProjectID       string            `json:"projectId"`
	Prune           bool              `json:"prune"`
	TargetNamespace string            `json:"targetNamespace"`
	Answers         map[string]string `json:"answers"`
}

type rancherList struct {
	Data []rancherDataItem `json:"data"`
}

type rancherAppList struct {
	Data []rancherAppItem `json:"data"`
}

type rancherStorageClassItem struct {
	Name        string            `json:"name"`
	Type        string            `json:"type"`
	Provisioner string            `json:"provisioner"`
	Parameters  map[string]string `json:"parameters"`
}

var requiredEnvVars = []string{"RANCHER_URL", "RANCHER_CLUSTER_TOKEN", "ALERTMANAGER_URL", "ALERTMANAGER_USERNAME", "ALERTMANAGER_PASSWORD", "ALERTMANAGER_INFRANAME"}

// doEnvCheck checks if the necessary env vars are set
func doEnvCheck() error {
	for _, eVar := range requiredEnvVars {
		if eVarCont := os.Getenv(eVar); eVarCont == "" {
			return fmt.Errorf("%s is not defined", eVar)
		}
	}
	return nil
}

// getRancherClusterIDFromName retrieves the cluster ID relative to the provider cluster if it is active
func getRancherClusterIDFromName(clusterName string) (string, error) {
	var clustersList rancherList

	// Prepare the cluster request
	req, err := prepareHTTPRequest(
		"GET",
		fmt.Sprintf("https://%s/v3/clusters", os.Getenv("RANCHER_URL")),
		nil,
	)
	if err != nil {
		return "", err
	}

	// Do the request
	resp, err := defaultHTTPClient().Do(req)
	if err != nil {
		return "", err
	}

	// Read the body
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	// Unmarshal the result
	err = json.Unmarshal(body, &clustersList)
	if err != nil {
		return "", err
	}

	// Search for the item
	for _, itm := range clustersList.Data {
		if itm.Name == clusterName && itm.State == "active" {
			return itm.ID, nil
		}
	}

	// If we got here, no cluster with the provided name has been found
	return "", errors.New("no cluster found with that name")
}

// getRancherProjectIDFromName retrieves the project ID relative to the provider project
func getRancherProjectIDFromName(clusterID, projectName string) (string, error) {
	var projectsList rancherList

	// Prepare the projects request
	req, err := prepareHTTPRequest(
		"GET",
		fmt.Sprintf("https://%s/v3/clusters/%s/projects", os.Getenv("RANCHER_URL"), clusterID),
		nil,
	)
	if err != nil {
		return "", err
	}

	// Do the request
	resp, err := defaultHTTPClient().Do(req)
	if err != nil {
		return "", err
	}

	// Read the body
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	// Unmarshal the result
	err = json.Unmarshal(body, &projectsList)
	if err != nil {
		return "", err
	}

	// Search for the item
	for _, itm := range projectsList.Data {
		if itm.Name == projectName {
			return itm.ID, nil
		}
	}

	// If we got here, no cluster with the provided name has been found
	return "", errors.New("no project found with that name")
}

// getRancherProjectIDFromName retrieves the project ID relative to the provider project
func getRancherAppsFromProject(projectID string) ([]appItem, error) {
	var appsList rancherAppList

	// Prepare the projects request
	req, err := prepareHTTPRequest(
		"GET",
		fmt.Sprintf("https://%s/v3/projects/%s/apps", os.Getenv("RANCHER_URL"), projectID),
		nil,
	)
	if err != nil {
		return []appItem{}, err
	}

	// Do the request
	resp, err := defaultHTTPClient().Do(req)
	if err != nil {
		return []appItem{}, err
	}

	// Read the body
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return []appItem{}, err
	}

	// Unmarshal the result
	err = json.Unmarshal(body, &appsList)
	if err != nil {
		return []appItem{}, err
	}

	// Prepare the final app list
	finalAppList := make([]appItem, 0)
	for _, itm := range appsList.Data {
		externalURL, err := url.Parse(itm.ExternalID)
		if err != nil {
			return []appItem{}, err
		}
		finalAppList = append(finalAppList, appItem{
			Name:      itm.Name,
			Catalog:   externalURL.Query().Get("catalog"),
			Template:  externalURL.Query().Get("template"),
			Version:   externalURL.Query().Get("version"),
			Namespace: itm.TargetNamespace,
			State:     itm.State,
			Answers:   itm.Answers,
		})
	}

	// return the list
	return finalAppList, nil
}

// refreshAutomiumCatalog refresh the Automium catalog
func refreshAutomiumCatalog() error {
	req, err := prepareHTTPRequest(
		"POST",
		fmt.Sprintf("https://%s/v3/catalogs/automium?action=refresh", os.Getenv("RANCHER_URL")),
		nil,
	)
	if err != nil {
		return err
	}
	resp, err := defaultHTTPClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("cannot refresh catalog - error: %s (%d)", resp.Status, resp.StatusCode)
	}

	return nil

}

// deployApplication installs or upgrades the provided application
func deployApplication(clusterID, projectID string, appManifest v1beta1.Application, newInstallation bool) error {
	var deployURL string

	// Refresh the catalog to be sure the local Rancher cache is updated
	err := refreshAutomiumCatalog()
	if err != nil {
		glog.Warningf("cannot manually refresh Automium catalog: %s -- continuing without refresh, applications may be not up-to-date\n", err.Error())
	}

	// Prepare the request content
	installBody := rancherAppItem{
		Name:            appManifest.Spec.Name,
		ProjectID:       projectID,
		ExternalID:      fmt.Sprintf("catalog://?catalog=automium&template=%s&version=%s", appManifest.Spec.Name, appManifest.Spec.Version),
		Prune:           false,
		TargetNamespace: appManifest.Spec.Namespace,
	}

	// Generate the answers
	appAnswers, err := buildAppAnswers(appManifest)
	if err != nil {
		return err
	}
	installBody.Answers = appAnswers

	if newInstallation {
		// Move the namespace into the project if needed
		err = moveNamespaceInProject(clusterID, projectID, appManifest.Spec.Namespace)
		if err != nil {
			return err
		}
		// Set the deploy URL
		deployURL = fmt.Sprintf("https://%s/v3/projects/%s/app", os.Getenv("RANCHER_URL"), projectID)
	} else {
		// Set the deploy URL for upgrade
		deployURL = fmt.Sprintf("https://%s/v3/projects/%s/apps/%s:automium-monitoring?action=upgrade", os.Getenv("RANCHER_URL"), projectID, strings.Split(projectID, ":")[1])
	}

	glog.V(5).Infof("app %s new installation? %t\n", appManifest.Spec.Name, newInstallation)
	glog.V(5).Infof("app %s deploy URL: %s\n", appManifest.Spec.Name, deployURL)

	// Convert to JSON
	installBodyJSON, err := json.Marshal(installBody)
	if err != nil {
		return err
	}

	// Prepare the request
	req, err := prepareHTTPRequest(
		"POST",
		deployURL,
		bytes.NewBuffer(installBodyJSON),
	)
	if err != nil {
		return err
	}

	// Make the request
	resp, err := defaultHTTPClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	bodyResp, _ := ioutil.ReadAll(resp.Body)
	glog.V(5).Infof("Response body: %s\n", string(bodyResp))

	if resp.StatusCode != 201 && resp.StatusCode != 204 {
		return fmt.Errorf("cannot create monitoring app - error: %s (%d)", resp.Status, resp.StatusCode)
	}

	return nil

}

// generateStorageClassConfiguration produces a map for provider-specific storage class configuration
func generateStorageClassConfiguration() (map[string]string, error) {
	values := make(map[string]string)

	// Check the provider
	switch os.Getenv("PLATFORM") {
	case "openstack":
		values["automium.storageClass.provisioner"] = "kubernetes.io/cinder"
		values["automium.storageClass.parameters.\"type\""] = "Top"
		if os.Getenv("PLATFORM_REGION") != "" {
			values["automium.storageClass.parameters.\"availability\""] = os.Getenv("PLATFORM_REGION")
		}
		return values, nil
	default:
		return values, fmt.Errorf("provider not supported")
	}

}

// getDefaultAnswersForApp return default answers for specific applications
func getDefaultAnswersForApp(appName string) (map[string]string, error) {
	switch appName {
	case "automium-monitoring":
		monitoringDefValues := map[string]string{
			"prometheus.server.persistentVolume.storageClass":                                                "automium-monitoring-sc",
			"grafana.persistence.storageClassName":                                                           "automium-monitoring-sc",
			"prometheus.serverFiles.prometheus\\.yml.alerting.alertmanagers[0].static_configs[0].targets[0]": os.Getenv("ALERTMANAGER_URL"),
			"prometheus.serverFiles.prometheus\\.yml.alerting.alertmanagers[0].basic_auth.username":          os.Getenv("ALERTMANAGER_USERNAME"),
			"prometheus.serverFiles.prometheus\\.yml.alerting.alertmanagers[0].basic_auth.password":          os.Getenv("ALERTMANAGER_PASSWORD"),
			"prometheus.serverFiles.prometheus\\.yml.alerting.alertmanagers[0].scheme":                       "https",
			"prometheus.server.global.external_labels.infraName":                                             os.Getenv("ALERTMANAGER_INFRANAME"),
		}
		// Generate the configuration for the storage class
		scMap, err := generateStorageClassConfiguration()
		if err != nil {
			return map[string]string{}, err
		}
		// Add the keys
		for k, v := range scMap {
			monitoringDefValues[k] = v
		}
		// Return the map
		return monitoringDefValues, nil
	default:
		return map[string]string{}, nil
	}
}

// moveNamespaceInProject moves a namespace into a Rancher project
func moveNamespaceInProject(clusterID, projectID, namespace string) error {

	var moveRequest struct {
		ProjectID string `json:"projectId"`
	}
	moveRequest.ProjectID = projectID
	moveRequestBytes, err := json.Marshal(moveRequest)
	if err != nil {
		return err
	}

	glog.V(5).Infof("moving namespace %s into project %s\n", namespace, projectID)

	req, err := prepareHTTPRequest(
		"POST",
		fmt.Sprintf("https://%s/v3/cluster/%s/namespaces/%s?action=move", os.Getenv("RANCHER_URL"), clusterID, namespace),
		bytes.NewBuffer(moveRequestBytes),
	)
	if err != nil {
		return err
	}
	resp, err := defaultHTTPClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		failureMsg, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("cannot move namespace to project - error: %s (%d) - %s", resp.Status, resp.StatusCode, string(failureMsg))
	}

	return nil
}

func buildAppAnswers(appManifest extrasv1beta1.Application) (map[string]string, error) {
	// Set the default answers for the application...
	appAnswers, err := getDefaultAnswersForApp(appManifest.Spec.Name)
	if err != nil {
		return map[string]string{}, err
	}

	// ...and add/override them with the provided ones
	for k, v := range appManifest.Spec.Parameters {
		appAnswers[k] = v
	}
	return appAnswers, nil
}

func answersEquals(deployedAppAnswers map[string]string, appManifest extrasv1beta1.Application) (bool, error) {
	// Generate the answers for the application
	desiredAnswers, err := buildAppAnswers(appManifest)
	if err != nil {
		return false, err
	}

	return reflect.DeepEqual(deployedAppAnswers, desiredAnswers), nil
}
