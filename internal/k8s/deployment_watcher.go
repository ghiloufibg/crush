package k8s

import (
	"context"
	"time"

	"github.com/charmbracelet/crush/internal/pubsub"
)

// DeploymentWatcher polls the cluster on a fixed interval and publishes the
// full current deployment list as a single snapshot on every tick. This is
// a concrete duplicate of [Watcher] rather than a generic Watcher[T]:
// generalizing would require every functional option (e.g.
// WithAllNamespaces) to be instantiated explicitly at each call site since
// Go can't infer a type parameter from a zero-argument option, which is a
// real syntax cost for only two resource types so far. Revisit generics if
// a third resource type makes the duplication itself the bigger cost.
type DeploymentWatcher struct {
	broker        *pubsub.Broker[[]Deployment]
	namespace     string
	allNamespaces bool
	interval      time.Duration
	// list defaults to the package-level ListDeployments; tests override it
	// to avoid shelling out to a real kubectl/cluster.
	list func(ctx context.Context, namespace string, allNamespaces bool) ([]Deployment, error)
}

// DeploymentOption configures a [DeploymentWatcher].
type DeploymentOption func(*DeploymentWatcher)

// WithDeploymentNamespace restricts polling to a single namespace.
func WithDeploymentNamespace(namespace string) DeploymentOption {
	return func(w *DeploymentWatcher) { w.namespace = namespace }
}

// WithDeploymentAllNamespaces polls deployments across every namespace,
// overriding WithDeploymentNamespace.
func WithDeploymentAllNamespaces() DeploymentOption {
	return func(w *DeploymentWatcher) { w.allNamespaces = true }
}

// WithDeploymentPollInterval overrides DefaultPollInterval.
func WithDeploymentPollInterval(d time.Duration) DeploymentOption {
	return func(w *DeploymentWatcher) { w.interval = d }
}

// NewDeploymentWatcher creates a DeploymentWatcher. Call Run in a goroutine
// to start polling.
func NewDeploymentWatcher(opts ...DeploymentOption) *DeploymentWatcher {
	w := &DeploymentWatcher{
		broker:   pubsub.NewBroker[[]Deployment](),
		interval: DefaultPollInterval,
		list:     ListDeployments,
	}
	for _, opt := range opts {
		opt(w)
	}
	return w
}

// Subscribe returns a channel of deployment-list snapshots. The channel
// closes when ctx is done.
func (w *DeploymentWatcher) Subscribe(ctx context.Context) <-chan pubsub.Event[[]Deployment] {
	return w.broker.Subscribe(ctx)
}

// Run polls immediately and then on the configured interval until ctx is
// done, publishing each snapshot it successfully retrieves. Poll errors are
// swallowed rather than surfaced, same rationale as [Watcher.Run].
func (w *DeploymentWatcher) Run(ctx context.Context) {
	w.poll(ctx)

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.poll(ctx)
		}
	}
}

func (w *DeploymentWatcher) poll(ctx context.Context) {
	deployments, err := w.list(ctx, w.namespace, w.allNamespaces)
	if err != nil {
		return
	}
	w.broker.Publish(pubsub.UpdatedEvent, deployments)
}
