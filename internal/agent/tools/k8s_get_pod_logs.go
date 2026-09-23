package tools

import (
	"context"
	_ "embed"
	"strings"

	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/k8s"
)

const K8sGetPodLogsToolName = "k8s_get_pod_logs"

//go:embed k8s_get_pod_logs.md
var k8sGetPodLogsDescription string

const (
	// k8sGetPodLogsDefaultTailLines is used when the agent doesn't specify
	// tail_lines. k8sGetPodLogsMaxTailLines is a hard ceiling applied even
	// when the agent explicitly asks for more: pod logs can be huge, and
	// an unbounded fetch defeats the point of a bounded tool. This is a
	// separate concern from the TUI's own ring-buffer cap on the live log
	// view (see [k8s.StreamPodLogs]) -- two different caps for two
	// different consumers of the same underlying logs.
	k8sGetPodLogsDefaultTailLines = 200
	k8sGetPodLogsMaxTailLines     = 500
)

type K8sGetPodLogsParams struct {
	Namespace string `json:"namespace" description:"Namespace the pod is in"`
	Name      string `json:"name" description:"Pod name"`
	TailLines int    `json:"tail_lines,omitempty" description:"Number of most recent log lines to fetch (default 200, capped at 500)"`
	Grep      string `json:"grep,omitempty" description:"Only return lines containing this substring (case-sensitive)"`
}

type K8sGetPodLogsResponseMetadata struct {
	NumberOfLines int `json:"number_of_lines"`
}

func NewK8sGetPodLogsTool() fantasy.AgentTool {
	return fantasy.NewAgentTool(
		K8sGetPodLogsToolName,
		k8sGetPodLogsDescription,
		func(ctx context.Context, params K8sGetPodLogsParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			tailLines := params.TailLines
			switch {
			case tailLines <= 0:
				tailLines = k8sGetPodLogsDefaultTailLines
			case tailLines > k8sGetPodLogsMaxTailLines:
				tailLines = k8sGetPodLogsMaxTailLines
			}

			logs, err := k8s.GetPodLogs(ctx, params.Namespace, params.Name, tailLines)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}

			var lines []string
			if trimmed := strings.TrimRight(logs, "\n"); trimmed != "" {
				lines = strings.Split(trimmed, "\n")
			}

			if params.Grep != "" {
				filtered := lines[:0]
				for _, line := range lines {
					if strings.Contains(line, params.Grep) {
						filtered = append(filtered, line)
					}
				}
				lines = filtered
			}

			result := strings.Join(lines, "\n")
			if result == "" {
				result = "No matching log lines"
			}

			return fantasy.WithResponseMetadata(
				fantasy.NewTextResponse(result),
				K8sGetPodLogsResponseMetadata{NumberOfLines: len(lines)},
			), nil
		},
	)
}
