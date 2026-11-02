package agent

import (
	"strings"
	"testing"
)

func TestParseShellArgsLenient(t *testing.T) {
	args, err := ParseShellArgs(`{"command":" ls ","explanation":"list files","safe_to_run":"true","destructive":1}`)
	if err != nil {
		t.Fatal(err)
	}
	if args.Command != "ls" || args.Description != "list files" {
		t.Fatalf("args = %+v", args)
	}
	if !args.SafeToRun || !args.Destructive {
		t.Fatalf("flags = %+v", args)
	}
	if _, err := ParseShellArgs(`{"host_id":"h1"}`); err == nil {
		t.Fatal("a call without a command passed")
	}
}

func TestSystemPromptNamesTheHost(t *testing.T) {
	prompt := SystemPrompt(Host{ID: "h1", Name: "web", Address: "me@web:22", OS: "linux"})
	for _, want := range []string{`"id":"h1"`, `"name":"web"`, `"address":"me@web:22"`, "about this host", "another host"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt does not contain %q: %s", want, prompt)
		}
	}
}

func TestLimitOutput(t *testing.T) {
	out, errOut, truncated := LimitOutput(strings.Repeat("a", 100), strings.Repeat("b", 100), 80)
	if !truncated {
		t.Fatal("long output was not truncated")
	}
	if !strings.Contains(out, "output truncated") || !strings.Contains(errOut, "output truncated") {
		t.Fatalf("out = %q, err = %q", out, errOut)
	}
	if runeLen(out) > 80 || runeLen(errOut) > 80 {
		t.Fatalf("limits = %d, %d", runeLen(out), runeLen(errOut))
	}
	short, shortErr, truncated := LimitOutput("hi", "there", 100)
	if truncated || short != "hi" || shortErr != "there" {
		t.Fatalf("short output = %q %q %v", short, shortErr, truncated)
	}
}

func TestSanitizeAnswersEveryCall(t *testing.T) {
	msgs := []*Message{
		{ID: "u", Role: "user", Content: "check"},
		{ID: "a", Role: "assistant", ToolCalls: []*ToolCall{
			{ID: "c1", Status: CallPending},
			{ID: "c2", Status: CallDone},
		}},
	}
	got := Sanitize(msgs)
	if len(got) != 4 {
		t.Fatalf("messages = %+v", got)
	}
	if got[1].ToolCalls[0].Status != CallDenied {
		t.Fatalf("pending call = %s, want denied", got[1].ToolCalls[0].Status)
	}
	if got[2].Role != "tool" || got[2].ToolCallID != "c1" || got[3].ToolCallID != "c2" {
		t.Fatalf("tool answers = %+v", got[2:])
	}

	withAnswer := []*Message{
		{ID: "a", Role: "assistant", ToolCalls: []*ToolCall{{ID: "c1"}}},
		{ID: "t", Role: "tool", ToolCallID: "c1", Content: "ok"},
	}
	again := Sanitize(withAnswer)
	if len(again) != 2 {
		t.Fatalf("sanitize added a duplicate answer: %+v", again)
	}
	if again[0].ToolCalls[0].Status != CallDone {
		t.Fatalf("status = %s, want done", again[0].ToolCalls[0].Status)
	}
}

func TestTitleOf(t *testing.T) {
	long := strings.Repeat("word ", 30)
	title := TitleOf([]*Message{
		{Role: "assistant", Content: "hello"},
		{Role: "user", Content: long},
	})
	if title == "" || strings.Contains(title, "\n") {
		t.Fatalf("title = %q", title)
	}
	if len([]rune(title)) > 49 {
		t.Fatalf("title is %d runes", len([]rune(title)))
	}
}
