package dialog

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"charm.land/catwalk/pkg/catwalk"
	"github.com/charmbracelet/crush/internal/commands"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/message"
	"github.com/charmbracelet/crush/internal/oauth"
	"github.com/charmbracelet/crush/internal/permission"
	"github.com/charmbracelet/crush/internal/session"
	"github.com/charmbracelet/crush/internal/skills"
	"github.com/charmbracelet/crush/internal/ui/common"
	"github.com/charmbracelet/crush/internal/ui/styles"
	"github.com/charmbracelet/crush/internal/ui/util"
)

// ActionClose is a message to close the current dialog.
type ActionClose struct{}

// ActionQuit is a message to quit the application.
type ActionQuit = tea.QuitMsg

// ActionOpenDialog is a message to open a dialog.
type ActionOpenDialog struct {
	DialogID string
}

// ActionSelectSession is a message indicating a session has been selected.
type ActionSelectSession struct {
	Session session.Session
}

// ActionDeletePod is a message indicating the user wants to delete a pod
// from the Pods panel. It carries only namespace/name, not a request to
// delete directly — the handler routes it through the agent so the normal
// k8s_delete_pod permission prompt still applies.
type ActionDeletePod struct {
	Namespace string
	Name      string
}

// ActionDeleteDeployment is a message indicating the user wants to delete a
// deployment from the Deployments panel. It carries only namespace/name,
// not a request to delete directly — the handler routes it through the
// agent so the normal k8s_delete_deployment permission prompt still
// applies. Mirrors ActionDeletePod.
type ActionDeleteDeployment struct {
	Namespace string
	Name      string
}

// ActionScaleDeployment is a message indicating the user wants to scale a
// deployment from the Deployments panel, after entering a target replica
// count in the panel's inline prompt. Like ActionDeleteDeployment, this is
// routed through the agent's permission-gated k8s_scale_deployment tool
// rather than scaling directly.
type ActionScaleDeployment struct {
	Namespace string
	Name      string
	Replicas  int
}

// ActionViewDeploymentPods is a message indicating the user wants to drill
// down from a deployment to the pods it owns. Unlike the other Deployment
// actions, this never reaches the agent — it's answered entirely from
// data the UI already has (the most recent pod snapshot, filtered by
// Selector), so the handler opens a dialog directly rather than sending a
// chat message.
type ActionViewDeploymentPods struct {
	Namespace string
	Name      string
	Selector  map[string]string
}

// ActionViewPodLogs is a message indicating the user wants to open a live,
// streaming view of a pod's logs. Like ActionViewDeploymentPods, this
// bypasses the agent entirely: the handler opens a dialog and starts
// K8sStreamPodLogs directly, since there's no mutation to permission-gate.
type ActionViewPodLogs struct {
	Namespace string
	Name      string
}

// ActionExecPod is a message indicating the user wants an interactive
// shell into a pod, via `kubectl exec -it`. Like ActionViewPodLogs, this
// bypasses the agent entirely — there's no mutation to permission-gate,
// only a raw terminal handoff. Unlike every other dialog action, its
// handler suspends the TUI itself (see UI.execIntoPod) rather than
// opening another dialog or sending a chat message.
type ActionExecPod struct {
	Namespace string
	Name      string
}

// ActionSetNamespace is a message indicating the user has chosen a
// namespace scope for the Pods/Deployments panels. Like
// ActionViewDeploymentPods, this bypasses the agent entirely: it's a pure
// TUI display-scope change, not a cluster mutation, so the handler calls
// Workspace.K8sSetNamespace directly rather than sending a chat message.
// AllNamespaces true means "every namespace" and Namespace is ignored.
type ActionSetNamespace struct {
	Namespace     string
	AllNamespaces bool
}

// ActionSelectModel is a message indicating a model has been selected.
type ActionSelectModel struct {
	Provider       catwalk.Provider
	Model          config.SelectedModel
	ModelType      config.SelectedModelType
	ReAuthenticate bool
}

