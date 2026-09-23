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

func newDeploymentsDialog(t *testing.T, deployments []k8s.Deployment) *Deployments {
	t.Helper()

	sty := styles.CharmtonePantera()
	return NewDeployments(&common.Common{Styles: &sty}, deployments)
}

func selectedDeployment(t *testing.T, d *Deployments) k8s.Deployment {
	t.Helper()

	item, ok := d.list.SelectedItem().(*DeploymentItem)
	require.True(t, ok, "no deployment selected")
	return item.deployment
}

func TestNewDeploymentsSeedsItemsInOrder(t *testing.T) {
	t.Parallel()

	deployments := []k8s.Deployment{
		{Namespace: "dev", Name: "api", Replicas: 1},
		{Namespace: "prod", Name: "web", Replicas: 2},
	}
	d := newDeploymentsDialog(t, deployments)

	require.Len(t, d.list.FilteredItems(), 2)
	require.Equal(t, "dev/api", selectedDeployment(t, d).Key())
}

func TestDeploymentsHandleMsgNextPreviousWraps(t *testing.T) {
	t.Parallel()

	deployments := []k8s.Deployment{
		{Namespace: "dev", Name: "api-1"},
		{Namespace: "dev", Name: "api-2"},
		{Namespace: "dev", Name: "api-3"},
	}
	d := newDeploymentsDialog(t, deployments)
	require.Equal(t, "dev/api-1", selectedDeployment(t, d).Key())

	require.Nil(t, d.HandleMsg(tea.KeyPressMsg{Code: 'j', Text: "j"}))
	require.Equal(t, "dev/api-2", selectedDeployment(t, d).Key())

	require.Nil(t, d.HandleMsg(tea.KeyPressMsg{Code: 'j', Text: "j"}))
	require.Equal(t, "dev/api-3", selectedDeployment(t, d).Key())

	// Next from the last item wraps to the first.
	require.Nil(t, d.HandleMsg(tea.KeyPressMsg{Code: 'j', Text: "j"}))
	require.Equal(t, "dev/api-1", selectedDeployment(t, d).Key())

	// Previous from the first item wraps to the last.
	require.Nil(t, d.HandleMsg(tea.KeyPressMsg{Code: 'k', Text: "k"}))
	require.Equal(t, "dev/api-3", selectedDeployment(t, d).Key())
}

func TestDeploymentsHandleMsgDeleteReturnsActionDeleteDeployment(t *testing.T) {
	t.Parallel()

	deployments := []k8s.Deployment{
		{Namespace: "dev", Name: "api-1"},
		{Namespace: "prod", Name: "web-1"},
	}
	d := newDeploymentsDialog(t, deployments)
	require.Nil(t, d.HandleMsg(tea.KeyPressMsg{Code: 'j', Text: "j"}))

	action := d.HandleMsg(tea.KeyPressMsg{Code: 'd', Text: "d"})
	del, ok := action.(ActionDeleteDeployment)
	require.True(t, ok)
	require.Equal(t, "prod", del.Namespace)
	require.Equal(t, "web-1", del.Name)
}

func TestDeploymentsHandleMsgCloseReturnsActionClose(t *testing.T) {
	t.Parallel()

	d := newDeploymentsDialog(t, nil)
	action := d.HandleMsg(tea.KeyPressMsg{Code: tea.KeyEsc})
	require.Equal(t, ActionClose{}, action)
}

func TestDeploymentsHandleMsgDeleteWithNoDeploymentsIsNoop(t *testing.T) {
	t.Parallel()

	d := newDeploymentsDialog(t, nil)
	require.Nil(t, d.HandleMsg(tea.KeyPressMsg{Code: 'd', Text: "d"}))
}

func TestDeploymentsSetDeploymentsPreservesSelectionByKey(t *testing.T) {
	t.Parallel()

	d := newDeploymentsDialog(t, []k8s.Deployment{
		{Namespace: "dev", Name: "api-1"},
		{Namespace: "dev", Name: "api-2"},
	})
	require.Nil(t, d.HandleMsg(tea.KeyPressMsg{Code: 'j', Text: "j"}))
	require.Equal(t, "dev/api-2", selectedDeployment(t, d).Key())

	// A fresh snapshot reorders and adds a deployment, but the previously
	// selected deployment (api-2) still exists and should remain selected.
	d.SetDeployments([]k8s.Deployment{
		{Namespace: "dev", Name: "api-0"},
		{Namespace: "dev", Name: "api-1"},
		{Namespace: "dev", Name: "api-2"},
	})
	require.Equal(t, "dev/api-2", selectedDeployment(t, d).Key())
}

func TestDeploymentsSetDeploymentsFallsBackToFirstWhenSelectionRemoved(t *testing.T) {
	t.Parallel()

	d := newDeploymentsDialog(t, []k8s.Deployment{
		{Namespace: "dev", Name: "api-1"},
		{Namespace: "dev", Name: "api-2"},
	})
	require.Nil(t, d.HandleMsg(tea.KeyPressMsg{Code: 'j', Text: "j"}))
	require.Equal(t, "dev/api-2", selectedDeployment(t, d).Key())

	d.SetDeployments([]k8s.Deployment{
		{Namespace: "dev", Name: "api-3"},
		{Namespace: "dev", Name: "api-4"},
	})
	require.Equal(t, "dev/api-3", selectedDeployment(t, d).Key())
}

