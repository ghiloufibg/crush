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
		Selector struct {
			MatchLabels map[string]string `json:"matchLabels"`
		} `json:"selector"`
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
//
// This is a thin, unfiltered/uncapped convenience wrapper around
// [ListDeploymentsFiltered] for callers — chiefly [DeploymentWatcher] —
// that always want the complete snapshot; agent tools wanting
// label/name filtering or a result cap should call
// ListDeploymentsFiltered directly.
func ListDeployments(ctx context.Context, namespace string, allNamespaces bool) ([]Deployment, error) {
	result, err := ListDeploymentsFiltered(ctx, ListDeploymentsOptions{Namespace: namespace, AllNamespaces: allNamespaces})
	if err != nil {
		return nil, err
	}
	return result.Deployments, nil
}

// ListDeploymentsOptions configures [ListDeploymentsFiltered]. The zero
// value lists every deployment in Namespace (or every namespace, if
// AllNamespaces), matching [ListDeployments]'s behavior.
type ListDeploymentsOptions struct {
	Namespace     string
	AllNamespaces bool
	// LabelSelector is passed straight through to
	// `kubectl get deployments -l`, so filtering happens inside the
	// kubectl process itself — it shrinks both the kubectl output and the
	// parsing work, not just the final formatted text.
	LabelSelector string
	// NameContains keeps only deployments whose name contains this
	// substring. kubectl has no server-side name-substring filter, so
	// unlike LabelSelector this is necessarily applied client-side, after
	// the (possibly label-filtered) list already came back.
	NameContains string
	// MaxResults caps the number of deployments returned. Zero (or
	// negative) means unlimited.
	MaxResults int
}

// ListDeploymentsResult is the outcome of [ListDeploymentsFiltered].
// TotalMatched is the count after LabelSelector/NameContains filtering but
// before the MaxResults cap, so a caller that hits the cap can report how
// many results were omitted instead of leaving an agent to conclude the
// cluster only has len(Deployments) matching deployments.
type ListDeploymentsResult struct {
	Deployments  []Deployment
	TotalMatched int
	Truncated    bool
}

// ListDeploymentsFiltered lists deployments via
// `kubectl get deployments -o json`, applying opts' server-side label
// selector, then opts' client-side name-substring filter and result cap.
// See [ListDeploymentsOptions] for how each filter is applied.
func ListDeploymentsFiltered(ctx context.Context, opts ListDeploymentsOptions) (ListDeploymentsResult, error) {
	args := []string{"get", "deployments", "-o", "json"}
	switch {
	case opts.AllNamespaces:
		args = append(args, "--all-namespaces")
	case opts.Namespace != "":
		args = append(args, "-n", opts.Namespace)
	}
	if opts.LabelSelector != "" {
		args = append(args, "-l", opts.LabelSelector)
	}

	cmdCtx, cancel := context.WithTimeout(ctx, listDeploymentsTimeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "kubectl", args...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if cmdCtx.Err() != nil {
			return ListDeploymentsResult{}, fmt.Errorf("kubectl get deployments timed out")
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return ListDeploymentsResult{}, fmt.Errorf("kubectl get deployments failed: %s", msg)
	}

	deployments, err := parseKubectlDeploymentList(stdout.String())
	if err != nil {
		return ListDeploymentsResult{}, err
	}

	return filterAndCapDeployments(deployments, opts.NameContains, opts.MaxResults), nil
}

// filterAndCapDeployments applies NameContains and MaxResults to an
// already-parsed, already-sorted deployment list. Split out from
// ListDeploymentsFiltered so this logic is unit-testable without shelling
// out to kubectl.
func filterAndCapDeployments(deployments []Deployment, nameContains string, maxResults int) ListDeploymentsResult {
	if nameContains != "" {
		filtered := make([]Deployment, 0, len(deployments))
		for _, d := range deployments {
			if strings.Contains(d.Name, nameContains) {
				filtered = append(filtered, d)
			}
		}
		deployments = filtered
	}

	result := ListDeploymentsResult{TotalMatched: len(deployments)}
	if maxResults > 0 && len(deployments) > maxResults {
		result.Deployments = deployments[:maxResults]
		result.Truncated = true
	} else {
		result.Deployments = deployments
	}
	return result
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
			Selector:  item.Spec.Selector.MatchLabels,
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

// FormatDeploymentListResult renders result as [FormatDeploymentList] does,
// appending an explicit truncation notice when result.Truncated — so an
// agent that hits the cap can tell it hit a cap, not conclude the cluster
// only has len(result.Deployments) matching deployments.
func FormatDeploymentListResult(result ListDeploymentsResult) string {
	s := FormatDeploymentList(result.Deployments)
	if result.Truncated {
		s += fmt.Sprintf("...%d more not shown, narrow with label_selector or name_contains\n", result.TotalMatched-len(result.Deployments))
	}
	return s
}
