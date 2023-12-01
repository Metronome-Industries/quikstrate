package k8s

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/lithammer/fuzzysearch/fuzzy"
	"github.com/metronome-industries/quikstrate/internal/creds"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	nodePoolLabels = []string{
		"karpenter.sh/nodepool",
		"eks.amazonaws.com/nodegroup",
	}

	skipDaemonSets = true
	match          = ""
)

type PoolList struct {
	Pools map[string]*Pool
}

type Pool struct {
	Name     string
	NodeList map[string]*Node
}

type Node struct {
	Name      string
	ShortName string
	Pods      map[string]corev1.Pod
	spec      corev1.Node
}

func NodesCmd(cmd *cobra.Command, args []string) {
	ctx := context.Background()

	skipDaemonSets, _ = strconv.ParseBool(cmd.Flag("skip-daemon-sets").Value.String())
	match, _ = cmd.Flags().GetString("match")

	kubeConfig, err := clientcmd.BuildConfigFromFlags("", creds.KubeConfigFile)
	if err != nil {
		log.Fatalf("Error getting kubernetes config: %v\n", err)
	}

	clientset, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		log.Fatalf("Error getting kubernetes client: %v\n", err)
	}
	pools := makePoolList(ctx, clientset)

	pools.prettyPrint()
}

func makePoolList(ctx context.Context, clientset *kubernetes.Clientset) *PoolList {
	nodes, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		log.Fatalf("Error getting nodes: %v\n", err)
	}
	pods, err := clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		log.Fatalf("Error getting pods: %v\n", err)
	}

	pl := PoolList{
		Pools: make(map[string]*Pool),
	}

	for _, node := range nodes.Items {
		node := Node{
			Name:      node.Name,
			ShortName: strings.Split(node.Name, ".")[0],
			Pods:      getPodsByNode(node.Name, pods),
			spec:      node,
		}
		pool := node.getPool()
		if pool == "" {
			log.Printf("Node %s missing pool label", node.Name)
			continue
		}
		if _, ok := pl.Pools[pool]; !ok {
			pl.Pools[pool] = &Pool{
				Name:     pool,
				NodeList: map[string]*Node{node.Name: &node},
			}
		} else {
			pl.Pools[pool].NodeList[node.Name] = &node
		}
	}

	return &pl
}

func getPodsByNode(nodeName string, pods *corev1.PodList) map[string]corev1.Pod {
	pl := make(map[string]corev1.Pod)
	for _, pod := range pods.Items {
		if pod.Spec.NodeName == nodeName {
			if skipDaemonSets && len(pod.OwnerReferences) > 0 && pod.OwnerReferences[0].Kind == "DaemonSet" {
				continue
			}
			if match != "" && fuzzy.MatchFold(match, pod.Name) == false {
				continue
			}

			pl[pod.Name] = pod
		}
	}
	return pl
}

func (n Node) getPool() string {
	for _, label := range nodePoolLabels {
		if n.spec.Labels[label] != "" {
			return n.spec.Labels[label]
		}
	}
	return ""
}

func (n Node) getPodCount() int {
	return len(n.Pods)
}

func (n Node) getStatus() string {
	var statuses []string
	for _, condition := range n.spec.Status.Conditions {
		if condition.Status == corev1.ConditionTrue {
			statuses = append(statuses, string(condition.Type))
		}
	}
	if len(statuses) > 0 {
		return strings.Join(statuses, ",")
	}
	return "NotReady"
}

func (p Pool) getPodCount() int {
	count := 0
	for _, node := range p.NodeList {
		count += node.getPodCount()
	}
	return count
}

func (pl PoolList) prettyPrint() {
	poolCounter := 0
	nodeCounter := 0
	podCounter := 0

	t := table.NewWriter()
	t.SetOutputMirror(os.Stdout)
	t.AppendHeader(table.Row{"pool", "node", "pod", "cpu", "memory", "status"})
	for _, pool := range pl.Pools {
		if pool.getPodCount() == 0 {
			continue
		}
		t.AppendRow(table.Row{pool.Name, "", "", "", "", ""})
		for _, node := range pool.NodeList {
			if node.getPodCount() == 0 {
				continue
			}
			cpuCapacity := node.spec.Status.Capacity.Cpu().String()
			memCapacity := node.spec.Status.Capacity.Memory().String()
			t.AppendRow(table.Row{"", node.ShortName, "", cpuCapacity, memCapacity, node.getStatus()})

			for _, pod := range node.Pods {
				cpuRequests := pod.Spec.Containers[0].Resources.Requests.Cpu().String()
				memRequests := pod.Spec.Containers[0].Resources.Requests.Memory().String()
				t.AppendRow(table.Row{"", "", pod.Name, cpuRequests, memRequests, pod.Status.Phase})
				podCounter++
			}
			t.AppendSeparator()
			nodeCounter++
		}
		t.AppendSeparator()
		poolCounter++
	}
	t.AppendFooter(table.Row{fmt.Sprintf("%d pools", poolCounter), fmt.Sprintf("%d nodes", nodeCounter), fmt.Sprintf("%d pods", podCounter), "", "", ""})
	t.Render()
}
