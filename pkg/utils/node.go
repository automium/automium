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

// NodesAreConsistent checks if all the Automium CRD Nodes in the provided array have the same image and flavor
func NodesAreConsistent(nodes []v1beta1.Node) bool {
	// Get first node
	compareNode := nodes[0]
	for idx, node := range nodes {
		if idx == 0 {
			// Useless to compare first node with itself
			continue
		}
		if node.Status.NodeProperties.ID == "non-existent-machine" {
			continue
		}
		if compareNode.Status.NodeProperties.Flavor != node.Status.NodeProperties.Flavor || compareNode.Status.NodeProperties.Image != node.Status.NodeProperties.Image {
			return false
		}
	}
	return true
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

// NodeIsHealthy check if the Automium CRD Node health checks are in status "Passing"
func NodeIsHealthy(node v1beta1.Node) bool {

	// Check the node ID
	if node.Status.NodeProperties.ID == "non-existent-machine" {
		return false
	}

	// Check if registered services are not in "passing" status
	for _, svc := range node.Status.NodeHealthChecks {
		if svc.Status != "passing" {
			return false
		}
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
	} else {
		return false
	}
}
