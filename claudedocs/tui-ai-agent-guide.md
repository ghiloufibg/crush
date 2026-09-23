# Wiring a live Kubernetes resource into the TUI + AI agent

This guide documents the pattern this codebase uses to connect a Kubernetes
resource type to both the interactive TUI panel and the AI agent's tools, so
that a human and the agent are looking at (and acting on) the same live
data. It was extracted by building the Deployments panel as a second,
independent instance of the pattern established by the Pods panel, and
recording what stayed generic versus what had to be decided fresh.

## The pattern in one picture

```
kubectl (client functions)
      │
      ▼
Watcher (poll loop) ──publish──► pubsub.Broker[[]T]
      │                                  │
      │ App.subscribe[T](...)            │ Subscribe(ctx)
      ▼                                  ▼
  App struct                    UI.Update (event bridge)
  (field + goroutine)                    │
                                          ▼
                                  Dialog (list + keybindings)
                                          │
                                    HandleMsg → Action
                                          │
                                          ▼
                            UI.Update's Action switch
                                          │
                          synthesizes a natural-language
                          chat message → sendMessage(...)
                                          │
                                          ▼
                                    Agent + its tools
                              (k8s_get_X / k8s_delete_X / ...)
                                          │
                                permission gate → kubectl
```

The reason this shape exists at all: the dialog is a *viewer with shortcuts*,
not an actor. When the user presses `d` to delete something, the dialog
doesn't call `kubectl delete` itself — it returns an `Action` describing
intent, which `UI.Update` turns into a chat message sent to the agent. The
agent then calls the same permission-gated tool a human typing in the chat
would trigger. This keeps exactly one code path — the agent tool — as the
place mutations happen and get permission-checked, whether the trigger was a
keypress or a typed request.

## The reusable pieces (unchanged between Pods and Deployments)

These didn't need a single decision the second time around — they were
copy-shape-and-rename:

1. **Domain struct** (`internal/k8s/<resource>.go`): a flat struct holding
   only what the TUI displays, plus a `Key() string` method
   (`namespace/name`) used for list identity and selection-preservation
   across snapshots.
2. **kubectl client functions** (`internal/k8s/<resource>_client.go`): hand-
   rolled JSON structs matching only the fields read (`kubectlXList` /
   `kubectlX`), not a dependency on `k8s.io/api/...` — avoids a heavy
   dependency for a read of 4-6 fields. Pattern per file: `List<X>(ctx,
   namespace, allNamespaces)`, a `parseKubectlXList(stdout string)` split out
   for unit testing without shelling out, any mutating actions
   (`Delete<X>`, `Scale<X>`, ...), and a `Format<X>List` tab-separated
   renderer for the agent's read tool to return as plain text.
3. **Watcher** (`internal/k8s/<resource>_watcher.go`): polls on a ticker,
   publishes full-snapshot-replace events to a `pubsub.Broker[[]T]`, and
   swallows poll errors so a transient `kubectl` failure doesn't blank out
   the last-good view. See the "Watcher generics" decision below for why
   this is a concrete duplicate rather than a generic type.
4. **Dialog + list item** (`internal/ui/dialog/<resource>.go`): a
   `list.FilterableList` of resource items, a `Set<X>(items)` method that
   replaces the snapshot while preserving selection by `Key()` (falling back
   to the first item if the previously-selected one is gone), and
   `HandleMsg` returning `Action` values rather than mutating anything
   directly.
5. **Action types** (`internal/ui/dialog/actions.go`): plain structs (e.g.
   `ActionDeleteDeployment{Namespace, Name}`) that only carry the identifying
   data for a request — never a description string or a pre-built command,
   because `UI.Update` is the single place that turns intent into the
   chat message text sent to the agent.
6. **Agent tools** (`internal/agent/tools/k8s_<verb>_<resource>.go` + a
   sibling `.md` description file): read tools have no permission gate;
   mutating tools request permission before calling the client function,
   mirroring whatever the equivalent Pod tool does.
