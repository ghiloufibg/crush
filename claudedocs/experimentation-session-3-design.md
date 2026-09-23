# Session 3 design: advanced k9s-equivalent features + guide hardening

Design only — nothing in this document has been implemented. It proposes
what the next live-experiment session should build, why each experiment
answers a question the first two sessions (Pods, Deployments) didn't, and
how the findings feed back into `claudedocs/tui-ai-agent-guide.md`.

## Decisions (confirmed)

The three open questions this design originally raised are now resolved:

1. **Scope: all three recommended experiments, not a subset.** Drill-down
   navigation, log streaming, and the namespace switcher are all in scope
   for this session. (Multi-select/bulk-delete and metrics/top remain
   explicitly deferred — they weren't part of what was being confirmed
   here. Flagging this interpretation explicitly: if "build everything"
   was meant to include those two as well, say so and this doc will be
   updated again before implementation starts.)
2. **Poll/subprocess over `client-go`, for both snapshot and streaming
   data.** See "Decision: staying subprocess-based" below — this also
   resolves how log streaming and exec should be implemented, not just how
   Pods/Deployments already are.
3. **Exec-into-a-pod is in scope for this session, not deferred to a
   session 4.** It's now a fourth build target alongside the original
   three, sequenced last since it's the most novel one and benefits from
   the log-streaming subprocess-passthrough groundwork done just before it.

The rest of this document is updated to reflect these four in-scope
experiments: drill-down navigation, log streaming, namespace switcher, and
exec-into-a-pod.

## Why a third session, and what it's for

Sessions 1–2 (Pods, Deployments) validated one interaction shape:
**list → select → single-parameter mutation**, fed by a poll-and-snapshot
watcher, mutated through an agent tool call. That shape is now well
understood and documented. It does not, however, cover several things a
real k9s-equivalent needs, and it hasn't yet had to think hard about either
token cost or TUI performance, because pod/deployment lists in a spike
cluster are small and few.

This session's purpose is narrower than "add more resource types": it's to
deliberately hit the architectural edges the first two panels didn't touch,
and to turn what's learned into explicit, checkable guidance — not just
"add a Services panel," which would just be a fourth copy of the same
shape.

## Current state (baseline, as of this design)

- Two resource types (Pods, Deployments), each with: domain struct, hand-
  rolled kubectl JSON client, a poll-based `Watcher`/`DeploymentWatcher`
  (fixed `DefaultPollInterval = 3s`, full-snapshot replace, no diffing), a
  list dialog, and 2–3 agent tools per resource.
- Every watcher runs for the entire `App` lifetime once constructed in
  `New()`, regardless of whether its dialog is ever opened.
- Agent read tools (`k8s_get_pods`, `k8s_get_deployments`) return the
  **entire** current list as one text blob on every call — no limit, no
  filter beyond namespace, no dedup against a prior identical call.
- No cross-resource navigation exists: each dialog is a flat, independent
  list. There's no way to go "from this Deployment, show me its Pods."
- No streaming/long-running data source exists. Every current data flow is
  bounded and snapshot-shaped.
- No raw-terminal passthrough exists (no exec, no interactive shell).

## Candidate features, and what each would newly test

