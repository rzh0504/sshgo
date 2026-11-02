package agent

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"sshgo/internal/llm"
)

// ShellArgs is what run_shell_command was called with.
type ShellArgs struct {
	Command     string
	Description string
	SafeToRun   bool
	Destructive bool
}

// ParseShellArgs reads a tool call's arguments. A model that spells a
// boolean as a string or a number still gets what it meant.
func ParseShellArgs(raw string) (ShellArgs, error) {
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return ShellArgs{}, err
	}
	args := ShellArgs{
		Command:     stringArg(m, "command"),
		Description: stringArg(m, "description", "explanation"),
		SafeToRun:   boolArg(m, "safe_to_run", "safeToRun"),
		Destructive: boolArg(m, "destructive"),
	}
	if strings.TrimSpace(args.Command) == "" {
		return ShellArgs{}, errors.New("the call has no command")
	}
	return args, nil
}

func stringArg(m map[string]any, names ...string) string {
	for _, name := range names {
		if v, ok := m[name]; ok {
			if s, ok := v.(string); ok {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

func boolArg(m map[string]any, names ...string) bool {
	for _, name := range names {
		if v, ok := m[name]; ok {
			switch t := v.(type) {
			case bool:
				return t
			case string:
				b, _ := strconv.ParseBool(strings.ToLower(strings.TrimSpace(t)))
				return b
			case float64:
				return t != 0
			}
		}
	}
	return false
}

// ShellTool is the one tool this Agent has: a shell command on the host
// the conversation is about.
func ShellTool() llm.Tool {
	return llm.Tool{
		Type: "function",
		Function: llm.ToolFunction{
			Name:        "run_shell_command",
			Description: "Run one non-interactive shell command on the host this conversation is about. The user reviews it before it runs and sees its output.",
			Parameters: map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"required":             []string{"command", "description", "safe_to_run", "destructive"},
				"properties": map[string]any{
					"command": map[string]any{
						"type":        "string",
						"description": "A complete, non-interactive shell command.",
					},
					"description": map[string]any{
						"type":        "string",
						"description": "A concise explanation of the command and any relevant risk.",
					},
					"safe_to_run": map[string]any{
						"type":        "boolean",
						"description": "True only for clearly read-only, idempotent, non-destructive commands.",
					},
					"destructive": map[string]any{
						"type":        "boolean",
						"description": "True when the command could lose data or take a service down. Use it for what a list cannot see: a path that matters, a script whose name says nothing.",
					},
				},
			},
		},
	}
}

// Policy is when a command may run without the user's review.
type Policy struct {
	// AutoRunSafe lets a command the app itself judges read-only, and the
	// model marks safe, run by itself.
	AutoRunSafe bool
}

// Allows says whether the command may run without asking.
func (p Policy) Allows(args ShellArgs, risk Risk) bool {
	return p.AutoRunSafe && args.SafeToRun && risk == RiskReadOnly
}
