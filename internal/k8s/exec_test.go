package k8s

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExecPodCommandBuildsInteractiveKubectlExec(t *testing.T) {
	t.Parallel()

	cmd := ExecPodCommand("dev", "api-1")
	require.Equal(t, []string{"kubectl", "exec", "-it", "api-1", "-n", "dev", "--", "sh"}, cmd.Args)
}
