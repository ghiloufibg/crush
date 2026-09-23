// Package workspace defines the Workspace interface used by all
// frontends (TUI, CLI) to interact with a running workspace. Two
// implementations exist: one wrapping a local app.App instance and one
// wrapping the HTTP client SDK.
package workspace

import (
	"context"
	"errors"
	"os/exec"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/catwalk/pkg/catwalk"
	mcptools "github.com/charmbracelet/crush/internal/agent/tools/mcp"
	"github.com/charmbracelet/crush/internal/commands"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/history"
	"github.com/charmbracelet/crush/internal/lsp"
	"github.com/charmbracelet/crush/internal/message"
	"github.com/charmbracelet/crush/internal/oauth"
	"github.com/charmbracelet/crush/internal/permission"
	"github.com/charmbracelet/crush/internal/proto"
	"github.com/charmbracelet/crush/internal/question"
	"github.com/charmbracelet/crush/internal/session"
	"github.com/charmbracelet/crush/internal/skills"
)

// Reasons the coder agent may be unavailable, returned by
// Workspace.AgentReadyErr so callers can tell a genuinely
// uninitialized agent apart from a lost server connection.
var (
	// ErrAgentNotInitialized means the workspace exists but its coder
	// agent has not been configured/initialized (e.g. no model set).
	ErrAgentNotInitialized = errors.New("coder agent is not initialized")
	// ErrServerUnreachable means the client could not reach the server
	// to determine the agent's status (server down, or the workspace was
	// torn down out from under the client).
	ErrServerUnreachable = errors.New("lost connection to the crush server")
	// ErrWorkspaceGone means the server is reachable but no longer knows
	// this client's workspace: it was torn down, or the server was
	// replaced underneath the client. The subscription loop re-registers
	// the workspace in the background when it sees this.
	ErrWorkspaceGone = errors.New("the server reset this workspace; reconnecting")
	// ErrStreamClosed means an established event stream ended.
	// Resubscribing usually succeeds immediately, but events published in
	// the meantime are lost for good, so the client treats it as a
	// degraded link that requires a resync.
	ErrStreamClosed = errors.New("the event stream closed; reconnecting")
)

// ConnectionState describes the health of the client-server link as
// reported by the [ClientWorkspace] subscription loop.
type ConnectionState int

const (
	// ConnectionDegraded means the event stream is down (or the workspace
	// was lost server-side) and the client is retrying or re-registering
	// in the background.
	ConnectionDegraded ConnectionState = iota
	// ConnectionRecovered means the event stream was re-established,
	// possibly against a re-created workspace.
	ConnectionRecovered
)

// ConnectionEvent is delivered to the TUI as a tea.Msg on degraded and
// recovered transitions of the client-server link. Local (in-process)
// workspaces never emit it.
type ConnectionEvent struct {
	State ConnectionState
	// Err is the most recent failure, set when State is
	// ConnectionDegraded.
	Err error
	// Stuck marks a degraded connection that has resisted repeated
	// recovery attempts. The loop keeps retrying regardless; the UI
	// should escalate from a transient notice to a persistent error.
	Stuck bool
}

// LSPClientInfo holds information about an LSP client's state. This is
// the frontend-facing type; implementations translate from the
// underlying app or proto representation.
type LSPClientInfo struct {
	Name            string
	State           lsp.ServerState
	Error           error
	DiagnosticCount int
	ConnectedAt     time.Time
}

// LSPEventType represents the type of LSP event.
type LSPEventType string

const (
	LSPEventStateChanged       LSPEventType = "state_changed"
	LSPEventDiagnosticsChanged LSPEventType = "diagnostics_changed"
)

// LSPEvent represents an LSP event forwarded to the TUI.
type LSPEvent struct {
	Type            LSPEventType
	Name            string
	State           lsp.ServerState
	Error           error
	DiagnosticCount int
}

// AgentModel holds the model information exposed to the UI.
type AgentModel struct {
	CatwalkCfg catwalk.Model
	ModelCfg   config.SelectedModel
}

