package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"sshgo/internal/llm"
)

type recordedRequest struct {
	Model    string        `json:"model"`
	Messages []llm.Message `json:"messages"`
	Tools    []llm.Tool    `json:"tools"`
	Stream   bool          `json:"stream"`
}

const diskCallArguments = `{"command":"df -h","description":"disk usage","safe_to_run":true,"destructive":false}`

// scripted replies to the first request with a tool call, and to a request
// carrying a tool result with a final answer.
func scriptedAgentServer(t *testing.T, requests *[]recordedRequest) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body recordedRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		*requests = append(*requests, body)
		answered := false
		for _, m := range body.Messages {
			if m.Role == "tool" {
				answered = true
			}
		}
		w.Header().Set("Content-Type", "text/event-stream")
		if !answered {
			fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Checking.\"}}]}\n\n")
			fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"type\":\"function\",\"function\":{\"name\":\"run_shell_command\",\"arguments\":%q}}]},\"finish_reason\":\"tool_calls\"}]}\n\n", diskCallArguments)
		} else {
			fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"All good.\"}}]}\n\n")
		}
		io.WriteString(w, "data: [DONE]\n\n")
	}))
}

func testEngine(url string, policy Policy, hooks Hooks) *Engine {
	return &Engine{
		Client: &llm.Client{BaseURL: url, Model: "test"},
		Host:   Host{ID: "h1", Name: "web", Address: "me@web:22", OS: "linux"},
		Policy: policy,
		Hooks:  hooks,
	}
}

func userMessage(text string) []*Message {
	return []*Message{{ID: "m1", Role: "user", Content: text, CreatedAt: time.Now().UnixMilli()}}
}

func TestRunRunsSafeReadOnlyAutomatically(t *testing.T) {
	var requests []recordedRequest
	srv := scriptedAgentServer(t, &requests)
	defer srv.Close()

	var (
		snapshot []*Message
		execs    []string
		reviews  []Proposal
	)
	engine := testEngine(srv.URL, Policy{AutoRunSafe: true}, Hooks{
		Messages: func(msgs []*Message) { snapshot = msgs },
		Exec: func(_ context.Context, hostID, command string) (ToolResult, error) {
			execs = append(execs, hostID+" "+command)
			return ToolResult{Stdout: "/dev/sda1  40G  12G  28G  30% /\n"}, nil
		},
		Review: func(_ context.Context, p Proposal) (Decision, error) {
			reviews = append(reviews, p)
			return DecisionRun, nil
		},
	})
	if err := engine.Run(context.Background(), userMessage("check disk")); err != nil {
		t.Fatal(err)
	}
	if len(reviews) != 0 {
		t.Fatalf("reviewed %d commands, want none for a safe read-only one", len(reviews))
	}
	if len(execs) != 1 || execs[0] != "h1 df -h" {
		t.Fatalf("execs = %q", execs)
	}
	if len(requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(requests))
	}
	if len(requests[0].Messages) != 2 || requests[0].Messages[1].Role != "user" {
		t.Fatalf("first request messages = %+v", requests[0].Messages)
	}
	if len(requests[1].Messages) != 4 {
		t.Fatalf("second request messages = %+v", requests[1].Messages)
	}
	if requests[1].Messages[3].Role != "tool" || !strings.Contains(requests[1].Messages[3].Content, "Stdout:") {
		t.Fatalf("tool answer = %+v", requests[1].Messages[3])
	}
	if len(requests[0].Tools) != 1 || requests[0].Tools[0].Function.Name != "run_shell_command" {
		t.Fatalf("tools = %+v", requests[0].Tools)
	}

	if len(snapshot) != 4 {
		t.Fatalf("conversation = %+v", snapshot)
	}
	if snapshot[0].Role != "user" || snapshot[1].Role != "assistant" || snapshot[2].Role != "tool" || snapshot[3].Role != "assistant" {
		t.Fatalf("roles = %s %s %s %s", snapshot[0].Role, snapshot[1].Role, snapshot[2].Role, snapshot[3].Role)
	}
	call := snapshot[1].ToolCalls[0]
	if call.Status != CallDone || call.HostID != "h1" || call.Result == nil || call.Result.ExitCode != 0 {
		t.Fatalf("call = %+v", call)
	}
	if snapshot[3].Content != "All good." {
		t.Fatalf("final answer = %q", snapshot[3].Content)
	}
}

func TestRunHonorsDenial(t *testing.T) {
	var requests []recordedRequest
	srv := scriptedAgentServer(t, &requests)
	defer srv.Close()

	var (
		snapshot []*Message
		execs    int
	)
	engine := testEngine(srv.URL, Policy{}, Hooks{
		Messages: func(msgs []*Message) { snapshot = msgs },
		Exec: func(context.Context, string, string) (ToolResult, error) {
			execs++
			return ToolResult{}, nil
		},
		Review: func(_ context.Context, p Proposal) (Decision, error) {
			if p.Risk != RiskReadOnly {
				t.Errorf("risk = %s, want read-only", p.Risk)
			}
			if p.Host.ID != "h1" {
				t.Errorf("host = %+v", p.Host)
			}
			return DecisionDeny, nil
		},
	})
	if err := engine.Run(context.Background(), userMessage("check disk")); err != nil {
		t.Fatal(err)
	}
	if execs != 0 {
		t.Fatalf("a denied command ran %d times", execs)
	}
	if len(snapshot) != 4 {
		t.Fatalf("conversation = %+v", snapshot)
	}
	call := snapshot[1].ToolCalls[0]
	if call.Status != CallDenied {
		t.Fatalf("status = %s, want denied", call.Status)
	}
	if !strings.Contains(snapshot[2].Content, "declined") {
		t.Fatalf("tool answer = %q", snapshot[2].Content)
	}
	if len(requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(requests))
	}
}

func TestRunStopsOnCancellation(t *testing.T) {
	var requests []recordedRequest
	srv := scriptedAgentServer(t, &requests)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	var snapshot []*Message
	engine := testEngine(srv.URL, Policy{}, Hooks{
		Messages: func(msgs []*Message) { snapshot = msgs },
		Review: func(ctx context.Context, _ Proposal) (Decision, error) {
			<-ctx.Done()
			return DecisionDeny, ctx.Err()
		},
	})
	err := engine.Run(ctx, userMessage("check disk"))
	if err == nil || !strings.Contains(err.Error(), "deadline exceeded") {
		t.Fatalf("Run = %v, want deadline exceeded", err)
	}
	if len(snapshot) < 3 {
		t.Fatalf("conversation = %+v", snapshot)
	}
	last := snapshot[len(snapshot)-1]
	if last.Role != "tool" || !strings.Contains(last.Content, "stopped") {
		t.Fatalf("last message = %+v", last)
	}
}
