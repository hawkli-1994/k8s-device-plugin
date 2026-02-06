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

// 无效注解格式测试
func TestAllocateTopologyInvalidAnnotationFormat(t *testing.T) {
	devices := rm.Devices{
		"GPU-0": {Device: pluginapi.Device{ID: "GPU-0", Health: pluginapi.Healthy}},
		"GPU-1": {Device: pluginapi.Device{ID: "GPU-1", Health: pluginapi.Healthy}},
	}

	// 无效JSON格式
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			Annotations: map[string]string{
				"xinchao.com/gpu-allocation-hint": "invalid-json",
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

// 缺少必填字段的注解测试
func TestAllocateTopologyMissingRequiredFields(t *testing.T) {
	devices := rm.Devices{
		"GPU-0": {Device: pluginapi.Device{ID: "GPU-0", Health: pluginapi.Healthy}},
		"GPU-1": {Device: pluginapi.Device{ID: "GPU-1", Health: pluginapi.Healthy}},
	}

	// 缺少node字段
	hint := map[string]interface{}{
		"switch_id":  "sw0",
		"device_ids": []string{"GPU-1"},
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

// 设备ID不存在测试
func TestAllocateTopologyDeviceIDNotFound(t *testing.T) {
	devices := rm.Devices{
		"GPU-0": {Device: pluginapi.Device{ID: "GPU-0", Health: pluginapi.Healthy}},
	}

	hint := testHint{
		Node:      "node-a",
		SwitchID:  "sw0",
		DeviceIDs: []string{"GPU-99"},
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

// 设备ID重复测试
func TestAllocateTopologyDuplicateDeviceIDs(t *testing.T) {
	devices := rm.Devices{
		"GPU-0": {Device: pluginapi.Device{ID: "GPU-0", Health: pluginapi.Healthy}},
		"GPU-1": {Device: pluginapi.Device{ID: "GPU-1", Health: pluginapi.Healthy}},
	}

	hint := testHint{
		Node:      "node-a",
		SwitchID:  "sw0",
		DeviceIDs: []string{"GPU-0", "GPU-0"},
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
			{DevicesIds: []string{"GPU-0", "GPU-1"}},
		},
	}

	response, err := plugin.Allocate(context.TODO(), request)
	require.NoError(t, err)
	require.Len(t, response.ContainerResponses, 1)
	container := response.ContainerResponses[0]
	require.Equal(t, "GPU-0,GPU-1", container.Envs["NVIDIA_VISIBLE_DEVICES"])
	_, ok := container.Envs["GPU_SWITCH_ID"]
	require.False(t, ok)
}

// 节点名称不匹配测试
func TestAllocateTopologyNodeNameMismatch(t *testing.T) {
	devices := rm.Devices{
		"GPU-0": {Device: pluginapi.Device{ID: "GPU-0", Health: pluginapi.Healthy}},
		"GPU-1": {Device: pluginapi.Device{ID: "GPU-1", Health: pluginapi.Healthy}},
	}

	hint := testHint{
		Node:      "wrong-node",
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
	require.Equal(t, "GPU-0", container.Envs["NVIDIA_VISIBLE_DEVICES"])
	_, ok := container.Envs["GPU_SWITCH_ID"]
	require.False(t, ok)
}

// 功能禁用测试
func TestAllocateTopologyFeatureDisabled(t *testing.T) {
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
		topologyAwareAlloc:       false, // 功能禁用
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

// NODE_NAME为空测试
func TestAllocateTopologyEmptyNodeName(t *testing.T) {
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
		nodeName:                 "", // 空节点名称
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

// 没有注解的Pod测试
func TestAllocateTopologyNoAnnotation(t *testing.T) {
	devices := rm.Devices{
		"GPU-0": {Device: pluginapi.Device{ID: "GPU-0", Health: pluginapi.Healthy}},
		"GPU-1": {Device: pluginapi.Device{ID: "GPU-1", Health: pluginapi.Healthy}},
	}

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
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

// Pod非Pending状态测试
func TestAllocateTopologyPodNotPending(t *testing.T) {
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
			Phase: v1.PodRunning, // 运行中状态
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
