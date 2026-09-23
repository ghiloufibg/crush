package tools

import (
	"context"
	_ "embed"
	"fmt"

	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/k8s"
	"github.com/charmbracelet/crush/internal/permission"
)

const K8sDeleteDeploymentToolName = "k8s_delete_deployment"

//go:embed k8s_delete_deployment.md
var k8sDeleteDeploymentDescription string

type K8sDeleteDeploymentParams struct {
	Namespace string `json:"namespace" description:"Namespace the deployment belongs to"`
	Name      string `json:"name" description:"Name of the deployment to delete"`
}

type K8sDeleteDeploymentPermissionsParams struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

type K8sDeleteDeploymentResponseMetadata struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

func NewK8sDeleteDeploymentTool(permissions permission.Service) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		K8sDeleteDeploymentToolName,
		k8sDeleteDeploymentDescription,
		func(ctx context.Context, params K8sDeleteDeploymentParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if params.Namespace == "" {
				return fantasy.NewTextErrorResponse("namespace is required"), nil
			}
			if params.Name == "" {
				return fantasy.NewTextErrorResponse("name is required"), nil
			}

			sessionID := GetSessionFromContext(ctx)
			if sessionID == "" {
				return fantasy.ToolResponse{}, fmt.Errorf("session ID is required for deleting a deployment")
			}

			granted, err := permissions.Request(
				ctx,
				permission.CreatePermissionRequest{
					SessionID:   sessionID,
					Path:        fmt.Sprintf("%s/%s", params.Namespace, params.Name),
					ToolCallID:  call.ID,
					ToolName:    K8sDeleteDeploymentToolName,
					Action:      "delete",
					Description: fmt.Sprintf("Delete deployment %s in namespace %s", params.Name, params.Namespace),
					Params:      K8sDeleteDeploymentPermissionsParams(params),
				},
			)
			if err != nil {
				return fantasy.ToolResponse{}, err
			}
			if !granted {
				return NewPermissionDeniedResponse(), nil
			}

			output, err := k8s.DeleteDeployment(ctx, params.Namespace, params.Name)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}

			return fantasy.WithResponseMetadata(
				fantasy.NewTextResponse(output),
				K8sDeleteDeploymentResponseMetadata{Namespace: params.Namespace, Name: params.Name},
			), nil
		},
	)
}
