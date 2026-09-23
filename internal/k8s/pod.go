// Package k8s provides a thin, kubectl-backed client for listing and
// deleting pods, plus a poll-based watcher that publishes live snapshots
// for the TUI. It deliberately does not depend on k8s.io/client-go or
// k8s.io/api: shelling out to kubectl and parsing the small subset of JSON
// this spike needs avoids dragging that dependency tree into a fork whose
// whole point is staying lightweight.
package k8s

// Pod is the subset of a Kubernetes pod's state the TUI displays.
type Pod struct {
	Namespace string
	Name      string
	Phase     string
	Restarts  int
}

// Key returns the pod's unique identifier within a cluster.
func (p Pod) Key() string {
	return p.Namespace + "/" + p.Name
}
