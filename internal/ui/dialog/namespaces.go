package dialog

import (
	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/crush/internal/ui/common"
	"github.com/charmbracelet/crush/internal/ui/list"
	"github.com/charmbracelet/crush/internal/ui/styles"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/sahilm/fuzzy"
)

const (
	// NamespacesID is the identifier for the namespace switcher dialog.
	NamespacesID              = "namespaces"
	namespacesDialogMaxWidth  = 50
	namespacesDialogMaxHeight = 12

	// allNamespacesID is the NamespaceItem ID/title for the "All
	// namespaces" option, distinct from any real namespace name.
	allNamespacesID = "*"
)

// Namespaces represents a dialog for switching the Pods/Deployments panels'
// namespace scope. Unlike Pods/Deployments, its item set is a live cluster
// listing fetched once when the dialog opens (see ui.startNamespacesLoad),
// not data already held by a running watcher.
type Namespaces struct {
	com   *common.Common
	help  help.Model
	list  *list.FilterableList
	input textinput.Model

	keyMap struct {
		Select   key.Binding
		Next     key.Binding
		Previous key.Binding
		UpDown   key.Binding
		Close    key.Binding
	}
}

// NamespaceItem represents a namespace list item. name == allNamespacesID
// represents the "All namespaces" option rather than a real namespace.
type NamespaceItem struct {
	*list.Versioned
	name      string
	isCurrent bool
	t         *styles.Styles
	m         fuzzy.Match
	cache     map[int]string
	focused   bool
}

// Finished implements list.Item. Namespace items are render-stable outside
// of explicit SetFocused / SetMatch.
func (n *NamespaceItem) Finished() bool {
	return true
}

var (
	_ Dialog   = (*Namespaces)(nil)
	_ ListItem = (*NamespaceItem)(nil)
)

// NewNamespaces creates a new namespace switcher dialog. namespaces is the
// live list of cluster namespace names; currentNamespace/currentAllNamespaces
// mark which item starts selected.
func NewNamespaces(com *common.Common, namespaces []string, currentNamespace string, currentAllNamespaces bool) *Namespaces {
	n := &Namespaces{com: com}

	h := help.New()
	h.Styles = com.Styles.DialogHelpStyles()
	n.help = h

	n.list = list.NewFilterableList()
	n.list.Focus()

	n.input = textinput.New()
	n.input.SetVirtualCursor(false)
	n.input.Placeholder = "Type to filter"
	n.input.SetStyles(com.Styles.TextInput)
	n.input.Focus()

	n.keyMap.Select = key.NewBinding(
		key.WithKeys("enter", "ctrl+y"),
		key.WithHelp("enter", "confirm"),
	)
	n.keyMap.Next = key.NewBinding(
		key.WithKeys("down", "ctrl+n"),
		key.WithHelp("↓", "next item"),
	)
	n.keyMap.Previous = key.NewBinding(
		key.WithKeys("up", "ctrl+p"),
		key.WithHelp("↑", "previous item"),
	)
	n.keyMap.UpDown = key.NewBinding(
		key.WithKeys("up", "down"),
		key.WithHelp("↑/↓", "choose"),
	)
	n.keyMap.Close = CloseKey

	n.setItems(namespaces, currentNamespace, currentAllNamespaces)
	return n
}

// ID implements Dialog.
func (n *Namespaces) ID() string {
	return NamespacesID
}

// HandleMsg implements [Dialog].
func (n *Namespaces) HandleMsg(msg tea.Msg) Action {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, n.keyMap.Close):
			return ActionClose{}
		case key.Matches(msg, n.keyMap.Previous):
			n.list.Focus()
			if n.list.IsSelectedFirst() {
				n.list.SelectLast()
				n.list.ScrollToBottom()
				break
			}
			n.list.SelectPrev()
			n.list.ScrollToSelected()
		case key.Matches(msg, n.keyMap.Next):
			n.list.Focus()
			if n.list.IsSelectedLast() {
				n.list.SelectFirst()
				n.list.ScrollToTop()
				break
			}
			n.list.SelectNext()
			n.list.ScrollToSelected()
		case key.Matches(msg, n.keyMap.Select):
			selectedItem := n.list.SelectedItem()
			if selectedItem == nil {
				break
			}
			nsItem, ok := selectedItem.(*NamespaceItem)
			if !ok {
				break
			}
			if nsItem.name == allNamespacesID {
				return ActionSetNamespace{AllNamespaces: true}
			}
			return ActionSetNamespace{Namespace: nsItem.name}
		default:
			prevValue := n.input.Value()
			var cmd tea.Cmd
			n.input, cmd = n.input.Update(msg)
			value := n.input.Value()
			if value != prevValue {
				n.list.SetFilter(value)
				n.list.ScrollToTop()
				n.list.SetSelected(0)
			}

			return ActionCmd{cmd}
		}
	}
	return nil
}

// Cursor returns the cursor position relative to the dialog.
func (n *Namespaces) Cursor() *tea.Cursor {
	return InputCursor(n.com.Styles, n.input.Cursor())
}

