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
	ID        string     `json:"id"`
	Title     string     `json:"title"`
	CreatedAt int64      `json:"createdAt"`
	UpdatedAt int64      `json:"updatedAt"`
	Messages  []*Message `json:"messages"`
}

// Message is one turn of a conversation.
type Message struct {
	ID         string      `json:"id"`
	Role       string      `json:"role"` // user, assistant or tool
	Content    string      `json:"content"`
	ToolCalls  []*ToolCall `json:"toolCalls,omitempty"`
	ToolCallID string      `json:"toolCallId,omitempty"`
	CreatedAt  int64       `json:"createdAt"`
}

// ToolCall is a call a model made, and what became of it.
type ToolCall struct {
	ID        string      `json:"id"`
	Name      string      `json:"name"`
	Arguments string      `json:"arguments"`
	Status    CallStatus  `json:"status,omitempty"`
	Result    *ToolResult `json:"result,omitempty"`
}

// ToolResult is what running a command left.
type ToolResult struct {
	HostID     string `json:"hostId,omitempty"`
	Stdout     string `json:"stdout,omitempty"`
	Stderr     string `json:"stderr,omitempty"`
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

// Proposal is a command the model wants to run, as the user reviews it.
type Proposal struct {
	CallID      string
	Host        Host
	Command     string
	Description string
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
