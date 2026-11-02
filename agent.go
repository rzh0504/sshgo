package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"

	"sshgo/internal/agent"
	"sshgo/internal/llm"
	"sshgo/internal/sshx"
	"sshgo/internal/store"
	"sshgo/locales"
)

const (
	agentSecretKey = "agent-api-key"
	agentTimeout   = 5 * time.Minute
)

// agentState is the Agent's conversations and where it stands. The mu
// keeps the engine's goroutine and the view apart; the fields after it
// belong to the window's thread.
type agentState struct {
	mu          sync.Mutex
	chats       []*agent.Chat
	current     string
	busy        bool
	err         string
	streamingID string
	pending     *agent.Proposal
	answer      chan agent.Decision
	cancel      context.CancelFunc
	saveMu      sync.Mutex

	open          bool // the Agent is what the main card shows
	showHistory   bool
	focusComposer bool
	composer      ui.Handle
	draft         ui.TextBuffer
	list          ui.ListState
	md            markdown.Options
	expanded      map[string]bool
}

type agentDeleteState struct {
	open  bool
	id    string
	title string
}

func (s *agentState) chatLocked(id string) *agent.Chat {
	for _, c := range s.chats {
		if c.ID == id {
			return c
		}
	}
	return nil
}

// loadAgent reads the conversations saved last time.
func (a *App) loadAgent() {
	file, err := store.LoadChats()
	if err != nil {
		a.toast = err.Error()
		return
	}
	a.agent.mu.Lock()
	a.agent.chats = file.Chats
	for _, c := range a.agent.chats {
		c.Messages = agent.Sanitize(c.Messages)
		if c.Title == "" {
			c.Title = agent.TitleOf(c.Messages)
		}
	}
	a.agent.current = file.Current
	if a.agent.chatLocked(a.agent.current) == nil {
		a.agent.current = ""
	}
	a.agent.mu.Unlock()
}

func (a *App) saveAgentChats() {
	a.agent.mu.Lock()
	file := store.ChatFile{Current: a.agent.current, Chats: agent.CloneChats(a.agent.chats)}
	a.agent.mu.Unlock()
	a.agent.saveMu.Lock()
	err := file.Save()
	a.agent.saveMu.Unlock()
	if err != nil {
		a.update(func() { a.toast = err.Error() })
	}
}

// repaint asks for a frame; the engine calls it from its goroutine.
func (a *App) repaint() {
	if a.win != nil {
		a.win.Invalidate()
	}
}

// agentOpen reports whether the main card shows the Agent.
func (a *App) agentOpen() bool { return a.agent != nil && a.agent.open }

// toggleAgent shows the Agent in the main card, or the shell again.
func (a *App) toggleAgent() {
	a.agent.open = !a.agent.open
	if a.agent.open {
		a.agent.showHistory = false
		a.agent.focusComposer = true
	} else {
		a.focusWanted = a.selected
		a.focusTerminal(a.selected)
	}
	a.updateTitle()
	a.repaint()
}

// newAgentChat starts a conversation at the empty composer.
func (a *App) newAgentChat() {
	a.agent.mu.Lock()
	if a.agent.busy {
		a.agent.mu.Unlock()
		return
	}
	a.agent.current = ""
	a.agent.err = ""
	a.agent.mu.Unlock()
	a.agent.showHistory = false
	a.agent.focusComposer = true
	a.saveAgentChats()
	a.repaint()
	a.updateTitle()
}

func (a *App) selectAgentChat(id string) {
	a.agent.mu.Lock()
	if a.agent.busy || a.agent.chatLocked(id) == nil {
		a.agent.mu.Unlock()
		return
	}
	a.agent.current = id
	a.agent.err = ""
	a.agent.mu.Unlock()
	a.agent.showHistory = false
	a.agent.focusComposer = true
	a.saveAgentChats()
	a.repaint()
	a.updateTitle()
}

func (a *App) deleteAgentChat(id string) {
	a.agent.mu.Lock()
	if a.agent.busy {
		a.agent.mu.Unlock()
		return
	}
	out := a.agent.chats[:0]
	for _, c := range a.agent.chats {
		if c.ID != id {
			out = append(out, c)
		}
	}
	a.agent.chats = out
	if a.agent.current == id {
		a.agent.current = ""
	}
	a.agent.err = ""
	a.agent.mu.Unlock()
	a.saveAgentChats()
	a.repaint()
	a.updateTitle()
}

func (a *App) openAgentDelete(id string) {
	a.agent.mu.Lock()
	if a.agent.busy {
		a.agent.mu.Unlock()
		return
	}
	chat := a.agent.chatLocked(id)
	if chat == nil {
		a.agent.mu.Unlock()
		return
	}
	title := chat.Title
	a.agent.mu.Unlock()
	if title == "" {
		title = locales.AgentUntitledChat()
	}
	a.agentDelete = &agentDeleteState{open: true, id: id, title: title}
	a.repaint()
}

