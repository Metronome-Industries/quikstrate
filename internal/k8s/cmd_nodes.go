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
	skipDaemonSets = true
	match          = ""
)

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
	nodes, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		log.Fatalf("Error getting nodes: %v\n", err)
	}
	pods, err := clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		log.Fatalf("Error getting pods: %v\n", err)
	}

	nodeList := NewNodeList(nodes, pods)

	var filters []podFilter
	if skipDaemonSets {
		filters = append(filters, func(p corev1.Pod) bool {
			if len(p.OwnerReferences) > 0 && p.OwnerReferences[0].Kind == "DaemonSet" {
				return false
			}
			return true
		})
	}

	if match != "" {
		filters = append(filters, func(p corev1.Pod) bool {
			return fuzzy.MatchFold(match, p.Name)
		})
	}

	nodeList.FilterPods(filters)

	PrettyPrint(nodeList)
}

func getPodsByNode(nodeName string, pods *corev1.PodList) map[string]*Pod {
	pl := make(map[string]*Pod)
	for _, pod := range pods.Items {
		if pod.Spec.NodeName == nodeName {
			if skipDaemonSets && len(pod.OwnerReferences) > 0 && pod.OwnerReferences[0].Kind == "DaemonSet" {
				continue
			}
			if match != "" && fuzzy.MatchFold(match, pod.Name) == false {
				continue
			}

			pod := Pod{
				Name: pod.Name,
				spec: pod,
			}

			pl[pod.Name] = &pod
		}
	}
	return pl
}

func (n Node) getPodCount() int {
	return len(n.Pods.Pods)
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

// nodeRow := table.Row{"", "", "", "", "", "", "", "", ""}
func buildNodeRow(n *Node) table.Row {
	return table.Row{
		n.Name, n.getStatus(), n.getAge(), n.getCPU(), n.getMemory(), n.getInstanceType(), n.getCapacityType(), n.getPool(),
	}
}

// podRow := table.Row{"", "", "", "", "", "", "", "", ""}
func buildPodRow(p *Pod) table.Row {
	return table.Row{
		fmt.Sprintf("\t%s", p.Name), p.spec.Status.Phase, p.Age, p.CPU, p.Memory,
	}
}

func PrettyPrint(nodes *NodeList) {
	poolCounter := 0
	nodeCounter := 0
	podCounter := 0

	headerRow := table.Row{"name", "status", "age", "cpu", "memory", "family", "type", "pool"}

	t := table.NewWriter()
	t.SetOutputMirror(os.Stdout)
	t.Style().Options.DrawBorder = false
	t.AppendHeader(headerRow)
	for _, node := range nodes.Nodes {
		if node.getPodCount() == 0 {
			continue
		}
		t.AppendRow(buildNodeRow(node))
		// t.AppendSeparator()
		for _, pod := range node.Pods.Pods {
			t.AppendRow(buildPodRow(pod))
			podCounter++
		}
		// t.AppendSeparator()
		nodeCounter++
	}
	poolCounter++

	t.AppendFooter(table.Row{fmt.Sprintf("%d pools", poolCounter), fmt.Sprintf("%d nodes", nodeCounter), fmt.Sprintf("%d pods", podCounter), "", "", ""})
	// t.SortBy([]table.SortBy{
	// 	{Name: "pool", Mode: table.Asc},
	// 	{Name: "age", Mode: table.Asc},
	// 	{Name: "node", Mode: table.Asc},
	// 	{Name: "pod", Mode: table.Asc},
	// })
	// t.SetColumnConfigs([]table.ColumnConfig{
	// 	{Number: 1, AutoMerge: true},
	// 	{Number: 2, AutoMerge: true},
	// 	{Number: 3, AutoMerge: true},
	// 	{Number: 4, AutoMerge: true},
	// 	{Number: 5, AutoMerge: true},
	// })
	t.Render()
}
