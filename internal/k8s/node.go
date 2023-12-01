package k8s

import (
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/duration"
)

type NodeList struct {
	Nodes []*Node
}

func (nl *NodeList) FilterPods(filters []podFilter) {
	for _, node := range nl.Nodes {
		node.FilterPods(filters)
	}
}

func NewNodeList(nl *corev1.NodeList, pl *corev1.PodList) *NodeList {
	var ret NodeList
	for _, node := range nl.Items {
		n := Node{
			Name: node.Name,
			Pods: NewPodList(pl, []podFilter{func(p corev1.Pod) bool {
				return p.Spec.NodeName == node.Name
			}}),
			spec: node,
		}
		ret.Nodes = append(ret.Nodes, &n)
	}
	return &ret
}

type Node struct {
	Name   string `json:"name"`
	Status string `json:"status"`

	Age          string `json:"age"`
	CPU          string `json:"cpu"`
	Memory       string `json:"memory"`
	InstanceType string `json:"instanceType"`
	CapacityType string `json:"capacityType"`
	Pool         string `json:"pool"`

	Pods *PodList `json:"pods"`
	spec corev1.Node
}

func (n Node) getPool() string {
	for _, label := range []string{
		"karpenter.sh/nodepool",
		"eks.amazonaws.com/nodegroup",
	} {
		if n.spec.Labels[label] != "" {
			return n.spec.Labels[label]
		}
	}
	return "unset"
}

func (n Node) getCapacityType() string {
	for _, label := range []string{
		"karpenter.sh/capacity-type",
		"eks.amazonaws.com/capacityType",
	} {
		if n.spec.Labels[label] != "" {
			return strings.ToLower(n.spec.Labels[label])
		}
	}
	return "unset"
}

func (n Node) getInstanceType() string {
	val, ok := n.spec.Labels["node.kubernetes.io/instance-type"]
	if ok {
		return strings.ToLower(val)
	}
	return "unset"
}

func (n Node) getZone() string {
	val, ok := n.spec.Labels["topology.kubernetes.io/zone"]
	if ok {
		return strings.ToLower(val)
	}
	return "unset"
}

func (n Node) getAge() string {
	return duration.ShortHumanDuration(time.Since(n.spec.CreationTimestamp.Time))
}

func (n Node) getCPU() string {
	return n.spec.Status.Capacity.Cpu().String()
}

func (n Node) getMemory() string {
	return n.spec.Status.Capacity.Memory().String()
}

func (n *Node) FilterPods(filters []podFilter) {
	n.Pods.Filter(filters)
}
