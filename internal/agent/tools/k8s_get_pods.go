package tools

import (
	"context"
	_ "embed"

	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/k8s"
)

const K8sGetPodsToolName = "k8s_get_pods"

//go:embed k8s_get_pods.md
var k8sGetPodsDescription string

type K8sGetPodsParams struct {
	Namespace     string `json:"namespace,omitempty" description:"Namespace to list pods from (defaults to the current kubeconfig context's namespace)"`
	AllNamespaces bool   `json:"all_namespaces,omitempty" description:"List pods across all namespaces, ignoring namespace"`
	LabelSelector string `json:"label_selector,omitempty" description:"Kubernetes label selector (e.g. \"app=web,tier=frontend\") applied by kubectl itself, narrowing results before they're even fetched. Prefer this over filtering the output yourself."`
	NameContains  string `json:"name_contains,omitempty" description:"Only include pods whose name contains this substring"`
	MaxResults    int    `json:"max_results,omitempty" description:"Cap on the number of pods returned; defaults to 50. If the cluster has more matches, the response says how many were omitted instead of silently truncating."`
}

type K8sGetPodsResponseMetadata struct {
	NumberOfPods int  `json:"number_of_pods"`
	TotalMatched int  `json:"total_matched"`
	Truncated    bool `json:"truncated"`
}

func NewK8sGetPodsTool() fantasy.AgentTool {
	return fantasy.NewAgentTool(
		K8sGetPodsToolName,
		k8sGetPodsDescription,
		func(ctx context.Context, params K8sGetPodsParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			maxResults := params.MaxResults
			if maxResults <= 0 {
				maxResults = k8s.DefaultListMaxResults
			}

			result, err := k8s.ListPodsFiltered(ctx, k8s.ListPodsOptions{
				Namespace:     params.Namespace,
				AllNamespaces: params.AllNamespaces,
				LabelSelector: params.LabelSelector,
				NameContains:  params.NameContains,
				MaxResults:    maxResults,
			})
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}

			return fantasy.WithResponseMetadata(
				fantasy.NewTextResponse(k8s.FormatPodListResult(result)),
				K8sGetPodsResponseMetadata{
					NumberOfPods: len(result.Pods),
					TotalMatched: result.TotalMatched,
					Truncated:    result.Truncated,
				},
			), nil
		},
	)
}
