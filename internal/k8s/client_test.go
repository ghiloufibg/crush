package k8s

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseKubectlPodListSortsByNamespaceThenName(t *testing.T) {
	t.Parallel()

	stdout := `{
		"items": [
			{
				"metadata": {"name": "web-2", "namespace": "prod"},
				"status": {"phase": "Running", "containerStatuses": [{"restartCount": 1}, {"restartCount": 2}]}
			},
			{
				"metadata": {"name": "web-1", "namespace": "prod"},
				"status": {"phase": "Pending", "containerStatuses": []}
			},
			{
				"metadata": {"name": "api-1", "namespace": "dev"},
				"status": {"phase": "Running"}
			}
		]
	}`

	pods, err := parseKubectlPodList(stdout)
	require.NoError(t, err)
	require.Equal(t, []Pod{
		{Namespace: "dev", Name: "api-1", Phase: "Running", Restarts: 0},
		{Namespace: "prod", Name: "web-1", Phase: "Pending", Restarts: 0},
		{Namespace: "prod", Name: "web-2", Phase: "Running", Restarts: 3},
	}, pods)
}

func TestParseKubectlPodListCapturesLabels(t *testing.T) {
	t.Parallel()

	stdout := `{
		"items": [
			{
				"metadata": {"name": "api-1", "namespace": "dev", "labels": {"app": "api", "tier": "backend"}},
				"status": {"phase": "Running"}
			}
		]
	}`

	pods, err := parseKubectlPodList(stdout)
	require.NoError(t, err)
	require.Equal(t, map[string]string{"app": "api", "tier": "backend"}, pods[0].Labels)
}

func TestParseKubectlPodListEmpty(t *testing.T) {
	t.Parallel()

	pods, err := parseKubectlPodList(`{"items": []}`)
	require.NoError(t, err)
	require.Empty(t, pods)
}

func TestParseKubectlPodListInvalidJSON(t *testing.T) {
	t.Parallel()

	_, err := parseKubectlPodList(`not json`)
	require.Error(t, err)
}

func TestFormatPodList(t *testing.T) {
	t.Parallel()

	require.Equal(t, "No pods found", FormatPodList(nil))

	got := FormatPodList([]Pod{
		{Namespace: "dev", Name: "api-1", Phase: "Running", Restarts: 2},
	})
	require.Equal(t, "NAMESPACE\tNAME\tSTATUS\tRESTARTS\ndev\tapi-1\tRunning\t2\n", got)
}

func TestFilterAndCapPodsNameContains(t *testing.T) {
	t.Parallel()

	pods := []Pod{
		{Namespace: "dev", Name: "api-1"},
		{Namespace: "dev", Name: "web-1"},
		{Namespace: "dev", Name: "api-2"},
	}

	result := filterAndCapPods(pods, "api", 0)
	require.Equal(t, []Pod{
		{Namespace: "dev", Name: "api-1"},
		{Namespace: "dev", Name: "api-2"},
	}, result.Pods)
	require.Equal(t, 2, result.TotalMatched)
	require.False(t, result.Truncated)
}

func TestFilterAndCapPodsMaxResults(t *testing.T) {
	t.Parallel()

	pods := []Pod{
		{Namespace: "dev", Name: "api-1"},
		{Namespace: "dev", Name: "api-2"},
		{Namespace: "dev", Name: "api-3"},
	}

	result := filterAndCapPods(pods, "", 2)
	require.Equal(t, pods[:2], result.Pods)
	require.Equal(t, 3, result.TotalMatched)
	require.True(t, result.Truncated)
}

func TestFilterAndCapPodsMaxResultsNotExceeded(t *testing.T) {
	t.Parallel()

	pods := []Pod{{Namespace: "dev", Name: "api-1"}}

	result := filterAndCapPods(pods, "", 50)
	require.Equal(t, pods, result.Pods)
	require.False(t, result.Truncated)
}

func TestFormatPodListResultTruncated(t *testing.T) {
	t.Parallel()

	result := ListPodsResult{
		Pods:         []Pod{{Namespace: "dev", Name: "api-1", Phase: "Running", Restarts: 0}},
		TotalMatched: 4,
		Truncated:    true,
	}
	got := FormatPodListResult(result)
	require.Equal(t,
		"NAMESPACE\tNAME\tSTATUS\tRESTARTS\ndev\tapi-1\tRunning\t0\n...3 more not shown, narrow with label_selector or name_contains\n",
		got,
	)
}

func TestFormatPodListResultNotTruncated(t *testing.T) {
	t.Parallel()

	result := ListPodsResult{Pods: nil, TotalMatched: 0, Truncated: false}
	require.Equal(t, "No pods found", FormatPodListResult(result))
}
