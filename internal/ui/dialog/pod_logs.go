package dialog

import (
	"context"
	"fmt"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/crush/internal/ui/common"
	uv "github.com/charmbracelet/ultraviolet"
)

const (
	// PodLogsID is the identifier for the live pod log view dialog.
	PodLogsID = "pod-logs"

	podLogsDialogMaxWidth  = podsDialogMaxWidth
	podLogsDialogMaxHeight = 28

	// podLogsMaxLines bounds the in-memory ring buffer of retained log
	// lines, dropping the oldest once exceeded. Independent of
	// k8s.StreamPodLogs's own initial-replay tail cap and the
	// k8s_get_pod_logs agent tool's fetch cap -- this is purely how much
	// scrollback the live view keeps before trimming.
	podLogsMaxLines = 2000
)

// PodLogs is a live, scrolling view of a single pod's logs opened via
// ActionViewPodLogs. It owns the cancel func for the streaming context
// used to run `kubectl logs -f`: HandleMsg cancels it itself when the
// user closes the dialog, mirroring MCPAuth's cancelAuth -- this is what
// stops a dismissed dialog from leaving a `kubectl logs -f` subprocess
// running in the background. The caller (UI) feeds it lines via
// AppendLine as they stream in and marks it finished via SetStopped.
type PodLogs struct {
	com  *common.Common
	help help.Model
	view viewport.Model

	namespace string
	name      string

	lines   []string
	stopped bool
	err     error
	cancel  context.CancelFunc

	keyMap struct {
		Close key.Binding
	}
}

var _ Dialog = (*PodLogs)(nil)

// NewPodLogs creates a new live pod-log dialog for namespace/name. cancel
// stops the underlying log stream and is called both when the dialog is
// closed by the user and, defensively, from Close if the caller replaces
// this dialog without going through the normal close path.
func NewPodLogs(com *common.Common, namespace, name string, cancel context.CancelFunc) *PodLogs {
	d := &PodLogs{
		com:       com,
		namespace: namespace,
		name:      name,
		cancel:    cancel,
	}

	h := help.New()
	h.Styles = com.Styles.DialogHelpStyles()
	d.help = h

	d.view = viewport.New()
	d.view.SoftWrap = true

	d.keyMap.Close = CloseKey

	return d
}

// ID implements [Dialog].
func (d *PodLogs) ID() string {
	return PodLogsID
}

// Namespace returns the namespace of the pod this dialog is viewing.
func (d *PodLogs) Namespace() string {
	return d.namespace
}

// Name returns the name of the pod this dialog is viewing.
func (d *PodLogs) Name() string {
	return d.name
}

// AppendLine appends a line of log output, dropping the oldest retained
// line once podLogsMaxLines is exceeded, and refreshes the viewport.
// Autoscrolls to the new content when the view was already at the bottom,
// preserving the user's scroll position otherwise.
func (d *PodLogs) AppendLine(line string) {
	d.lines = append(d.lines, line)
	if len(d.lines) > podLogsMaxLines {
		d.lines = d.lines[len(d.lines)-podLogsMaxLines:]
	}
	atBottom := d.view.AtBottom()
	d.view.SetContentLines(d.lines)
	if atBottom {
		d.view.GotoBottom()
	}
}

// SetStopped marks the stream as finished, optionally with an error (e.g.
// ErrLogStreamingUnsupported in client/remote mode, or a kubectl failure).
func (d *PodLogs) SetStopped(err error) {
	d.stopped = true
	d.err = err
}

// Close cancels the underlying log stream, if still running. Safe to call
// more than once or after the stream has already stopped on its own.
func (d *PodLogs) Close() {
	if d.cancel != nil {
		d.cancel()
		d.cancel = nil
	}
}

// HandleMsg implements [Dialog].
func (d *PodLogs) HandleMsg(msg tea.Msg) Action {
	if kp, ok := msg.(tea.KeyPressMsg); ok && key.Matches(kp, d.keyMap.Close) {
		d.Close()
		return ActionClose{}
	}
	var cmd tea.Cmd
	d.view, cmd = d.view.Update(msg)
	if cmd != nil {
		return ActionCmd{cmd}
	}
	return nil
}

// Draw implements [Dialog].
func (d *PodLogs) Draw(scr uv.Screen, area uv.Rectangle) *tea.Cursor {
	t := d.com.Styles
	width := max(0, min(podLogsDialogMaxWidth, area.Dx()-t.Dialog.View.GetHorizontalBorderSize()))
	height := max(0, min(podLogsDialogMaxHeight, area.Dy()-t.Dialog.View.GetVerticalBorderSize()))
	innerWidth := width - t.Dialog.View.GetHorizontalFrameSize()
	heightOffset := t.Dialog.Title.GetVerticalFrameSize() + titleContentHeight +
		t.Dialog.HelpView.GetVerticalFrameSize() +
		t.Dialog.View.GetVerticalFrameSize()

	d.view.SetWidth(innerWidth)
	d.view.SetHeight(max(0, height-heightOffset))

	rc := NewRenderContext(t, width)
	rc.Title = fmt.Sprintf("Logs · %s/%s", d.namespace, d.name)

	switch {
	case d.err != nil:
		rc.AddPart(t.Dialog.NormalItem.Render(fmt.Sprintf("error: %v", d.err)))
	case len(d.lines) == 0:
		if d.stopped {
			rc.AddPart(t.Dialog.NormalItem.Render("No logs"))
		} else {
			rc.AddPart(t.Dialog.NormalItem.Render("Waiting for logs…"))
		}
	default:
		rc.AddPart(d.view.View())
		if d.stopped {
			rc.AddPart(t.Dialog.ListItem.InfoBlurred.Render("[stream ended]"))
		}
	}
	rc.Help = renderDialogHelp(t, &d.help, d, innerWidth)

	view := rc.Render()
	DrawCenter(scr, area, view)
	return nil
}

// ShortHelp implements [help.KeyMap].
func (d *PodLogs) ShortHelp() []key.Binding {
	return []key.Binding{
		d.view.KeyMap.Up,
		d.view.KeyMap.Down,
		d.keyMap.Close,
	}
}

// FullHelp implements [help.KeyMap].
func (d *PodLogs) FullHelp() [][]key.Binding {
	return [][]key.Binding{d.ShortHelp()}
}
