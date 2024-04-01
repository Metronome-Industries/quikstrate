package creds

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

type NodeList struct {
	Nodes map[string]Node
}
type PodList struct {
	Pods map[string]Pod
}
type Node struct {
	Name string
	Pods PodList
	spec corev1.Node
}
type Pod struct {
	Name string
	Type string
	spec corev1.Pod
}

func NodesCmd(cmd *cobra.Command, args []string) {
	ctx := context.Background()

	kubeConfig, err := clientcmd.BuildConfigFromFlags("", kubeConfigFile)
	if err != nil {
		log.Fatalf("Error getting kubernetes config: %v\n", err)
	}

	clientset, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		log.Fatalf("Error getting kubernetes client: %v\n", err)
	}
	nodes := makeNodeList(ctx, clientset)

	nodes.PrintPools()
}

func makeNodeList(ctx context.Context, clientset *kubernetes.Clientset) NodeList {
	nl := NodeList{
		Nodes: make(map[string]Node),
	}

	nodes, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		log.Fatalf("Error getting nodes: %v\n", err)
	}
	pods, err := clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		log.Fatalf("Error getting pods: %v\n", err)
	}

	for _, node := range nodes.Items {
		nl.Nodes[node.Name] = Node{
			Name: node.Name,
			Pods: getPodsByNodes(node.Name, pods),
			spec: node,
		}
	}
	return nl
}

func getPodsByNodes(nodeName string, pods *corev1.PodList) PodList {
	pl := PodList{
		Pods: make(map[string]Pod),
	}
	for _, pod := range pods.Items {
		if pod.Spec.NodeName == nodeName {
			pl.Pods[pod.Name] = Pod{
				Name: pod.Name,
				spec: pod,
			}
		}
	}
	return pl
}

func (nl NodeList) getPools() (pools []string) {
	for _, node := range nl.Nodes {
		if node.spec.Labels["pool"] == "" {
			log.Fatalf("Node %s missing pool label", node.Name)
		}
		pools = append(pools, node.spec.Labels["pool"])
	}
	return pools
}

func (nl NodeList) PrintPools() {
	out := make(map[string]map[string][]string)
	for _, pool := range nl.getPools() {
		out[pool] = make(map[string][]string)
		for _, node := range nl.Nodes {
			if node.spec.Labels["pool"] == pool {
				out[pool][node.Name] = make([]string, 0)
				for _, pod := range node.Pods.Pods {
					if len(pod.spec.GetOwnerReferences()) != 1 {
						continue
					}
					kind := pod.spec.GetOwnerReferences()[0].Kind
					switch kind {
					case "DaemonSet":
						continue
					case "ReplicaSet":
					case "StatefulSet":
					case "Job":
						out[pool][node.Name] = append(out[pool][node.Name], pod.Name)
					default:
						log.Fatalf("Unknown kind: %s", kind)
					}
					out[pool][node.Name] = append(out[pool][node.Name], pod.Name)
				}
			}
		}
	}
	jsonData, _ := json.MarshalIndent(out, "", "  ")
	fmt.Printf("%s\n", jsonData)

}