| Feature | New architectural question it forces | k9s equivalent |
|---|---|---|
| **Drill-down navigation** (select a Deployment → view only its Pods) | Panel-to-panel navigation with a filter/context carried between them; a "back" path | `d` / enter on a resource |
| **Log streaming** (tail a pod's logs, live) | Append-only/streaming data instead of snapshot-replace; unbounded-output token risk for the agent tool | `l` (logs) |
| **Namespace switcher** (change the active namespace without restarting) | Runtime reconfiguration of a *running* watcher, not just construction-time options | `:ns` |
| **Exec into a pod** (interactive shell) | Raw PTY passthrough, orthogonal to bubbletea's render loop — likely means suspending the TUI, not rendering a terminal-in-a-terminal | `s` (shell) |
| Multi-select + bulk delete | Selection-set state, and whether the agent gets one batched tool call or N sequential ones | multi-select then `d` |
| Metrics/top (CPU/mem) | Numeric time-series rendering (sparklines); depends on metrics-server being installed, which isn't guaranteed | `:pulses` / header metrics |

### Build order: all four, in dependency order

All four confirmed experiments, in the order they should be built (each
one after the first reuses something the previous one established):

1. **Drill-down navigation** — lowest risk, directly extends the existing
   dialog architecture (no new data-flow primitive), and is a real gap:
   right now there's no way to answer "which pods belong to this
   deployment" from the TUI at all. Build first because nothing else
   depends on it, and it's the cheapest way to confirm the panel-to-panel
   navigation primitive before other features assume it exists.
2. **Log streaming** — highest value as a k9s-equivalent feature, and the
   one most likely to break the "poll a full snapshot" assumption baked
   into every part of the architecture so far. Also the clearest place to
   make token-budget decisions concrete, since raw log output can be
   arbitrarily large. Build second: it's the first streaming/subprocess-
   passthrough primitive, and exec (built fourth) is a strict superset of
   the problems it solves (bidirectional instead of one-directional), so
   getting one-directional streaming right first de-risks exec.
3. **Namespace switcher** — smaller in scope than it sounds, but it's a
   genuine functional gap (today, namespace scope is fixed for the process
   lifetime) and it's the only one of the four that tests *reconfiguring*
   a running watcher rather than building a new one. Independent of 1 and
   2, so it can be built third without waiting on anything from them.
4. **Exec into a pod** — the most architecturally different feature (raw
   PTY, likely requires suspending bubbletea entirely and handing the
   terminal to `kubectl exec -it` directly, similar to how `$EDITOR`
   integration usually works). Built last and deliberately right after log
   streaming: both are long-lived `kubectl` subprocess streams wired into
   the TUI, and exec adds bidirectional I/O and raw-mode terminal handoff
   on top of what log streaming will have already proven out for
   one-directional output.

Still deferred, unchanged from the original recommendation (not part of
what was confirmed this round):

- **Multi-select + bulk delete** — an incremental variation on the
  existing delete pattern (a set instead of a single item), not a new
  architectural question. Cheap to add later, and more useful once drill-
  down navigation exists to scope a list first.
- **Metrics/top** — blocked on an external dependency (metrics-server) not
  guaranteed present in every cluster. Worth a footnote in the guide
  (numeric/sparkline rendering is a real future case) without spending a
  session on it now.

## Token usage optimization — design

Problem: every agent read-tool call currently returns the full formatted
table, unfiltered and uncapped. That's fine at spike scale (a handful of
pods) and gets expensive fast at real-cluster scale, and it compounds
whenever the agent re-queries after an action to confirm the result.

Proposed changes (to validate during the session, not assumed correct
until tried):

1. **Server-side filtering params** on read tools — `label_selector`,
   `name_contains` — so the agent narrows scope in the `kubectl` call
   itself instead of receiving everything and reasoning over it in-context.
   This is strictly better than client-side filtering: it reduces token
   cost *and* reduces kubectl output size.
2. **Result cap with an explicit truncation notice** — e.g. default
   `max_results` (50), with a trailing line like `"...23 more not shown,
   narrow with label_selector or name_contains"` rather than silently
   truncating. An agent that hits the cap should be able to tell it hit a
   cap, not conclude the cluster only has 50 pods.
3. **Tool-call-embedded state over re-discovery** — already true for
   delete/scale today (the dialog's synthesized chat message carries
   namespace+name directly, so the agent doesn't need a preceding
   `k8s_get_pods` call just to learn what to act on). State this explicitly
   as a principle for every new tool: if the TUI already knows the target,
   put it in the message, don't make the agent re-query for it.
