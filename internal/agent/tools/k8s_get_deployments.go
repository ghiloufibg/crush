package tools

import (
	"context"
	_ "embed"

	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/k8s"
)

const K8sGetDeploymentsToolName = "k8s_get_deployments"

//go:embed k8s_get_deployments.md
var k8sGetDeploymentsDescription string

type K8sGetDeploymentsParams struct {
	Namespace     string `json:"namespace,omitempty" description:"Namespace to list deployments from (defaults to the current kubeconfig context's namespace)"`
	AllNamespaces bool   `json:"all_namespaces,omitempty" description:"List deployments across all namespaces, ignoring namespace"`
	LabelSelector string `json:"label_selector,omitempty" description:"Kubernetes label selector (e.g. \"app=web,tier=frontend\") applied by kubectl itself, narrowing results before they're even fetched. Prefer this over filtering the output yourself."`
	NameContains  string `json:"name_contains,omitempty" description:"Only include deployments whose name contains this substring"`
	MaxResults    int    `json:"max_results,omitempty" description:"Cap on the number of deployments returned; defaults to 50. If the cluster has more matches, the response says how many were omitted instead of silently truncating."`
}

type K8sGetDeploymentsResponseMetadata struct {
	NumberOfDeployments int  `json:"number_of_deployments"`
	TotalMatched        int  `json:"total_matched"`
	Truncated           bool `json:"truncated"`
}

func NewK8sGetDeploymentsTool() fantasy.AgentTool {
	return fantasy.NewAgentTool(
		K8sGetDeploymentsToolName,
		k8sGetDeploymentsDescription,
		func(ctx context.Context, params K8sGetDeploymentsParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			maxResults := params.MaxResults
			if maxResults <= 0 {
				maxResults = k8s.DefaultListMaxResults
			}

			result, err := k8s.ListDeploymentsFiltered(ctx, k8s.ListDeploymentsOptions{
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
				fantasy.NewTextResponse(k8s.FormatDeploymentListResult(result)),
				K8sGetDeploymentsResponseMetadata{
					NumberOfDeployments: len(result.Deployments),
					TotalMatched:        result.TotalMatched,
					Truncated:           result.Truncated,
				},
			), nil
		},
	)
}
