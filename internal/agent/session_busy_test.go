package agent

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// A prompt is accepted before its run registers as active. Anything that
// asks whether a session is safe to touch during that window must be told
// the session is busy, or it acts on a live run.
func TestSessionHasWork_CoversAcceptedButNotYetActiveRuns(t *testing.T) {
	t.Parallel()
	sa, _ := newCancelTestAgent(t)

	require.False(t, sa.SessionHasWork("sid"), "an untouched session is idle")

	accept := sa.BeginAccepted("sid")
	require.True(t, sa.SessionHasWork("sid"),
		"a session with an accepted run is busy before the run starts")

	accept.Close()
	require.False(t, sa.SessionHasWork("sid"), "closing the last accept frees it")
}

// Sibling accepts must not free the session until the last one closes.
func TestSessionHasWork_StaysBusyWhileAnyAcceptRemains(t *testing.T) {
	t.Parallel()
	sa, _ := newCancelTestAgent(t)

	first := sa.BeginAccepted("sid")
	second := sa.BeginAccepted("sid")

	first.Close()
	require.True(t, sa.SessionHasWork("sid"), "one accept still outstanding")

	second.Close()
	require.False(t, sa.SessionHasWork("sid"))
}

// A sub-agent runs in its own session against its own agent instance, so
// the main agent knows nothing about that session. The run holding it is
// the parent's, and callers that settle or repair an "idle" session would
// otherwise write into a sub-agent whose tools are still running.
func TestCoordinatorIsSessionBusy_ResolvesSubSessionThroughParent(t *testing.T) {
	t.Parallel()
	env := testEnv(t)
	main, _ := newCancelTestAgent(t)
	coord := &coordinator{sessions: env.sessions, mainAgent: main, mainAgentName: "coder"}

	parent, err := env.sessions.Create(t.Context(), "Parent")
	require.NoError(t, err)
	child, err := env.sessions.CreateTaskSession(t.Context(), "tool-1", parent.ID, "Child")
	require.NoError(t, err)

	require.False(t, coord.IsSessionBusy(child.ID), "nothing running yet")

	accept := main.BeginAccepted(parent.ID)
	t.Cleanup(accept.Close)

	require.True(t, coord.IsSessionBusy(parent.ID), "the parent is busy")
	require.True(t, coord.IsSessionBusy(child.ID),
		"a sub-session is busy while the run that owns it is")
}

// A top-level session has no parent to fall back on, and an unknown
// session must not report busy just because the lookup failed.
func TestCoordinatorIsSessionBusy_TopLevelAndUnknownSessions(t *testing.T) {
	t.Parallel()
	env := testEnv(t)
	main, _ := newCancelTestAgent(t)
	coord := &coordinator{sessions: env.sessions, mainAgent: main, mainAgentName: "coder"}

	solo, err := env.sessions.Create(t.Context(), "Solo")
	require.NoError(t, err)

	require.False(t, coord.IsSessionBusy(solo.ID))
	require.False(t, coord.IsSessionBusy("does-not-exist"))

	accept := main.BeginAccepted(solo.ID)
	t.Cleanup(accept.Close)
	require.True(t, coord.IsSessionBusy(solo.ID))
}

// Sibling sub-agents under different parents must not be confused: a run
// in one parent says nothing about another parent's children.
func TestCoordinatorIsSessionBusy_DoesNotLeakAcrossParents(t *testing.T) {
	t.Parallel()
	env := testEnv(t)
	main, _ := newCancelTestAgent(t)
	coord := &coordinator{sessions: env.sessions, mainAgent: main, mainAgentName: "coder"}

	busyParent, err := env.sessions.Create(t.Context(), "Busy")
	require.NoError(t, err)
	busyChild, err := env.sessions.CreateTaskSession(t.Context(), "tool-1", busyParent.ID, "Child")
	require.NoError(t, err)

	idleParent, err := env.sessions.Create(t.Context(), "Idle")
	require.NoError(t, err)
	idleChild, err := env.sessions.CreateTaskSession(t.Context(), "tool-2", idleParent.ID, "Child")
	require.NoError(t, err)

	accept := main.BeginAccepted(busyParent.ID)
	t.Cleanup(accept.Close)

	require.True(t, coord.IsSessionBusy(busyChild.ID))
	require.False(t, coord.IsSessionBusy(idleChild.ID),
		"another parent's run must not mark this sub-session busy")
}

// Run and Summarize ask IsSessionBusy to decide whether to queue behind an
// existing turn. A caller holds its own accept reservation while it asks,
// so counting accepts there would make a run queue behind itself.
func TestIsSessionBusy_IgnoresTheCallersOwnAccept(t *testing.T) {
	t.Parallel()
	sa, _ := newCancelTestAgent(t)

	accept := sa.BeginAccepted("sid")
	t.Cleanup(accept.Close)

	require.False(t, sa.IsSessionBusy("sid"),
		"an accept alone is not an active turn")
	require.True(t, sa.SessionHasWork("sid"),
		"but observers must still see work in flight")
}
