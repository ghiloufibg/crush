package k8s

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseKubectlDeploymentListSortsByNamespaceThenName(t *testing.T) {
	t.Parallel()

	stdout := `{
		"items": [
			{
				"metadata": {"name": "web", "namespace": "prod"},
				"spec": {"replicas": 3},
				"status": {"replicas": 3, "readyReplicas": 2, "updatedReplicas": 3, "availableReplicas": 2}
			},
			{
				"metadata": {"name": "api", "namespace": "dev"},
				"spec": {"replicas": 1},
				"status": {"replicas": 1, "readyReplicas": 1, "updatedReplicas": 1, "availableReplicas": 1}
			}
		]
	}`

	deployments, err := parseKubectlDeploymentList(stdout)
	require.NoError(t, err)
	require.Equal(t, []Deployment{
		{Namespace: "dev", Name: "api", Ready: "1/1", UpToDate: 1, Available: 1, Replicas: 1},
		{Namespace: "prod", Name: "web", Ready: "2/3", UpToDate: 3, Available: 2, Replicas: 3},
	}, deployments)
}

func TestParseKubectlDeploymentListEmpty(t *testing.T) {
	t.Parallel()

	deployments, err := parseKubectlDeploymentList(`{"items": []}`)
	require.NoError(t, err)
	require.Empty(t, deployments)
}

func TestParseKubectlDeploymentListInvalidJSON(t *testing.T) {
	t.Parallel()

	_, err := parseKubectlDeploymentList(`not json`)
	require.Error(t, err)
}

func TestFormatDeploymentList(t *testing.T) {
	t.Parallel()

	require.Equal(t, "No deployments found", FormatDeploymentList(nil))

	got := FormatDeploymentList([]Deployment{
		{Namespace: "dev", Name: "api", Ready: "1/1", UpToDate: 1, Available: 1, Replicas: 1},
	})
	require.Equal(t, "NAMESPACE\tNAME\tREADY\tUP-TO-DATE\tAVAILABLE\ndev\tapi\t1/1\t1\t1\n", got)
}

func TestScaleDeploymentRejectsNegativeReplicas(t *testing.T) {
	t.Parallel()

	_, err := ScaleDeployment(t.Context(), "dev", "api", -1)
	require.Error(t, err)
}
