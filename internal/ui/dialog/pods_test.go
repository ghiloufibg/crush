package dialog

import (
	"image"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/crush/internal/k8s"
	"github.com/charmbracelet/crush/internal/ui/common"
	"github.com/charmbracelet/crush/internal/ui/styles"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/stretchr/testify/require"
)

func newPodsDialog(t *testing.T, pods []k8s.Pod) *Pods {
	t.Helper()

	sty := styles.CharmtonePantera()
	return NewPods(&common.Common{Styles: &sty}, pods)
}

func selectedPod(t *testing.T, d *Pods) k8s.Pod {
	t.Helper()

	item, ok := d.list.SelectedItem().(*PodItem)
	require.True(t, ok, "no pod selected")
	return item.pod
}

func TestNewPodsSeedsItemsInOrder(t *testing.T) {
	t.Parallel()

	pods := []k8s.Pod{
		{Namespace: "dev", Name: "api-1", Phase: "Running"},
		{Namespace: "prod", Name: "web-1", Phase: "Pending"},
	}
	d := newPodsDialog(t, pods)

	require.Len(t, d.list.FilteredItems(), 2)
	require.Equal(t, "dev/api-1", selectedPod(t, d).Key())
}

func TestPodsHandleMsgNextPreviousWraps(t *testing.T) {
	t.Parallel()

	pods := []k8s.Pod{
		{Namespace: "dev", Name: "api-1"},
		{Namespace: "dev", Name: "api-2"},
		{Namespace: "dev", Name: "api-3"},
	}
	d := newPodsDialog(t, pods)
	require.Equal(t, "dev/api-1", selectedPod(t, d).Key())

	require.Nil(t, d.HandleMsg(tea.KeyPressMsg{Code: 'j', Text: "j"}))
	require.Equal(t, "dev/api-2", selectedPod(t, d).Key())

	require.Nil(t, d.HandleMsg(tea.KeyPressMsg{Code: 'j', Text: "j"}))
	require.Equal(t, "dev/api-3", selectedPod(t, d).Key())

	// Next from the last item wraps to the first.
	require.Nil(t, d.HandleMsg(tea.KeyPressMsg{Code: 'j', Text: "j"}))
	require.Equal(t, "dev/api-1", selectedPod(t, d).Key())

	// Previous from the first item wraps to the last.
	require.Nil(t, d.HandleMsg(tea.KeyPressMsg{Code: 'k', Text: "k"}))
	require.Equal(t, "dev/api-3", selectedPod(t, d).Key())
}

func TestPodsHandleMsgDeleteReturnsActionDeletePod(t *testing.T) {
	t.Parallel()

	pods := []k8s.Pod{
		{Namespace: "dev", Name: "api-1"},
		{Namespace: "prod", Name: "web-1"},
	}
	d := newPodsDialog(t, pods)
	require.Nil(t, d.HandleMsg(tea.KeyPressMsg{Code: 'j', Text: "j"}))

	action := d.HandleMsg(tea.KeyPressMsg{Code: 'd', Text: "d"})
	del, ok := action.(ActionDeletePod)
	require.True(t, ok)
	require.Equal(t, "prod", del.Namespace)
	require.Equal(t, "web-1", del.Name)
}

func TestPodsHandleMsgLogsReturnsActionViewPodLogs(t *testing.T) {
	t.Parallel()

	pods := []k8s.Pod{
		{Namespace: "dev", Name: "api-1"},
		{Namespace: "prod", Name: "web-1"},
	}
	d := newPodsDialog(t, pods)
	require.Nil(t, d.HandleMsg(tea.KeyPressMsg{Code: 'j', Text: "j"}))

	action := d.HandleMsg(tea.KeyPressMsg{Code: 'l', Text: "l"})
	logs, ok := action.(ActionViewPodLogs)
	require.True(t, ok)
	require.Equal(t, "prod", logs.Namespace)
	require.Equal(t, "web-1", logs.Name)
}

