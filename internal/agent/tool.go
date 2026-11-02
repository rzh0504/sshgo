package agent

import (
	"context"
	"encoding/json"

	"sshgo/internal/llm"
)

// Tool is one action a model may propose. Definition is what the model
// sees; Prepare parses a call's arguments into what the app reviews and
// runs.
type Tool struct {
	Definition llm.Tool
	Prepare    func(arguments string) (Prepared, error)
}

// AutoClass is how a tool can run without asking. A read-only tool can
// only read; a read-only command is a shell command whose shape the local
// check recognises as read-only.
type AutoClass int

const (
	AutoNone        AutoClass = iota // always asks
	AutoReadTool                     // the tool itself only reads
	AutoReadCommand                  // the command shape is read-only
)

// Prepared is one call, parsed and judged.
type Prepared struct {
	Summary     string
	Description string
	Command     string // shell preview
	Path        string // file tools
	Content     string // write_file preview
	Risk        Risk
	Auto        AutoClass
	ModelSafe   bool // used by AutoReadCommand
	Run         func(context.Context, Env) (ToolResult, error)
	Format      func(Host, ToolResult) string
}

// Env is how a tool reaches the conversation's host.
type Env struct {
	Host      Host
	Shell     func(context.Context, string) (ToolResult, error)
	ReadFile  func(context.Context, string, int64) (FileData, error)
	WriteFile func(context.Context, string, []byte) error
}

// FileData is a file read, cut to the caller's limit.
type FileData struct {
	Data      []byte
	Size      int64
	Truncated bool
}

// ToolOptions chooses what a conversation offers.
type ToolOptions struct {
	Shell     bool
	ReadFile  bool
	WriteFile bool
}

// Tools builds the tools the options ask for. The result is never nil, so
// an empty one means no tools rather than the defaults.
func Tools(options ToolOptions) []Tool {
	tools := make([]Tool, 0, 3)
	if options.Shell {
		tools = append(tools, shellTool())
	}
	if options.ReadFile {
		tools = append(tools, readFileTool())
	}
	if options.WriteFile {
		tools = append(tools, writeFileTool())
	}
	return tools
}

// BuiltinTools are the tools every conversation offers by default.
func BuiltinTools() []Tool {
	return Tools(ToolOptions{Shell: true, ReadFile: true, WriteFile: true})
}

// Prepare reads a tool call's arguments, keeping the leniency the app has
// always had for a model that spells a boolean or a number loosely.
func parseArguments(raw string) (map[string]any, error) {
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil, err
	}
	return m, nil
}

func toolName(t Tool) string { return t.Definition.Function.Name }