// sendAgent sends text to the Agent, starting a conversation when there
// is none. It reports whether the turn started.
func (a *App) sendAgent(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	a.agent.mu.Lock()
	if a.agent.busy {
		a.agent.mu.Unlock()
		return false
	}
	if !a.agentConfigured() {
		a.agent.err = locales.AgentNotConfigured()
		a.agent.mu.Unlock()
		a.repaint()
		return false
	}
	chat := a.agent.chatLocked(a.agent.current)
	if chat == nil {
		chat = &agent.Chat{ID: agent.NewID("chat"), CreatedAt: time.Now().UnixMilli()}
		a.agent.chats = append(a.agent.chats, chat)
		a.agent.current = chat.ID
	}
	now := time.Now().UnixMilli()
	chat.Messages = append(chat.Messages, &agent.Message{
		ID:        agent.NewID("msg"),
		Role:      "user",
		Content:   text,
		CreatedAt: now,
	})
	chat.UpdatedAt = now
	if chat.Title == "" {
		chat.Title = agent.TitleOf(chat.Messages)
	}
	msgs := agent.CloneMessages(chat.Messages)
	ctx, cancel := context.WithCancel(context.Background())
	a.agent.busy = true
	a.agent.err = ""
	a.agent.streamingID = ""
	a.agent.cancel = cancel
	chatID := chat.ID
	a.agent.mu.Unlock()

	a.agent.draft.Set("")
	a.saveAgentChats()
	a.repaint()
	a.updateTitle()

	engine := &agent.Engine{
		Client: &llm.Client{
			BaseURL: a.state.AgentBaseURL,
			APIKey:  a.savedSecret(agentSecretKey),
			Model:   a.state.AgentModel,
		},
		Hosts:       a.agentHosts(),
		DefaultHost: a.agentDefaultHost(),
		Policy:      agent.Policy{AutoRunSafe: a.state.AgentAutoRunSafe},
		Hooks: agent.Hooks{
			Messages: func(messages []*agent.Message) {
				a.applyAgentMessages(chatID, messages)
			},
			Delta: func(id, content string) {
				a.applyAgentDelta(chatID, id, content)
			},
			Review: func(ctx context.Context, p agent.Proposal) (agent.Decision, error) {
				return a.reviewAgentTool(ctx, p)
			},
			Exec: func(ctx context.Context, hostID, command string) (agent.ToolResult, error) {
				return a.execAgentCommand(ctx, hostID, command)
			},
		},
	}
	go func() {
		err := engine.Run(ctx, msgs)
		cancel()
		a.agent.mu.Lock()
		a.agent.busy = false
		a.agent.cancel = nil
		a.agent.pending = nil
		a.agent.answer = nil
		a.agent.streamingID = ""
		if err != nil && !errors.Is(err, context.Canceled) {
			a.agent.err = err.Error()
		}
		a.agent.mu.Unlock()
		a.saveAgentChats()
		a.repaint()
	}()
	return true
}

// applyAgentMessages is the engine's conversation, as it changes.
func (a *App) applyAgentMessages(chatID string, msgs []*agent.Message) {
	a.agent.mu.Lock()
	chat := a.agent.chatLocked(chatID)
	if chat != nil {
		chat.Messages = msgs
		chat.UpdatedAt = time.Now().UnixMilli()
		if chat.Title == "" {
			chat.Title = agent.TitleOf(msgs)
		}
		if a.agent.busy {
			a.agent.streamingID = ""
			if len(msgs) > 0 {
				if last := msgs[len(msgs)-1]; last.Role == "assistant" {
					a.agent.streamingID = last.ID
				}
			}
		}
	}
	a.agent.mu.Unlock()
	a.saveAgentChats()
	a.repaint()
}

func (a *App) applyAgentDelta(chatID, msgID, content string) {
	a.agent.mu.Lock()
	if chat := a.agent.chatLocked(chatID); chat != nil {
		for _, m := range chat.Messages {
			if m.ID == msgID {
				m.Content = content
				break
			}
		}
	}
	a.agent.mu.Unlock()
	a.repaint()
}

func (a *App) reviewAgentTool(ctx context.Context, p agent.Proposal) (agent.Decision, error) {
	ch := make(chan agent.Decision, 1)
	a.agent.mu.Lock()
	a.agent.pending = &p
	a.agent.answer = ch
	a.agent.mu.Unlock()
	a.repaint()
	var decision agent.Decision
	select {
	case decision = <-ch:
	case <-ctx.Done():
		decision = agent.DecisionDeny
	}
	a.agent.mu.Lock()
	if a.agent.answer == ch {
		a.agent.pending = nil
		a.agent.answer = nil
	}
	a.agent.mu.Unlock()
	a.repaint()
	if err := ctx.Err(); err != nil {
		return agent.DecisionDeny, err
	}
	return decision, nil
}

