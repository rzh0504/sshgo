package agent

import (
	"context"
	"strings"
	"testing"
)

func TestBuiltinToolNames(t *testing.T) {
	names := map[string]bool{}
	for _, tool := range BuiltinTools() {
		names[tool.Definition.Function.Name] = true
	}
	for _, want := range []string{
		"run_shell_command", "read_file", "write_file", "list_dir",
		"terminal_screen", "memory", "web_fetch", "ask_user",
	} {
		if !names[want] {
			t.Fatalf("built-in tools = %+v, missing %s", names, want)
		}
	}
}

const listDirCallArguments = `{"path":"/etc","description":"config"}`

func TestRunListsDirectoryAutomatically(t *testing.T) {
	var requests []recordedRequest
	srv := scriptedToolServer(t, &requests, "list_dir", listDirCallArguments)
	defer srv.Close()

	var (
		snapshot []*Message
		reviews  int
	)
	engine := testEngine(srv.URL, Policy{ReadTools: true}, Hooks{
		Messages: func(msgs []*Message) { snapshot = msgs },
		ListDir: func(_ context.Context, hostID, path string, max int) (DirListing, error) {
			return DirListing{Entries: []DirEntry{
				{Name: "log", Size: 10, Mode: "-rw-r--r--"},
				{Name: "etc", IsDir: true, Mode: "drwxr-xr-x"},
			}}, nil
		},
		Review: func(context.Context, Proposal) (Decision, error) {
			reviews++
			return DecisionRun, nil
		},
	})
	if err := engine.Run(context.Background(), userMessage("list /etc")); err != nil {
		t.Fatal(err)
	}
	if reviews != 0 {
		t.Fatalf("reviewed a safe directory listing %d times", reviews)
	}
	call := snapshot[1].ToolCalls[0]
	if call.Status != CallDone || call.Result == nil || !strings.Contains(call.Result.Content, "etc/") {
		t.Fatalf("call = %+v", call)
	}
	if !strings.Contains(snapshot[2].Content, "Entries:") {
		t.Fatalf("tool answer = %q", snapshot[2].Content)
	}
}

func TestRunReadsTerminalAutomatically(t *testing.T) {
	var requests []recordedRequest
	srv := scriptedToolServer(t, &requests, "terminal_screen", `{}`)
	defer srv.Close()

	var snapshot []*Message
	engine := testEngine(srv.URL, Policy{ReadTools: true}, Hooks{
		Messages: func(msgs []*Message) { snapshot = msgs },
		Terminal: func(context.Context, string, int) (string, error) {
			return "root@web:~# uptime\n", nil
		},
		Review: func(context.Context, Proposal) (Decision, error) {
			t.Fatal("a read-only terminal read was reviewed")
			return DecisionRun, nil
		},
	})
	if err := engine.Run(context.Background(), userMessage("read terminal")); err != nil {
		t.Fatal(err)
	}
	call := snapshot[1].ToolCalls[0]
	if call.Status != CallDone || call.Result == nil || !strings.Contains(call.Result.Content, "uptime") {
		t.Fatalf("call = %+v", call)
	}
}

const memoryReadArguments = `{"action":"read","description":"check notes"}`

func TestRunReadsMemoryAutomatically(t *testing.T) {
	var requests []recordedRequest
	srv := scriptedToolServer(t, &requests, "memory", memoryReadArguments)
	defer srv.Close()

	var snapshot []*Message
	engine := testEngine(srv.URL, Policy{ReadTools: true}, Hooks{
		Messages: func(msgs []*Message) { snapshot = msgs },
		MemoryRead: func(context.Context) (string, error) {
			return "The user prefers Chinese.", nil
		},
		Review: func(context.Context, Proposal) (Decision, error) {
			t.Fatal("a memory read was reviewed")
			return DecisionRun, nil
		},
	})
	if err := engine.Run(context.Background(), userMessage("read memory")); err != nil {
		t.Fatal(err)
	}
	call := snapshot[1].ToolCalls[0]
	if call.Status != CallDone || call.Result == nil || !strings.Contains(call.Result.Content, "Chinese") {
		t.Fatalf("call = %+v", call)
	}
}

const memoryWriteArguments = `{"action":"append","content":"Use tabs.","description":"remember style"}`

func TestRunReviewsMemoryWrite(t *testing.T) {
	var requests []recordedRequest
	srv := scriptedToolServer(t, &requests, "memory", memoryWriteArguments)
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
		MemoryWrite: func(context.Context, string, bool) (int, error) {
			writes++
			return 0, nil
		},
	})
	if err := engine.Run(context.Background(), userMessage("remember style")); err != nil {
		t.Fatal(err)
	}
	if len(reviews) != 1 || reviews[0].Risk != RiskCaution || !strings.Contains(reviews[0].Content, "Use tabs") {
		t.Fatalf("reviews = %+v", reviews)
	}
	if writes != 0 {
		t.Fatalf("a denied memory write ran %d times", writes)
	}
	if snapshot[1].ToolCalls[0].Status != CallDenied {
		t.Fatalf("call = %+v", snapshot[1].ToolCalls[0])
	}
}

