package k8s

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/duration"
)

type podFilter func(corev1.Pod) bool

type PodList struct {
	Pods []*Pod `json:"pods"`
}

// Filter removes all pods that do match the filters
func (pl *PodList) Filter(filters []podFilter) {
	var newPods []*Pod

	for _, pod := range pl.Pods {
		filterMatch := true
		for _, f := range filters {
			if !f(pod.spec) {
				filterMatch = false
				break
			}
		}
		if filterMatch {
			newPods = append(newPods, pod)
		}
	}

	pl.Pods = newPods

}

// NewPodList returns a new PodList where all pods *match* the filters
func NewPodList(pl *corev1.PodList, filters []podFilter) *PodList {
	var ret PodList
	for _, pod := range pl.Items {
		ret.Pods = append(ret.Pods, NewPod(pod))
	}

	ret.Filter(filters)

	return &ret
}

type Pod struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Status    string `json:"status"`
	Age       string `json:"age"`
	CPU       string `json:"cpu"`
	Memory    string `json:"memory"`

	spec corev1.Pod
}

func NewPod(pod corev1.Pod) *Pod {
	return &Pod{
		Name:      pod.Name,
		Namespace: pod.Namespace,
		Status:    string(pod.Status.Phase),
		Age:       getAge(pod),
		CPU:       getCPURequests(pod),
		Memory:    getMemoryRequests(pod),

		spec: pod,
	}
}

func getAge(p corev1.Pod) string {
	return duration.ShortHumanDuration(time.Since(p.Status.StartTime.Time))
}

func getCPURequests(p corev1.Pod) string {
	ret := resource.NewMilliQuantity(0, resource.DecimalSI)
	for _, container := range p.Spec.Containers {
		if container.Resources.Requests.Cpu() != nil {
			ret.Add(*container.Resources.Requests.Cpu())
		}
	}
	return ret.String()
}

func getMemoryRequests(p corev1.Pod) string {
	ret := resource.NewQuantity(0, resource.DecimalSI)
	for _, container := range p.Spec.Containers {
		if container.Resources.Requests.Memory() != nil {
			ret.Add(*container.Resources.Requests.Memory())
		}
	}
	return ret.String()
}
