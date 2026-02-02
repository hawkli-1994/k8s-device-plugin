package plugin

import (
	"context"
	"encoding/json"
	"fmt"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

const switchIDEnvVar = "GPU_SWITCH_ID"

type allocationHint struct {
	Node      string   `json:"node"`
	SwitchID  string   `json:"switch_id"`
	DeviceIDs []string `json:"device_ids"`
}

func (plugin *nvidiaDevicePlugin) useTopologyAllocation() bool {
	if !plugin.topologyAwareAlloc {
		return false
	}
	if plugin.nodeName == "" {
		return false
	}
	if plugin.kubeClient == nil && plugin.podLister == nil {
		return false
	}
	_, name := plugin.rm.Resource().Split()
	return name == "gpu"
}

func (plugin *nvidiaDevicePlugin) listNodePods(ctx context.Context) ([]v1.Pod, error) {
	if plugin.podLister != nil {
		return plugin.podLister(ctx)
	}
	if plugin.kubeClient == nil {
		return nil, fmt.Errorf("kube client not configured")
	}
	pods, err := plugin.kubeClient.CoreV1().Pods(metav1.NamespaceAll).List(ctx, metav1.ListOptions{
		FieldSelector: "spec.nodeName=" + plugin.nodeName,
	})
	if err != nil {
		return nil, err
	}
	klog.V(4).Infof("topology-aware-alloc: listed %d pods on node %q", len(pods.Items), plugin.nodeName)
	return pods.Items, nil
}

func (plugin *nvidiaDevicePlugin) findAllocationHint(pods []v1.Pod, requestCount int) (*allocationHint, bool) {
	if plugin.allocationHintAnnotation == "" || requestCount <= 0 {
		return nil, false
	}
	for _, pod := range pods {
		if pod.Status.Phase != v1.PodPending {
			continue
		}
		if pod.Spec.NodeName != plugin.nodeName {
			continue
		}
		rawHint := ""
		if pod.Annotations != nil {
			rawHint = pod.Annotations[plugin.allocationHintAnnotation]
		}
		if rawHint == "" {
			continue
		}
		hint, err := parseAllocationHint(rawHint)
		if err != nil {
			klog.V(4).Infof("invalid allocation hint on pod %s/%s: %v", pod.Namespace, pod.Name, err)
			continue
		}
		if hint.Node != plugin.nodeName {
			klog.V(4).Infof("allocation hint node mismatch on pod %s/%s: hint=%q node=%q", pod.Namespace, pod.Name, hint.Node, plugin.nodeName)
			continue
		}
		if len(hint.DeviceIDs) != requestCount {
			klog.V(4).Infof("allocation hint device count mismatch on pod %s/%s: hint=%d request=%d", pod.Namespace, pod.Name, len(hint.DeviceIDs), requestCount)
			continue
		}
		if !allUnique(hint.DeviceIDs) {
			klog.V(4).Infof("allocation hint has duplicate device IDs on pod %s/%s", pod.Namespace, pod.Name)
			continue
		}
		if !plugin.rm.Devices().Contains(hint.DeviceIDs...) {
			klog.V(4).Infof("allocation hint has unknown device IDs on pod %s/%s", pod.Namespace, pod.Name)
			continue
		}
		klog.V(2).Infof("topology-aware-alloc: selected hint from pod %s/%s (switch=%q, devices=%v)", pod.Namespace, pod.Name, hint.SwitchID, hint.DeviceIDs)
		return hint, true
	}
	return nil, false
}

func parseAllocationHint(raw string) (*allocationHint, error) {
	var hint allocationHint
	if err := json.Unmarshal([]byte(raw), &hint); err != nil {
		return nil, err
	}
	if hint.Node == "" {
		return nil, fmt.Errorf("missing node")
	}
	if len(hint.DeviceIDs) == 0 {
		return nil, fmt.Errorf("empty device_ids")
	}
	return &hint, nil
}

func allUnique(ids []string) bool {
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if _, exists := seen[id]; exists {
			return false
		}
		seen[id] = struct{}{}
	}
	return true
}
