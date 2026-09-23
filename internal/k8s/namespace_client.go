package k8s

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"
)

const listNamespacesTimeout = 15 * time.Second

// kubectlNamespaceList is a hand-rolled subset of `kubectl get namespaces
// -o json`'s shape — only the fields this client reads. See the package doc
// for why this avoids importing k8s.io/api/core/v1.
type kubectlNamespaceList struct {
	Items []struct {
		Metadata struct {
			Name string `json:"name"`
		} `json:"metadata"`
	} `json:"items"`
}

// ListNamespaces lists namespace names via `kubectl get namespaces -o
// json`, sorted alphabetically.
func ListNamespaces(ctx context.Context) ([]string, error) {
	cmdCtx, cancel := context.WithTimeout(ctx, listNamespacesTimeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "kubectl", "get", "namespaces", "-o", "json")
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if cmdCtx.Err() != nil {
			return nil, fmt.Errorf("kubectl get namespaces timed out")
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("kubectl get namespaces failed: %s", msg)
	}

	return parseKubectlNamespaceList(stdout.String())
}

// parseKubectlNamespaceList parses `kubectl get namespaces -o json` output
// into namespace names, sorted alphabetically. Split out from ListNamespaces
// so the parsing and sorting logic can be unit tested without shelling out
// to kubectl.
func parseKubectlNamespaceList(stdout string) ([]string, error) {
	var list kubectlNamespaceList
	if err := json.Unmarshal([]byte(stdout), &list); err != nil {
		return nil, fmt.Errorf("error parsing kubectl output: %w", err)
	}

	namespaces := make([]string, len(list.Items))
	for i, item := range list.Items {
		namespaces[i] = item.Metadata.Name
	}
	sort.Strings(namespaces)
	return namespaces, nil
}