4. **Logs-specific bounding** — this is the one truly new risk. A logs tool
   must never default to unbounded output. Concretely: default
   `tail_lines` (e.g. 200), an optional `grep`/substring filter applied
   server-side (via `kubectl logs --tail` plus a grep pass, not by dumping
   everything and letting the agent search in-context), and a hard
   ceiling even when the agent asks for more.
5. **Cheap unchanged-result signal** *(exploratory, may not pan out)* — if
   the agent calls the same read tool with identical params twice within a
   short window and the underlying watcher hasn't published a new snapshot
   since, consider returning a short "unchanged since last call" response
   instead of the full table again. This needs a cache keyed on
   (tool, params, watcher-revision) and is only worth building if the
   session's experiments show repeated re-querying actually happens in
   practice — don't build it speculatively.

## TUI performance / data-freshness optimization — design

Problem: every watcher polls on a fixed 3s interval for the entire process
lifetime, whether or not its panel is open, and repaints happen on every
tick regardless of whether the snapshot changed.

Proposed changes to validate:

1. **Watcher lifecycle tied to dialog visibility** — start the poll
   goroutine when a dialog opens, stop it when it closes, instead of
   running continuously from `App.New()`. Trades a small first-open latency
   (one poll cycle) for not spawning a `kubectl` process every 3s for
   panels nobody is looking at. This matters more as more resource types
   are added — today it's 2 watchers running forever; it shouldn't become
   6+.
2. **Skip redraw on unchanged snapshot** — verify whether
   `list.FilterableList`/`Draw` already no-ops on an identical snapshot;
   if not, add an equality check before `SetItems` so polling doesn't cause
   a visible repaint (or wasted render work) every 3 seconds when nothing
   changed.
3. **Poll-vs-watch: resolved, staying subprocess-based.** See the
   dedicated decision writeup immediately below — this now covers both the
   existing snapshot resources and the two new streaming features.
4. **Streaming view memory bound** — a log view must cap retained lines
   (e.g. a ring buffer at ~2000 lines) rather than growing unbounded for a
   long-lived tail, independent of the token-budget cap on the *agent
   tool's* one-shot log read (these are two different caps for two
   different consumers of the same underlying stream).

## Decision: staying subprocess-based (no `client-go`)

This was an open question in the original design; it's now resolved for
both the existing snapshot resources and the two new streaming features
(logs, exec).

**Decision: keep shelling out to `kubectl` everywhere — polling for
snapshot resources, long-lived subprocesses (`kubectl logs -f`, `kubectl
exec -it`) for streaming ones. Do not introduce `client-go`.**

Reasoning:

- **Auth/config is the real cost of `client-go`, not the API calls
  themselves.** `kubectl` already resolves kubeconfig, contexts, and every
  installed auth plugin (OIDC, exec-credential plugins, cloud-provider
  IAM, etc.) correctly, because that's its whole job. Adopting `client-go`
  directly means either reimplementing that resolution (a real amount of
  work and an ongoing maintenance surface, since auth plugins evolve) or
  pulling in `client-go`'s own kubeconfig-loading packages, which is most
  of the dependency weight this codebase has been avoiding, for none of
  its watch/informer benefit unless that's also adopted. There's no
  middle ground that gets *only* the small piece needed.
- **Exec is naturally a subprocess problem, not a natural fit for
  `client-go`'s API either.** Real interactive exec against a cluster goes
  over SPDY/WebSocket via `client-go`'s `remotecommand` executor, which is
  itself a nontrivial API to wire correctly (stdin/stdout/stderr streams,
  terminal resize events, TTY negotiation). `kubectl exec -it` already
  does all of that correctly and is a single subprocess call with the
  terminal handed to it directly — which is *also* the simplest way to get
  a real PTY passthrough in a bubbletea app (suspend the TUI, hand the
  terminal over, resume on exit), so the "avoid a new dependency" choice
  and the "simplest implementation" choice happen to be the same choice
  here, not a trade-off.
