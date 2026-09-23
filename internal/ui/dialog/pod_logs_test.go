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

func newPodLogsDialog(t *testing.T, cancel func()) *PodLogs {
	t.Helper()

	sty := styles.CharmtonePantera()
	return NewPodLogs(&common.Common{Styles: &sty}, "dev", "api-1", cancel)
}

func TestNewPodLogsSetsNamespaceAndName(t *testing.T) {
	t.Parallel()

	d := newPodLogsDialog(t, func() {})
	require.Equal(t, "dev", d.Namespace())
	require.Equal(t, "api-1", d.Name())
}

func TestPodLogsAppendLineTrimsToMaxLines(t *testing.T) {
	t.Parallel()

	d := newPodLogsDialog(t, func() {})
	for i := range podLogsMaxLines + 10 {
		d.AppendLine("line " + string(rune('a'+i%26)))
	}

	require.Len(t, d.lines, podLogsMaxLines)
}

func TestPodLogsHandleMsgCloseCancelsStreamAndReturnsActionClose(t *testing.T) {
	t.Parallel()

	cancelled := false
	d := newPodLogsDialog(t, func() { cancelled = true })

	action := d.HandleMsg(tea.KeyPressMsg{Code: tea.KeyEsc})
	require.Equal(t, ActionClose{}, action)
	require.True(t, cancelled)
}

func TestPodLogsCloseIsIdempotent(t *testing.T) {
	t.Parallel()

	calls := 0
	d := newPodLogsDialog(t, func() { calls++ })

	d.Close()
	d.Close()
	require.Equal(t, 1, calls)
}

func TestPodLogsSetStoppedRecordsError(t *testing.T) {
	t.Parallel()

	d := newPodLogsDialog(t, func() {})
	d.SetStopped(nil)
	require.True(t, d.stopped)
	require.NoError(t, d.err)
}

func TestPodLogsDrawWithNoLinesShowsWaitingState(t *testing.T) {
	t.Parallel()

	d := newPodLogsDialog(t, func() {})
	scr := uv.NewScreenBuffer(80, 30)
	require.NotPanics(t, func() {
		d.Draw(scr, image.Rect(0, 0, 80, 30))
	})
}

func TestPodLogsDrawWithLinesRendersContent(t *testing.T) {
	t.Parallel()

	d := newPodLogsDialog(t, func() {})
	d.AppendLine("hello from the pod")
	scr := uv.NewScreenBuffer(80, 30)
	require.NotPanics(t, func() {
		d.Draw(scr, image.Rect(0, 0, 80, 30))
	})
}
