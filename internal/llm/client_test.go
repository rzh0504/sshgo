package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestChatStreamsTextAndToolCalls(t *testing.T) {
	var gotPath string
	var gotBody chatRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		events := []string{
			`{"choices":[{"delta":{"content":"Hel"}}]}`,
			`{"choices":[{"delta":{"content":"lo"}}]}`,
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"run_shell_command","arguments":"{\"host_id\":\"h1\","}}]}}]}`,
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"command\":\"ls\"}"}}]},"finish_reason":"tool_calls"}]}`,
		}
		for _, e := range events {
			fmt.Fprintf(w, "data: %s\n\n", e)
			if flusher != nil {
				flusher.Flush()
			}
		}
		io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	var deltas []string
	client := &Client{
		BaseURL:   srv.URL + "/v1",
		APIKey:    "secret",
		Model:     "test-model",
		Reasoning: "high",
	}
	reply, err := client.Chat(context.Background(), []Message{
		{Role: "user", Content: "list files"},
	}, []Tool{{Type: "function", Function: ToolFunction{Name: "run_shell_command"}}}, func(text string) {
		deltas = append(deltas, text)
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/v1/chat/completions" {
		t.Fatalf("path = %q, want /v1/chat/completions", gotPath)
	}
	if gotBody.Model != "test-model" || !gotBody.Stream || len(gotBody.Messages) != 1 || gotBody.ReasoningEffort != "high" {
		t.Fatalf("request = %+v", gotBody)
	}
	if reply.Content != "Hello" {
		t.Fatalf("content = %q, want Hello", reply.Content)
	}
	if len(deltas) != 2 || deltas[0] != "Hel" || deltas[1] != "Hello" {
		t.Fatalf("deltas = %q", deltas)
	}
	if len(reply.ToolCalls) != 1 {
		t.Fatalf("tool calls = %+v", reply.ToolCalls)
	}
	call := reply.ToolCalls[0]
	if call.ID != "call_1" || call.Function.Name != "run_shell_command" {
		t.Fatalf("call = %+v", call)
	}
	if call.Function.Arguments != `{"host_id":"h1","command":"ls"}` {
		t.Fatalf("arguments = %q", call.Function.Arguments)
	}
	if reply.FinishReason != "tool_calls" {
		t.Fatalf("finish reason = %q", reply.FinishReason)
	}
}

func TestChatReportsAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, `{"error":{"message":"bad key"}}`)
	}))
	defer srv.Close()
	client := &Client{BaseURL: srv.URL, Model: "m"}
	_, err := client.Chat(context.Background(), nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "bad key") {
		t.Fatalf("Chat = %v, want bad key", err)
	}
}

func TestChatNeedsConfiguration(t *testing.T) {
	_, err := (&Client{}).Chat(context.Background(), nil, nil, nil)
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("Chat = %v, want ErrNotConfigured", err)
	}
}

func TestEndpointOf(t *testing.T) {
	tests := map[string]string{
		"https://api.openai.com/v1":            "https://api.openai.com/v1/chat/completions",
		"https://api.openai.com/v1/":           "https://api.openai.com/v1/chat/completions",
		"http://localhost:11434/v1":            "http://localhost:11434/v1/chat/completions",
		"api.deepseek.com/v1/chat/completions": "https://api.deepseek.com/v1/chat/completions",
	}
	for in, want := range tests {
		got, err := endpointOf(in)
		if err != nil {
			t.Fatalf("endpointOf(%q) = %v", in, err)
		}
		if got != want {
			t.Fatalf("endpointOf(%q) = %q, want %q", in, got, want)
		}
	}
	if _, err := endpointOf("ftp://example.com"); err == nil {
		t.Fatal("endpointOf accepted a non-HTTP scheme")
	}
}

func TestModels(t *testing.T) {
	var gotPath, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"data":[{"id":"b"},{"id":"a"},{"id":"a"}]}`)
	}))
	defer srv.Close()

	models, err := (&Client{BaseURL: srv.URL + "/v1", APIKey: "secret"}).Models(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/v1/models" || gotAuth != "Bearer secret" {
		t.Fatalf("request = %s, auth %q", gotPath, gotAuth)
	}
	if len(models) != 2 || models[0] != "b" || models[1] != "a" {
		t.Fatalf("models = %q, want [b a]", models)
	}
}

func TestModelsReportsAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, `{"error":{"message":"bad key"}}`)
	}))
	defer srv.Close()
	_, err := (&Client{BaseURL: srv.URL, APIKey: "x"}).Models(context.Background())
	if err == nil || !strings.Contains(err.Error(), "bad key") {
		t.Fatalf("Models = %v, want bad key", err)
	}
}

func TestModelsEndpointOf(t *testing.T) {
	tests := map[string]string{
		"https://api.openai.com/v1":                  "https://api.openai.com/v1/models",
		"https://api.openai.com/v1/":                 "https://api.openai.com/v1/models",
		"https://api.openai.com/v1/chat/completions": "https://api.openai.com/v1/models",
		"https://api.openai.com/v1/models":           "https://api.openai.com/v1/models",
		"api.deepseek.com/v1":                        "https://api.deepseek.com/v1/models",
	}
	for in, want := range tests {
		got, err := modelsEndpointOf(in)
		if err != nil {
			t.Fatalf("modelsEndpointOf(%q) = %v", in, err)
		}
		if got != want {
			t.Fatalf("modelsEndpointOf(%q) = %q, want %q", in, got, want)
		}
	}
}
