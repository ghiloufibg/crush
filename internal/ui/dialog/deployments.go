package dialog

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/crush/internal/k8s"
	"github.com/charmbracelet/crush/internal/ui/common"
	"github.com/charmbracelet/crush/internal/ui/list"
	"github.com/charmbracelet/crush/internal/ui/styles"
	"github.com/charmbracelet/crush/internal/ui/util"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/sahilm/fuzzy"
)

const (
	// DeploymentsID is the identifier for the Kubernetes deployments panel
	// dialog.
	DeploymentsID = "deployments"

	deploymentsDialogMaxWidth  = 100
	deploymentsDialogMaxHeight = 20
)

// Deployments is a read-mostly panel listing the deployments in the
// current Kubernetes context, mirroring [Pods]. It is fed live snapshots
// from a [k8s.DeploymentWatcher] via SetDeployments. Scaling the selected
// deployment needs a replica-count parameter that isn't known at keypress
// time, unlike pod deletion — so the panel collects it inline with a
// [textinput.Model] rather than delegating collection to the chat/agent
// flow, then routes the result through the same agent-tool-call path as
// Delete once the count is known.
type Deployments struct {
	com  *common.Common
	help help.Model
	list *list.FilterableList

	scaling       bool
	replicasInput textinput.Model
	scalingTarget k8s.Deployment

	keyMap struct {
		Next     key.Binding
		Previous key.Binding
		UpDown   key.Binding
		Delete   key.Binding
		Scale    key.Binding
		Confirm  key.Binding
		Close    key.Binding
	}
}

// DeploymentItem represents a single deployment row in the list.
type DeploymentItem struct {
	*list.Versioned
	deployment k8s.Deployment
	t          *styles.Styles
	m          fuzzy.Match
	cache      map[int]string
	focused    bool
}

// Finished implements list.Item. Deployment items are render-stable outside
// of explicit SetFocused / SetMatch calls.
func (i *DeploymentItem) Finished() bool {
	return true
}

var (
	_ Dialog   = (*Deployments)(nil)
	_ ListItem = (*DeploymentItem)(nil)
)

// NewDeployments creates a new Kubernetes deployments panel dialog seeded
// with deployments.
func NewDeployments(com *common.Common, deployments []k8s.Deployment) *Deployments {
	d := &Deployments{com: com}

	h := help.New()
	h.Styles = com.Styles.DialogHelpStyles()
	d.help = h

	d.list = list.NewFilterableList()
	d.list.Focus()

	d.keyMap.Next = key.NewBinding(
		key.WithKeys("down", "j", "ctrl+n"),
		key.WithHelp("↓", "next deployment"),
	)
	d.keyMap.Previous = key.NewBinding(
		key.WithKeys("up", "k", "ctrl+p"),
		key.WithHelp("↑", "previous deployment"),
	)
	d.keyMap.UpDown = key.NewBinding(
		key.WithKeys("up", "down"),
		key.WithHelp("↑/↓", "navigate"),
	)
	d.keyMap.Delete = key.NewBinding(
		key.WithKeys("d"),
		key.WithHelp("d", "delete deployment"),
	)
	d.keyMap.Scale = key.NewBinding(
		key.WithKeys("s"),
		key.WithHelp("s", "scale deployment"),
	)
	d.keyMap.Confirm = key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "confirm"),
	)
	d.keyMap.Close = CloseKey

	d.replicasInput = textinput.New()
	d.replicasInput.SetVirtualCursor(false)
	d.replicasInput.SetStyles(com.Styles.TextInput)
	d.replicasInput.Prompt = "> "

	d.SetDeployments(deployments)
	return d
}

// ID implements [Dialog].
func (d *Deployments) ID() string {
	return DeploymentsID
}

// SetDeployments replaces the panel's deployment list with a fresh
// snapshot, keeping the current selection on the same deployment where it
// still exists.
func (d *Deployments) SetDeployments(deployments []k8s.Deployment) {
	var selectedKey string
	if item, ok := d.list.SelectedItem().(*DeploymentItem); ok {
		selectedKey = item.deployment.Key()
	}

	items := make([]list.FilterableItem, len(deployments))
	selectedIndex := 0
	for i, dep := range deployments {
		items[i] = &DeploymentItem{Versioned: list.NewVersioned(), deployment: dep, t: d.com.Styles}
		if dep.Key() == selectedKey {
			selectedIndex = i
		}
	}
	d.list.SetItems(items...)
	d.list.SetSelected(selectedIndex)
}

// HandleMsg implements [Dialog].
func (d *Deployments) HandleMsg(msg tea.Msg) Action {
	if d.scaling {
		return d.handleScalingMsg(msg)
	}

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
			item, ok := d.list.SelectedItem().(*DeploymentItem)
			if !ok {
				break
			}
			return ActionDeleteDeployment{Namespace: item.deployment.Namespace, Name: item.deployment.Name}
		case key.Matches(msg, d.keyMap.Scale):
			item, ok := d.list.SelectedItem().(*DeploymentItem)
			if !ok {
				break
			}
			d.startScaling(item.deployment)
		}
	}
	return nil
}

