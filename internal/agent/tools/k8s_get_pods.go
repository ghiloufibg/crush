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
}

type K8sGetPodsResponseMetadata struct {
	NumberOfPods int `json:"number_of_pods"`
}

func NewK8sGetPodsTool() fantasy.AgentTool {
	return fantasy.NewAgentTool(
		K8sGetPodsToolName,
		k8sGetPodsDescription,
		func(ctx context.Context, params K8sGetPodsParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			pods, err := k8s.ListPods(ctx, params.Namespace, params.AllNamespaces)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}

			return fantasy.WithResponseMetadata(
				fantasy.NewTextResponse(k8s.FormatPodList(pods)),
				K8sGetPodsResponseMetadata{NumberOfPods: len(pods)},
			), nil
		},
	)
}
