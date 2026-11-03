package agent

import (
	"context"
	"encoding/json"
	"time"

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
	Direct      bool // asks the user itself, so it skips the review card
	Run         func(context.Context, Env) (ToolResult, error)
	Format      func(Host, ToolResult) string
}

// Env is how a tool reaches the conversation's host.
type Env struct {
	Host        Host
	Shell       func(context.Context, string) (ToolResult, error)
	ReadFile    func(context.Context, string, int64) (FileData, error)
	WriteFile   func(context.Context, string, []byte) error
	ListDir     func(context.Context, string, int) (DirListing, error)
	Terminal    func(context.Context, int) (string, error)
	MemoryRead  func(context.Context) (string, error)
	MemoryWrite func(context.Context, string, bool) (int, error)
	WebFetch    func(context.Context, WebRequest) (WebData, error)
	Ask         func(context.Context, AskRequest) (AskResult, error)
}

// AskOption is one choice in an ask_user question.
type AskOption struct {
	Label       string
	Description string
	Value       string
}

// AskQuestion is one question ask_user puts to the user.
type AskQuestion struct {
	ID          string
	Question    string
	Detail      string
	Options     []AskOption
	Multiple    bool
	Secret      bool
	Placeholder string
}

// AskRequest is what ask_user asks.
type AskRequest struct {
	Questions []AskQuestion
}

// AskAnswer is the user's answer to one question. Text is a secret handle
// when the question was secret.
type AskAnswer struct {
	ID     string
	Values []string
	Text   string
}

// AskResult is what the user answered; Cancelled when they closed the form.
type AskResult struct {
	Answers   []AskAnswer
	Cancelled bool
}

// WebRequest is one page or API call.
type WebRequest struct {
	URL     string
	Headers map[string]string
}

// WebData is what a page or API returned, cut to the caller's limit.
type WebData struct {
	Status      int
	ContentType string
	Body        string
	Size        int64
	Truncated   bool
}

// DirEntry is one entry of a directory listing.
type DirEntry struct {
	Name     string
	IsDir    bool
	Size     int64
	Mode     string
	Modified time.Time
}

// DirListing is a directory read, cut to the caller's limit.
type DirListing struct {
	Entries   []DirEntry
	Truncated bool
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
	ListDir   bool
	Terminal  bool
	Memory    bool
	WebFetch  bool
	AskUser   bool
}

// Tools builds the tools the options ask for. The result is never nil, so
// an empty one means no tools rather than the defaults.
func Tools(options ToolOptions) []Tool {
	tools := make([]Tool, 0, 8)
	if options.Shell {
		tools = append(tools, shellTool())
	}
	if options.ReadFile {
		tools = append(tools, readFileTool())
	}
	if options.WriteFile {
		tools = append(tools, writeFileTool())
	}
	if options.ListDir {
		tools = append(tools, listDirTool())
	}
	if options.Terminal {
		tools = append(tools, terminalScreenTool())
	}
	if options.Memory {
		tools = append(tools, memoryTool())
	}
	if options.WebFetch {
		tools = append(tools, webFetchTool())
	}
	if options.AskUser {
		tools = append(tools, askUserTool())
	}
	return tools
}

// BuiltinTools are the tools every conversation offers by default.
func BuiltinTools() []Tool {
	return Tools(ToolOptions{Shell: true, ReadFile: true, WriteFile: true, ListDir: true, Terminal: true, Memory: true, WebFetch: true, AskUser: true})
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