// startScaling switches the panel into its inline scale-input sub-state,
// pre-filling the replica count with the deployment's current value.
func (d *Deployments) startScaling(target k8s.Deployment) {
	d.scaling = true
	d.scalingTarget = target
	d.replicasInput.SetValue(strconv.Itoa(int(target.Replicas)))
	d.replicasInput.Focus()
}

// handleScalingMsg handles input while the inline replica-count prompt is
// active.
func (d *Deployments) handleScalingMsg(msg tea.Msg) Action {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, d.keyMap.Close):
			d.scaling = false
			return nil
		case key.Matches(msg, d.keyMap.Confirm):
			replicas, err := strconv.Atoi(strings.TrimSpace(d.replicasInput.Value()))
			if err != nil || replicas < 0 {
				return ActionCmd{Cmd: util.ReportWarn("Enter a non-negative replica count.")}
			}
			d.scaling = false
			return ActionScaleDeployment{
				Namespace: d.scalingTarget.Namespace,
				Name:      d.scalingTarget.Name,
				Replicas:  replicas,
			}
		default:
			var cmd tea.Cmd
			d.replicasInput, cmd = d.replicasInput.Update(msg)
			return ActionCmd{Cmd: cmd}
		}
	case tea.PasteMsg:
		var cmd tea.Cmd
		d.replicasInput, cmd = d.replicasInput.Update(msg)
		return ActionCmd{Cmd: cmd}
	}
	return nil
}

// Draw implements [Dialog].
func (d *Deployments) Draw(scr uv.Screen, area uv.Rectangle) *tea.Cursor {
	t := d.com.Styles
	width := max(0, min(deploymentsDialogMaxWidth, area.Dx()-t.Dialog.View.GetHorizontalBorderSize()))
	height := max(0, min(deploymentsDialogMaxHeight, area.Dy()-t.Dialog.View.GetVerticalBorderSize()))
	innerWidth := width - t.Dialog.View.GetHorizontalFrameSize()
	heightOffset := t.Dialog.Title.GetVerticalFrameSize() + titleContentHeight +
		t.Dialog.HelpView.GetVerticalFrameSize() +
		t.Dialog.View.GetVerticalFrameSize()

	d.list.SetSize(innerWidth, max(0, height-heightOffset))

	rc := NewRenderContext(t, width)
	rc.Title = "Deployments"

	if d.scaling {
		d.replicasInput.SetWidth(innerWidth - t.Dialog.InputPrompt.GetHorizontalFrameSize() - 1)
		label := fmt.Sprintf("Scale %s (currently %d replicas):", d.scalingTarget.Key(), d.scalingTarget.Replicas)
		rc.AddPart(lipgloss.JoinVertical(lipgloss.Left,
			t.Dialog.NormalItem.Render(label),
			t.Dialog.InputPrompt.Render(d.replicasInput.View()),
		))
		rc.Help = renderDialogHelp(t, &d.help, d, innerWidth)

		view := rc.Render()
		DrawCenter(scr, area, view)
		return nil
	}

	visibleCount := len(d.list.FilteredItems())
	if d.list.Height() >= visibleCount {
		d.list.ScrollToTop()
	} else {
		d.list.ScrollToSelected()
	}

	if visibleCount == 0 {
		rc.AddPart(t.Dialog.NormalItem.Render("No deployments found"))
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
func (d *Deployments) ShortHelp() []key.Binding {
	if d.scaling {
		return []key.Binding{d.keyMap.Confirm, d.keyMap.Close}
	}
	return []key.Binding{
		d.keyMap.UpDown,
		d.keyMap.Scale,
		d.keyMap.Delete,
		d.keyMap.Close,
	}
}

// FullHelp implements [help.KeyMap].
func (d *Deployments) FullHelp() [][]key.Binding {
	return [][]key.Binding{d.ShortHelp()}
}

// Filter returns the filterable value for the deployment item.
func (i *DeploymentItem) Filter() string {
	return i.deployment.Key()
}

// ID returns the unique identifier for the deployment item.
func (i *DeploymentItem) ID() string {
	return i.deployment.Key()
}

// SetFocused sets the focus state of the deployment item.
func (i *DeploymentItem) SetFocused(focused bool) {
	if i.focused == focused {
		return
	}
	i.cache = nil
	i.focused = focused
	if i.Versioned != nil {
		i.Bump()
	}
}

// SetMatch sets the fuzzy match for the deployment item.
func (i *DeploymentItem) SetMatch(m fuzzy.Match) {
	if sameFuzzyMatch(i.m, m) {
		return
	}
	i.cache = nil
	i.m = m
	if i.Versioned != nil {
		i.Bump()
	}
}

// Render returns the string representation of the deployment item.
func (i *DeploymentItem) Render(width int) string {
	info := fmt.Sprintf("ready %s · up-to-date %d · available %d", i.deployment.Ready, i.deployment.UpToDate, i.deployment.Available)
	st := ListItemStyles{
		ItemBlurred:     i.t.Dialog.NormalItem,
		ItemFocused:     i.t.Dialog.SelectedItem,
		InfoTextBlurred: i.t.Dialog.ListItem.InfoBlurred,
		InfoTextFocused: i.t.Dialog.ListItem.InfoFocused,
	}
	return renderItem(st, i.deployment.Key(), info, i.focused, width, i.cache, &i.m)
}
