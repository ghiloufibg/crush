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
}

type K8sGetDeploymentsResponseMetadata struct {
	NumberOfDeployments int `json:"number_of_deployments"`
}

func NewK8sGetDeploymentsTool() fantasy.AgentTool {
	return fantasy.NewAgentTool(
		K8sGetDeploymentsToolName,
		k8sGetDeploymentsDescription,
		func(ctx context.Context, params K8sGetDeploymentsParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			deployments, err := k8s.ListDeployments(ctx, params.Namespace, params.AllNamespaces)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}

			return fantasy.WithResponseMetadata(
				fantasy.NewTextResponse(k8s.FormatDeploymentList(deployments)),
				K8sGetDeploymentsResponseMetadata{NumberOfDeployments: len(deployments)},
			), nil
		},
	)
}