func TestDeploymentsDrawWithNoDeploymentsShowsEmptyState(t *testing.T) {
	t.Parallel()

	d := newDeploymentsDialog(t, nil)
	scr := uv.NewScreenBuffer(80, 30)
	require.NotPanics(t, func() {
		d.Draw(scr, image.Rect(0, 0, 80, 30))
	})
}

func TestDeploymentItemRenderIncludesReadyUpToDateAvailable(t *testing.T) {
	t.Parallel()

	d := newDeploymentsDialog(t, []k8s.Deployment{
		{Namespace: "dev", Name: "api", Ready: "2/3", UpToDate: 3, Available: 2, Replicas: 3},
	})
	item, ok := d.list.SelectedItem().(*DeploymentItem)
	require.True(t, ok)

	rendered := item.Render(120)
	require.Contains(t, rendered, "dev/api")
	require.Contains(t, rendered, "ready 2/3")
	require.Contains(t, rendered, "up-to-date 3")
	require.Contains(t, rendered, "available 2")
}

func TestDeploymentsScaleKeyEntersScalingModeWithPrefilledReplicas(t *testing.T) {
	t.Parallel()

	d := newDeploymentsDialog(t, []k8s.Deployment{
		{Namespace: "dev", Name: "api", Replicas: 3},
	})

	require.Nil(t, d.HandleMsg(tea.KeyPressMsg{Code: 's', Text: "s"}))
	require.True(t, d.scaling)
	require.Equal(t, "3", d.replicasInput.Value())
	require.Equal(t, "dev/api", d.scalingTarget.Key())
}

func TestDeploymentsScaleConfirmWithValidReplicasReturnsActionScaleDeployment(t *testing.T) {
	t.Parallel()

	d := newDeploymentsDialog(t, []k8s.Deployment{
		{Namespace: "dev", Name: "api", Replicas: 1},
	})
	require.Nil(t, d.HandleMsg(tea.KeyPressMsg{Code: 's', Text: "s"}))

	d.replicasInput.SetValue("5")
	action := d.HandleMsg(tea.KeyPressMsg{Code: tea.KeyEnter})

	scale, ok := action.(ActionScaleDeployment)
	require.True(t, ok)
	require.Equal(t, "dev", scale.Namespace)
	require.Equal(t, "api", scale.Name)
	require.Equal(t, 5, scale.Replicas)
	require.False(t, d.scaling)
}

func TestDeploymentsScaleConfirmWithInvalidReplicasReturnsWarningAndStaysInScalingMode(t *testing.T) {
	t.Parallel()

	d := newDeploymentsDialog(t, []k8s.Deployment{
		{Namespace: "dev", Name: "api", Replicas: 1},
	})
	require.Nil(t, d.HandleMsg(tea.KeyPressMsg{Code: 's', Text: "s"}))

	d.replicasInput.SetValue("-1")
	action := d.HandleMsg(tea.KeyPressMsg{Code: tea.KeyEnter})

	_, ok := action.(ActionCmd)
	require.True(t, ok, "expected a warning ActionCmd for a negative replica count")
	require.True(t, d.scaling, "should remain in scaling mode after an invalid value")

	d.replicasInput.SetValue("not-a-number")
	action = d.HandleMsg(tea.KeyPressMsg{Code: tea.KeyEnter})
	_, ok = action.(ActionCmd)
	require.True(t, ok, "expected a warning ActionCmd for a non-numeric value")
	require.True(t, d.scaling)
}

func TestDeploymentsViewPodsReturnsActionViewDeploymentPods(t *testing.T) {
	t.Parallel()

	d := newDeploymentsDialog(t, []k8s.Deployment{
		{Namespace: "dev", Name: "api", Selector: map[string]string{"app": "api"}},
	})

	action := d.HandleMsg(tea.KeyPressMsg{Code: tea.KeyEnter})
	view, ok := action.(ActionViewDeploymentPods)
	require.True(t, ok)
	require.Equal(t, "dev", view.Namespace)
	require.Equal(t, "api", view.Name)
	require.Equal(t, map[string]string{"app": "api"}, view.Selector)
}

func TestDeploymentsScaleCloseCancelsWithoutSubmitting(t *testing.T) {
	t.Parallel()

	d := newDeploymentsDialog(t, []k8s.Deployment{
		{Namespace: "dev", Name: "api", Replicas: 1},
	})
	require.Nil(t, d.HandleMsg(tea.KeyPressMsg{Code: 's', Text: "s"}))
	require.True(t, d.scaling)

	action := d.HandleMsg(tea.KeyPressMsg{Code: tea.KeyEsc})
	require.Nil(t, action)
	require.False(t, d.scaling)
}