// Messages for commands
type (
	ActionNewSession              struct{}
	ActionToggleHelp              struct{}
	ActionToggleCompactMode       struct{}
	ActionToggleThinking          struct{}
	ActionTogglePills             struct{}
	ActionExternalEditor          struct{}
	ActionToggleYoloMode          struct{}
	ActionToggleNotifications     struct{}
	ActionSelectNotificationStyle struct {
		Style string
	}
	ActionToggleTransparentBackground struct{}
	ActionToggleMouseSupport          struct{}
	ActionSwitchTheme                 struct {
		Theme string
	}
	ActionPreviewTheme struct {
		Theme string
	}
	ActionRevertThemePreview  struct{}
	ActionPreviewThemePalette struct {
		Base    string
		Palette styles.Palette
	}
	ActionSaveThemePalette struct {
		Name    string
		Base    string
		Palette styles.Palette
	}
	ActionEditTheme struct {
		Name string
	}
	ActionRevertThemePalette    struct{}
	ActionRevertOverriddenTheme struct {
		Name string
	}
	ActionCreateTheme struct {
		Name string
		Base string
	}
	ActionRenameTheme struct {
		OldName string
		NewName string
	}
	ActionDeleteTheme struct {
		Name string
	}
	ActionInitializeProject struct{}
	ActionSummarize         struct {
		SessionID string
	}
	// ActionSelectReasoningEffort is a message indicating a reasoning effort
	// has been selected.
	ActionSelectReasoningEffort struct {
		Effort string
	}
	ActionPermissionResponse struct {
		Permission permission.PermissionRequest
		Action     PermissionAction
	}
	// ActionRunCustomCommand is a message to run a custom command.
	ActionRunCustomCommand struct {
		Content   string
		Arguments []commands.Argument
		Args      map[string]string // Actual argument values
		Skill     *skills.Skill     // Set when this is a skill command
	}
	// ActionAttachSkill is sent when a skill is selected from the commands
	// dialog to be attached to the conversation as a markdown attachment.
	ActionAttachSkill struct {
		ID   string
		Name string
	}
	// ActionRunMCPPrompt is a message to run a custom command.
	ActionRunMCPPrompt struct {
		Title       string
		Description string
		PromptID    string
		ClientID    string
		Arguments   []commands.Argument
		Args        map[string]string // Actual argument values
	}
	// ActionEnableDockerMCP is a message to enable Docker MCP.
	ActionEnableDockerMCP struct{}
	// ActionDisableDockerMCP is a message to disable Docker MCP.
	ActionDisableDockerMCP struct{}
)

// Messages for MCP OAuth authentication dialog.
type (
	// ActionMCPAuthStarted is sent when the user approves authentication
	// for an MCP server. The UI should initiate the actual auth flow
	// using the provided context, which the dialog will cancel if the
	// user closes it.
	ActionMCPAuthStarted struct {
		Name string
		Ctx  context.Context
	}

	// ActionMCPAuthComplete is sent when MCP authentication succeeds.
	ActionMCPAuthComplete struct {
		Name string
	}

	// ActionMCPAuthErrored is sent when MCP authentication fails.
	ActionMCPAuthErrored struct {
		Name  string
		Error error
	}
)

// Messages for API key input dialog.
type (
	ActionChangeAPIKeyState struct {
		State APIKeyInputState
	}
)

// Messages for OAuth2 device flow dialog.
type (
	// ActionInitiateOAuth is sent when the device auth is initiated
	// successfully.
	ActionInitiateOAuth struct {
		DeviceCode      string
		UserCode        string
		ExpiresIn       int
		VerificationURL string
		Interval        int
	}

	// ActionCompleteOAuth is sent when the device flow completes successfully.
	ActionCompleteOAuth struct {
		Token *oauth.Token
	}

	// ActionOAuthErrored is sent when the device flow encounters an error.
	ActionOAuthErrored struct {
		Error error
	}

	// ActionCloseOAuth closes the OAuth dialog and runs the given cleanup
	// command, cancelling any in-flight authorization. It exists so a
	// dismissed dialog does not leave a poller or loopback listener
	// running in the background.
	ActionCloseOAuth struct {
		Cmd tea.Cmd
	}

	// ActionSelectAuthMethod is sent when the user picks how to
	// authenticate a provider that supports both OAuth and API keys.
	ActionSelectAuthMethod struct {
		Provider  catwalk.Provider
		Model     config.SelectedModel
		ModelType config.SelectedModelType
		UseOAuth  bool
	}
)

// ActionCmd represents an action that carries a [tea.Cmd] to be passed to the
// Bubble Tea program loop.
type ActionCmd struct {
	Cmd tea.Cmd
}

// ActionFilePickerSelected is a message indicating a file has been selected in
// the file picker dialog.
type ActionFilePickerSelected struct {
	Path string
}

// Cmd returns a command that reads the file at path and sends a
// [message.Attachement] to the program.
func (a ActionFilePickerSelected) Cmd() tea.Cmd {
	path := a.Path
	if path == "" {
		return nil
	}
	return func() tea.Msg {
		isFileLarge, err := common.IsFileTooBig(path, common.MaxAttachmentSize)
		if err != nil {
			return util.InfoMsg{
				Type: util.InfoTypeError,
				Msg:  fmt.Sprintf("unable to read the image: %v", err),
			}
		}
		if isFileLarge {
			return util.InfoMsg{
				Type: util.InfoTypeError,
				Msg:  "file too large, max 5MB",
			}
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return util.InfoMsg{
				Type: util.InfoTypeError,
				Msg:  fmt.Sprintf("unable to read the image: %v", err),
			}
		}

		mimeBufferSize := min(512, len(content))
		mimeType := http.DetectContentType(content[:mimeBufferSize])
		fileName := filepath.Base(path)

		return message.Attachment{
			FilePath: path,
			FileName: fileName,
			MimeType: mimeType,
			Content:  content,
		}
	}
}
