package plugins

import (
	"context"
	"fmt"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	frameworkruntime "k8s.io/kubernetes/pkg/scheduler/framework/runtime"
)

// settings plugin name
const (
	Name              = "sample-plugin"
	preFilterStateKey = "Prefilter" + Name
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
	args   *SampleArags
	handle framework.Handle
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
	preState, err := getPreFilterState(state)
	if err != nil {
		return framework.NewStatus(framework.Error, err.Error())
	}
	//logic
	klog.V(3).Infof("filter pod : %v, node: %v, pre state: %v", pod.Name, nodeInfo.Node().Name, preState)
	return framework.NewStatus(framework.Success)
}

// type PluginFactoryWithFts func(runtime.Object, framework.Handle, plfeature.Features) (framework.Plugin, error)
func New(object runtime.Object, f framework.Handle) (framework.Plugin, error) {
	args, err := getSampleArgs(object)
	if err != nil {
		return nil, err
	}
	// validata args
	klog.V(3).Infof("get plugin config args: %+v", args)
	return &Sample{
		args:   args,
		handle: f,
	}, nil
}
