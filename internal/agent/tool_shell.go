package agent

import (
	"context"
	"errors"
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

// ParseShellArgs reads a run_shell_command call. A model that spells a
// boolean as a string or a number still gets what it meant.
func ParseShellArgs(raw string) (ShellArgs, error) {
	m, err := parseArguments(raw)
	if err != nil {
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

// ShellTool is the shell command tool, as the model sees it.
func ShellTool() llm.Tool {
	return llm.Tool{
		Type: "function",
		Function: llm.ToolFunction{
			Name:        "run_shell_command",
			Description: "Run one non-interactive shell command on the host this conversation is about. The user reviews it before it runs unless read-only actions may run by themselves, and sees its output.",
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

func shellTool() Tool {
	return Tool{
		Definition: ShellTool(),
		Prepare: func(raw string) (Prepared, error) {
			args, err := ParseShellArgs(raw)
			if err != nil {
				return Prepared{}, err
			}
			risk := Classify(args.Command)
			if args.Destructive {
				risk = RiskDestructive
			}
			return Prepared{
				Summary:     args.Command,
				Description: args.Description,
				Command:     args.Command,
				Risk:        risk,
				Auto:        AutoReadCommand,
				ModelSafe:   args.SafeToRun,
				Run: func(ctx context.Context, env Env) (ToolResult, error) {
					if env.Shell == nil {
						return ToolResult{Error: "This app cannot run commands."}, nil
					}
					res, err := env.Shell(ctx, args.Command)
					if err != nil && res.Error == "" {
						res.Error = err.Error()
					}
					res.Stdout, res.Stderr, res.Truncated = LimitOutput(res.Stdout, res.Stderr, MaxOutput)
					if res.Error != "" || res.ExitCode != 0 {
						res.Summary = "The command failed."
					} else {
						res.Summary = "The command finished."
					}
					return res, err
				},
				Format: FormatResult,
			}, nil
		},
	}
}
