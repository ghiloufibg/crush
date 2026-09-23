package tools

import (
	"context"
	_ "embed"
	"fmt"

	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/k8s"
	"github.com/charmbracelet/crush/internal/permission"
)

const K8sScaleDeploymentToolName = "k8s_scale_deployment"

//go:embed k8s_scale_deployment.md
var k8sScaleDeploymentDescription string

type K8sScaleDeploymentParams struct {
	Namespace string `json:"namespace" description:"Namespace the deployment belongs to"`
	Name      string `json:"name" description:"Name of the deployment to scale"`
	Replicas  int    `json:"replicas" description:"Target replica count; must not be negative"`
}

type K8sScaleDeploymentPermissionsParams struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Replicas  int    `json:"replicas"`
}

type K8sScaleDeploymentResponseMetadata struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Replicas  int    `json:"replicas"`
}

func NewK8sScaleDeploymentTool(permissions permission.Service) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		K8sScaleDeploymentToolName,
		k8sScaleDeploymentDescription,
		func(ctx context.Context, params K8sScaleDeploymentParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if params.Namespace == "" {
				return fantasy.NewTextErrorResponse("namespace is required"), nil
			}
			if params.Name == "" {
				return fantasy.NewTextErrorResponse("name is required"), nil
			}
			if params.Replicas < 0 {
				return fantasy.NewTextErrorResponse("replicas must not be negative"), nil
			}

			sessionID := GetSessionFromContext(ctx)
			if sessionID == "" {
				return fantasy.ToolResponse{}, fmt.Errorf("session ID is required for scaling a deployment")
			}

			granted, err := permissions.Request(
				ctx,
				permission.CreatePermissionRequest{
					SessionID:   sessionID,
					Path:        fmt.Sprintf("%s/%s", params.Namespace, params.Name),
					ToolCallID:  call.ID,
					ToolName:    K8sScaleDeploymentToolName,
					Action:      "scale",
					Description: fmt.Sprintf("Scale deployment %s in namespace %s to %d replicas", params.Name, params.Namespace, params.Replicas),
					Params:      K8sScaleDeploymentPermissionsParams(params),
				},
			)
			if err != nil {
				return fantasy.ToolResponse{}, err
			}
			if !granted {
				return NewPermissionDeniedResponse(), nil
			}

			output, err := k8s.ScaleDeployment(ctx, params.Namespace, params.Name, int32(params.Replicas))
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}

			return fantasy.WithResponseMetadata(
				fantasy.NewTextResponse(output),
				K8sScaleDeploymentResponseMetadata{Namespace: params.Namespace, Name: params.Name, Replicas: params.Replicas},
			), nil
		},
	)
}
