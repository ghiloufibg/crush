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
