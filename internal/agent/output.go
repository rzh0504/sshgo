package agent

import (
	"fmt"
	"strings"
)

// MaxOutput is how much of a command's output reaches the model and the
// chat, in characters.
const MaxOutput = 32000

const truncationMarker = "\n\n[... output truncated ...]\n\n"

// LimitOutput keeps a command's output within max characters, at both
// ends: the start says what it was doing, the end how it went.
func LimitOutput(stdout, stderr string, max int) (string, string, bool) {
	if max <= 0 {
		max = MaxOutput
	}
	total := runeLen(stdout) + runeLen(stderr)
	if total <= max {
		return stdout, stderr, false
	}
	if stderr == "" {
		return limitText(stdout, max), "", true
	}
	stdoutBudget := max * 11 / 16
	return limitText(stdout, stdoutBudget), limitText(stderr, max-stdoutBudget), true
}

func limitText(s string, budget int) string {
	r := []rune(s)
	if len(r) <= budget {
		return s
	}
	if budget <= 0 {
		return ""
	}
	marker := []rune(truncationMarker)
	if budget <= len(marker) {
		return string(marker[:budget])
	}
	visible := budget - len(marker)
	head := visible / 2
	tail := visible - head
	return string(r[:head]) + truncationMarker + string(r[len(r)-tail:])
}

func runeLen(s string) int { return len([]rune(s)) }

// FormatResult is what the model reads of a command it ran.
func FormatResult(host Host, res ToolResult) string {
	var b strings.Builder
	writeHostHeader(&b, host)
	if res.Error != "" {
		fmt.Fprintf(&b, "Error: %s\n", res.Error)
	}
	fmt.Fprintf(&b, "Exit code: %d\n", res.ExitCode)
	if res.DurationMS > 0 {
		fmt.Fprintf(&b, "Duration: %.1fs\n", float64(res.DurationMS)/1000)
	}
	if res.Stdout != "" {
		b.WriteString("Stdout:\n")
		b.WriteString(res.Stdout)
		if !strings.HasSuffix(res.Stdout, "\n") {
			b.WriteString("\n")
		}
	}
	if res.Stderr != "" {
		b.WriteString("Stderr:\n")
		b.WriteString(res.Stderr)
		if !strings.HasSuffix(res.Stderr, "\n") {
			b.WriteString("\n")
		}
	}
	if res.Stdout == "" && res.Stderr == "" && res.Error == "" {
		b.WriteString("No output.\n")
	}
	if res.Truncated {
		b.WriteString("The output was truncated in the middle.\n")
	}
	return b.String()
}

// writeHostHeader names the host a result came from, when there is one.
func writeHostHeader(b *strings.Builder, host Host) {
	if host.Name != "" || host.Address != "" {
		fmt.Fprintf(b, "Host: %s (%s), id %s\n", host.Name, host.Address, host.ID)
	} else if host.ID != "" {
		fmt.Fprintf(b, "Host id: %s\n", host.ID)
	}
}

// formatFailure is what the model reads when a call never reached a tool.
func formatFailure(host Host, res ToolResult) string {
	var b strings.Builder
	writeHostHeader(&b, host)
	if res.Error != "" {
		fmt.Fprintf(&b, "Error: %s\n", res.Error)
	}
	return b.String()
}