// Workspace is the main abstraction consumed by the TUI and CLI. It
// groups every operation a frontend needs to perform against a running
// workspace, regardless of whether the workspace is in-process or
// remote.
type Workspace interface {
	// Sessions
	CreateSession(ctx context.Context, title string) (session.Session, error)
	GetSession(ctx context.Context, sessionID string) (session.Session, error)
	ListSessions(ctx context.Context) ([]session.Session, error)
	SaveSession(ctx context.Context, sess session.Session) (session.Session, error)
	DeleteSession(ctx context.Context, sessionID string) error
	CreateAgentToolSessionID(messageID, toolCallID string) string
	ParseAgentToolSessionID(sessionID string) (messageID string, toolCallID string, ok bool)
	// SetCurrentSession reports the session this client is currently
	// viewing. Empty sessionID clears the entry (e.g. landing screen).
	// In single-client local mode this is a no-op. In client/server
	// mode it informs the server's per-client presence map so other
	// observers can compute attached-client counts per session.
	SetCurrentSession(ctx context.Context, sessionID string) error

	// Messages
	ListMessages(ctx context.Context, sessionID string) ([]message.Message, error)
	ListUserMessages(ctx context.Context, sessionID string) ([]message.Message, error)
	ListAllUserMessages(ctx context.Context) ([]message.Message, error)

	// Agent
	AgentRun(ctx context.Context, sessionID, prompt string, attachments ...message.Attachment) error
	AgentRunShellCommand(ctx context.Context, sessionID, command string, termWidth int, onProgress func(string), isFirstMessage bool) (proto.ShellCommandResponse, error)
	AgentCancel(sessionID string)
	AgentIsBusy() bool
	AgentIsSessionBusy(sessionID string) bool
	AgentModel() AgentModel
	AgentIsReady() bool
	// AgentReadyErr reports nil when the coder agent is ready to accept
	// work, or a descriptive error otherwise: ErrAgentNotInitialized
	// when the agent simply isn't set up, or ErrServerUnreachable
	// (wrapped) when the client could not reach the server to find out.
	// It lets the UI show an actionable message instead of collapsing
	// both cases into "agent offline".
	AgentReadyErr() error
	AgentQueuedPrompts(sessionID string) int
	AgentQueuedPromptsList(sessionID string) []string
	AgentClearQueue(sessionID string)
	AgentSetMain(agentID string) error
	AgentSummarize(ctx context.Context, sessionID string) error
	UpdateAgentModel(ctx context.Context) error
	InitCoderAgent(ctx context.Context) error
	InitCoderAgentNonInteractive(ctx context.Context) error
	GetDefaultSmallModel(providerID string) config.SelectedModel

	// Permissions
	//
	// PermissionGrant, PermissionGrantPersistent, and PermissionDeny
	// return true if the call resolved the pending request and false if
	// it had already been resolved by another subscriber (or is no
	// longer pending). A false return is not an error; the modal can
	// still close locally because the resolution will arrive via the
	// PermissionNotification event stream regardless of which client
	// won the race.
	PermissionGrant(perm permission.PermissionRequest) bool
	PermissionGrantPersistent(perm permission.PermissionRequest) bool
	PermissionDeny(perm permission.PermissionRequest) bool
	PermissionSkipRequests() bool
	PermissionSetSkipRequests(skip bool)

	// Questions
	//
	// QuestionAnswer resolves the pending question with responses.
	QuestionAnswer(responses []question.Answer) bool

	// QuestionCancel cancels the pending question.
	QuestionCancel() bool

	// FileTracker
	FileTrackerRecordRead(ctx context.Context, sessionID, path string)
	FileTrackerLastReadTime(ctx context.Context, sessionID, path string) time.Time
	FileTrackerListReadFiles(ctx context.Context, sessionID string) ([]string, error)

	// History
	ListSessionHistory(ctx context.Context, sessionID string) ([]history.File, error)

	// LSP
	LSPStart(ctx context.Context, path string)
	LSPStopAll(ctx context.Context)
	LSPGetStates() map[string]LSPClientInfo
	LSPGetDiagnosticCounts(name string) lsp.DiagnosticCounts

	// Config (read-only data)
	Config() *config.Config
	WorkingDir() string
	Resolver() config.VariableResolver

	// Config mutations (proxied to server in client mode)
	UpdatePreferredModel(scope config.Scope, modelType config.SelectedModelType, model config.SelectedModel) error
	SetCompactMode(scope config.Scope, enabled bool) error
	SetProviderAPIKey(scope config.Scope, providerID string, apiKey any) error
	SetConfigField(scope config.Scope, key string, value any) error
	SetConfigFields(scope config.Scope, fields map[string]any) error
	RemoveConfigField(scope config.Scope, key string) error
	ImportCopilot() (*oauth.Token, bool)
	RefreshOAuthToken(ctx context.Context, scope config.Scope, providerID string) error

	// Project lifecycle
	ProjectNeedsInitialization() (bool, error)
	MarkProjectInitialized() error
	InitializePrompt() (string, error)
	ListSkills(ctx context.Context) ([]skills.CatalogEntry, error)
	ReadSkill(ctx context.Context, skillID string) ([]byte, skills.SkillReadResult, error)

	// MCP operations (server-side in client mode)
	MCPGetStates() map[string]mcptools.ClientInfo
	MCPRefreshPrompts(ctx context.Context, name string)
	MCPRefreshResources(ctx context.Context, name string)
	RefreshMCPTools(ctx context.Context, name string)
	ReadMCPResource(ctx context.Context, name, uri string) ([]MCPResourceContents, error)
	ListMCPPrompts(ctx context.Context) ([]commands.MCPPrompt, error)
	GetMCPPrompt(clientID, promptID string, args map[string]string) (string, error)
	EnableDockerMCP(ctx context.Context) error
	DisableDockerMCP() error
	MCPAuthenticate(ctx context.Context, name string) error
	MCPPendingAuth() []mcptools.PendingAuthServer
	MCPAuthURL(name string) string

	// Kubernetes
	//
	// K8sStreamPodLogs streams logs for the given pod, invoking onLine
	// once per line as it arrives. It blocks until ctx is cancelled or
	// the stream ends. Unlike the Pods/Deployments panels' data (which
	// rides the generic pubsub event pipeline set up once at app
	// startup), starting a log stream is a UI-triggered, on-demand
	// action, so it goes through Workspace like everything else a
	// frontend action initiates. Not every implementation can honor
	// this: see ErrLogStreamingUnsupported.
	K8sStreamPodLogs(ctx context.Context, namespace, name string, onLine func(string)) error

	// K8sListNamespaces lists the cluster's namespace names.
	K8sListNamespaces(ctx context.Context) ([]string, error)

	// K8sSetNamespace retargets the Pods/Deployments panels' already-running
	// watchers at namespace (or every namespace, when allNamespaces is
	// true). Like K8sStreamPodLogs, this is a UI-triggered, on-demand
	// action, so it goes through Workspace rather than the generic pubsub
	// pipeline. It is a pure TUI display-scope change, not a cluster
	// mutation, so it never goes through the agent. Not every
	// implementation can honor this: see ErrNamespaceSwitchUnsupported.
	K8sSetNamespace(namespace string, allNamespaces bool) error

	// K8sNamespace reports the Pods/Deployments panels' current namespace
	// scope.
	K8sNamespace() (namespace string, allNamespaces bool)

	// K8sExecPodCommand builds the local `kubectl exec -it` command for an
	// interactive shell into a pod, for the caller to hand directly to the
	// terminal (e.g. via tea.ExecProcess). Unlike K8sStreamPodLogs, this
	// cannot be proxied over RPC even in principle: a raw PTY handoff only
	// makes sense when the TUI process itself has direct terminal and
	// kubectl access. Not every implementation can honor this: see
	// ErrExecUnsupported.
	K8sExecPodCommand(namespace, name string) (*exec.Cmd, error)

	// K8sAcquirePodWatcher/K8sReleasePodWatcher and
	// K8sAcquireDeploymentWatcher/K8sReleaseDeploymentWatcher mark a
	// panel that reads pod or deployment snapshots (Pods, Deployments,
	// or the DeploymentPods drill-down) as visible or no longer visible,
	// so the underlying watcher's poll loop only runs while something is
	// actually displaying its data. Calls are reference-counted and must
	// be paired by the caller; an unpaired release is a no-op. These are
	// a pure local performance optimization, not a cluster mutation or a
	// display-scope change, so unlike K8sSetNamespace there is nothing
	// for a workspace implementation to fail at: implementations that
	// cannot control watcher lifecycle remotely (see [ClientWorkspace])
	// simply no-op, leaving the server-side watcher running continuously
	// as it always has.
	K8sAcquirePodWatcher()
	K8sReleasePodWatcher()
	K8sAcquireDeploymentWatcher()
	K8sReleaseDeploymentWatcher()

	// Events
	Subscribe(program *tea.Program)
	Shutdown()
}

