package plugin

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"

	cfg "github.com/NVIDIA/k8s-device-plugin/api/config/v1"
	"github.com/NVIDIA/k8s-device-plugin/internal/cdi"
	"github.com/NVIDIA/k8s-device-plugin/internal/rm"
)

type testHint struct {
	Node      string   `json:"node"`
	SwitchID  string   `json:"switch_id"`
	DeviceIDs []string `json:"device_ids"`
}

func TestAllocateTopologyHintApplied(t *testing.T) {
	devices := rm.Devices{
		"GPU-0": {Device: pluginapi.Device{ID: "GPU-0", Health: pluginapi.Healthy}},
		"GPU-1": {Device: pluginapi.Device{ID: "GPU-1", Health: pluginapi.Healthy}},
	}

	hint := testHint{
		Node:      "node-a",
		SwitchID:  "sw0",
		DeviceIDs: []string{"GPU-1"},
	}
	hintJSON, err := json.Marshal(hint)
	require.NoError(t, err)

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			Annotations: map[string]string{
				"xinchao.com/gpu-allocation-hint": string(hintJSON),
			},
		},
		Spec: v1.PodSpec{
			NodeName: "node-a",
		},
		Status: v1.PodStatus{
			Phase: v1.PodPending,
		},
	}

	plugin := nvidiaDevicePlugin{
		rm: &rm.ResourceManagerMock{
			ValidateRequestFunc: func(annotatedIDs rm.AnnotatedIDs) error {
				return nil
			},
			DevicesFunc: func() rm.Devices {
				return devices
			},
			ResourceFunc: func() cfg.ResourceName {
				return cfg.ResourceName("nvidia.com/gpu")
			},
		},
		config: &cfg.Config{
			Flags: cfg.Flags{
				CommandLineFlags: cfg.CommandLineFlags{
					Plugin: &cfg.PluginCommandLineFlags{
						DeviceIDStrategy: ptr(cfg.DeviceIDStrategyUUID),
					},
				},
			},
		},
		cdiHandler: &cdi.InterfaceMock{
			QualifiedNameFunc: func(c string, s string) string {
				return "nvidia.com/" + c + "=" + s
			},
		},
		deviceListStrategies:     cfg.DeviceListStrategies{"envvar": true},
		nodeName:                 "node-a",
		topologyAwareAlloc:       true,
		allocationHintAnnotation: "xinchao.com/gpu-allocation-hint",
		podLister: func(ctx context.Context) ([]v1.Pod, error) {
			return []v1.Pod{*pod}, nil
		},
	}

	request := &pluginapi.AllocateRequest{
		ContainerRequests: []*pluginapi.ContainerAllocateRequest{
			{DevicesIds: []string{"GPU-0"}},
		},
	}

	response, err := plugin.Allocate(context.TODO(), request)
	require.NoError(t, err)
	require.Len(t, response.ContainerResponses, 1)
	container := response.ContainerResponses[0]
	require.Equal(t, "GPU-1", container.Envs["NVIDIA_VISIBLE_DEVICES"])
	require.Equal(t, "sw0", container.Envs["GPU_SWITCH_ID"])
}