func (a *App) answerAgentCall(callID string, decision agent.Decision) {
	a.agent.mu.Lock()
	if a.agent.pending == nil || a.agent.pending.CallID != callID || a.agent.answer == nil {
		a.agent.mu.Unlock()
		return
	}
	ch := a.agent.answer
	a.agent.pending = nil
	a.agent.answer = nil
	a.agent.mu.Unlock()
	select {
	case ch <- decision:
	default:
	}
	a.repaint()
}

func (a *App) stopAgent() {
	a.agent.mu.Lock()
	cancel := a.agent.cancel
	ch := a.agent.answer
	a.agent.pending = nil
	a.agent.answer = nil
	a.agent.mu.Unlock()
	if ch != nil {
		select {
		case ch <- agent.DecisionDeny:
		default:
		}
	}
	if cancel != nil {
		cancel()
	}
	a.repaint()
}

// execAgentCommand runs command on a host that is connected now.
func (a *App) execAgentCommand(ctx context.Context, hostID, command string) (agent.ToolResult, error) {
	if a.win == nil {
		return agent.ToolResult{}, errors.New("sshgo is closing")
	}
	type picked struct {
		sess *sshx.Session
		err  error
	}
	ch := make(chan picked, 1)
	a.win.Update(func() {
		rt := a.runtimes[hostID]
		if rt == nil || rt.state != connConnected || rt.ssh == nil {
			ch <- picked{err: fmt.Errorf("host %s is not connected", hostID)}
			return
		}
		ch <- picked{sess: rt.ssh}
	})
	var p picked
	select {
	case p = <-ch:
	case <-ctx.Done():
		return agent.ToolResult{}, ctx.Err()
	}
	if p.err != nil {
		return agent.ToolResult{Error: p.err.Error()}, nil
	}
	start := time.Now()
	runCtx, cancel := context.WithTimeout(ctx, agentTimeout)
	defer cancel()
	if err := runCtx.Err(); err != nil {
		return agent.ToolResult{Error: "The command was stopped."}, nil
	}
	res, err := p.sess.Exec(runCtx, command)
	out := agent.ToolResult{
		Stdout:     res.Stdout,
		Stderr:     res.Stderr,
		ExitCode:   res.ExitCode,
		DurationMS: time.Since(start).Milliseconds(),
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		out.Error = "The command timed out after 5 minutes."
	case errors.Is(err, context.Canceled):
		out.Error = "The command was stopped."
	case err != nil:
		out.Error = err.Error()
	}
	return out, nil
}

// agentHosts are the hosts the Agent may work on: those connected now.
func (a *App) agentHosts() []agent.Host {
	var out []agent.Host
	for _, h := range a.state.Hosts {
		rt := a.runtimes[h.ID]
		if rt == nil || rt.state != connConnected || rt.ssh == nil {
			continue
		}
		out = append(out, agent.Host{ID: h.ID, Name: h.Name, Address: h.Address(), OS: h.OS})
	}
	return out
}

func (a *App) agentDefaultHost() string {
	if h, ok := a.selectedHost(); ok {
		if rt := a.runtimes[h.ID]; rt != nil && rt.state == connConnected && rt.ssh != nil {
			return h.ID
		}
	}
	if hosts := a.agentHosts(); len(hosts) > 0 {
		return hosts[0].ID
	}
	return ""
}

func (a *App) agentConfigured() bool {
	return strings.TrimSpace(a.state.AgentBaseURL) != "" && strings.TrimSpace(a.state.AgentModel) != ""
}

func (a *App) agentCurrentTitle() string {
	a.agent.mu.Lock()
	defer a.agent.mu.Unlock()
	if chat := a.agent.chatLocked(a.agent.current); chat != nil {
		return chat.Title
	}
	return ""
}

func (a *App) openAgentSettings() {
	a.openSettings()
	a.ss.page = "agent"
	if a.setWin != nil {
		a.setWin.Invalidate()
	}
}

func (a *App) loadAgentKey() {
	if a.agentKeyLoaded {
		return
	}
	a.agentKeyEdit = a.savedSecret(agentSecretKey)
	a.agentKeyLoaded = true
}

func (a *App) saveAgentKey(notify bool) {
	key := strings.TrimSpace(a.agentKeyEdit)
	var err error
	if key == "" {
		err = mygo.Secrets.Delete(agentSecretKey)
		if errors.Is(err, mygo.ErrSecretNotFound) || errors.Is(err, errors.ErrUnsupported) {
			err = nil
		}
	} else {
		err = mygo.Secrets.Set(agentSecretKey, []byte(key))
	}
	if err != nil {
		a.toast = err.Error()
		return
	}
	a.agentKeyDirty = false
	if notify {
		a.toast = locales.AgentKeySaved()
	}
}
