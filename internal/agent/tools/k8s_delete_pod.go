package tools

import (
	"context"
	_ "embed"
	"fmt"

	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/k8s"
	"github.com/charmbracelet/crush/internal/permission"
)

const K8sDeletePodToolName = "k8s_delete_pod"

//go:embed k8s_delete_pod.md
var k8sDeletePodDescription string

type K8sDeletePodParams struct {
	Namespace          string `json:"namespace" description:"Namespace the pod belongs to"`
	Name               string `json:"name" description:"Name of the pod to delete"`
	GracePeriodSeconds int    `json:"grace_period_seconds,omitempty" description:"Override the pod's termination grace period, in seconds"`
}

type K8sDeletePodPermissionsParams struct {
	Namespace          string `json:"namespace"`
	Name               string `json:"name"`
	GracePeriodSeconds int    `json:"grace_period_seconds"`
}

type K8sDeletePodResponseMetadata struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

func NewK8sDeletePodTool(permissions permission.Service) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		K8sDeletePodToolName,
		k8sDeletePodDescription,
		func(ctx context.Context, params K8sDeletePodParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if params.Namespace == "" {
				return fantasy.NewTextErrorResponse("namespace is required"), nil
			}
			if params.Name == "" {
				return fantasy.NewTextErrorResponse("name is required"), nil
			}

			sessionID := GetSessionFromContext(ctx)
			if sessionID == "" {
				return fantasy.ToolResponse{}, fmt.Errorf("session ID is required for deleting a pod")
			}

			granted, err := permissions.Request(
				ctx,
				permission.CreatePermissionRequest{
					SessionID:   sessionID,
					Path:        fmt.Sprintf("%s/%s", params.Namespace, params.Name),
					ToolCallID:  call.ID,
					ToolName:    K8sDeletePodToolName,
					Action:      "delete",
					Description: fmt.Sprintf("Delete pod %s in namespace %s", params.Name, params.Namespace),
					Params:      K8sDeletePodPermissionsParams(params),
				},
			)
			if err != nil {
				return fantasy.ToolResponse{}, err
			}
			if !granted {
				return NewPermissionDeniedResponse(), nil
			}

			output, err := k8s.DeletePod(ctx, params.Namespace, params.Name, params.GracePeriodSeconds)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}

			return fantasy.WithResponseMetadata(
				fantasy.NewTextResponse(output),
				K8sDeletePodResponseMetadata{Namespace: params.Namespace, Name: params.Name},
			), nil
		},
	)
}
