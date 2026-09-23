package k8s

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStreamPodLogsRejectsMissingNamespaceOrName(t *testing.T) {
	t.Parallel()

	err := StreamPodLogs(t.Context(), "", "web-1", func(string) {})
	require.Error(t, err)

	err = StreamPodLogs(t.Context(), "dev", "", func(string) {})
	require.Error(t, err)
}
