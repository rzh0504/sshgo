package agent

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestParseReadFileArgs(t *testing.T) {
	args, err := ParseReadFileArgs(`{"path":" /etc/os-release ","description":"system"}`)
	if err != nil {
		t.Fatal(err)
	}
	if args.Path != "/etc/os-release" || args.Description != "system" {
		t.Fatalf("args = %+v", args)
	}
	if _, err := ParseReadFileArgs(`{"path":"etc/os-release"}`); err == nil {
		t.Fatal("a relative path passed")
	}
	if _, err := ParseReadFileArgs(`{"description":"no path"}`); err == nil {
		t.Fatal("a call without a path passed")
	}
}

func TestParseWriteFileArgs(t *testing.T) {
	args, err := ParseWriteFileArgs(`{"path":"/tmp/x","content":"","description":"empty"}`)
	if err != nil {
		t.Fatal(err)
	}
	if args.Path != "/tmp/x" || args.Content != "" || args.Description != "empty" {
		t.Fatalf("args = %+v", args)
	}
	if _, err := ParseWriteFileArgs(`{"path":"/tmp/x","description":"no content"}`); err == nil {
		t.Fatal("a call without content passed")
	}
	if _, err := ParseWriteFileArgs(`{"path":"tmp/x","content":"x"}`); err == nil {
		t.Fatal("a relative path passed")
	}
}

func TestWriteFileToolRejectsOversizeContent(t *testing.T) {
	content := strings.Repeat("a", MaxWriteBytes+1)
	raw := fmt.Sprintf(`{"path":"/tmp/x","content":%q,"description":"big"}`, content)
	if _, err := writeFileTool().Prepare(raw); err == nil {
		t.Fatal("oversize content passed")
	}
}

func TestToolsFilter(t *testing.T) {
	tools := Tools(ToolOptions{ReadFile: true})
	if len(tools) != 1 || toolName(tools[0]) != "read_file" {
		t.Fatalf("tools = %+v", tools)
	}
	none := Tools(ToolOptions{})
	if none == nil || len(none) != 0 {
		t.Fatalf("empty tools = %#v, want a non-nil empty slice", none)
	}
}

const readCallArguments = `{"path":"/etc/os-release","description":"system info"}`

func TestRunReadsSafeFileAutomatically(t *testing.T) {
	var requests []recordedRequest
	srv := scriptedToolServer(t, &requests, "read_file", readCallArguments)
	defer srv.Close()

	var (
		snapshot []*Message
		reads    []string
		reviews  int
	)
	engine := testEngine(srv.URL, Policy{ReadTools: true}, Hooks{
		Messages: func(msgs []*Message) { snapshot = msgs },
		ReadFile: func(_ context.Context, hostID, path string, max int64) (FileData, error) {
			reads = append(reads, fmt.Sprintf("%s %s %d", hostID, path, max))
			content := "PRETTY_NAME=\"Test\"\n"
			return FileData{Data: []byte(content), Size: int64(len(content))}, nil
		},
		Review: func(context.Context, Proposal) (Decision, error) {
			reviews++
			return DecisionRun, nil
		},
	})
	if err := engine.Run(context.Background(), userMessage("read os-release")); err != nil {
		t.Fatal(err)
	}
	if reviews != 0 {
		t.Fatalf("reviewed %d safe reads, want none", reviews)
	}
	if len(reads) != 1 || !strings.HasPrefix(reads[0], "h1 /etc/os-release ") {
		t.Fatalf("reads = %q", reads)
	}
	call := snapshot[1].ToolCalls[0]
	if call.Status != CallDone || call.Result == nil || !strings.Contains(call.Result.Content, "PRETTY_NAME") {
		t.Fatalf("call = %+v", call)
	}
	if !strings.Contains(snapshot[2].Content, "Content:") || !strings.Contains(snapshot[2].Content, "PRETTY_NAME") {
		t.Fatalf("tool answer = %q", snapshot[2].Content)
	}
}

const writeCallArguments = `{"path":"/etc/hosts","content":"127.0.0.1 localhost\n","description":"fix hosts"}`

func TestRunReviewsWriteFile(t *testing.T) {
	var requests []recordedRequest
	srv := scriptedToolServer(t, &requests, "write_file", writeCallArguments)
	defer srv.Close()

	var (
		snapshot []*Message
		reviews  []Proposal
		writes   int
	)
	engine := testEngine(srv.URL, Policy{ReadTools: true, ReadCommands: true}, Hooks{
		Messages: func(msgs []*Message) { snapshot = msgs },
		Review: func(_ context.Context, p Proposal) (Decision, error) {
			reviews = append(reviews, p)
			return DecisionDeny, nil
		},
		WriteFile: func(context.Context, string, string, []byte) error {
			writes++
			return nil
		},
	})
	if err := engine.Run(context.Background(), userMessage("fix hosts")); err != nil {
		t.Fatal(err)
	}
	if len(reviews) != 1 {
		t.Fatalf("reviews = %d, want 1", len(reviews))
	}
	review := reviews[0]
	if review.Tool != "write_file" || review.Risk != RiskCaution ||
		review.Path != "/etc/hosts" || !strings.Contains(review.Content, "127.0.0.1") {
		t.Fatalf("proposal = %+v", review)
	}
	if writes != 0 {
		t.Fatalf("a denied write ran %d times", writes)
	}
	if snapshot[1].ToolCalls[0].Status != CallDenied {
		t.Fatalf("call = %+v", snapshot[1].ToolCalls[0])
	}
}
