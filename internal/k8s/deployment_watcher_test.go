package k8s

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDeploymentWatcherRunPublishesImmediatelyAndOnEachTick(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	snapshots := [][]Deployment{
		{{Namespace: "dev", Name: "api"}},
		{{Namespace: "dev", Name: "api"}, {Namespace: "dev", Name: "web"}},
	}

	w := NewDeploymentWatcher(WithDeploymentPollInterval(5 * time.Millisecond))
	w.list = func(context.Context, string, bool) ([]Deployment, error) {
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

func TestDeploymentWatcherPollSwallowsErrorsAndKeepsLastGoodSnapshot(t *testing.T) {
	t.Parallel()

	w := NewDeploymentWatcher()
	w.list = func(context.Context, string, bool) ([]Deployment, error) {
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

func TestDeploymentWatcherPassesNamespaceAndAllNamespacesOptions(t *testing.T) {
	t.Parallel()

	var gotNamespace string
	var gotAll bool
	w := NewDeploymentWatcher(WithDeploymentNamespace("staging"), WithDeploymentAllNamespaces())
	w.list = func(_ context.Context, namespace string, allNamespaces bool) ([]Deployment, error) {
		gotNamespace = namespace
		gotAll = allNamespaces
		return nil, nil
	}

	w.poll(context.Background())

	require.Equal(t, "staging", gotNamespace)
	require.True(t, gotAll)
}
