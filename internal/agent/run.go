package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"sshgo/internal/llm"
)

// ErrRoundLimit is what a model in a loop fails with: the conversation did
// not settle in time.
var ErrRoundLimit = errors.New("the tool loop did not settle")

// Hooks are what the engine tells the app, and asks it.
type Hooks struct {
	// Messages receives the conversation whenever it changes.
	Messages func([]*Message)
	// Delta receives the text of a streaming reply, complete so far.
	Delta func(messageID, content string)
	// Review shows a proposal and waits for the user's answer.
	Review func(context.Context, Proposal) (Decision, error)
	// Exec runs command on a connected host.
	Exec func(ctx context.Context, hostID, command string) (ToolResult, error)
	// ReadFile reads at most max bytes of path from a connected host.
	ReadFile func(ctx context.Context, hostID, path string, max int64) (FileData, error)
	// WriteFile replaces path on a connected host.
	WriteFile func(ctx context.Context, hostID, path string, data []byte) error
	// ListDir reads at most max entries of a directory on a connected host.
	ListDir func(ctx context.Context, hostID, path string, max int) (DirListing, error)
	// Terminal reads at most max characters of the host's open terminal.
	Terminal func(ctx context.Context, hostID string, max int) (string, error)
	// MemoryRead reads the Agent's cross-chat notes.
	MemoryRead func(ctx context.Context) (string, error)
	// MemoryWrite replaces or appends to the notes.
	MemoryWrite func(ctx context.Context, content string, appendTo bool) (int, error)
	// WebFetch reads a page or API from this device.
	WebFetch func(ctx context.Context, request WebRequest) (WebData, error)
	// Ask shows a form and waits for the user's answers.
	Ask func(ctx context.Context, request AskRequest) (AskResult, error)
}

// Engine runs one conversation with the model.
type Engine struct {
	Client *llm.Client
	// Host is the one machine this conversation is about.
	Host   Host
	Policy Policy
	Hooks  Hooks
	// Tools is what the model may call. Nil means BuiltinTools; an empty
	// slice means none.
	Tools []Tool
	// MaxRounds bounds how many tool rounds one prompt may take.
	MaxRounds int
}

// Run continues messages with the model, running the commands it
// proposes, until it answers without one. The messages are the engine's
// copy; Hooks.Messages publishes them.
func (e *Engine) Run(ctx context.Context, messages []*Message) error {
	if e.Client == nil {
		return llm.ErrNotConfigured
	}
	msgs := Sanitize(messages)
	e.emit(msgs)
	byName, definitions := e.toolSet()
	system := SystemPromptTools(e.Host, definitionNames(definitions))
	rounds := e.MaxRounds
	if rounds <= 0 {
		rounds = 6
	}
	for round := 0; round < rounds; round++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		assistant := &Message{
			ID:        NewID("msg"),
			Role:      "assistant",
			CreatedAt: time.Now().UnixMilli(),
		}
		msgs = append(msgs, assistant)
		e.emit(msgs)
		reply, err := e.Client.Chat(ctx, apiMessages(msgs[:len(msgs)-1], system), definitions, func(text string) {
			assistant.Content = text
			if e.Hooks.Delta != nil {
				e.Hooks.Delta(assistant.ID, text)
			}
		})
		if err != nil {
			if assistant.Content == "" {
				msgs = msgs[:len(msgs)-1]
			}
			e.emit(msgs)
			return err
		}
		assistant.Content = reply.Content
		for _, call := range reply.ToolCalls {
			assistant.ToolCalls = append(assistant.ToolCalls, &ToolCall{
				ID:        call.ID,
				Name:      call.Function.Name,
				Arguments: call.Function.Arguments,
				Status:    CallPending,
			})
		}
		e.emit(msgs)
		if len(assistant.ToolCalls) == 0 {
			return nil
		}
		for i, call := range assistant.ToolCalls {
			if err := ctx.Err(); err != nil {
				e.stopRemaining(msgs, assistant.ToolCalls[i:], err)
				return err
			}
			var err error
			msgs, err = e.runCall(ctx, msgs, call, byName)
			if err != nil {
				e.stopRemaining(msgs, assistant.ToolCalls[i+1:], err)
				return err
			}
		}
	}
	return ErrRoundLimit
}

