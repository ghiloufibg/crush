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
	deletePodTimeout = 45 * time.Second
)

// kubectlPodList and kubectlPod are a hand-rolled subset of `kubectl get
// pods -o json`'s shape — only the fields this client reads. See the
// package doc for why this avoids importing k8s.io/api/core/v1.
type kubectlPodList struct {
	Items []kubectlPod `json:"items"`
}

type kubectlPod struct {
	Metadata struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
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
func ListPods(ctx context.Context, namespace string, allNamespaces bool) ([]Pod, error) {
	args := []string{"get", "pods", "-o", "json"}
	switch {
	case allNamespaces:
		args = append(args, "--all-namespaces")
	case namespace != "":
		args = append(args, "-n", namespace)
	}

	cmdCtx, cancel := context.WithTimeout(ctx, listPodsTimeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "kubectl", args...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if cmdCtx.Err() != nil {
			return nil, fmt.Errorf("kubectl get pods timed out")
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("kubectl get pods failed: %s", msg)
	}

	return parseKubectlPodList(stdout.String())
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
