package k8s

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// logStreamTailLines bounds how much history `kubectl logs -f` replays when
// a stream starts, so opening the log view on a long-running pod doesn't
// dump its entire history before switching to live tailing. This is
// independent of any cap an agent tool applies to its own one-shot log
// reads — see StreamPodLogs's doc comment.
const logStreamTailLines = 500

// StreamPodLogs runs `kubectl logs -f` for the given pod and invokes onLine
// once per line of output as it arrives, blocking until ctx is cancelled or
// the underlying process exits (e.g. because the pod terminated). Unlike
// [ListPods] and the poll-based [Watcher], this is not snapshot-replace:
// `kubectl logs -f` itself keeps the connection open and pushes new lines
// as the pod writes them, which is the right shape for append-only log
// data — re-fetching and re-diffing the whole log on a poll interval would
// be both wasteful and lossy for anything written between polls.
//
// Callers that also want to display or retain the streamed lines are
// responsible for their own bounding (e.g. a ring buffer): the tail cap
// here only limits kubectl's initial replay, not how much a caller keeps.
func StreamPodLogs(ctx context.Context, namespace, name string, onLine func(string)) error {
	if namespace == "" || name == "" {
		return fmt.Errorf("namespace and name are required")
	}

	args := []string{"logs", "-f", name, "-n", namespace, fmt.Sprintf("--tail=%d", logStreamTailLines)}
	cmd := exec.CommandContext(ctx, "kubectl", args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("kubectl logs: %w", err)
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("kubectl logs failed to start: %w", err)
	}

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		onLine(scanner.Text())
	}

	waitErr := cmd.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if waitErr != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = waitErr.Error()
		}
		return fmt.Errorf("kubectl logs failed: %s", msg)
	}
	return nil
}