7. **Wiring points** — five files touched for every new resource, none of
   them resource-generic:
   - `internal/ui/model/keys.go`: one `key.Binding` field + entry in
     `DefaultKeyMap()`.
   - `internal/ui/model/ui.go`: a `lastKnown<X> []k8s.X` field, a
     `pubsub.Event[[]k8s.X]` case that stores the snapshot and forwards it to
     an open dialog, `Action<Verb><X>` case(s) that build the chat message
     and call `sendMessage`, a keypress case that opens the dialog, a
     dialog-reopen case (`dialog.<X>ID`) for returning to a panel that was
     backgrounded, and an `open<X>Dialog() tea.Cmd` helper.
   - `internal/agent/coordinator.go`: register the new tool(s) in
     `buildTools()`.
   - `internal/app/app.go`: a `<resource>Watcher *k8s.<X>Watcher` field, its
     construction in `New()`, a `go app.<resource>Watcher.Run(...)`
     goroutine, and an `app.subscribe(ctx, "k8s-<resource>", ...)` call in
     `setupEvents()`.

That's the checklist for adding a new resource type — see the bottom of this
document for it as a standalone list.

## Decisions made building the second panel (Deployments)

Everything above was known going in, from tracing the Pods implementation.
Two things were *not* obvious until building a second, non-trivial panel
forced a choice.

### 1. Watcher: concrete duplicate, not `Watcher[T any]`

The natural instinct on seeing `Watcher` and `DeploymentWatcher` side by side
is "make it generic." That was evaluated and rejected for a specific,
mechanical reason: `Watcher`'s configuration is a functional-options
pattern, and one of the options is zero-argument —
`WithAllNamespaces()`. Go cannot infer a type parameter from a
zero-argument function call, so a generic `Watcher[T]` would force every
call site to instantiate explicitly: `WithAllNamespaces[[]k8s.Pod]()`,
`WithAllNamespaces[[]k8s.Deployment]()`, and so on for every option, at
every use. That's a real, visible syntax cost paid on every call site,
forever — not a one-time refactor cost. For two resource types, duplicating
~90 lines of poll-loop boilerplate is cheaper than that syntax tax. The
rationale is written directly into `DeploymentWatcher`'s doc comment in
`internal/k8s/deployment_watcher.go` so it doesn't have to be re-derived.

This is a threshold judgment, not a permanent rule: if a third resource type
arrives, re-run the comparison. At three duplicates, the generics syntax
cost may start looking cheap relative to the duplication, especially if a
way to avoid the zero-arg-option inference problem presents itself (e.g.
splitting `WithAllNamespaces` into a resource-specific concrete option that
wraps a generic core).

### 2. Collecting a mutation parameter: inline sub-state, not a new dialog

Pod delete needs nothing beyond what's already on screen — the selected
pod's namespace and name are known at keypress time. Deployment scale needs
a replica count, which is not known until the user types it. This is the
first case in the codebase where a dialog-triggered action needs
interactive input before it can produce an `Action`.

Two existing patterns were read as candidates before deciding:

- **`internal/ui/dialog/arguments.go`** (`Arguments` dialog): a full
  multi-field, viewport-scrolled dialog for collecting several named
  arguments (used for command/MCP-prompt arguments). Built for the general
  case of "N fields, each possibly multi-line."
- **`internal/ui/dialog/api_key_input.go`** (`APIKeyInput` dialog): a
  single-field `textinput.Model` dialog, its own top-level `Dialog`.

Neither fit well. `Arguments` is designed for a scenario this isn't
(multiple fields, dialog-per-field-set) and pulling it in for one integer
would mean carrying its viewport/multi-field machinery for no benefit.
`APIKeyInput` fits the "one field" shape but as a *separate* dialog it would
require an extra round trip: close the Deployments list, open a new dialog,
collect the value, then somehow get back to (or past) the list — plus a new
`DialogID` and wiring through the same five files as a full new panel, for
what is really a momentary sub-state of an existing one.

The chosen third option: keep it inside `Deployments` as an inline sub-state
— a `scaling bool` flag plus a single `textinput.Model` field
(`replicasInput`) and a `scalingTarget k8s.Deployment` to remember which row
triggered it. `HandleMsg` branches at the top on `d.scaling` to a dedicated
`handleScalingMsg`, and `Draw` branches the same way to render either the
list or the prompt. Pressing `s` on a selected row calls `startScaling`,
which pre-fills the input with the deployment's *current* replica count
(so confirming with no edits is a no-op scale, and the common case — nudge
the number up or down — is a short edit rather than typing from scratch).
Enter validates (non-negative integer) and, on success, returns
`ActionScaleDeployment{Namespace, Name, Replicas}` through the same
Action-message-indirection path as delete; on failure it returns an
`ActionCmd` wrapping a `util.ReportWarn(...)` toast and stays in scaling
mode rather than closing. Escape cancels back to the list without emitting
an Action.