const webCallArguments = `{"url":"https://example.com/docs","description":"read docs"}`

func TestRunReviewsWebFetch(t *testing.T) {
	var requests []recordedRequest
	srv := scriptedToolServer(t, &requests, "web_fetch", webCallArguments)
	defer srv.Close()

	var (
		snapshot []*Message
		reviews  []Proposal
		fetches  int
	)
	engine := testEngine(srv.URL, Policy{ReadTools: true, ReadCommands: true}, Hooks{
		Messages: func(msgs []*Message) { snapshot = msgs },
		Review: func(_ context.Context, p Proposal) (Decision, error) {
			reviews = append(reviews, p)
			return DecisionRun, nil
		},
		WebFetch: func(_ context.Context, request WebRequest) (WebData, error) {
			fetches++
			if request.URL != "https://example.com/docs" {
				t.Fatalf("url = %q", request.URL)
			}
			return WebData{Status: 200, Body: "hello docs", Size: 10}, nil
		},
	})
	if err := engine.Run(context.Background(), userMessage("read docs")); err != nil {
		t.Fatal(err)
	}
	if len(reviews) != 1 || reviews[0].Risk != RiskUnknown || reviews[0].Tool != "web_fetch" {
		t.Fatalf("reviews = %+v", reviews)
	}
	if fetches != 1 {
		t.Fatalf("fetches = %d", fetches)
	}
	if snapshot[1].ToolCalls[0].Result == nil || !strings.Contains(snapshot[1].ToolCalls[0].Result.Content, "hello docs") {
		t.Fatalf("call = %+v", snapshot[1].ToolCalls[0])
	}
}

const askCallArguments = `{"questions":[{"id":"choice","question":"Which one?","options":[{"label":"A","value":"a"},{"label":"B","value":"b"}]},{"id":"note","question":"Anything else?","secret":true}]}`

func TestRunAsksUserWithoutReview(t *testing.T) {
	var requests []recordedRequest
	srv := scriptedToolServer(t, &requests, "ask_user", askCallArguments)
	defer srv.Close()

	var (
		snapshot []*Message
		asks     int
	)
	engine := testEngine(srv.URL, Policy{}, Hooks{
		Messages: func(msgs []*Message) { snapshot = msgs },
		Review: func(context.Context, Proposal) (Decision, error) {
			t.Fatal("ask_user was sent through the review card")
			return DecisionRun, nil
		},
		Ask: func(_ context.Context, request AskRequest) (AskResult, error) {
			asks++
			if len(request.Questions) != 2 || request.Questions[1].Secret != true {
				t.Fatalf("request = %+v", request)
			}
			return AskResult{Answers: []AskAnswer{
				{ID: "choice", Values: []string{"a"}},
				{ID: "note", Text: "sec-test"},
			}}, nil
		},
	})
	if err := engine.Run(context.Background(), userMessage("ask me")); err != nil {
		t.Fatal(err)
	}
	if asks != 1 {
		t.Fatalf("asks = %d", asks)
	}
	call := snapshot[1].ToolCalls[0]
	if call.Status != CallDone || call.Result == nil ||
		!strings.Contains(call.Result.Content, "a") || !strings.Contains(call.Result.Content, "sec-test") {
		t.Fatalf("call = %+v", call)
	}
}

func TestParseAskArgs(t *testing.T) {
	args, err := ParseAskArgs(`{"questions":[{"question":"Pick","options":["A","B"]},{"id":"secret","question":"Token","secret":true}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(args.Questions) != 2 || args.Questions[0].ID != "q1" ||
		args.Questions[0].Options[0].Value != "A" || !args.Questions[1].Secret {
		t.Fatalf("args = %+v", args)
	}
	if _, err := ParseAskArgs(`{"questions":[{"id":"x","question":"Pick","secret":true,"options":["A"]}]}`); err == nil {
		t.Fatal("a secret question with options passed")
	}
	if _, err := ParseAskArgs(`{"questions":[]}`); err == nil {
		t.Fatal("an empty question list passed")
	}
}

func TestParseWebArgsRejectsOtherSchemes(t *testing.T) {
	if _, err := ParseWebArgs(`{"url":"ftp://example.com/x","description":"x"}`); err == nil {
		t.Fatal("an ftp URL passed")
	}
	args, err := ParseWebArgs(`{"url":"https://example.com","headers":{"Authorization":"{{sec-1}}"},"description":"x"}`)
	if err != nil {
		t.Fatal(err)
	}
	if args.Headers["Authorization"] != "{{sec-1}}" {
		t.Fatalf("headers = %+v", args.Headers)
	}
}
