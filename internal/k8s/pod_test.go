package k8s

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPodKey(t *testing.T) {
	t.Parallel()

	p := Pod{Namespace: "prod", Name: "web-1"}
	require.Equal(t, "prod/web-1", p.Key())
}
