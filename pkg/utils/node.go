package utils

import (
	"encoding/json"
	"regexp"
	"sort"
	"strconv"

	"github.com/automium/automium/pkg/apis/core/v1beta1"
	"github.com/golang/glog"
)

// SortNodesByNumber sorts the provided Automium CRD Node array in ascending order based on final number of node name
func SortNodesByNumber(nodes []v1beta1.Node) error {
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

// EqNodeImage checks if the provided Automium CRD Node images matches the provided image name
func EqNodeImage(node v1beta1.Node, image string) bool {
	if node.Status.NodeProperties.Image == image {
		return true
	}
	return false
}

// EqNodeFlavor checks if the provided Automium CRD Node flavor matches the provided flavor name
func EqNodeFlavor(node v1beta1.Node, flavor string) bool {
	if node.Status.NodeProperties.Flavor == flavor {
		return true
	}
	return false
}

// NodesAreConsistent checks if all the Automium CRD Nodes in the provided array have the same image and flavor and returns a pointer to an healthy node for other comparisons
func NodesAreConsistent(nodes []v1beta1.Node) (bool, v1beta1.Node) {

	// Search for an healthy node for comparison
	var healthyNode v1beta1.Node

	for _, node := range nodes {
		if NodeIsHealthy(node) {
			healthyNode = node
			glog.V(5).Infof("found healthy node for comparison: %s\n", healthyNode.Status.NodeProperties.Node)
			break
		}
	}

	// If no node is healthy, they are consistently broken
	if healthyNode.Status.NodeProperties.Node == "" {
		glog.V(5).Infof("no healthy node found for comparison")
		return true, healthyNode
	}

	// Now check if nodes are consistent
	for _, node := range nodes {

		// Avoid comparing the healthy node with itself
		if node.Spec.Hostname == healthyNode.Spec.Hostname {
			continue
		}

		// Avoid checking non-available nodes
		if !NodeExists(node) {
			glog.V(5).Infof("node %s does not exist yet in the provider -- skipping\n", node.Spec.Hostname)
			continue
		}

		// Retrieve flavor and image data
		if !EqNodeFlavor(healthyNode, node.Status.NodeProperties.Flavor) || !EqNodeImage(healthyNode, node.Status.NodeProperties.Image) {
			glog.Warningf("node %s mismatch image and/or flavor -- rolling upgrade necessary for consistency\n", node.Status.NodeProperties.Node)
			return false, healthyNode
		}
	}

	// Nodes are consistent
	return true, healthyNode

}

// GetNodesCount returns the number of existent Automium CRD Nodes
func GetNodesCount(nodes []v1beta1.Node) int {
	count := 0
	for _, node := range nodes {
		if node.Status.NodeProperties.ID != "non-existent-machine" {
			count++
		}
	}
	return count
}

// NodeExists check if the Automium CRD Node ID is valid
func NodeExists(node v1beta1.Node) bool {
	if node.Status.NodeProperties.ID == "" || node.Status.NodeProperties.ID == "non-existent-machine" {
		return false
	}
	return true
}

// NodeIsHealthy check if the Automium CRD Node health checks are in status "Passing"
func NodeIsHealthy(node v1beta1.Node) bool {

	// Check the node ID
	if !NodeExists(node) {
		return false
	}

	// Check if registered services are not in "passing" status
	for _, svc := range node.Status.NodeHealthChecks {
		if svc.Status != "passing" {
			return false
		}
	}

	// Check if the image and flavor metadata has been set
	if node.Status.NodeProperties.Image == "" || node.Status.NodeProperties.Flavor == "" {
		return false
	}

	return true
}

// CompareStatuses compares two Automium CRD Node status. If there are equals, returns true
func CompareStatuses(old, new v1beta1.NodeStatus) bool {

	oldStatusJSON, err := json.Marshal(old)
	if err != nil {
		glog.Errorf("cannot marshal old status: %s\n", err.Error())
		return false
	}

	newStatusJSON, err := json.Marshal(new)
	if err != nil {
		glog.Errorf("cannot marshal new status: %s\n", err.Error())
		return false
	}

	if string(oldStatusJSON) == string(newStatusJSON) {
		return true
	}
	return false
}
