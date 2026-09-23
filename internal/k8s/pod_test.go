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

func TestPodMatchesSelector(t *testing.T) {
	t.Parallel()

	p := Pod{Labels: map[string]string{"app": "api", "tier": "backend"}}

	require.True(t, p.MatchesSelector(map[string]string{"app": "api"}))
	require.True(t, p.MatchesSelector(map[string]string{"app": "api", "tier": "backend"}))
	require.False(t, p.MatchesSelector(map[string]string{"app": "web"}))
	require.False(t, p.MatchesSelector(map[string]string{"app": "api", "tier": "frontend"}))
	require.False(t, p.MatchesSelector(map[string]string{"missing": "label"}))
}

func TestPodMatchesSelectorEmptySelectorMatchesNothing(t *testing.T) {
	t.Parallel()

	p := Pod{Labels: map[string]string{"app": "api"}}
	require.False(t, p.MatchesSelector(nil))
	require.False(t, p.MatchesSelector(map[string]string{}))
}
