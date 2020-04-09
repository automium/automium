package monitoring

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"strings"

	"github.com/golang/glog"
)

type appItem struct {
	Name      string
	Catalog   string
	Template  string
	Version   string
	Namespace string
	State     string
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

var requiredEnvVars = []string{"RANCHER_URL", "RANCHER_CLUSTER_TOKEN", "ALERTMANAGER_URL", "ALERTMANAGER_USERNAME", "ALERTMANAGER_PASSWORD"}

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

	if resp.StatusCode != 200 {
		return fmt.Errorf("cannot refresh catalog - error: %s (%d)", resp.Status, resp.StatusCode)
	}

	return nil

}

// deployMonitoringApp installs or upgrades the monitoring app
func deployMonitoringApp(clusterID, projectID, version string, newInstallation bool) error {
	var deployURL string

	// Refresh the catalog to be sure the local Rancher cache is updated
	err := refreshAutomiumCatalog()
	if err != nil {
		glog.Warningf("cannot manually refresh Automium catalog: %s -- continuing without refresh, applications may be not up-to-date\n", err.Error())
	}

	// Prepare the request content
	installBody := rancherAppItem{
		Name:            "automium-monitoring",
		ProjectID:       projectID,
		ExternalID:      fmt.Sprintf("catalog://?catalog=automium&template=automium-monitoring&version=%s", version),
		Prune:           false,
		TargetNamespace: "automium-prometheus",
	}

	// Set the answers
	installBody.Answers = map[string]string{
		"prometheus.server.persistentVolume.storageClass":                                                "automium-monitoring-sc",
		"grafana.persistence.storageClassName":                                                           "automium-monitoring-sc",
		"prometheus.serverFiles.prometheus\\.yml.alerting.alertmanagers[0].static_configs[0].targets[0]": os.Getenv("ALERTMANAGER_URL"),
		"prometheus.serverFiles.prometheus\\.yml.alerting.alertmanagers[0].basic_auth.username":          os.Getenv("ALERTMANAGER_USERNAME"),
		"prometheus.serverFiles.prometheus\\.yml.alerting.alertmanagers[0].basic_auth.password":          os.Getenv("ALERTMANAGER_PASSWORD"),
	}

	if newInstallation {
		// Create the storage class
		if err := createMonitoringStorageClass(clusterID); err != nil {
			return err
		}
		// Set the deploy URL
		deployURL = fmt.Sprintf("https://%s/v3/projects/%s/app", os.Getenv("RANCHER_URL"), projectID)
	} else {
		// Set the deploy URL for upgrade
		deployURL = fmt.Sprintf("https://%s/v3/projects/%s/apps/%s:automium-monitoring?action=upgrade", os.Getenv("RANCHER_URL"), projectID, strings.Split(projectID, ":")[1])
	}

	glog.V(5).Infof("monitoring new installation? %t\n", newInstallation)
	glog.V(5).Infof("monitoring deploy URL: %s\n", deployURL)

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

	if resp.StatusCode != 201 {
		return fmt.Errorf("cannot create monitoring app - error: %s (%d)", resp.Status, resp.StatusCode)
	}

	return nil

}

// createMonitoringStorageClass creates a storage class used for monitoring persistent data
func createMonitoringStorageClass(clusterID string) error {
	var provisioner string

	parameters := make(map[string]string)

	// Check the provider
	switch os.Getenv("PLATFORM") {
	case "openstack":
		provisioner = "kubernetes.io/cinder"
		parameters["type"] = "Top"
		if os.Getenv("PLATFORM_REGION") != "" {
			parameters["availability"] = os.Getenv("PLATFORM_REGION")
		}
		break
	default:
		return fmt.Errorf("provider not supported")
	}

	// Prepare the request content
	requestBody := rancherStorageClassItem{
		Name:        "automium-monitoring-sc",
		Type:        "storageClass",
		Provisioner: provisioner,
		Parameters:  parameters,
	}

	// Convert to JSON
	requestBodyJSON, err := json.Marshal(requestBody)
	if err != nil {
		return err
	}

	// Prepare the request
	req, err := prepareHTTPRequest(
		"POST",
		fmt.Sprintf("https://%s/v3/clusters/%s/storageclass", os.Getenv("RANCHER_URL"), clusterID),
		bytes.NewBuffer(requestBodyJSON),
	)
	if err != nil {
		return err
	}

	// Make the request
	_, err = defaultHTTPClient().Do(req)
	return err

}