// ErrLogStreamingUnsupported is returned by K8sStreamPodLogs
// implementations that cannot honor live streaming (currently
// [ClientWorkspace], which has no remote-log-streaming RPC in this spike).
// Callers should surface it as a clear, immediate error rather than
// leaving the caller waiting for lines that will never arrive.
var ErrLogStreamingUnsupported = errors.New("log streaming is not supported for this workspace")

// ErrNamespaceSwitchUnsupported is returned by K8sSetNamespace
// implementations that cannot honor runtime namespace switching (currently
// [ClientWorkspace], which has no remote watcher-reconfiguration RPC in
// this spike).
var ErrNamespaceSwitchUnsupported = errors.New("namespace switching is not supported for this workspace")

// ErrExecUnsupported is returned by K8sExecPodCommand implementations that
// cannot honor an interactive pod shell (currently [ClientWorkspace]: a raw
// PTY handoff has no remote equivalent in this spike, unlike streaming
// output over RPC).
var ErrExecUnsupported = errors.New("exec is not supported for this workspace")

// MCPResourceContents holds the contents of an MCP resource.
type MCPResourceContents struct {
	URI      string `json:"uri"`
	MIMEType string `json:"mime_type,omitempty"`
	Text     string `json:"text,omitempty"`
	Blob     []byte `json:"blob,omitempty"`
}
