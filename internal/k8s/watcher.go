package k8s

import (
	"context"
	"sync"
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
	broker   *pubsub.Broker[[]Pod]
	interval time.Duration
	// list defaults to the package-level ListPods; tests override it to
	// avoid shelling out to a real kubectl/cluster.
	list func(ctx context.Context, namespace string, allNamespaces bool) ([]Pod, error)

	// mu guards namespace/allNamespaces: poll (running in the Run
	// goroutine) reads them and SetNamespace (called from the UI) writes
	// them, so both need synchronization even though there's no complex
	// invariant to protect.
	mu            sync.Mutex
	namespace     string
	allNamespaces bool

	// reconfigured wakes Run's select loop immediately after SetNamespace,
	// so a namespace switch takes effect right away instead of waiting up
	// to interval for the next tick.
	reconfigured chan struct{}
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
		broker:       pubsub.NewBroker[[]Pod](),
		interval:     DefaultPollInterval,
		list:         ListPods,
		reconfigured: make(chan struct{}, 1),
	}
	for _, opt := range opts {
		opt(w)
	}
	return w
}

// SetNamespace retargets the watcher at namespace (or every namespace, when
// allNamespaces is true), taking effect immediately rather than waiting for
// the next scheduled poll. Safe to call concurrently with Run.
func (w *Watcher) SetNamespace(namespace string, allNamespaces bool) {
	w.mu.Lock()
	w.namespace = namespace
	w.allNamespaces = allNamespaces
	w.mu.Unlock()

	select {
	case w.reconfigured <- struct{}{}:
	default:
		// A reconfigure is already pending; poll will pick up the latest
		// values above once it runs.
	}
}

// Namespace reports the watcher's current namespace scope.
func (w *Watcher) Namespace() (namespace string, allNamespaces bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.namespace, w.allNamespaces
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
		case <-w.reconfigured:
			w.poll(ctx)
			ticker.Reset(w.interval)
		}
	}
}

func (w *Watcher) poll(ctx context.Context) {
	namespace, allNamespaces := w.Namespace()
	pods, err := w.list(ctx, namespace, allNamespaces)
	if err != nil {
		return
	}
	w.broker.Publish(pubsub.UpdatedEvent, pods)
}
