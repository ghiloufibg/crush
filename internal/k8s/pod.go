// Package k8s provides a thin, kubectl-backed client for listing and
// deleting pods, plus a poll-based watcher that publishes live snapshots
// for the TUI. It deliberately does not depend on k8s.io/client-go or
// k8s.io/api: shelling out to kubectl and parsing the small subset of JSON
// this spike needs avoids dragging that dependency tree into a fork whose
// whole point is staying lightweight.
package k8s

import "maps"

// Pod is the subset of a Kubernetes pod's state the TUI displays.
type Pod struct {
	Namespace string
	Name      string
	Phase     string
	Restarts  int
	// Labels holds the pod's metadata.labels, used to match it against a
	// Deployment's Selector for drill-down navigation (Deployment -> its
	// Pods). Not shown in the Pods panel itself.
	Labels map[string]string
}

// Key returns the pod's unique identifier within a cluster.
func (p Pod) Key() string {
	return p.Namespace + "/" + p.Name
}

// Equal reports whether p and other represent the same displayed state.
// Used to skip rebuilding and redrawing the Pods panel when a poll comes
// back identical to what's already on screen.
func (p Pod) Equal(other Pod) bool {
	return p.Namespace == other.Namespace &&
		p.Name == other.Name &&
		p.Phase == other.Phase &&
		p.Restarts == other.Restarts &&
		maps.Equal(p.Labels, other.Labels)
}

// MatchesSelector reports whether the pod's labels satisfy selector, i.e.
// every key/value pair in selector is present in the pod's labels. An
// empty selector matches no pods — a Deployment with no selector captured
// (e.g. from an older snapshot) should not fall back to showing every pod
// in the namespace.
func (p Pod) MatchesSelector(selector map[string]string) bool {
	if len(selector) == 0 {
		return false
	}
	for k, v := range selector {
		if p.Labels[k] != v {
			return false
		}
	}
	return true
}