// Draw implements [Dialog].
func (n *Namespaces) Draw(scr uv.Screen, area uv.Rectangle) *tea.Cursor {
	t := n.com.Styles
	width := max(0, min(namespacesDialogMaxWidth, area.Dx()-t.Dialog.View.GetHorizontalBorderSize()))
	height := max(0, min(namespacesDialogMaxHeight, area.Dy()-t.Dialog.View.GetVerticalBorderSize()))
	innerWidth := width - t.Dialog.View.GetHorizontalFrameSize()
	heightOffset := t.Dialog.Title.GetVerticalFrameSize() + titleContentHeight +
		t.Dialog.InputPrompt.GetVerticalFrameSize() + inputContentHeight +
		t.Dialog.HelpView.GetVerticalFrameSize() +
		t.Dialog.View.GetVerticalFrameSize()

	n.input.SetWidth(dialogInputTextWidth(t, n.input, innerWidth))
	n.list.SetSize(innerWidth, max(0, height-heightOffset))

	rc := NewRenderContext(t, width)
	rc.Title = "Namespace"
	inputView := t.Dialog.InputPrompt.Render(n.input.View())
	rc.AddPart(inputView)

	visibleCount := len(n.list.FilteredItems())
	if n.list.Height() >= visibleCount {
		n.list.ScrollToTop()
	} else {
		n.list.ScrollToSelected()
	}

	listView := t.Dialog.List.Height(n.list.Height()).Render(n.list.Render())
	rc.AddPart(listView)
	rc.Help = renderDialogHelp(t, &n.help, n, innerWidth)

	view := rc.Render()

	cur := n.Cursor()
	DrawCenterCursor(scr, area, view, cur)
	return cur
}

// ShortHelp implements [help.KeyMap].
func (n *Namespaces) ShortHelp() []key.Binding {
	return []key.Binding{
		n.keyMap.UpDown,
		n.keyMap.Select,
		n.keyMap.Close,
	}
}

// FullHelp implements [help.KeyMap].
func (n *Namespaces) FullHelp() [][]key.Binding {
	m := [][]key.Binding{}
	slice := []key.Binding{
		n.keyMap.Select,
		n.keyMap.Next,
		n.keyMap.Previous,
		n.keyMap.Close,
	}
	for i := 0; i < len(slice); i += 4 {
		end := min(i+4, len(slice))
		m = append(m, slice[i:end])
	}
	return m
}

func (n *Namespaces) setItems(namespaces []string, currentNamespace string, currentAllNamespaces bool) {
	items := make([]list.FilterableItem, 0, len(namespaces)+1)
	selectedIndex := 0

	allItem := &NamespaceItem{
		Versioned: list.NewVersioned(),
		name:      allNamespacesID,
		isCurrent: currentAllNamespaces,
		t:         n.com.Styles,
	}
	if currentAllNamespaces {
		selectedIndex = len(items)
	}
	items = append(items, allItem)

	for _, ns := range namespaces {
		item := &NamespaceItem{
			Versioned: list.NewVersioned(),
			name:      ns,
			isCurrent: !currentAllNamespaces && ns == currentNamespace,
			t:         n.com.Styles,
		}
		if item.isCurrent {
			selectedIndex = len(items)
		}
		items = append(items, item)
	}

	n.list.SetItems(items...)
	n.list.SetSelected(selectedIndex)
	n.list.ScrollToSelected()
}

// Filter returns the filter value for the namespace item.
func (n *NamespaceItem) Filter() string {
	return n.title()
}

// ID returns the unique identifier for the namespace item.
func (n *NamespaceItem) ID() string {
	return n.name
}

// SetFocused sets the focus state of the namespace item.
func (n *NamespaceItem) SetFocused(focused bool) {
	if n.focused == focused {
		return
	}
	n.cache = nil
	n.focused = focused
	if n.Versioned != nil {
		n.Bump()
	}
}

// SetMatch sets the fuzzy match for the namespace item.
func (n *NamespaceItem) SetMatch(m fuzzy.Match) {
	if sameFuzzyMatch(n.m, m) {
		return
	}
	n.cache = nil
	n.m = m
	if n.Versioned != nil {
		n.Bump()
	}
}

func (n *NamespaceItem) title() string {
	if n.name == allNamespacesID {
		return "All namespaces"
	}
	return n.name
}

// Render returns the string representation of the namespace item.
func (n *NamespaceItem) Render(width int) string {
	info := ""
	if n.isCurrent {
		info = "current"
	}
	st := ListItemStyles{
		ItemBlurred:     n.t.Dialog.NormalItem,
		ItemFocused:     n.t.Dialog.SelectedItem,
		InfoTextBlurred: n.t.Dialog.ListItem.InfoBlurred,
		InfoTextFocused: n.t.Dialog.ListItem.InfoFocused,
	}
	return renderItem(st, n.title(), info, n.focused, width, n.cache, &n.m)
}