func TestPodsHandleMsgExecReturnsActionExecPod(t *testing.T) {
	t.Parallel()

	pods := []k8s.Pod{
		{Namespace: "dev", Name: "api-1"},
		{Namespace: "prod", Name: "web-1"},
	}
	d := newPodsDialog(t, pods)
	require.Nil(t, d.HandleMsg(tea.KeyPressMsg{Code: 'j', Text: "j"}))

	action := d.HandleMsg(tea.KeyPressMsg{Code: 'e', Text: "e"})
	exec, ok := action.(ActionExecPod)
	require.True(t, ok)
	require.Equal(t, "prod", exec.Namespace)
	require.Equal(t, "web-1", exec.Name)
}

func TestPodsHandleMsgCloseReturnsActionClose(t *testing.T) {
	t.Parallel()

	d := newPodsDialog(t, nil)
	action := d.HandleMsg(tea.KeyPressMsg{Code: tea.KeyEsc})
	require.Equal(t, ActionClose{}, action)
}

func TestPodsHandleMsgDeleteWithNoPodsIsNoop(t *testing.T) {
	t.Parallel()

	d := newPodsDialog(t, nil)
	require.Nil(t, d.HandleMsg(tea.KeyPressMsg{Code: 'd', Text: "d"}))
}

func TestPodsSetPodsPreservesSelectionByKey(t *testing.T) {
	t.Parallel()

	d := newPodsDialog(t, []k8s.Pod{
		{Namespace: "dev", Name: "api-1"},
		{Namespace: "dev", Name: "api-2"},
	})
	require.Nil(t, d.HandleMsg(tea.KeyPressMsg{Code: 'j', Text: "j"}))
	require.Equal(t, "dev/api-2", selectedPod(t, d).Key())

	// A fresh snapshot reorders and adds a pod, but the previously
	// selected pod (api-2) still exists and should remain selected.
	d.SetPods([]k8s.Pod{
		{Namespace: "dev", Name: "api-0"},
		{Namespace: "dev", Name: "api-1"},
		{Namespace: "dev", Name: "api-2"},
	})
	require.Equal(t, "dev/api-2", selectedPod(t, d).Key())
}

func TestPodsSetPodsFallsBackToFirstWhenSelectionRemoved(t *testing.T) {
	t.Parallel()

	d := newPodsDialog(t, []k8s.Pod{
		{Namespace: "dev", Name: "api-1"},
		{Namespace: "dev", Name: "api-2"},
	})
	require.Nil(t, d.HandleMsg(tea.KeyPressMsg{Code: 'j', Text: "j"}))
	require.Equal(t, "dev/api-2", selectedPod(t, d).Key())

	d.SetPods([]k8s.Pod{
		{Namespace: "dev", Name: "api-3"},
		{Namespace: "dev", Name: "api-4"},
	})
	require.Equal(t, "dev/api-3", selectedPod(t, d).Key())
}

func TestPodsDrawWithNoPodsShowsEmptyState(t *testing.T) {
	t.Parallel()

	d := newPodsDialog(t, nil)
	scr := uv.NewScreenBuffer(80, 30)
	require.NotPanics(t, func() {
		d.Draw(scr, image.Rect(0, 0, 80, 30))
	})
}

func TestPodItemRenderIncludesPhaseAndRestarts(t *testing.T) {
	t.Parallel()

	d := newPodsDialog(t, []k8s.Pod{
		{Namespace: "dev", Name: "api-1", Phase: "CrashLoopBackOff", Restarts: 7},
	})
	item, ok := d.list.SelectedItem().(*PodItem)
	require.True(t, ok)

	rendered := item.Render(80)
	require.Contains(t, rendered, "dev/api-1")
	require.Contains(t, rendered, "CrashLoopBackOff")
	require.Contains(t, rendered, "7 restarts")
}
