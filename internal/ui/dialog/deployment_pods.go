package dialog

import (
	"fmt"
	"slices"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/crush/internal/k8s"
	"github.com/charmbracelet/crush/internal/ui/common"
	"github.com/charmbracelet/crush/internal/ui/list"
	uv "github.com/charmbracelet/ultraviolet"
)

const (
	// DeploymentPodsID is the identifier for the drill-down panel showing
	// the pods owned by a single deployment.
	DeploymentPodsID = "deployment-pods"

	deploymentPodsDialogMaxWidth  = podsDialogMaxWidth
	deploymentPodsDialogMaxHeight = podsDialogMaxHeight
)

// DeploymentPods is the drill-down panel opened from the Deployments panel
// via ActionViewDeploymentPods. It's a near-duplicate of [Pods] rather than
// a variant of it — same reasoning as DeploymentWatcher vs Watcher: the
// two dialogs' HandleMsg/Draw are small enough that sharing a struct would
// cost more in conditional branching than the duplication costs in lines.
// Unlike Pods, this panel never talks to kubectl or a watcher directly: it
// is always re-seeded from the caller with the Pods watcher's existing
// snapshot (see UI's pubsub.Event[[]k8s.Pod] handling, which calls
// SetAllPods), filtered client-side by Selector. Closing it (Escape) just
// pops it off the dialog stack, revealing the Deployments panel
// revealing the Deployments panel underneath — that's the whole
// "breadcrumb", no separate navigation stack needed.
type DeploymentPods struct {
	com  *common.Common
	help help.Model
	list *list.FilterableList

	namespace      string
	deploymentName string
	selector       map[string]string

	// pods is the last filtered snapshot rendered into list, kept so
	// SetAllPods can skip rebuilding and redrawing when a poll comes back
	// unchanged for this deployment — the common case at a 3s poll
	// interval, since most polls see no cluster change at all.
	pods []k8s.Pod

	keyMap struct {
		Next     key.Binding
		Previous key.Binding
		UpDown   key.Binding
		Delete   key.Binding
		Logs     key.Binding
		Close    key.Binding
	}
}

var _ Dialog = (*DeploymentPods)(nil)

// NewDeploymentPods creates a drill-down pods panel for the deployment
// identified by namespace/name, seeded by filtering allPods with selector.
func NewDeploymentPods(com *common.Common, namespace, deploymentName string, selector map[string]string, allPods []k8s.Pod) *DeploymentPods {
	d := &DeploymentPods{
		com:            com,
		namespace:      namespace,
		deploymentName: deploymentName,
		selector:       selector,
	}

	h := help.New()
	h.Styles = com.Styles.DialogHelpStyles()
	d.help = h

	d.list = list.NewFilterableList()
	d.list.Focus()

	d.keyMap.Next = key.NewBinding(
		key.WithKeys("down", "j", "ctrl+n"),
		key.WithHelp("↓", "next pod"),
	)
	d.keyMap.Previous = key.NewBinding(
		key.WithKeys("up", "k", "ctrl+p"),
		key.WithHelp("↑", "previous pod"),
	)
	d.keyMap.UpDown = key.NewBinding(
		key.WithKeys("up", "down"),
		key.WithHelp("↑/↓", "navigate"),
	)
	d.keyMap.Delete = key.NewBinding(
		key.WithKeys("d"),
		key.WithHelp("d", "delete pod"),
	)
	d.keyMap.Logs = key.NewBinding(
		key.WithKeys("l"),
		key.WithHelp("l", "view logs"),
	)
	d.keyMap.Close = CloseKey

	d.SetAllPods(allPods)
	return d
}

// ID implements [Dialog].
func (d *DeploymentPods) ID() string {
	return DeploymentPodsID
}

