package k8s

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	listDeploymentsTimeout  = 15 * time.Second
	scaleDeploymentTimeout  = 15 * time.Second
	deleteDeploymentTimeout = 15 * time.Second
)

// kubectlDeploymentList and kubectlDeployment are a hand-rolled subset of
// `kubectl get deployments -o json`'s shape — only the fields this client
// reads. See the package doc for why this avoids importing k8s.io/api/apps/v1.
type kubectlDeploymentList struct {
	Items []kubectlDeployment `json:"items"`
}

type kubectlDeployment struct {
	Metadata struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"metadata"`
	Spec struct {
		Replicas int32 `json:"replicas"`
	} `json:"spec"`
	Status struct {
		Replicas          int32 `json:"replicas"`
		ReadyReplicas     int32 `json:"readyReplicas"`
		UpdatedReplicas   int32 `json:"updatedReplicas"`
		AvailableReplicas int32 `json:"availableReplicas"`
	} `json:"status"`
}

// ListDeployments lists deployments via `kubectl get deployments -o json`,
// sorted by namespace then name. When allNamespaces is true, namespace is
// ignored and deployments are listed across the whole cluster; otherwise an
// empty namespace uses the current kubeconfig context's default namespace.
func ListDeployments(ctx context.Context, namespace string, allNamespaces bool) ([]Deployment, error) {
	args := []string{"get", "deployments", "-o", "json"}
	switch {
	case allNamespaces:
		args = append(args, "--all-namespaces")
	case namespace != "":
		args = append(args, "-n", namespace)
	}

	cmdCtx, cancel := context.WithTimeout(ctx, listDeploymentsTimeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "kubectl", args...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if cmdCtx.Err() != nil {
			return nil, fmt.Errorf("kubectl get deployments timed out")
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("kubectl get deployments failed: %s", msg)
	}

	return parseKubectlDeploymentList(stdout.String())
}

// parseKubectlDeploymentList parses `kubectl get deployments -o json` output
// into Deployments, sorted by namespace then name. Split out from
// ListDeployments so the parsing and sorting logic can be unit tested
// without shelling out to kubectl.
func parseKubectlDeploymentList(stdout string) ([]Deployment, error) {
	var list kubectlDeploymentList
	if err := json.Unmarshal([]byte(stdout), &list); err != nil {
		return nil, fmt.Errorf("error parsing kubectl output: %w", err)
	}

	deployments := make([]Deployment, len(list.Items))
	for i, item := range list.Items {
		deployments[i] = Deployment{
			Namespace: item.Metadata.Namespace,
			Name:      item.Metadata.Name,
			Ready:     fmt.Sprintf("%d/%d", item.Status.ReadyReplicas, item.Spec.Replicas),
			UpToDate:  item.Status.UpdatedReplicas,
			Available: item.Status.AvailableReplicas,
			Replicas:  item.Spec.Replicas,
		}
	}
	sort.Slice(deployments, func(i, j int) bool {
		if deployments[i].Namespace != deployments[j].Namespace {
			return deployments[i].Namespace < deployments[j].Namespace
		}
		return deployments[i].Name < deployments[j].Name
	})
	return deployments, nil
}

// ScaleDeployment scales a deployment to the given replica count via
// `kubectl scale deployment`. Namespace and name are both required.
// Replicas must not be negative.
func ScaleDeployment(ctx context.Context, namespace, name string, replicas int32) (string, error) {
	if replicas < 0 {
		return "", fmt.Errorf("replica count must not be negative")
	}

	args := []string{"scale", "deployment", name, "-n", namespace, "--replicas=" + strconv.Itoa(int(replicas))}

	cmdCtx, cancel := context.WithTimeout(ctx, scaleDeploymentTimeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "kubectl", args...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if cmdCtx.Err() != nil {
			return "", fmt.Errorf("kubectl scale deployment timed out")
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("kubectl scale deployment failed: %s", msg)
	}

	output := strings.TrimSpace(stdout.String())
	if output == "" {
		output = fmt.Sprintf("deployment %q in namespace %q scaled to %d replicas", name, namespace, replicas)
	}
	return output, nil
}

// DeleteDeployment deletes a deployment via `kubectl delete deployment`.
// Namespace and name are both required.
func DeleteDeployment(ctx context.Context, namespace, name string) (string, error) {
	args := []string{"delete", "deployment", name, "-n", namespace}

	cmdCtx, cancel := context.WithTimeout(ctx, deleteDeploymentTimeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "kubectl", args...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if cmdCtx.Err() != nil {
			return "", fmt.Errorf("kubectl delete deployment timed out")
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("kubectl delete deployment failed: %s", msg)
	}

	output := strings.TrimSpace(stdout.String())
	if output == "" {
		output = fmt.Sprintf("deployment %q deleted from namespace %q", name, namespace)
	}
	return output, nil
}

// FormatDeploymentList renders deployments as a tab-separated table, for
// tools that return plain text to an agent.
func FormatDeploymentList(deployments []Deployment) string {
	if len(deployments) == 0 {
		return "No deployments found"
	}

	var b strings.Builder
	b.WriteString("NAMESPACE\tNAME\tREADY\tUP-TO-DATE\tAVAILABLE\n")
	for _, d := range deployments {
		fmt.Fprintf(&b, "%s\t%s\t%s\t%d\t%d\n", d.Namespace, d.Name, d.Ready, d.UpToDate, d.Available)
	}
	return b.String()
}
