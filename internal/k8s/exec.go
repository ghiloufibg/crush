package k8s

import "os/exec"

// execShell is the shell requested inside the target container. sh is
// nearly universally present, including in minimal/busybox-based images
// that omit bash, so it's the safer default for a container we don't
// control the image of.
const execShell = "sh"

// ExecPodCommand builds (but does not run) the `kubectl exec -it` command
// for an interactive shell into a pod. Unlike every other function in this
// package, the caller is expected to hand the returned *exec.Cmd's
// stdin/stdout/stderr to the terminal directly (e.g. via tea.ExecProcess)
// rather than capture its output: this is a raw PTY passthrough, not a
// query, so there is no stdout to parse and no timeout to apply — the
// command runs for as long as the user's interactive session lasts.
func ExecPodCommand(namespace, name string) *exec.Cmd {
	return exec.Command("kubectl", "exec", "-it", name, "-n", namespace, "--", execShell)
}
