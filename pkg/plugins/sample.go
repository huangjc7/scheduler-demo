package plugins

import (
	"context"
	"fmt"
	"github.com/prometheus/client_golang/api"
	promeV1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	frameworkruntime "k8s.io/kubernetes/pkg/scheduler/framework/runtime"
	"time"
)

// settings plugin name
const (
	Name              = "sample-plugin"
	preFilterStateKey = "Prefilter" + Name
	promtheusURL      = "http://monitor-kube-prometheus-prometheus:9090"
)

// accomplish preFilter and Filter interfaces

var _ framework.PreFilterPlugin = &Sample{}
var _ framework.FilterPlugin = &Sample{}

// sample-scheduler args
type SampleArags struct {
	FavoriteColor  string `json:"favoriteColor,omitempty"`
	FavoriteNumber int    `json:"favoriteNumber,omitempty"`
	ThanksTo       string `json:"thanksTo,omitempty"`
}

// get plugin configuration args
func getSampleArgs(object runtime.Object) (*SampleArags, error) {
	sa := &SampleArags{}
	// get objecct struct args
	if err := frameworkruntime.DecodeInto(object, sa); err != nil {
		return sa, err
	}
	return sa, nil
}

//var _ framework.FilterPlugin = &PodTopologySpread{}

type preFilterState struct {
	framework.Resource // requests, limit
}

func (s *preFilterState) Clone() framework.StateData {
	return s
}

func getPreFilterState(state *framework.CycleState) (*preFilterState, error) {
	data, err := state.Read(preFilterStateKey)
	if err != nil {
		return nil, err
	}
	s, ok := data.(*preFilterState)
	if !ok {
		return nil, fmt.Errorf("%+v convet to samplePlugin preFilterState error", data)
	}
	return s, nil
}

type Sample struct {
	args             *SampleArags
	handle           framework.Handle // inherit scheduling framework
	prometheusClient promeV1.API      //prometheus client
}

// type PluginFactoryWithFts func(runtime.Object, framework.Handle, plfeature.Features) (framework.Plugin, error)
func New(object runtime.Object, f framework.Handle) (framework.Plugin, error) {
	args, err := getSampleArgs(object)
	if err != nil {
		return nil, err
	}

	// init promtheus clinet
	client, err := api.NewClient(api.Config{
		Address: promtheusURL,
	})
	if err != nil {
		return nil, fmt.Errorf("creating prometheus client failed: %v", err)
	}

	//create prometheus API client
	promeClient := promeV1.NewAPI(client)

	// validata args
	klog.V(3).Infof("get plugin config args: %+v", args)
	return &Sample{ //will plugin registry scheduling framework
		args:             args,
		handle:           f,
		prometheusClient: promeClient,
	}, nil
}

func (s *Sample) Name() string {
	return Name
}

func computePodResourceLimit(pod *v1.Pod) *preFilterState {
	result := &preFilterState{}
	for _, container := range pod.Spec.Containers {
		// limit data in preFilter lavel add to container
		result.Add(container.Resources.Limits)
	}
	return result
}

// PreFilterPlugin interface member
func (s *Sample) PreFilter(ctx context.Context, state *framework.CycleState, p *v1.Pod) (*framework.PreFilterResult, *framework.Status) {
	klog.V(3).Infof("prefilter pod: %v", p.Name)
	state.Write(preFilterStateKey, computePodResourceLimit(p))

	return nil, framework.NewStatus(framework.Success)
}

// PreFilterPlugin interface member
func (s *Sample) PreFilterExtensions() framework.PreFilterExtensions {
	return nil
}

// FilterPlugin interface member
func (s *Sample) Filter(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeInfo *framework.NodeInfo) *framework.Status {
	//query prometheus get cpu rate
	// 查询 Prometheus 获取节点的 CPU 使用率
	cpuUsage, err := s.queryNodeCPUUsage(ctx, nodeInfo.Node().Name)
	if err != nil {
		return framework.NewStatus(framework.Error, err.Error())
	}

	// 定义 CPU 使用率的阈值
	const maxAllowedCPUUsage = 0.7 // 70%

	// 如果节点的 CPU 使用率超过阈值，则不调度 Pod 到该节点
	if cpuUsage > maxAllowedCPUUsage {
		return framework.NewStatus(framework.Unschedulable, fmt.Sprintf("node's CPU usage is too high: %.2f", cpuUsage))
	}

	preState, err := getPreFilterState(state)
	if err != nil {
		return framework.NewStatus(framework.Error, err.Error())
	}
	//logic
	klog.V(3).Infof("filter pod : %v, node: %v, pre state: %v", pod.Name, nodeInfo.Node().Name, preState)
	return framework.NewStatus(framework.Success)
}

func (s *Sample) queryNodeCPUUsage(ctx context.Context, nodeName string) (float64, error) {
	// 构建 Prometheus 查询
	query := fmt.Sprintf("rate(node_cpu_seconds_total{mode='idle',instance='%s'}[1m])", nodeName)
	val, _, err := s.prometheusClient.Query(ctx, query, time.Now())
	if err != nil {
		return 0, fmt.Errorf("querying Prometheus failed: %v", err)
	}

	// 解析查询结果
	vec, ok := val.(model.Vector)
	if !ok || len(vec) == 0 {
		return 0, fmt.Errorf("invalid Prometheus response")
	}

	// 获取 CPU 空闲率
	idleCPU := float64(vec[0].Value)

	// CPU 使用率为 1 减去 CPU 空闲率
	cpuUsage := 1 - idleCPU

	return cpuUsage, nil
}
