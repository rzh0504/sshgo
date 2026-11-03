// Package agent is the conversation an SSH operations assistant has with
// a model: the tools it may call, the commands it proposes, and how those
// are judged before they run.
package agent

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

// CallStatus is where a tool call stands.
type CallStatus string

const (
	CallPending CallStatus = "pending" // waiting for the user's approval
	CallRunning CallStatus = "running"
	CallDone    CallStatus = "done"
	CallDenied  CallStatus = "denied"
	CallError   CallStatus = "error"
)

// Chat is one conversation, as the app keeps it.
type Chat struct {
	ID         string     `json:"id"`
	Title      string     `json:"title"`
	HostID     string     `json:"hostId,omitempty"`
	ProviderID string     `json:"providerId,omitempty"`
	Model      string     `json:"model,omitempty"`
	Reasoning  string     `json:"reasoning,omitempty"`
	CreatedAt  int64      `json:"createdAt"`
	UpdatedAt  int64      `json:"updatedAt"`
	Messages   []*Message `json:"messages"`
	// TitleDone tells that a model named the conversation, or that it
	// came with its title, so it is not named again.
	TitleDone bool `json:"-"`
}

// Kinds of attachment a message may carry.
const (
	AttachmentText  = "text"
	AttachmentImage = "image"
)

// Attachment is a file the user gave the conversation as context: text
// inline, or an image kept in the app's attachment store.
type Attachment struct {
	ID      string `json:"id,omitempty"`
	Kind    string `json:"kind,omitempty"`
	Name    string `json:"name"`
	MIME    string `json:"mime,omitempty"`
	Size    int64  `json:"size,omitempty"`
	Content string `json:"content,omitempty"`
	// Data and Path are the image's data URL and file for the request and
	// the preview; they are read from the store, never saved with the chat.
	Data string `json:"-"`
	Path string `json:"-"`
}

// Message is one turn of a conversation.
type Message struct {
	ID          string       `json:"id"`
	Role        string       `json:"role"` // user, assistant or tool
	Content     string       `json:"content"`
	Attachments []Attachment `json:"attachments,omitempty"`
	ToolCalls   []*ToolCall  `json:"toolCalls,omitempty"`
	ToolCallID  string       `json:"toolCallId,omitempty"`
	CreatedAt   int64        `json:"createdAt"`
}

// ToolCall is a call a model made, and what became of it.
type ToolCall struct {
	ID        string      `json:"id"`
	Name      string      `json:"name"`
	Arguments string      `json:"arguments"`
	HostID    string      `json:"hostId,omitempty"`
	Status    CallStatus  `json:"status,omitempty"`
	Result    *ToolResult `json:"result,omitempty"`
}

// ToolResult is what running a tool left. The shell fields are empty for
// the file tools; Path, Content and Bytes are empty for a command.
type ToolResult struct {
	HostID     string `json:"hostId,omitempty"`
	Summary    string `json:"summary,omitempty"`
	Stdout     string `json:"stdout,omitempty"`
	Stderr     string `json:"stderr,omitempty"`
	Path       string `json:"path,omitempty"`
	URL        string `json:"url,omitempty"`
	Content    string `json:"content,omitempty"`
	Bytes      int64  `json:"bytes,omitempty"`
	ExitCode   int    `json:"exitCode"`
	DurationMS int64  `json:"durationMs,omitempty"`
	Truncated  bool   `json:"truncated,omitempty"`
	Error      string `json:"error,omitempty"`
}

// Host is one connected machine the Agent may work on.
type Host struct {
	ID      string
	Name    string
	Address string
	OS      string
}

// Proposal is a tool call the model wants to make, as the user reviews it.
type Proposal struct {
	CallID      string
	Host        Host
	Tool        string
	Summary     string
	Description string
	Command     string // shell
	Path        string // file tools
	Content     string // write_file
	Risk        Risk
	Auto        bool // the policy lets it run without asking
}

// Decision is what the user answered a proposal with.
type Decision int

const (
	DecisionRun Decision = iota
	DecisionDeny
)

// NewID makes an identifier for a message or a chat.
func NewID(prefix string) string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return prefix + "-" + time.Now().Format("150405.000000000")
	}
	return prefix + "-" + hex.EncodeToString(b[:])
}

// CloneMessages copies a conversation, so that a view can read it while
// the engine writes it.
func CloneMessages(msgs []*Message) []*Message {
	out := make([]*Message, len(msgs))
	for i, m := range msgs {
		out[i] = CloneMessage(m)
	}
	return out
}

// CloneChat copies a conversation.
func CloneChat(c *Chat) *Chat {
	if c == nil {
		return nil
	}
	cp := *c
	cp.Messages = CloneMessages(c.Messages)
	return &cp
}

// CloneMessage copies one message.
func CloneMessage(m *Message) *Message {
	if m == nil {
		return nil
	}
	cp := *m
	if m.Attachments != nil {
		cp.Attachments = append([]Attachment(nil), m.Attachments...)
	}
	if m.ToolCalls != nil {
		cp.ToolCalls = make([]*ToolCall, len(m.ToolCalls))
		for i, c := range m.ToolCalls {
			call := *c
			if c.Result != nil {
				res := *c.Result
				call.Result = &res
			}
			cp.ToolCalls[i] = &call
		}
	}
	return &cp
}