// SetAllPods filters allPods down to those in namespace matching selector
// and replaces the panel's list with the result, keeping the current
// selection on the same pod where it still exists. A no-op when the
// filtered result is identical to the currently displayed snapshot, so an
// unchanged poll doesn't rebuild every row or force a redraw.
func (d *DeploymentPods) SetAllPods(allPods []k8s.Pod) {
	var filtered []k8s.Pod
	for _, pod := range allPods {
		if pod.Namespace == d.namespace && pod.MatchesSelector(d.selector) {
			filtered = append(filtered, pod)
		}
	}

	if slices.EqualFunc(d.pods, filtered, k8s.Pod.Equal) {
		return
	}
	d.pods = filtered

	var selectedKey string
	if item, ok := d.list.SelectedItem().(*PodItem); ok {
		selectedKey = item.pod.Key()
	}

	items := make([]list.FilterableItem, len(filtered))
	selectedIndex := 0
	for i, pod := range filtered {
		items[i] = &PodItem{Versioned: list.NewVersioned(), pod: pod, t: d.com.Styles}
		if pod.Key() == selectedKey {
			selectedIndex = i
		}
	}
	d.list.SetItems(items...)
	d.list.SetSelected(selectedIndex)
}

// HandleMsg implements [Dialog].
func (d *DeploymentPods) HandleMsg(msg tea.Msg) Action {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, d.keyMap.Close):
			return ActionClose{}
		case key.Matches(msg, d.keyMap.Previous):
			d.list.Focus()
			if d.list.IsSelectedFirst() {
				d.list.SelectLast()
				d.list.ScrollToBottom()
				break
			}
			d.list.SelectPrev()
			d.list.ScrollToSelected()
		case key.Matches(msg, d.keyMap.Next):
			d.list.Focus()
			if d.list.IsSelectedLast() {
				d.list.SelectFirst()
				d.list.ScrollToTop()
				break
			}
			d.list.SelectNext()
			d.list.ScrollToSelected()
		case key.Matches(msg, d.keyMap.Delete):
			item, ok := d.list.SelectedItem().(*PodItem)
			if !ok {
				break
			}
			return ActionDeletePod{Namespace: item.pod.Namespace, Name: item.pod.Name}
		case key.Matches(msg, d.keyMap.Logs):
			item, ok := d.list.SelectedItem().(*PodItem)
			if !ok {
				break
			}
			return ActionViewPodLogs{Namespace: item.pod.Namespace, Name: item.pod.Name}
		}
	}
	return nil
}

// Draw implements [Dialog].
func (d *DeploymentPods) Draw(scr uv.Screen, area uv.Rectangle) *tea.Cursor {
	t := d.com.Styles
	width := max(0, min(deploymentPodsDialogMaxWidth, area.Dx()-t.Dialog.View.GetHorizontalBorderSize()))
	height := max(0, min(deploymentPodsDialogMaxHeight, area.Dy()-t.Dialog.View.GetVerticalBorderSize()))
	innerWidth := width - t.Dialog.View.GetHorizontalFrameSize()
	heightOffset := t.Dialog.Title.GetVerticalFrameSize() + titleContentHeight +
		t.Dialog.HelpView.GetVerticalFrameSize() +
		t.Dialog.View.GetVerticalFrameSize()

	d.list.SetSize(innerWidth, max(0, height-heightOffset))

	rc := NewRenderContext(t, width)
	rc.Title = fmt.Sprintf("Pods · %s/%s", d.namespace, d.deploymentName)

	visibleCount := len(d.list.FilteredItems())
	if d.list.Height() >= visibleCount {
		d.list.ScrollToTop()
	} else {
		d.list.ScrollToSelected()
	}

	if visibleCount == 0 {
		rc.AddPart(t.Dialog.NormalItem.Render("No pods found"))
	} else {
		listView := t.Dialog.List.Height(d.list.Height()).Render(d.list.Render())
		rc.AddPart(listView)
	}
	rc.Help = renderDialogHelp(t, &d.help, d, innerWidth)

	view := rc.Render()
	DrawCenter(scr, area, view)
	return nil
}

// ShortHelp implements [help.KeyMap].
func (d *DeploymentPods) ShortHelp() []key.Binding {
	return []key.Binding{
		d.keyMap.UpDown,
		d.keyMap.Delete,
		d.keyMap.Logs,
		d.keyMap.Close,
	}
}

// FullHelp implements [help.KeyMap].
func (d *DeploymentPods) FullHelp() [][]key.Binding {
	return [][]key.Binding{d.ShortHelp()}
}
