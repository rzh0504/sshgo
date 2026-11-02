// Package llm talks to an OpenAI-compatible chat endpoint: streaming
// replies, and the tool calls a model makes while it writes them.
package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// ErrNotConfigured is what a client without an endpoint or a model fails
// with.
var ErrNotConfigured = errors.New("llm: not configured")

// Client is one OpenAI-compatible endpoint and the account on it.
type Client struct {
	BaseURL string
	APIKey  string
	Model   string
	HTTP    *http.Client
}

// Message is one message of a conversation, in the shape the API takes.
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// ToolCall is a call a model made: its name, and the arguments as JSON.
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

// FunctionCall is the function of a tool call.
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// Tool is a function a model may call.
type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

// ToolFunction describes a tool.
type ToolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// Reply is what one completion said.
type Reply struct {
	Content      string
	ToolCalls    []ToolCall
	FinishReason string
}

// Chat streams a completion. onDelta, when set, receives the reply's text
// so far as it comes.
func (c *Client) Chat(ctx context.Context, messages []Message, tools []Tool, onDelta func(string)) (Reply, error) {
	if c == nil {
		return Reply{}, ErrNotConfigured
	}
	model := strings.TrimSpace(c.Model)
	if model == "" {
		return Reply{}, ErrNotConfigured
	}
	endpoint, err := endpointOf(c.BaseURL)
	if err != nil {
		return Reply{}, err
	}
	body, err := json.Marshal(chatRequest{
		Model:    model,
		Messages: messages,
		Tools:    tools,
		Stream:   true,
	})
	if err != nil {
		return Reply{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return Reply{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if key := strings.TrimSpace(c.APIKey); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	res, err := httpClient.Do(req)
	if err != nil {
		return Reply{}, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		detail := strings.TrimSpace(readError(res.Body))
		if detail == "" {
			detail = res.Status
		}
		return Reply{}, fmt.Errorf("llm: %s", detail)
	}

	var (
		content strings.Builder
		calls   = map[int]*ToolCall{}
		order   []int
		finish  string
	)
	scan := bufio.NewScanner(res.Body)
	scan.Buffer(make([]byte, 0, 64<<10), 4<<20)
	for scan.Scan() {
		line := strings.TrimSpace(scan.Text())
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		data, ok := strings.CutPrefix(line, "data:")
		if !ok {
			continue
		}
		data = strings.TrimSpace(data)
		if data == "[DONE]" {
			break
		}
		var chunk streamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		if chunk.Error != nil && chunk.Error.Message != "" {
			return Reply{}, fmt.Errorf("llm: %s", chunk.Error.Message)
		}
		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				content.WriteString(choice.Delta.Content)
				if onDelta != nil {
					onDelta(content.String())
				}
			}
			for _, part := range choice.Delta.ToolCalls {
				call := calls[part.Index]
				if call == nil {
					call = &ToolCall{Type: "function"}
					calls[part.Index] = call
					order = append(order, part.Index)
				}
				if part.ID != "" {
					call.ID = part.ID
				}
				if part.Function.Name != "" {
					call.Function.Name = part.Function.Name
				}
				call.Function.Arguments += part.Function.Arguments
			}
			if choice.FinishReason != "" {
				finish = choice.FinishReason
			}
		}
	}
	if err := scan.Err(); err != nil {
		if ctx.Err() != nil {
			return Reply{}, ctx.Err()
		}
		return Reply{}, err
	}
	if ctx.Err() != nil {
		return Reply{}, ctx.Err()
	}
	reply := Reply{Content: content.String(), FinishReason: finish}
	for _, i := range order {
		call := calls[i]
		if call.ID == "" {
			call.ID = fmt.Sprintf("call_%d", i)
		}
		reply.ToolCalls = append(reply.ToolCalls, *call)
	}
	return reply, nil
}

func endpointOf(base string) (string, error) {
	base = strings.TrimSpace(base)
	if base == "" {
		return "", ErrNotConfigured
	}
	if !strings.Contains(base, "://") {
		base = "https://" + base
	}
	u, err := url.Parse(base)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", fmt.Errorf("llm: invalid endpoint %q", base)
	}
	base = strings.TrimRight(base, "/")
	if strings.HasSuffix(base, "/chat/completions") {
		return base, nil
	}
	return base + "/chat/completions", nil
}

func readError(r io.Reader) string {
	b, err := io.ReadAll(io.LimitReader(r, 8<<10))
	if err != nil {
		return ""
	}
	var body struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
		Message string `json:"message"`
	}
	if json.Unmarshal(b, &body) == nil {
		if body.Error.Message != "" {
			return body.Error.Message
		}
		if body.Message != "" {
			return body.Message
		}
	}
	return string(b)
}

type chatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Tools    []Tool    `json:"tools,omitempty"`
	Stream   bool      `json:"stream"`
}

type streamChunk struct {
	Choices []struct {
		Delta struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}
