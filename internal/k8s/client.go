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
	listPodsTimeout = 15 * time.Second
	// deletePodTimeout must comfortably exceed Kubernetes' default pod
	// termination grace period (30s): `kubectl delete pod` blocks until
	// the pod actually terminates, so a timeout equal to that grace
	// period races it and can report a spurious timeout on a delete
	// that in fact succeeded.
	deletePodTimeout  = 45 * time.Second
	getPodLogsTimeout = 15 * time.Second

	// DefaultListMaxResults is the result cap agent tools apply to list
	// reads by default, keeping a single tool call's formatted response
	// bounded at real-cluster scale unless the caller narrows scope
	// (LabelSelector/NameContains) or explicitly raises the cap.
	DefaultListMaxResults = 50
)

// kubectlPodList and kubectlPod are a hand-rolled subset of `kubectl get
// pods -o json`'s shape — only the fields this client reads. See the
// package doc for why this avoids importing k8s.io/api/core/v1.
type kubectlPodList struct {
	Items []kubectlPod `json:"items"`
}

type kubectlPod struct {
	Metadata struct {
		Name      string            `json:"name"`
		Namespace string            `json:"namespace"`
		Labels    map[string]string `json:"labels"`
	} `json:"metadata"`
	Status struct {
		Phase             string `json:"phase"`
		ContainerStatuses []struct {
			RestartCount int `json:"restartCount"`
		} `json:"containerStatuses"`
	} `json:"status"`
}

// ListPods lists pods via `kubectl get pods -o json`, sorted by namespace
// then name. When allNamespaces is true, namespace is ignored and pods
// are listed across the whole cluster; otherwise an empty namespace uses
// the current kubeconfig context's default namespace.
//
// This is a thin, unfiltered/uncapped convenience wrapper around
// [ListPodsFiltered] for callers — chiefly [Watcher] — that always want
// the complete snapshot; agent tools wanting label/name filtering or a
// result cap should call ListPodsFiltered directly.
func ListPods(ctx context.Context, namespace string, allNamespaces bool) ([]Pod, error) {
	result, err := ListPodsFiltered(ctx, ListPodsOptions{Namespace: namespace, AllNamespaces: allNamespaces})
	if err != nil {
		return nil, err
	}
	return result.Pods, nil
}

// ListPodsOptions configures [ListPodsFiltered]. The zero value lists every
// pod in Namespace (or every namespace, if AllNamespaces), matching
// [ListPods]'s behavior.
type ListPodsOptions struct {
	Namespace     string
	AllNamespaces bool
	// LabelSelector is passed straight through to `kubectl get pods -l`,
	// so filtering happens inside the kubectl process itself — it shrinks
	// both the kubectl output and the parsing work, not just the final
	// formatted text.
	LabelSelector string
	// NameContains keeps only pods whose name contains this substring.
	// kubectl has no server-side name-substring filter, so unlike
	// LabelSelector this is necessarily applied client-side, after the
	// (possibly label-filtered) list already came back.
	NameContains string
	// MaxResults caps the number of pods returned. Zero (or negative)
	// means unlimited.
	MaxResults int
}

// ListPodsResult is the outcome of [ListPodsFiltered]. TotalMatched is the
// count after LabelSelector/NameContains filtering but before the
// MaxResults cap, so a caller that hits the cap can report how many
// results were omitted instead of leaving an agent to conclude the
// cluster only has len(Pods) matching pods.
type ListPodsResult struct {
	Pods         []Pod
	TotalMatched int
	Truncated    bool
}

