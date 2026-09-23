package k8s

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestWatcherRunPublishesImmediatelyAndOnEachTick(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	snapshots := [][]Pod{
		{{Namespace: "dev", Name: "api-1"}},
		{{Namespace: "dev", Name: "api-1"}, {Namespace: "dev", Name: "api-2"}},
	}

	w := NewWatcher(WithPollInterval(5 * time.Millisecond))
	w.list = func(context.Context, string, bool) ([]Pod, error) {
		i := calls.Add(1) - 1
		if int(i) >= len(snapshots) {
			return snapshots[len(snapshots)-1], nil
		}
		return snapshots[i], nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	sub := w.Subscribe(ctx)

	done := make(chan struct{})
	go func() {
		w.Run(ctx)
		close(done)
	}()

	first := <-sub
	require.Equal(t, snapshots[0], first.Payload)

	second := <-sub
	require.Equal(t, snapshots[1], second.Payload)

	cancel()
	<-done
}

func TestWatcherPollSwallowsErrorsAndKeepsLastGoodSnapshot(t *testing.T) {
	t.Parallel()

	w := NewWatcher()
	w.list = func(context.Context, string, bool) ([]Pod, error) {
		return nil, errors.New("kubectl unreachable")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sub := w.Subscribe(ctx)

	w.poll(ctx)

	select {
	case ev := <-sub:
		t.Fatalf("expected no publish on poll error, got %+v", ev)
	case <-time.After(20 * time.Millisecond):
	}
}

func TestWatcherPassesNamespaceAndAllNamespacesOptions(t *testing.T) {
	t.Parallel()

	var gotNamespace string
	var gotAll bool
	w := NewWatcher(WithNamespace("staging"), WithAllNamespaces())
	w.list = func(_ context.Context, namespace string, allNamespaces bool) ([]Pod, error) {
		gotNamespace = namespace
		gotAll = allNamespaces
		return nil, nil
	}

	w.poll(context.Background())

	require.Equal(t, "staging", gotNamespace)
	require.True(t, gotAll)
}
