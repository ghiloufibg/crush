package k8s

import (
	"context"
	"time"

	"github.com/charmbracelet/crush/internal/pubsub"
)

// DefaultPollInterval is how often the watcher re-lists pods when no
// interval is supplied via WithPollInterval.
const DefaultPollInterval = 3 * time.Second

// Watcher polls the cluster on a fixed interval and publishes the full
// current pod list as a single snapshot on every tick. The UI only ever
// needs "replace the displayed list with the latest snapshot", so this
// intentionally does not diff against the previous poll or emit
// per-pod created/updated/deleted events — kubectl's watch mode doesn't
// reliably surface deletions across versions anyway, and a snapshot
// replace is simpler to get right for a validation spike.
type Watcher struct {
	broker        *pubsub.Broker[[]Pod]
	namespace     string
	allNamespaces bool
	interval      time.Duration
	// list defaults to the package-level ListPods; tests override it to
	// avoid shelling out to a real kubectl/cluster.
	list func(ctx context.Context, namespace string, allNamespaces bool) ([]Pod, error)
}

// Option configures a [Watcher].
type Option func(*Watcher)

// WithNamespace restricts polling to a single namespace.
func WithNamespace(namespace string) Option {
	return func(w *Watcher) { w.namespace = namespace }
}

// WithAllNamespaces polls pods across every namespace, overriding
// WithNamespace.
func WithAllNamespaces() Option {
	return func(w *Watcher) { w.allNamespaces = true }
}

// WithPollInterval overrides DefaultPollInterval.
func WithPollInterval(d time.Duration) Option {
	return func(w *Watcher) { w.interval = d }
}

// NewWatcher creates a Watcher. Call Run in a goroutine to start polling.
func NewWatcher(opts ...Option) *Watcher {
	w := &Watcher{
		broker:   pubsub.NewBroker[[]Pod](),
		interval: DefaultPollInterval,
		list:     ListPods,
	}
	for _, opt := range opts {
		opt(w)
	}
	return w
}

// Subscribe returns a channel of pod-list snapshots. The channel closes
// when ctx is done.
func (w *Watcher) Subscribe(ctx context.Context) <-chan pubsub.Event[[]Pod] {
	return w.broker.Subscribe(ctx)
}

// Run polls immediately and then on the configured interval until ctx is
// done, publishing each snapshot it successfully retrieves. Poll errors
// (e.g. kubectl not installed, no cluster reachable) are swallowed rather
// than surfaced: the panel simply keeps showing the last good snapshot
// until polling recovers, which is preferable to flashing an error every
// interval while a cluster is briefly unreachable.
func (w *Watcher) Run(ctx context.Context) {
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

func (w *Watcher) poll(ctx context.Context) {
	pods, err := w.list(ctx, w.namespace, w.allNamespaces)
	if err != nil {
		return
	}
	w.broker.Publish(pubsub.UpdatedEvent, pods)
}