func TestAllocateTopologyHintFallbackOnCountMismatch(t *testing.T) {
	devices := rm.Devices{
		"GPU-0": {Device: pluginapi.Device{ID: "GPU-0", Health: pluginapi.Healthy}},
		"GPU-1": {Device: pluginapi.Device{ID: "GPU-1", Health: pluginapi.Healthy}},
	}

	hint := testHint{
		Node:      "node-a",
		SwitchID:  "sw0",
		DeviceIDs: []string{"GPU-0", "GPU-1"},
	}
	hintJSON, err := json.Marshal(hint)
	require.NoError(t, err)

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			Annotations: map[string]string{
				"xinchao.com/gpu-allocation-hint": string(hintJSON),
			},
		},
		Spec: v1.PodSpec{
			NodeName: "node-a",
		},
		Status: v1.PodStatus{
			Phase: v1.PodPending,
		},
	}

	plugin := nvidiaDevicePlugin{
		rm: &rm.ResourceManagerMock{
			ValidateRequestFunc: func(annotatedIDs rm.AnnotatedIDs) error {
				return nil
			},
			DevicesFunc: func() rm.Devices {
				return devices
			},
			ResourceFunc: func() cfg.ResourceName {
				return cfg.ResourceName("nvidia.com/gpu")
			},
		},
		config: &cfg.Config{
			Flags: cfg.Flags{
				CommandLineFlags: cfg.CommandLineFlags{
					Plugin: &cfg.PluginCommandLineFlags{
						DeviceIDStrategy: ptr(cfg.DeviceIDStrategyUUID),
					},
				},
			},
		},
		cdiHandler: &cdi.InterfaceMock{
			QualifiedNameFunc: func(c string, s string) string {
				return "nvidia.com/" + c + "=" + s
			},
		},
		deviceListStrategies:     cfg.DeviceListStrategies{"envvar": true},
		nodeName:                 "node-a",
		topologyAwareAlloc:       true,
		allocationHintAnnotation: "xinchao.com/gpu-allocation-hint",
		podLister: func(ctx context.Context) ([]v1.Pod, error) {
			return []v1.Pod{*pod}, nil
		},
	}

	request := &pluginapi.AllocateRequest{
		ContainerRequests: []*pluginapi.ContainerAllocateRequest{
			{DevicesIds: []string{"GPU-0"}},
		},
	}

	response, err := plugin.Allocate(context.TODO(), request)
	require.NoError(t, err)
	require.Len(t, response.ContainerResponses, 1)
	container := response.ContainerResponses[0]
	require.Equal(t, "GPU-0", container.Envs["NVIDIA_VISIBLE_DEVICES"])
	_, ok := container.Envs["GPU_SWITCH_ID"]
	require.False(t, ok)
}

func TestAllocateTopologyHintIgnoredForNonGPUResource(t *testing.T) {
	devices := rm.Devices{
		"GPU-0": {Device: pluginapi.Device{ID: "GPU-0", Health: pluginapi.Healthy}},
	}

	hint := testHint{
		Node:      "node-a",
		SwitchID:  "sw0",
		DeviceIDs: []string{"GPU-0"},
	}
	hintJSON, err := json.Marshal(hint)
	require.NoError(t, err)

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			Annotations: map[string]string{
				"xinchao.com/gpu-allocation-hint": string(hintJSON),
			},
		},
		Spec: v1.PodSpec{
			NodeName: "node-a",
		},
		Status: v1.PodStatus{
			Phase: v1.PodPending,
		},
	}

	plugin := nvidiaDevicePlugin{
		rm: &rm.ResourceManagerMock{
			ValidateRequestFunc: func(annotatedIDs rm.AnnotatedIDs) error {
				return nil
			},
			DevicesFunc: func() rm.Devices {
				return devices
			},
			ResourceFunc: func() cfg.ResourceName {
				return cfg.ResourceName("nvidia.com/mig-1g.10gb")
			},
		},
		config: &cfg.Config{
			Flags: cfg.Flags{
				CommandLineFlags: cfg.CommandLineFlags{
					Plugin: &cfg.PluginCommandLineFlags{
						DeviceIDStrategy: ptr(cfg.DeviceIDStrategyUUID),
					},
				},
			},
		},
		cdiHandler: &cdi.InterfaceMock{
			QualifiedNameFunc: func(c string, s string) string {
				return "nvidia.com/" + c + "=" + s
			},
		},
		deviceListStrategies:     cfg.DeviceListStrategies{"envvar": true},
		nodeName:                 "node-a",
		topologyAwareAlloc:       true,
		allocationHintAnnotation: "xinchao.com/gpu-allocation-hint",
		podLister: func(ctx context.Context) ([]v1.Pod, error) {
			return []v1.Pod{*pod}, nil
		},
	}

	request := &pluginapi.AllocateRequest{
		ContainerRequests: []*pluginapi.ContainerAllocateRequest{
			{DevicesIds: []string{"GPU-0"}},
		},
	}

	response, err := plugin.Allocate(context.TODO(), request)
	require.NoError(t, err)
	require.Len(t, response.ContainerResponses, 1)
	container := response.ContainerResponses[0]
	require.Equal(t, "GPU-0", container.Envs["NVIDIA_VISIBLE_DEVICES"])
	_, ok := container.Envs["GPU_SWITCH_ID"]
	require.False(t, ok)
}
