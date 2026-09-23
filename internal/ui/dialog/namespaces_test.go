package dialog

import (
	"image"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/crush/internal/ui/common"
	"github.com/charmbracelet/crush/internal/ui/styles"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/stretchr/testify/require"
)

func newNamespacesDialog(t *testing.T, namespaces []string, currentNamespace string, currentAllNamespaces bool) *Namespaces {
	t.Helper()

	sty := styles.CharmtonePantera()
	return NewNamespaces(&common.Common{Styles: &sty}, namespaces, currentNamespace, currentAllNamespaces)
}

func selectedNamespace(t *testing.T, d *Namespaces) string {
	t.Helper()

	item, ok := d.list.SelectedItem().(*NamespaceItem)
	require.True(t, ok, "no namespace selected")
	return item.name
}

func TestNewNamespacesSeedsAllNamespacesOptionFirst(t *testing.T) {
	t.Parallel()

	d := newNamespacesDialog(t, []string{"dev", "prod"}, "dev", false)

	require.Len(t, d.list.FilteredItems(), 3)
	first, ok := d.list.FilteredItems()[0].(*NamespaceItem)
	require.True(t, ok)
	require.Equal(t, allNamespacesID, first.name)
}

func TestNewNamespacesSelectsCurrentNamespace(t *testing.T) {
	t.Parallel()

	d := newNamespacesDialog(t, []string{"dev", "prod"}, "prod", false)
	require.Equal(t, "prod", selectedNamespace(t, d))
}

func TestNewNamespacesSelectsAllNamespacesWhenCurrentlyAll(t *testing.T) {
	t.Parallel()

	d := newNamespacesDialog(t, []string{"dev", "prod"}, "", true)
	require.Equal(t, allNamespacesID, selectedNamespace(t, d))
}

func TestNamespacesHandleMsgSelectReturnsActionSetNamespace(t *testing.T) {
	t.Parallel()

	d := newNamespacesDialog(t, []string{"dev", "prod"}, "dev", false)
	require.Equal(t, "dev", selectedNamespace(t, d))

	action := d.HandleMsg(tea.KeyPressMsg{Code: tea.KeyEnter})
	set, ok := action.(ActionSetNamespace)
	require.True(t, ok)
	require.Equal(t, "dev", set.Namespace)
	require.False(t, set.AllNamespaces)
}

func TestNamespacesHandleMsgSelectAllNamespacesReturnsActionSetNamespace(t *testing.T) {
	t.Parallel()

	d := newNamespacesDialog(t, []string{"dev", "prod"}, "", true)
	require.Equal(t, allNamespacesID, selectedNamespace(t, d))

	action := d.HandleMsg(tea.KeyPressMsg{Code: tea.KeyEnter})
	set, ok := action.(ActionSetNamespace)
	require.True(t, ok)
	require.True(t, set.AllNamespaces)
}

func TestNamespacesHandleMsgCloseReturnsActionClose(t *testing.T) {
	t.Parallel()

	d := newNamespacesDialog(t, nil, "", true)
	action := d.HandleMsg(tea.KeyPressMsg{Code: tea.KeyEsc})
	require.Equal(t, ActionClose{}, action)
}

func TestNamespacesDrawDoesNotPanic(t *testing.T) {
	t.Parallel()

	d := newNamespacesDialog(t, []string{"dev", "prod"}, "dev", false)
	scr := uv.NewScreenBuffer(80, 30)
	require.NotPanics(t, func() {
		d.Draw(scr, image.Rect(0, 0, 80, 30))
	})
}