Rule of thumb for the next parameterized action: if it's one field and
logically "modifies the currently selected row," prefer inline sub-state in
the existing dialog over a new dialog. Reach for a separate dialog (or
`Arguments`) only when the parameters aren't tied to a single selected row,
or there's more than one or two fields.

## Beyond list+mutate: three more interaction shapes

Pods and Deployments are both "list a resource, mutate a selected row."
Three other features needed in this codebase don't fit that shape, and each
required a different structural answer rather than a variation on the
watcher/dialog/tool checklist above.

### Drill-down / contextual navigation (Deployment → its Pods)

`DeploymentPods` (`internal/ui/dialog/deployment_pods.go`) shows the pods
belonging to a selected deployment. The tempting design is a second
watcher scoped to that deployment's selector. Instead it reuses the *same*
pod snapshot the Pods panel already has: `ActionViewDeploymentPods` passes
`m.lastKnownPods` (the UI's last-known Pods watcher snapshot) straight into
`dialog.NewDeploymentPods(...)`, and `SetAllPods` filters that snapshot
client-side by `pod.Namespace == d.namespace && pod.MatchesSelector(d.selector)`
on every subsequent update. There's no separate navigation stack, either —
opening a drill-down for a different deployment while one is already open
just closes the existing `DeploymentPodsID` dialog and opens a new one, and
because the drill-down depends on the same pod watcher as the Pods panel,
it acquires/releases that watcher independently of whether Pods itself
happens to be open (see the watcher-lifecycle item below).

Why: the pod-belongs-to-deployment relationship is already fully expressed
by the Deployment's label selector, and the Pods watcher is already
streaming every pod in scope. Filtering an existing live snapshot is
free; a second `kubectl` poll loop scoped to one deployment would be a
second watcher, a second wiring pass, and a second source of poll-interval
lag for data this process already has in memory.

### Streaming vs. snapshot-replace data (pod logs)

Every watcher so far replaces its entire snapshot on each poll — the right
shape for "what pods exist right now." Logs are different: `StreamPodLogs`
(`internal/k8s/log_stream.go`) runs `kubectl logs -f` and keeps the process
open, pushing each new line to an `onLine` callback as it's written, rather
than polling and diffing. Re-fetching and re-diffing log output on an
interval would be both wasteful (re-reading unbounded history each poll)
and lossy (anything written between polls would need to be reconstructed
from a diff instead of just arriving in order).

Because this is a long-lived subprocess rather than a one-shot list call,
something has to own cancelling it. `PodLogs` (`internal/ui/dialog/pod_logs.go`)
holds the stream's `context.CancelFunc` directly and calls it from its own
`Close()`, mirroring `MCPAuth.cancelAuth` — the dialog that started the
subprocess is the thing responsible for stopping it, so a dismissed dialog
can't leak a running `kubectl logs -f`.

One underlying `kubectl logs` stream also ends up with three independent
caps, each solving a different consumer's problem rather than one cap
serving all three:

- `k8s.logStreamTailLines = 500` — how much history `kubectl` itself
  replays when the live stream starts.
- `dialog.podLogsMaxLines = 2000` — the TUI's in-memory scrollback ring
  buffer, bounding how much a long session can accumulate.
- `tools.k8sGetPodLogsDefaultTailLines/MaxTailLines = 200/500` — the agent
  tool's one-shot fetch bound, independent of whatever the TUI dialog is
  currently showing.

None of these could stand in for another: a human scrolling a live view, a
TUI holding scrollback in memory, and an agent doing a single bounded read
are three different consumers with three different "how much is enough"
answers.

### Runtime-reconfigurable watchers (namespace switcher)

`Watcher`/`DeploymentWatcher` both expose `SetNamespace(namespace string,
allNamespaces bool)`, which mutates the watcher's guarded namespace state
and then does a non-blocking send on a buffered `reconfigured chan
struct{}` that `Run`'s `select` loop also watches alongside its poll
ticker. That's what makes a namespace switch take effect immediately
instead of waiting up to one poll interval — the alternative (just mutate
the field and let the next tick pick it up) would leave the panel showing
the old namespace's data for up to the full poll interval after the user
switched.

The `Namespaces` dialog itself (`internal/ui/dialog/namespaces.go`) is
seeded differently from every other dialog in this guide: it's a one-shot
`K8sListNamespaces` call (`startNamespacesLoad`), not a subscription to a
running watcher, because the set of namespaces in a cluster changes rarely
enough that polling it continuously isn't worth a third watcher. Selecting
an item returns `ActionSetNamespace{Namespace}` or `ActionSetNamespace{AllNamespaces:
true}`, and `UI.Update` calls `Workspace.K8sSetNamespace`, which retargets
*both* the pod and deployment watchers together — so Pods and Deployments
always agree on scope, rather than each panel having its own independent
namespace filter.

### One more shape that looked like a fit but isn't: exec into a pod

`ExecPodCommand` (`internal/k8s/exec.go`) builds a `kubectl exec -it ... sh`
`*exec.Cmd` but never runs it directly — `execIntoPod` hands it to
`tea.ExecProcess`, the same mechanism `openEditor` uses for `$EDITOR`
handoff, generalized from a one-shot text editor session to a long-lived
interactive shell. `tea.ExecProcess` suspends the TUI's own terminal
control for the duration, so stdin/stdout/stderr go straight to the
subprocess as a raw, bidirectional passthrough — the counterpart to pod
logs' one-directional streaming capture. `ActionExecPod` closes the Pods
dialog and releases the pod watcher *before* handing off the terminal,
since a poll loop still ticking (and potentially trying to redraw) while
the terminal belongs to another process is at best wasted work and at
worst a redraw racing a suspended TUI.

## Token budget checklist for agent tools

Concrete practices this codebase settled on for keeping a single tool
call's response bounded at real-cluster scale, gathered from the
`k8s_get_pods`/`k8s_get_deployments`/`k8s_get_pod_logs` tools:

- **Prefer server-side filtering over client-side, when the CLI supports
  it.** `label_selector` is passed straight through to `kubectl get ... -l`,
  so it narrows what `kubectl` itself returns and what this process has to
  parse. `name_contains` can't be server-side — `kubectl` has no
  substring-on-name flag — so it's applied after fetching. Both are
  offered, but the tool's parameter description says so honestly rather
  than presenting them as equivalent; an agent choosing between them should
  know one is cheaper than the other.
- **Always cap list results, and never truncate silently.** `max_results`
  defaults to `k8s.DefaultListMaxResults = 50`; when the cap is hit, the
  formatted response appends how many more matched instead of just cutting
  the list off. Silent truncation is the failure mode this avoids: without
  it, an agent has no way to tell "the cluster has exactly N matches" from
  "the cluster has more than N matches and I only saw the first N."
- **Bound the same underlying stream independently at each layer that
  touches it.** The pod-logs tool doesn't reuse the TUI dialog's 2000-line
  scrollback cap or `kubectl`'s own 500-line replay window — it has its own
  200/500 default/hard-ceiling tail bound, because a human scrolling a live
  view and an agent doing a single bounded read have different "how much is
  enough" answers even though they're reading the same command's output.
- **Give read-then-narrow tools a server-applied filter, not just a raw
  fetch.** `k8s_get_pod_logs`'s `Grep` parameter filters lines before they
  reach the response, so an agent looking for one error doesn't have to
  fetch and reason over lines it doesn't need.
- **Put identifying state directly in the tool-triggering message, don't
  make the agent re-discover it.** Delete/scale actions already have the
  namespace/name (or replica count) at keypress time, so `UI.Update` builds
  a fully-specified instruction ("Delete the pod X in namespace Y") from the
  `Action` struct's fields — the agent never needs a `k8s_get_pods` round
  trip just to find identifiers the TUI already had.
- **Treat "cheap unchanged-result signal for agent tools" as something to
  evaluate, not a default to build.** This was considered during design and
  deliberately left unbuilt: it's only worth the complexity if repeated
  re-querying of the same data is actually observed happening in practice,
  and that hasn't been observed here. Don't build a cache-invalidation
  scheme speculatively against a problem you haven't confirmed exists.

## TUI freshness/perf checklist

Concrete practices this codebase settled on for keeping the TUI responsive
and its background work proportional to what's actually visible:

- **Tie watcher lifecycle to dialog visibility, not app lifetime.**
  Ref-counted `K8sAcquire<X>Watcher`/`K8sRelease<X>Watcher` methods on the
  `Workspace` interface (no-op in `ClientWorkspace`, real pause/resume in
  `AppWorkspace`) mean a watcher's poll ticker only runs while at least one
  open dialog needs its data — opening a dialog acquires, closing it (or an
  exec-into-pod handoff suspending the terminal) releases. Without this, an
  idle background panel would keep polling `kubectl` on a fixed interval
  for as long as the app runs.
- **Skip redraw work when a poll returns an unchanged snapshot.** Give the
  domain struct an `Equal` method and compare the current vs. incoming
  slice with `slices.EqualFunc` before rebuilding any list items — this
  relies on the deterministic sort order `ListPods`/`ListDeployments`
  already produce, since the comparison is positional. Applied consistently
  across `Pods`, `Deployments`, and `DeploymentPods`: the first two compare
  against the raw incoming snapshot, but `DeploymentPods.SetAllPods` must
  compare against the *filtered* result (matching this dialog's own
  namespace + selector), not the raw `allPods` input — that's what's
  actually rendered, and comparing against the unfiltered snapshot would
  both miss real changes (a filtered-out pod changed) and flag false ones
  (an out-of-scope pod changed).
- **Give a background subprocess its own owner for cancellation.**
  Streaming dialogs hold the `context.CancelFunc` for their own subprocess
  and call it from their own `Close()`, mirroring the existing
  `MCPAuth.cancelAuth` pattern, so a dismissed dialog can't leak a running
  `kubectl` process.
- **Retarget a poll loop immediately rather than waiting out the
  interval.** `SetNamespace` mutates guarded watcher state and does a
  non-blocking send on a buffered `reconfigured` channel that `Run`'s
  `select` loop watches alongside its ticker, so a namespace switch is
  picked up on the next loop iteration instead of up to one poll interval
  later.
- **Bound long-lived in-memory views with a ring buffer, not an
  ever-growing slice.** `podLogsMaxLines = 2000`, enforced in `AppendLine`,
  keeps a log stream open for an entire session from accumulating unbounded
  memory.

## Checklist: adding a new resource type

The steps below are for a resource that fits the list+mutate,
watcher/pubsub shape Pods and Deployments both use. They don't apply to
logs or exec: both are subprocess streams (`kubectl logs -f`, `kubectl exec
-it`), not polled/diffed resources, so forcing them through a
watcher-per-resource-type checklist would be the wrong generalization —
see "Beyond list+mutate" above for how each of those was actually built.

1. `internal/k8s/<resource>.go` — domain struct + `Key()`.
2. `internal/k8s/<resource>_client.go` — hand-rolled kubectl JSON types,
   `List<X>`, `parseKubectlXList` (unit-testable), any mutating functions,
   `Format<X>List`.
3. `internal/k8s/<resource>_watcher.go` — concrete watcher wrapping
   `pubsub.Broker[[]X]` (see the generics decision above before reaching for
   `Watcher[T]`).
4. `internal/ui/dialog/<resource>.go` — dialog + list item implementing
   `Dialog` and `ListItem`, `Set<X>` with selection preservation, `HandleMsg`
   returning `Action` values only.
5. `internal/ui/dialog/actions.go` — one `Action<Verb><X>` struct per
   mutating interaction, carrying only identifying/parameter data.
6. `internal/agent/tools/k8s_<verb>_<resource>.go` (+ `.md`) — one tool per
   verb; permission-gate mutating tools, mirroring the equivalent Pod tool's
   shape.
7. `internal/agent/coordinator.go` — register the new tool(s) in
   `buildTools()`.
8. `internal/ui/model/keys.go` — key binding.
9. `internal/ui/model/ui.go` — five touch points: `lastKnown<X>` field,
   pubsub event case, `Action<Verb><X>` case(s), open-dialog keypress case,
   dialog-reopen case, `open<X>Dialog()` helper.
10. `internal/app/app.go` — watcher field, construction, `Run` goroutine,
    `subscribe` call.
11. Tests: `<resource>_client_test.go` (parse/format, no shell-out),
    `<resource>_watcher_test.go` (override the `list` field to avoid a real
    `kubectl`), `<resource>_test.go` in `ui/dialog` (seed order,
    next/previous wraparound, each mutating key → its Action, close → Action,
    no-items-is-noop, `Set<X>` selection preservation + fallback, empty-state
    draw, item render content — plus dedicated tests for any inline
    parameterized sub-state).

## Friction points worth knowing about

- **`gofmt` struct-field alignment**: hand-written struct literals/fields
  (e.g. a new `key.Binding` field next to existing ones) will usually need a
  `go fmt ./...` pass after adding them — Go's formatter re-aligns columns
  across all fields in the block, not just the new one.
- **Render width in tests**: `ListItem.Render(width)` truncates with an
  ellipsis at narrow widths. A test asserting on the tail of a rendered
  string (the last field in a multi-field info line) needs a wide enough
  `width` argument or it will flake against truncation rather than content.
