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
	"github.com/charmbracelet/crush/internal/ui/styles"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/sahilm/fuzzy"
)

const (
	// PodsID is the identifier for the Kubernetes pods panel dialog.
	PodsID = "pods"

	podsDialogMaxWidth  = 100
	podsDialogMaxHeight = 20
)

// Pods is a read-mostly panel listing the pods in the current
// Kubernetes context. It is fed live snapshots from a [k8s.Watcher] via
// SetPods, called from outside as snapshots arrive — the dialog has no
// subscription of its own to manage. Deleting the selected pod is routed
// through the agent's normal permission-gated k8s_delete_pod tool rather
// than shelling out directly, so a delete triggered from the panel gets
// the same confirmation prompt as one the agent initiates on its own.
type Pods struct {
	com  *common.Common
	help help.Model
	list *list.FilterableList

	// pods is the last snapshot rendered into list, kept so SetPods can
	// skip rebuilding and redrawing when a poll comes back unchanged —
	// the common case at a 3s poll interval, since most polls see no
	// cluster change at all.
	pods []k8s.Pod

	keyMap struct {
		Next     key.Binding
		Previous key.Binding
		UpDown   key.Binding
		Delete   key.Binding
		Logs     key.Binding
		Exec     key.Binding
		Close    key.Binding
	}
}

// PodItem represents a single pod row in the list.
type PodItem struct {
	*list.Versioned
	pod     k8s.Pod
	t       *styles.Styles
	m       fuzzy.Match
	cache   map[int]string
	focused bool
}

// Finished implements list.Item. Pod items are render-stable outside of
// explicit SetFocused / SetMatch calls.
func (p *PodItem) Finished() bool {
	return true
}

var (
	_ Dialog   = (*Pods)(nil)
	_ ListItem = (*PodItem)(nil)
)

// NewPods creates a new Kubernetes pods panel dialog seeded with pods.
func NewPods(com *common.Common, pods []k8s.Pod) *Pods {
	d := &Pods{com: com}

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
	d.keyMap.Exec = key.NewBinding(
		key.WithKeys("e"),
		key.WithHelp("e", "exec shell"),
	)
	d.keyMap.Close = CloseKey

	d.SetPods(pods)
	return d
}

// ID implements [Dialog].
func (d *Pods) ID() string {
	return PodsID
}

// SetPods replaces the panel's pod list with a fresh snapshot, keeping
// the current selection on the same pod where it still exists. A no-op
// when pods is identical to the currently displayed snapshot, so an
// unchanged poll doesn't rebuild every row or force a redraw.
func (d *Pods) SetPods(pods []k8s.Pod) {
	if slices.EqualFunc(d.pods, pods, k8s.Pod.Equal) {
		return
	}
	d.pods = pods

	var selectedKey string
	if item, ok := d.list.SelectedItem().(*PodItem); ok {
		selectedKey = item.pod.Key()
	}

	items := make([]list.FilterableItem, len(pods))
	selectedIndex := 0
	for i, pod := range pods {
		items[i] = &PodItem{Versioned: list.NewVersioned(), pod: pod, t: d.com.Styles}
		if pod.Key() == selectedKey {
			selectedIndex = i
		}
	}
	d.list.SetItems(items...)
	d.list.SetSelected(selectedIndex)
}

// HandleMsg implements [Dialog].
func (d *Pods) HandleMsg(msg tea.Msg) Action {
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
		case key.Matches(msg, d.keyMap.Exec):
			item, ok := d.list.SelectedItem().(*PodItem)
			if !ok {
				break
			}
			return ActionExecPod{Namespace: item.pod.Namespace, Name: item.pod.Name}
		}
	}
	return nil
}

// Draw implements [Dialog].
func (d *Pods) Draw(scr uv.Screen, area uv.Rectangle) *tea.Cursor {
	t := d.com.Styles
	width := max(0, min(podsDialogMaxWidth, area.Dx()-t.Dialog.View.GetHorizontalBorderSize()))
	height := max(0, min(podsDialogMaxHeight, area.Dy()-t.Dialog.View.GetVerticalBorderSize()))
	innerWidth := width - t.Dialog.View.GetHorizontalFrameSize()
	heightOffset := t.Dialog.Title.GetVerticalFrameSize() + titleContentHeight +
		t.Dialog.HelpView.GetVerticalFrameSize() +
		t.Dialog.View.GetVerticalFrameSize()

	d.list.SetSize(innerWidth, max(0, height-heightOffset))

	rc := NewRenderContext(t, width)
	rc.Title = "Pods"

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
func (d *Pods) ShortHelp() []key.Binding {
	return []key.Binding{
		d.keyMap.UpDown,
		d.keyMap.Delete,
		d.keyMap.Logs,
		d.keyMap.Exec,
		d.keyMap.Close,
	}
}

// FullHelp implements [help.KeyMap].
func (d *Pods) FullHelp() [][]key.Binding {
	return [][]key.Binding{d.ShortHelp()}
}

// Filter returns the filterable value for the pod item.
func (p *PodItem) Filter() string {
	return p.pod.Key()
}

// ID returns the unique identifier for the pod item.
func (p *PodItem) ID() string {
	return p.pod.Key()
}

// SetFocused sets the focus state of the pod item.
func (p *PodItem) SetFocused(focused bool) {
	if p.focused == focused {
		return
	}
	p.cache = nil
	p.focused = focused
	if p.Versioned != nil {
		p.Bump()
	}
}

// SetMatch sets the fuzzy match for the pod item.
func (p *PodItem) SetMatch(m fuzzy.Match) {
	if sameFuzzyMatch(p.m, m) {
		return
	}
	p.cache = nil
	p.m = m
	if p.Versioned != nil {
		p.Bump()
	}
}

// Render returns the string representation of the pod item.
func (p *PodItem) Render(width int) string {
	info := fmt.Sprintf("%s · %d restarts", p.pod.Phase, p.pod.Restarts)
	st := ListItemStyles{
		ItemBlurred:     p.t.Dialog.NormalItem,
		ItemFocused:     p.t.Dialog.SelectedItem,
		InfoTextBlurred: p.t.Dialog.ListItem.InfoBlurred,
		InfoTextFocused: p.t.Dialog.ListItem.InfoFocused,
	}
	return renderItem(st, p.pod.Key(), info, p.focused, width, p.cache, &p.m)
}
