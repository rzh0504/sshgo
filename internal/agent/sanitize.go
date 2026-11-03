package agent

import (
	"strings"
	"time"
)

// Sanitize makes a stored conversation one the API accepts: every tool
// call has a tool answer after it, even when the app closed in the middle
// of one, and no call is left running.
func Sanitize(msgs []*Message) []*Message {
	out := make([]*Message, 0, len(msgs))
	for i := 0; i < len(msgs); i++ {
		m := msgs[i]
		out = append(out, m)
		if m.Role != "assistant" || len(m.ToolCalls) == 0 {
			continue
		}
		present := map[string]bool{}
		j := i + 1
		for ; j < len(msgs) && msgs[j].Role == "tool"; j++ {
			out = append(out, msgs[j])
			present[msgs[j].ToolCallID] = true
		}
		i = j - 1
		for _, call := range m.ToolCalls {
			if call.Status == CallPending || call.Status == CallRunning {
				call.Status = CallDenied
				if call.Result == nil {
					call.Result = &ToolResult{Error: "The app closed before this finished."}
				}
			}
			if present[call.ID] {
				if call.Status == "" || call.Status == CallPending || call.Status == CallRunning {
					call.Status = CallDone
				}
				continue
			}
			content := "The app closed before this call finished."
			if call.Status == CallDenied {
				content = "The user declined to run this command."
			}
			out = append(out, &Message{
				ID:         NewID("msg"),
				Role:       "tool",
				Content:    content,
				ToolCallID: call.ID,
				CreatedAt:  time.Now().UnixMilli(),
			})
			if call.Status == "" {
				call.Status = CallDenied
			}
		}
	}
	return out
}

// TitleOf is a conversation's title: its first user message, on one line
// and no longer than the sidebar shows.
func TitleOf(msgs []*Message) string {
	for _, m := range msgs {
		if m.Role != "user" {
			continue
		}
		title := strings.Join(strings.Fields(m.Content), " ")
		if title == "" && len(m.Attachments) > 0 {
			title = m.Attachments[0].Name
		}
		if title == "" {
			continue
		}
		if r := []rune(title); len(r) > 48 {
			title = strings.TrimSpace(string(r[:48])) + "…"
		}
		return title
	}
	return ""
}

// CleanTitle is a model's name for a conversation: one line, without the
// quotes and closing period models add, and no longer than TitleOf's.
func CleanTitle(s string) string {
	title := strings.Join(strings.Fields(s), " ")
	title = strings.Trim(title, "\"'“”‘’`《》")
	title = strings.TrimSpace(strings.TrimRight(title, "。.！!？?…"))
	if r := []rune(title); len(r) > 48 {
		title = strings.TrimSpace(string(r[:48])) + "…"
	}
	return title
}
