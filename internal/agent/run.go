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
}

// Engine runs one conversation with the model.
type Engine struct {
	Client *llm.Client
	// Host is the one machine this conversation is about.
	Host   Host
	Policy Policy
	Hooks  Hooks
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
	system := SystemPrompt(e.Host)
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
		reply, err := e.Client.Chat(ctx, apiMessages(msgs[:len(msgs)-1], system), []llm.Tool{ShellTool()}, func(text string) {
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
			msgs, err = e.runCall(ctx, msgs, call)
			if err != nil {
				e.stopRemaining(msgs, assistant.ToolCalls[i+1:], err)
				return err
			}
		}
	}
	return ErrRoundLimit
}

func (e *Engine) runCall(ctx context.Context, msgs []*Message, call *ToolCall) ([]*Message, error) {
	if call.Name != "run_shell_command" {
		return e.finishCall(msgs, call, ToolResult{Error: fmt.Sprintf("Unknown tool %q.", call.Name)}), nil
	}
	args, err := ParseShellArgs(call.Arguments)
	if err != nil {
		return e.finishCall(msgs, call, ToolResult{Error: "The arguments are not valid JSON: " + err.Error()}), nil
	}
	host := e.Host
	if host.ID == "" {
		return e.finishCall(msgs, call, ToolResult{Error: "This conversation has no host. Ask the user to choose a connected host in the composer."}), nil
	}
	call.HostID = host.ID
	risk := Classify(args.Command)
	if args.Destructive {
		risk = RiskDestructive
	}
	proposal := Proposal{
		CallID:      call.ID,
		Host:        host,
		Command:     args.Command,
		Description: args.Description,
		Risk:        risk,
		Auto:        e.Policy.Allows(args, risk),
	}
	if !proposal.Auto {
		if e.Hooks.Review == nil {
			return e.finishCall(msgs, call, ToolResult{Error: "No one could approve this command."}), nil
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
			msgs = e.appendTool(msgs, call, "The user declined to run this command. Do not run it; ask what to do instead.")
			return msgs, nil
		}
	}
	call.Status = CallRunning
	e.emit(msgs)
	if e.Hooks.Exec == nil {
		return e.finishCall(msgs, call, ToolResult{Error: "This app cannot run commands."}), nil
	}
	res, err := e.Hooks.Exec(ctx, host.ID, args.Command)
	if err != nil && res.Error == "" {
		res.Error = err.Error()
	}
	res.HostID = host.ID
	res.Stdout, res.Stderr, res.Truncated = LimitOutput(res.Stdout, res.Stderr, MaxOutput)
	call.Result = &res
	if res.Error != "" || res.ExitCode != 0 {
		call.Status = CallError
	} else {
		call.Status = CallDone
	}
	e.emit(msgs)
	msgs = e.appendTool(msgs, call, FormatResult(host, res))
	if ctx.Err() != nil {
		return msgs, ctx.Err()
	}
	return msgs, nil
}

func (e *Engine) finishCall(msgs []*Message, call *ToolCall, res ToolResult) []*Message {
	call.Result = &res
	call.Status = CallError
	e.emit(msgs)
	return e.appendTool(msgs, call, FormatResult(Host{ID: res.HostID}, res))
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
