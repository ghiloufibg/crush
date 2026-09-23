package k8s

import "maps"

// Deployment is the subset of a Kubernetes deployment's state the TUI
// displays.
type Deployment struct {
	Namespace string
	Name      string
	Ready     string
	UpToDate  int32
	Available int32
	Replicas  int32
	// Selector holds the deployment's spec.selector.matchLabels, used for
	// drill-down navigation to find the Pods it owns (a Pod whose Labels
	// are a superset of Selector). Not shown in the Deployments panel
	// itself.
	Selector map[string]string
}

// Key returns the deployment's unique identifier within a cluster.
func (d Deployment) Key() string {
	return d.Namespace + "/" + d.Name
}

// Equal reports whether d and other represent the same displayed state.
// Used to skip rebuilding and redrawing the Deployments panel when a poll
// comes back identical to what's already on screen.
func (d Deployment) Equal(other Deployment) bool {
	return d.Namespace == other.Namespace &&
		d.Name == other.Name &&
		d.Ready == other.Ready &&
		d.UpToDate == other.UpToDate &&
		d.Available == other.Available &&
		d.Replicas == other.Replicas &&
		maps.Equal(d.Selector, other.Selector)
}