- **The stated purpose of this codebase is a lightweight validation spike
  for the TUI-plus-agent pattern, not a production k9s replacement.**
  `client-go`'s real payoff — event-driven watch, informer caching, lower
  steady-state latency — matters far more at production scale (hundreds of
  resources, many watchers, latency-sensitive dashboards) than it does
  here, where resource counts are small and a 3s poll is already
  imperceptible. Paying the dependency and complexity cost now buys
  headroom this spike isn't trying to use.
- **Consistency**: every existing List/Delete/Scale call already shells out
  to `kubectl`. Introducing `client-go` for only the two new streaming
  features would mean the codebase authenticates to the cluster two
  different ways depending on which resource type or verb is involved —
  a confusing split with no corresponding benefit, since the two new
  features don't need anything `client-go` offers that `kubectl` doesn't
  already provide via a subprocess.

This is a threshold decision, same as the Watcher-generics one: revisit if
a future feature genuinely needs true push-based events (not just
lower-latency polling) that only an informer/watch stream can provide
efficiently — e.g. a live "resource changed" indicator across dozens of
resource types simultaneously, where dozens of polling loops would become
the actual bottleneck. Nothing in this session's four features reaches
that threshold.

## Guide update plan

After the session, extend `claudedocs/tui-ai-agent-guide.md` with:

- A new section, **"Beyond list+mutate: three more interaction shapes"**,
  covering drill-down/contextual navigation, streaming vs snapshot data,
  and runtime-reconfigurable watchers — each with the concrete decision
  made and why, in the same style as the existing Watcher-generics and
  inline-scale write-ups.
- A **"Token budget checklist for agent tools"** section formalizing
  whichever of the five optimizations above prove out, so the next new
  tool has a checklist instead of needing to re-derive them.
- A **"TUI freshness/perf checklist"** section formalizing the watcher-
  lifecycle and redraw-skipping decisions.
- An update to the existing **"Checklist: adding a new resource type"**
  noting when the standard watcher-per-resource-type pattern does *not*
  apply (logs and exec are not watcher/pubsub resources at all — they're
  subprocess streams — and forcing them into the existing pattern would be
  the wrong generalization).

## Proposed sequencing

1. Drill-down navigation (Deployment → its Pods), reusing the existing Pods
   panel with a label/owner-reference filter and a minimal back/breadcrumb
   action. No dependencies — build first.
2. Log streaming: new streaming primitive (not a `Watcher`), a log-view
   dialog, and a bounded `k8s_get_pod_logs` agent tool, implemented as a
   `kubectl logs -f` subprocess per the decision above. Sequenced second so
   its subprocess-passthrough plumbing (spawn, stream lines into the TUI,
   tear down cleanly) de-risks exec before exec's bidirectional/raw-mode
   version of the same problem is attempted.
3. Namespace switcher: runtime reconfiguration of a running watcher's
   namespace/all-namespaces setting, triggered from a small command
   dialog. Independent of 1 and 2 — could swap with either, but sequenced
   third since it's the most self-contained of the four.
4. Exec into a pod: suspend the TUI, hand the terminal to
   `kubectl exec -it`, resume on exit — the same subprocess-handoff shape
   as log streaming, extended to bidirectional I/O and raw-mode terminal
   passthrough. Built last and only after log streaming lands, since it's
   the most novel of the four and benefits most from that groundwork.
5. Only if time remains: retroactively apply the token/perf optimizations
   above to the existing Pods/Deployments tools and watchers, as validation
   that the checklist items are real improvements rather than theoretical
   ones.
6. Guide rewrite (the four sections above).

## Open questions

None remaining — see "Decisions (confirmed)" at the top of this document.
All three questions originally listed here (scope, `client-go` vs.
subprocess, and exec scheduling) are resolved there and reflected
throughout the rest of this document.