// toolSet is what this conversation offers, by name and as the API takes it.
func (e *Engine) toolSet() (map[string]Tool, []llm.Tool) {
	tools := e.Tools
	if tools == nil {
		tools = BuiltinTools()
	}
	byName := make(map[string]Tool, len(tools))
	definitions := make([]llm.Tool, 0, len(tools))
	for _, tool := range tools {
		name := toolName(tool)
		if name == "" || tool.Prepare == nil {
			continue
		}
		byName[name] = tool
		definitions = append(definitions, tool.Definition)
	}
	return byName, definitions
}

func definitionNames(definitions []llm.Tool) []string {
	names := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		names = append(names, definition.Function.Name)
	}
	return names
}

func (e *Engine) runCall(ctx context.Context, msgs []*Message, call *ToolCall, tools map[string]Tool) ([]*Message, error) {
	tool, ok := tools[call.Name]
	if !ok {
		return e.finishCall(msgs, call, ToolResult{Error: fmt.Sprintf("Unknown tool %q.", call.Name)}, formatFailure), nil
	}
	prep, err := tool.Prepare(call.Arguments)
	if err != nil {
		return e.finishCall(msgs, call, ToolResult{Error: "The arguments are not valid: " + err.Error()}, formatFailure), nil
	}
	host := e.Host
	if host.ID == "" {
		return e.finishCall(msgs, call, ToolResult{Error: "This conversation has no host. Ask the user to choose a connected host in the composer."}, formatFailure), nil
	}
	call.HostID = host.ID
	proposal := Proposal{
		CallID:      call.ID,
		Host:        host,
		Tool:        call.Name,
		Summary:     prep.Summary,
		Description: prep.Description,
		Command:     prep.Command,
		Path:        prep.Path,
		Content:     prep.Content,
		Risk:        prep.Risk,
		Auto:        e.Policy.Allows(prep),
	}
	if !prep.Direct && !proposal.Auto {
		if e.Hooks.Review == nil {
			return e.finishCall(msgs, call, ToolResult{Error: "No one could approve this action."}, formatFailure), nil
		}
		decision, err := e.Hooks.Review(ctx, proposal)
		if err != nil {
			call.Status = CallDenied
			e.emit(msgs)
			msgs = e.appendTool(msgs, call, "The turn was stopped before this ran.")
			return msgs, err
		}
		if decision != DecisionRun {
			call.Status = CallDenied
			e.emit(msgs)
			msgs = e.appendTool(msgs, call, "The user declined to run this action. Do not run it; ask what to do instead.")
			return msgs, nil
		}
	}
	call.Status = CallRunning
	e.emit(msgs)
	if prep.Run == nil {
		return e.finishCall(msgs, call, ToolResult{Error: "This tool is not available."}, prep.Format), nil
	}
	res, err := prep.Run(ctx, e.envFor(host))
	if err != nil && res.Error == "" {
		res.Error = err.Error()
	}
	res.HostID = host.ID
	call.Result = &res
	if res.Error != "" || res.ExitCode != 0 {
		call.Status = CallError
	} else {
		call.Status = CallDone
	}
	e.emit(msgs)
	format := prep.Format
	if format == nil {
		format = formatFailure
	}
	msgs = e.appendTool(msgs, call, format(host, res))
	if ctx.Err() != nil {
		return msgs, ctx.Err()
	}
	return msgs, nil
}

func (e *Engine) finishCall(msgs []*Message, call *ToolCall, res ToolResult, format func(Host, ToolResult) string) []*Message {
	call.Result = &res
	call.Status = CallError
	e.emit(msgs)
	if format == nil {
		format = formatFailure
	}
	return e.appendTool(msgs, call, format(Host{ID: res.HostID}, res))
}

