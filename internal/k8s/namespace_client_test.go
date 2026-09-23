package k8s

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseKubectlNamespaceListSortsAlphabetically(t *testing.T) {
	t.Parallel()

	stdout := `{
		"items": [
			{"metadata": {"name": "prod"}},
			{"metadata": {"name": "default"}},
			{"metadata": {"name": "dev"}}
		]
	}`

	namespaces, err := parseKubectlNamespaceList(stdout)
	require.NoError(t, err)
	require.Equal(t, []string{"default", "dev", "prod"}, namespaces)
}

func TestParseKubectlNamespaceListEmpty(t *testing.T) {
	t.Parallel()

	namespaces, err := parseKubectlNamespaceList(`{"items": []}`)
	require.NoError(t, err)
	require.Empty(t, namespaces)
}

func TestParseKubectlNamespaceListInvalidJSON(t *testing.T) {
	t.Parallel()

	_, err := parseKubectlNamespaceList(`not json`)
	require.Error(t, err)
}