// ListPodsFiltered lists pods via `kubectl get pods -o json`, applying
// opts' server-side label selector, then opts' client-side name-substring
// filter and result cap. See [ListPodsOptions] for how each filter is
// applied.
func ListPodsFiltered(ctx context.Context, opts ListPodsOptions) (ListPodsResult, error) {
	args := []string{"get", "pods", "-o", "json"}
	switch {
	case opts.AllNamespaces:
		args = append(args, "--all-namespaces")
	case opts.Namespace != "":
		args = append(args, "-n", opts.Namespace)
	}
	if opts.LabelSelector != "" {
		args = append(args, "-l", opts.LabelSelector)
	}

	cmdCtx, cancel := context.WithTimeout(ctx, listPodsTimeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "kubectl", args...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if cmdCtx.Err() != nil {
			return ListPodsResult{}, fmt.Errorf("kubectl get pods timed out")
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return ListPodsResult{}, fmt.Errorf("kubectl get pods failed: %s", msg)
	}

	pods, err := parseKubectlPodList(stdout.String())
	if err != nil {
		return ListPodsResult{}, err
	}

	return filterAndCapPods(pods, opts.NameContains, opts.MaxResults), nil
}

// filterAndCapPods applies NameContains and MaxResults to an
// already-parsed, already-sorted pod list. Split out from
// ListPodsFiltered so this logic is unit-testable without shelling out to
// kubectl.
func filterAndCapPods(pods []Pod, nameContains string, maxResults int) ListPodsResult {
	if nameContains != "" {
		filtered := make([]Pod, 0, len(pods))
		for _, pod := range pods {
			if strings.Contains(pod.Name, nameContains) {
				filtered = append(filtered, pod)
			}
		}
		pods = filtered
	}

	result := ListPodsResult{TotalMatched: len(pods)}
	if maxResults > 0 && len(pods) > maxResults {
		result.Pods = pods[:maxResults]
		result.Truncated = true
	} else {
		result.Pods = pods
	}
	return result
}

// parseKubectlPodList parses `kubectl get pods -o json` output into Pods,
// sorted by namespace then name. Split out from ListPods so the parsing and
// sorting logic can be unit tested without shelling out to kubectl.
func parseKubectlPodList(stdout string) ([]Pod, error) {
	var list kubectlPodList
	if err := json.Unmarshal([]byte(stdout), &list); err != nil {
		return nil, fmt.Errorf("error parsing kubectl output: %w", err)
	}

	pods := make([]Pod, len(list.Items))
	for i, item := range list.Items {
		restarts := 0
		for _, cs := range item.Status.ContainerStatuses {
			restarts += cs.RestartCount
		}
		pods[i] = Pod{
			Namespace: item.Metadata.Namespace,
			Name:      item.Metadata.Name,
			Phase:     item.Status.Phase,
			Restarts:  restarts,
			Labels:    item.Metadata.Labels,
		}
	}
	sort.Slice(pods, func(i, j int) bool {
		if pods[i].Namespace != pods[j].Namespace {
			return pods[i].Namespace < pods[j].Namespace
		}
		return pods[i].Name < pods[j].Name
	})
	return pods, nil
}

// DeletePod deletes a pod via `kubectl delete pod`, optionally overriding
// its termination grace period. Namespace and name are both required.
func DeletePod(ctx context.Context, namespace, name string, gracePeriodSeconds int) (string, error) {
	args := []string{"delete", "pod", name, "-n", namespace}
	if gracePeriodSeconds > 0 {
		args = append(args, "--grace-period="+strconv.Itoa(gracePeriodSeconds))
	}

	cmdCtx, cancel := context.WithTimeout(ctx, deletePodTimeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "kubectl", args...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if cmdCtx.Err() != nil {
			return "", fmt.Errorf("kubectl delete pod timed out")
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("kubectl delete pod failed: %s", msg)
	}

	output := strings.TrimSpace(stdout.String())
	if output == "" {
		output = fmt.Sprintf("pod %q deleted from namespace %q", name, namespace)
	}
	return output, nil
}

// GetPodLogs fetches the most recent tailLines lines of a pod's logs via
// `kubectl logs`, for one-shot reads such as the k8s_get_pod_logs agent
// tool. Unlike [StreamPodLogs], this does not follow (-f): it returns once
// kubectl exits. tailLines must be positive; callers (e.g. the agent tool)
// own any further bounding policy such as defaulting or capping it.
func GetPodLogs(ctx context.Context, namespace, name string, tailLines int) (string, error) {
	if namespace == "" || name == "" {
		return "", fmt.Errorf("namespace and name are required")
	}
	if tailLines <= 0 {
		return "", fmt.Errorf("tailLines must be positive")
	}

	args := []string{"logs", name, "-n", namespace, "--tail=" + strconv.Itoa(tailLines)}

	cmdCtx, cancel := context.WithTimeout(ctx, getPodLogsTimeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "kubectl", args...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if cmdCtx.Err() != nil {
			return "", fmt.Errorf("kubectl logs timed out")
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("kubectl logs failed: %s", msg)
	}

	return stdout.String(), nil
}

// FormatPodList renders pods as a tab-separated table, for tools that
// return plain text to an agent.
func FormatPodList(pods []Pod) string {
	if len(pods) == 0 {
		return "No pods found"
	}

	var b strings.Builder
	b.WriteString("NAMESPACE\tNAME\tSTATUS\tRESTARTS\n")
	for _, pod := range pods {
		fmt.Fprintf(&b, "%s\t%s\t%s\t%d\n", pod.Namespace, pod.Name, pod.Phase, pod.Restarts)
	}
	return b.String()
}

// FormatPodListResult renders result as [FormatPodList] does, appending an
// explicit truncation notice when result.Truncated — so an agent that hits
// the cap can tell it hit a cap, not conclude the cluster only has
// len(result.Pods) matching pods.
func FormatPodListResult(result ListPodsResult) string {
	s := FormatPodList(result.Pods)
	if result.Truncated {
		s += fmt.Sprintf("...%d more not shown, narrow with label_selector or name_contains\n", result.TotalMatched-len(result.Pods))
	}
	return s
}
