package k8s

// Deployment is the subset of a Kubernetes deployment's state the TUI
// displays.
type Deployment struct {
	Namespace string
	Name      string
	Ready     string
	UpToDate  int32
	Available int32
	Replicas  int32
}

// Key returns the deployment's unique identifier within a cluster.
func (d Deployment) Key() string {
	return d.Namespace + "/" + d.Name
}