// envFor is how a tool reaches the host, and what it says when the app
// cannot do that at all.
func (e *Engine) envFor(host Host) Env {
	return Env{
		Host: host,
		Shell: func(ctx context.Context, command string) (ToolResult, error) {
			if e.Hooks.Exec == nil {
				return ToolResult{Error: "This app cannot run commands."}, nil
			}
			return e.Hooks.Exec(ctx, host.ID, command)
		},
		ReadFile: func(ctx context.Context, path string, max int64) (FileData, error) {
			if e.Hooks.ReadFile == nil {
				return FileData{}, errors.New("this app cannot read files")
			}
			return e.Hooks.ReadFile(ctx, host.ID, path, max)
		},
		WriteFile: func(ctx context.Context, path string, data []byte) error {
			if e.Hooks.WriteFile == nil {
				return errors.New("this app cannot write files")
			}
			return e.Hooks.WriteFile(ctx, host.ID, path, data)
		},
		ListDir: func(ctx context.Context, path string, max int) (DirListing, error) {
			if e.Hooks.ListDir == nil {
				return DirListing{}, errors.New("this app cannot list directories")
			}
			return e.Hooks.ListDir(ctx, host.ID, path, max)
		},
		Terminal: func(ctx context.Context, max int) (string, error) {
			if e.Hooks.Terminal == nil {
				return "", errors.New("this app cannot read the terminal")
			}
			return e.Hooks.Terminal(ctx, host.ID, max)
		},
		MemoryRead: func(ctx context.Context) (string, error) {
			if e.Hooks.MemoryRead == nil {
				return "", errors.New("this app has no memory")
			}
			return e.Hooks.MemoryRead(ctx)
		},
		MemoryWrite: func(ctx context.Context, content string, appendTo bool) (int, error) {
			if e.Hooks.MemoryWrite == nil {
				return 0, errors.New("this app has no memory")
			}
			return e.Hooks.MemoryWrite(ctx, content, appendTo)
		},
		WebFetch: func(ctx context.Context, request WebRequest) (WebData, error) {
			if e.Hooks.WebFetch == nil {
				return WebData{}, errors.New("this app cannot fetch web pages")
			}
			return e.Hooks.WebFetch(ctx, request)
		},
		Ask: func(ctx context.Context, request AskRequest) (AskResult, error) {
			if e.Hooks.Ask == nil {
				return AskResult{}, errors.New("this app cannot ask the user")
			}
			return e.Hooks.Ask(ctx, request)
		},
	}
}

// stopRemaining answers the calls a stopped turn never ran, so the
// conversation stays one the API accepts when it is resumed.
func (e *Engine) stopRemaining(msgs []*Message, calls []*ToolCall, cause error) []*Message {
	reason := "The turn was stopped before this ran."
	if cause != nil && !errors.Is(cause, context.Canceled) {
		reason = "The turn stopped: " + cause.Error()
	}
	for _, call := range calls {
		if call.Status != CallPending && call.Status != CallRunning {
			continue
		}
		call.Status = CallDenied
		msgs = e.appendTool(msgs, call, reason)
	}
	return msgs
}

func (e *Engine) appendTool(msgs []*Message, call *ToolCall, content string) []*Message {
	msgs = append(msgs, &Message{
		ID:         NewID("msg"),
		Role:       "tool",
		Content:    content,
		ToolCallID: call.ID,
		CreatedAt:  time.Now().UnixMilli(),
	})
	if e.Hooks.Messages != nil {
		e.Hooks.Messages(CloneMessages(msgs))
	}
	return msgs
}

func (e *Engine) emit(msgs []*Message) {
	if e.Hooks.Messages != nil {
		e.Hooks.Messages(CloneMessages(msgs))
	}
}

// apiMessages is a conversation in the shape the API takes.
func apiMessages(msgs []*Message, system string) []llm.Message {
	out := make([]llm.Message, 0, len(msgs)+1)
	out = append(out, llm.Message{Role: "system", Content: system})
	for _, m := range msgs {
		switch m.Role {
		case "user":
			text := m.Content
			var b strings.Builder
			b.WriteString(text)
			var images []llm.ContentPart
			for _, attachment := range m.Attachments {
				if attachment.Kind == AttachmentImage {
					if attachment.Data != "" {
						images = append(images, llm.ContentPart{
							Type:     "image_url",
							ImageURL: &llm.ImageURL{URL: attachment.Data},
						})
					}
					continue
				}
				if b.Len() > 0 {
					b.WriteString("\n\n")
				}
				fmt.Fprintf(&b, "<attachment name=%q bytes=%d>\n%s\n</attachment>",
					attachment.Name, attachment.Size, attachment.Content)
			}
			text = b.String()
			if len(images) == 0 {
				out = append(out, llm.Message{Role: "user", Content: text})
				break
			}
			parts := []llm.ContentPart{}
			if text != "" {
				parts = append(parts, llm.ContentPart{Type: "text", Text: text})
			}
			parts = append(parts, images...)
			out = append(out, llm.Message{Role: "user", Content: parts})
		case "assistant":
			msg := llm.Message{Role: "assistant", Content: m.Content}
			for _, call := range m.ToolCalls {
				msg.ToolCalls = append(msg.ToolCalls, llm.ToolCall{
					ID:       call.ID,
					Type:     "function",
					Function: llm.FunctionCall{Name: call.Name, Arguments: call.Arguments},
				})
			}
			out = append(out, msg)
		case "tool":
			out = append(out, llm.Message{Role: "tool", Content: m.Content, ToolCallID: m.ToolCallID})
		}
	}
	return out
}
