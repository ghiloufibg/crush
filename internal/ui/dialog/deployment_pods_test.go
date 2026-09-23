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

func newDeploymentPodsDialog(t *testing.T, namespace, name string, selector map[string]string, allPods []k8s.Pod) *DeploymentPods {
	t.Helper()

	sty := styles.CharmtonePantera()
	return NewDeploymentPods(&common.Common{Styles: &sty}, namespace, name, selector, allPods)
}

func selectedDeploymentPod(t *testing.T, d *DeploymentPods) k8s.Pod {
	t.Helper()

	item, ok := d.list.SelectedItem().(*PodItem)
	require.True(t, ok, "no pod selected")
	return item.pod
}

func TestNewDeploymentPodsFiltersByNamespaceAndSelector(t *testing.T) {
	t.Parallel()

	allPods := []k8s.Pod{
		{Namespace: "dev", Name: "api-1", Labels: map[string]string{"app": "api"}},
		{Namespace: "dev", Name: "web-1", Labels: map[string]string{"app": "web"}},
		{Namespace: "prod", Name: "api-1", Labels: map[string]string{"app": "api"}},
	}
	d := newDeploymentPodsDialog(t, "dev", "api", map[string]string{"app": "api"}, allPods)

	require.Len(t, d.list.FilteredItems(), 1)
	require.Equal(t, "dev/api-1", selectedDeploymentPod(t, d).Key())
}

func TestDeploymentPodsHandleMsgDeleteReturnsActionDeletePod(t *testing.T) {
	t.Parallel()

	allPods := []k8s.Pod{
		{Namespace: "dev", Name: "api-1", Labels: map[string]string{"app": "api"}},
	}
	d := newDeploymentPodsDialog(t, "dev", "api", map[string]string{"app": "api"}, allPods)

	action := d.HandleMsg(tea.KeyPressMsg{Code: 'd', Text: "d"})
	del, ok := action.(ActionDeletePod)
	require.True(t, ok)
	require.Equal(t, "dev", del.Namespace)
	require.Equal(t, "api-1", del.Name)
}

func TestDeploymentPodsHandleMsgLogsReturnsActionViewPodLogs(t *testing.T) {
	t.Parallel()

	allPods := []k8s.Pod{
		{Namespace: "dev", Name: "api-1", Labels: map[string]string{"app": "api"}},
	}
	d := newDeploymentPodsDialog(t, "dev", "api", map[string]string{"app": "api"}, allPods)

	action := d.HandleMsg(tea.KeyPressMsg{Code: 'l', Text: "l"})
	logs, ok := action.(ActionViewPodLogs)
	require.True(t, ok)
	require.Equal(t, "dev", logs.Namespace)
	require.Equal(t, "api-1", logs.Name)
}

func TestDeploymentPodsHandleMsgCloseReturnsActionClose(t *testing.T) {
	t.Parallel()

	d := newDeploymentPodsDialog(t, "dev", "api", map[string]string{"app": "api"}, nil)
	action := d.HandleMsg(tea.KeyPressMsg{Code: tea.KeyEsc})
	require.Equal(t, ActionClose{}, action)
}

func TestDeploymentPodsSetAllPodsPreservesSelectionByKeyAndRefilters(t *testing.T) {
	t.Parallel()

	d := newDeploymentPodsDialog(t, "dev", "api", map[string]string{"app": "api"}, []k8s.Pod{
		{Namespace: "dev", Name: "api-1", Labels: map[string]string{"app": "api"}},
		{Namespace: "dev", Name: "api-2", Labels: map[string]string{"app": "api"}},
	})
	require.Nil(t, d.HandleMsg(tea.KeyPressMsg{Code: 'j', Text: "j"}))
	require.Equal(t, "dev/api-2", selectedDeploymentPod(t, d).Key())

	// A fresh full-cluster snapshot adds an unrelated pod and reorders, but
	// the previously selected pod (api-2) still exists and matches the
	// selector, so it should remain selected.
	d.SetAllPods([]k8s.Pod{
		{Namespace: "dev", Name: "web-1", Labels: map[string]string{"app": "web"}},
		{Namespace: "dev", Name: "api-1", Labels: map[string]string{"app": "api"}},
		{Namespace: "dev", Name: "api-2", Labels: map[string]string{"app": "api"}},
	})
	require.Len(t, d.list.FilteredItems(), 2)
	require.Equal(t, "dev/api-2", selectedDeploymentPod(t, d).Key())
}

func TestDeploymentPodsSetAllPodsFallsBackToFirstWhenSelectionRemoved(t *testing.T) {
	t.Parallel()

	d := newDeploymentPodsDialog(t, "dev", "api", map[string]string{"app": "api"}, []k8s.Pod{
		{Namespace: "dev", Name: "api-1", Labels: map[string]string{"app": "api"}},
		{Namespace: "dev", Name: "api-2", Labels: map[string]string{"app": "api"}},
	})
	require.Nil(t, d.HandleMsg(tea.KeyPressMsg{Code: 'j', Text: "j"}))
	require.Equal(t, "dev/api-2", selectedDeploymentPod(t, d).Key())

	d.SetAllPods([]k8s.Pod{
		{Namespace: "dev", Name: "api-3", Labels: map[string]string{"app": "api"}},
	})
	require.Equal(t, "dev/api-3", selectedDeploymentPod(t, d).Key())
}

func TestDeploymentPodsDrawWithNoPodsShowsEmptyState(t *testing.T) {
	t.Parallel()

	d := newDeploymentPodsDialog(t, "dev", "api", map[string]string{"app": "api"}, nil)
	scr := uv.NewScreenBuffer(80, 30)
	require.NotPanics(t, func() {
		d.Draw(scr, image.Rect(0, 0, 80, 30))
	})
}
