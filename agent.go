package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"

	"sshgo/internal/agent"
	"sshgo/internal/llm"
	"sshgo/internal/sshx"
	"sshgo/internal/store"
	"sshgo/locales"
)

const (
	agentSecretKey   = "agent-api-key"
	agentTimeout     = 5 * time.Minute
	agentFileTimeout = 30 * time.Second
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
	queued      *agentQueuedSend
	saveMu      sync.Mutex
	memoryMu    sync.Mutex
	secrets     map[string]string

	open              bool // the Agent is what the main card shows
	showHistory       bool
	target            string // the connected host this conversation runs on
	provider          string // the provider this conversation uses
	model             string // the model this conversation uses
	reasoning         string // how hard the model should think
	modelMenuOpen     bool
	hostMenuOpen      bool
	reasoningMenuOpen bool
	focusComposer     bool
	composer          ui.Handle
	draft             ui.TextBuffer
	attachments       []agent.Attachment
	previews          map[string]ui.ImageSource
	list              ui.ListState
	md                markdown.Options
	expanded          map[string]bool
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

// loadAgent reads the conversations saved last time, and the current
// one's messages.
func (a *App) loadAgent() {
	idx, err := store.LoadChatIndex()
	if err != nil {
		a.toast = err.Error()
		return
	}
	a.agent.mu.Lock()
	a.agent.chats = make([]*agent.Chat, 0, len(idx.Chats))
	for _, meta := range idx.Chats {
		a.agent.chats = append(a.agent.chats, &agent.Chat{
			ID: meta.ID, Title: meta.Title, HostID: meta.HostID,
			CreatedAt: meta.CreatedAt, UpdatedAt: meta.UpdatedAt,
		})
	}
	a.agent.current = idx.Current
	if a.agent.chatLocked(a.agent.current) == nil {
		a.agent.current = ""
	}
	current := a.agent.current
	a.agent.mu.Unlock()
	a.agent.target = ""
	if current == "" {
		return
	}
	if err := a.loadAgentChat(current); err != nil {
		a.toast = err.Error()
		a.agent.mu.Lock()
		a.agent.current = ""
		a.agent.mu.Unlock()
		a.saveAgentIndex()
		return
	}
	a.agent.mu.Lock()
	if chat := a.agent.chatLocked(current); chat != nil && chat.HostID != "" {
		a.agent.target = chat.HostID
	}
	a.agent.mu.Unlock()
}

// loadAgentChat reads one conversation's messages.
func (a *App) loadAgentChat(id string) error {
	loaded, err := store.LoadChat(id)
	if err != nil {
		return err
	}
	loaded.Messages = agent.Sanitize(loaded.Messages)
	a.hydrateAgentAttachments(loaded.Messages)
	if loaded.Title == "" {
		loaded.Title = agent.TitleOf(loaded.Messages)
	}
	a.agent.mu.Lock()
	if chat := a.agent.chatLocked(id); chat != nil {
		chat.Messages = loaded.Messages
		chat.Title = loaded.Title
		chat.HostID = loaded.HostID
		chat.ProviderID = loaded.ProviderID
		chat.Model = loaded.Model
		chat.CreatedAt = loaded.CreatedAt
		chat.UpdatedAt = loaded.UpdatedAt
	}
	a.agent.mu.Unlock()
	return nil
}

func (s *agentState) indexLocked() store.ChatIndex {
	idx := store.ChatIndex{Current: s.current}
	for _, c := range s.chats {
		idx.Chats = append(idx.Chats, store.ChatMeta{
			ID: c.ID, Title: c.Title, HostID: c.HostID, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
		})
	}
	return idx
}

// saveAgentIndex writes the list of conversations, without reading or
// writing any conversation's messages.
func (a *App) saveAgentIndex() {
	a.agent.mu.Lock()
	idx := a.agent.indexLocked()
	a.agent.mu.Unlock()
	a.agent.saveMu.Lock()
	err := idx.Save()
	a.agent.saveMu.Unlock()
	if err != nil {
		a.agentSaveFailed(err)
	}
}

// saveAgentChat writes one conversation and the list.
func (a *App) saveAgentChat(chatID string) {
	a.agent.mu.Lock()
	var chat *agent.Chat
	for _, c := range a.agent.chats {
		if c.ID == chatID && c.Messages != nil {
			chat = agent.CloneChat(c)
			break
		}
	}
	idx := a.agent.indexLocked()
	a.agent.mu.Unlock()
	a.agent.saveMu.Lock()
	defer a.agent.saveMu.Unlock()
	if chat != nil {
		if err := store.SaveChat(chat); err != nil {
			a.agentSaveFailed(err)
			return
		}
	}
	if err := idx.Save(); err != nil {
		a.agentSaveFailed(err)
	}
}

func (a *App) agentSaveFailed(err error) {
	a.update(func() { a.toast = err.Error() })
}

// repaint asks for a frame; the engine calls it from its goroutine.
func (a *App) repaint() {
	if a.win != nil {
		a.win.Invalidate()
	}
}

// agentOpen reports whether the main card shows the Agent.
func (a *App) agentOpen() bool { return a.agent != nil && a.agent.open }

func (a *App) closeAgentMenus() {
	a.agent.modelMenuOpen = false
	a.agent.hostMenuOpen = false
	a.agent.reasoningMenuOpen = false
}

// toggleAgent shows the Agent in the main card, or the shell again.
func (a *App) toggleAgent() {
	a.agent.open = !a.agent.open
	if a.agent.open {
		a.agent.showHistory = false
		a.agent.focusComposer = true
	} else {
		a.closeAgentMenus()
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
	a.agent.secrets = nil
	a.agent.mu.Unlock()
	a.closeAgentMenus()
	a.agent.target = ""
	a.syncAgentModel(nil)
	a.syncAgentReasoning(nil)
	a.agent.showHistory = false
	a.agent.focusComposer = true
	a.saveAgentIndex()
	a.repaint()
	a.updateTitle()
}

func (a *App) selectAgentChat(id string) {
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
	needsLoad := chat.Messages == nil
	a.agent.mu.Unlock()
	if needsLoad {
		if err := a.loadAgentChat(id); err != nil {
			a.agent.mu.Lock()
			a.agent.err = err.Error()
			a.agent.mu.Unlock()
			a.repaint()
			return
		}
	}
	a.agent.mu.Lock()
	if a.agent.busy {
		a.agent.mu.Unlock()
		return
	}
	chat = a.agent.chatLocked(id)
	if chat == nil {
		a.agent.mu.Unlock()
		return
	}
	a.agent.current = id
	a.closeAgentMenus()
	if chat.HostID != "" {
		a.agent.target = chat.HostID
	} else {
		a.agent.target = ""
	}
	a.agent.err = ""
	a.agent.mu.Unlock()
	a.syncAgentModel(chat)
	a.syncAgentReasoning(chat)
	a.agent.showHistory = false
	a.agent.focusComposer = true
	a.saveAgentIndex()
	a.repaint()
	a.updateTitle()
}

func (a *App) deleteAgentChat(id string) {
	a.agent.mu.Lock()
	if a.agent.busy {
		a.agent.mu.Unlock()
		return
	}
	var imageIDs []string
	for _, c := range a.agent.chats {
		if c.ID == id {
			if c.Messages == nil {
				if loaded, err := store.LoadChat(id); err == nil {
					c.Messages = loaded.Messages
				}
			}
			for _, m := range c.Messages {
				for _, attachment := range m.Attachments {
					if attachment.Kind == agent.AttachmentImage && attachment.ID != "" {
						imageIDs = append(imageIDs, attachment.ID)
					}
				}
			}
			break
		}
	}
	out := a.agent.chats[:0]
	for _, c := range a.agent.chats {
		if c.ID != id {
			out = append(out, c)
		}
	}
	a.agent.chats = out
	wasCurrent := a.agent.current == id
	if wasCurrent {
		a.agent.current = ""
		a.agent.secrets = nil
	}
	a.agent.err = ""
	a.agent.mu.Unlock()
	if wasCurrent {
		a.agent.target = ""
		a.closeAgentMenus()
		a.syncAgentModel(nil)
		a.syncAgentReasoning(nil)
	}
	if err := store.DeleteChat(id); err != nil {
		a.agentSaveFailed(err)
	}
	for _, attachmentID := range imageIDs {
		_ = store.DeleteAttachment(attachmentID)
		delete(a.agent.previews, attachmentID)
	}
	a.saveAgentIndex()
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

// pickAgentHost points this conversation at another host, connected or not.
func (a *App) pickAgentHost(id string) {
	if _, ok := a.state.Host(id); !ok {
		return
	}
	a.agent.mu.Lock()
	if a.agent.busy {
		a.agent.mu.Unlock()
		return
	}
	chat := a.agent.chatLocked(a.agent.current)
	if chat != nil && (chat.HostID != "" || len(chat.Messages) > 0) {
		a.agent.mu.Unlock()
		return
	}
	a.agent.target = id
	a.agent.err = ""
	if chat != nil {
		chat.HostID = id
		chat.UpdatedAt = time.Now().UnixMilli()
	}
	a.agent.mu.Unlock()
	if chat != nil {
		a.saveAgentChat(chat.ID)
	}
	a.repaint()
}

// sendAgent sends text to the Agent, starting a conversation when there
// is none. It reports whether the turn started now; a host that is not
// connected asks first.
func (a *App) sendAgent(text string) bool {
	text = strings.TrimSpace(text)
	attachments := append([]agent.Attachment(nil), a.agent.attachments...)
	if text == "" && len(attachments) == 0 {
		return false
	}
	a.agent.mu.Lock()
	if a.agent.busy {
		a.agent.mu.Unlock()
		return false
	}
	a.closeAgentMenus()
	if !a.agentConfigured() {
		a.agent.err = locales.AgentNotConfigured()
		a.agent.mu.Unlock()
		a.repaint()
		return false
	}
	host, ok := a.agentTargetHost()
	if !ok {
		a.agent.err = locales.AgentPickHost()
		a.agent.mu.Unlock()
		a.repaint()
		return false
	}
	if _, _, ok := a.resolveAgentModel(a.agent.chatLocked(a.agent.current)); !ok {
		a.agent.err = locales.AgentPickModel()
		a.agent.mu.Unlock()
		a.repaint()
		return false
	}
	a.agent.mu.Unlock()
	if !a.agentHostConnected(host.ID) {
		a.agentConnect = &agentConnectState{open: true, host: host, text: text, attachments: attachments}
		a.repaint()
		return false
	}
	return a.startAgentTurn(text, attachments)
}

// startAgentTurn adds the prompt to the conversation and starts the
// engine on a host connected now.
func (a *App) startAgentTurn(text string, attachments []agent.Attachment) bool {
	a.agent.mu.Lock()
	if a.agent.busy {
		a.agent.mu.Unlock()
		return false
	}
	a.closeAgentMenus()
	if !a.agentConfigured() {
		a.agent.err = locales.AgentNotConfigured()
		a.agent.mu.Unlock()
		a.repaint()
		return false
	}
	host, ok := a.agentTargetHost()
	if !ok {
		a.agent.err = locales.AgentPickHost()
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
	provider, model, ok := a.resolveAgentModel(chat)
	if !ok {
		a.agent.err = locales.AgentPickModel()
		a.agent.mu.Unlock()
		a.repaint()
		return false
	}
	chat.HostID = host.ID
	chat.ProviderID = provider.ID
	chat.Model = model
	reasoning := normalizeReasoning(a.agent.reasoning)
	chat.Reasoning = reasoning
	a.agent.provider, a.agent.model = provider.ID, model
	a.agent.reasoning = reasoning
	now := time.Now().UnixMilli()
	chat.Messages = append(chat.Messages, &agent.Message{
		ID:          agent.NewID("msg"),
		Role:        "user",
		Content:     text,
		Attachments: attachments,
		CreatedAt:   now,
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
	a.agent.attachments = nil
	a.saveAgentChat(chatID)
	a.repaint()
	a.updateTitle()

	engine := &agent.Engine{
		Client: &llm.Client{
			BaseURL:   provider.BaseURL,
			APIKey:    a.savedSecret(agentProviderSecret(provider.ID)),
			Model:     model,
			Reasoning: reasoning,
		},
		Host:   host,
		Tools:  a.agentTools(),
		Policy: a.agentPolicy(),
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
			ReadFile: func(ctx context.Context, hostID, path string, max int64) (agent.FileData, error) {
				return a.readAgentFile(ctx, hostID, path, max)
			},
			WriteFile: func(ctx context.Context, hostID, path string, data []byte) error {
				return a.writeAgentFile(ctx, hostID, path, data)
			},
			ListDir: func(ctx context.Context, hostID, path string, max int) (agent.DirListing, error) {
				return a.listAgentDir(ctx, hostID, path, max)
			},
			Terminal: func(ctx context.Context, hostID string, max int) (string, error) {
				return a.readAgentTerminal(ctx, hostID, max)
			},
			MemoryRead: func(ctx context.Context) (string, error) {
				return a.readAgentMemory(ctx)
			},
			MemoryWrite: func(ctx context.Context, content string, appendTo bool) (int, error) {
				return a.writeAgentMemory(ctx, content, appendTo)
			},
			WebFetch: func(ctx context.Context, request agent.WebRequest) (agent.WebData, error) {
				return a.fetchAgentWeb(ctx, request)
			},
			Ask: func(ctx context.Context, request agent.AskRequest) (agent.AskResult, error) {
				return a.askAgentUser(ctx, request)
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
	a.saveAgentChat(chatID)
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
	if a.agentAsk != nil {
		ask := a.agentAsk
		a.agentAsk = nil
		select {
		case ask.answer <- agent.AskResult{Cancelled: true}:
		default:
		}
	}
	a.agent.mu.Lock()
	cancel := a.agent.cancel
	ch := a.agent.answer
	a.agent.pending = nil
	a.agent.answer = nil
	if a.agent.queued != nil {
		a.agent.queued = nil
		a.agent.busy = false
	}
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

// agentSession is the connected session a tool call runs on.
func (a *App) agentSession(ctx context.Context, hostID string) (*sshx.Session, error) {
	if a.win == nil {
		return nil, errors.New("sshgo is closing")
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
	select {
	case p := <-ch:
		return p.sess, p.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// execAgentCommand runs command on a host that is connected now.
func (a *App) execAgentCommand(ctx context.Context, hostID, command string) (agent.ToolResult, error) {
	sess, err := a.agentSession(ctx, hostID)
	if err != nil {
		return agent.ToolResult{Error: err.Error()}, nil
	}
	start := time.Now()
	runCtx, cancel := context.WithTimeout(ctx, agentTimeout)
	defer cancel()
	if err := runCtx.Err(); err != nil {
		return agent.ToolResult{Error: "The command was stopped."}, nil
	}
	res, err := sess.Exec(runCtx, command)
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

func (a *App) readAgentFile(ctx context.Context, hostID, path string, max int64) (agent.FileData, error) {
	sess, err := a.agentSession(ctx, hostID)
	if err != nil {
		return agent.FileData{}, err
	}
	runCtx, cancel := context.WithTimeout(ctx, agentFileTimeout)
	defer cancel()
	file, err := sess.ReadFile(runCtx, path, max)
	if err != nil {
		return agent.FileData{}, err
	}
	return agent.FileData{Data: file.Data, Size: file.Size, Truncated: file.Truncated}, nil
}

func (a *App) writeAgentFile(ctx context.Context, hostID, path string, data []byte) error {
	sess, err := a.agentSession(ctx, hostID)
	if err != nil {
		return err
	}
	runCtx, cancel := context.WithTimeout(ctx, agentFileTimeout)
	defer cancel()
	return sess.WriteFile(runCtx, path, data)
}

func (a *App) listAgentDir(ctx context.Context, hostID, path string, max int) (agent.DirListing, error) {
	sess, err := a.agentSession(ctx, hostID)
	if err != nil {
		return agent.DirListing{}, err
	}
	runCtx, cancel := context.WithTimeout(ctx, agentFileTimeout)
	defer cancel()
	listing, err := sess.ListDir(runCtx, path, max)
	if err != nil {
		return agent.DirListing{}, err
	}
	out := agent.DirListing{
		Entries:   make([]agent.DirEntry, 0, len(listing.Entries)),
		Truncated: listing.Truncated,
	}
	for _, entry := range listing.Entries {
		out.Entries = append(out.Entries, agent.DirEntry{
			Name: entry.Name, IsDir: entry.IsDir, Size: entry.Size,
			Mode: entry.Mode, Modified: entry.Modified,
		})
	}
	return out, nil
}

func (a *App) fetchAgentWeb(ctx context.Context, request agent.WebRequest) (agent.WebData, error) {
	runCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(runCtx, http.MethodGet, request.URL, nil)
	if err != nil {
		return agent.WebData{}, err
	}
	for name, value := range request.Headers {
		req.Header.Set(name, a.resolveAgentSecrets(value))
	}
	req.Header.Set("User-Agent", "sshgo/0.1")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return agent.WebData{}, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, agent.MaxWebBytes+1))
	if err != nil {
		return agent.WebData{}, err
	}
	truncated := len(body) > agent.MaxWebBytes
	if truncated {
		body = body[:agent.MaxWebBytes]
	}
	return agent.WebData{
		Status:      res.StatusCode,
		ContentType: res.Header.Get("Content-Type"),
		Body:        strings.ToValidUTF8(string(body), "\uFFFD"),
		Size:        int64(len(body)),
		Truncated:   truncated,
	}, nil
}

func (a *App) newAgentSecret(value string) string {
	handle := agent.NewID("sec")
	a.agent.mu.Lock()
	if a.agent.secrets == nil {
		a.agent.secrets = map[string]string{}
	}
	a.agent.secrets[handle] = value
	a.agent.mu.Unlock()
	return handle
}

func (a *App) resolveAgentSecrets(text string) string {
	a.agent.mu.Lock()
	defer a.agent.mu.Unlock()
	if len(a.agent.secrets) == 0 {
		return text
	}
	pairs := make([]string, 0, len(a.agent.secrets)*4)
	for handle, value := range a.agent.secrets {
		pairs = append(pairs, "{{"+handle+"}}", value, handle, value)
	}
	return strings.NewReplacer(pairs...).Replace(text)
}

func (a *App) readAgentMemory(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	a.agent.memoryMu.Lock()
	defer a.agent.memoryMu.Unlock()
	return store.LoadAgentMemory()
}

func (a *App) writeAgentMemory(ctx context.Context, content string, appendTo bool) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	a.agent.memoryMu.Lock()
	defer a.agent.memoryMu.Unlock()
	next := content
	if appendTo {
		old, err := store.LoadAgentMemory()
		if err != nil {
			return 0, err
		}
		if old != "" && !strings.HasSuffix(old, "\n") {
			old += "\n"
		}
		next = old + content
		if next != "" && !strings.HasSuffix(next, "\n") {
			next += "\n"
		}
	}
	if len(next) > agent.MaxMemoryBytes {
		return 0, fmt.Errorf("memory is limited to %d bytes", agent.MaxMemoryBytes)
	}
	if err := store.SaveAgentMemory(next); err != nil {
		return 0, err
	}
	return len(content), nil
}

func (a *App) readAgentTerminal(ctx context.Context, hostID string, max int) (string, error) {
	if a.win == nil {
		return "", errors.New("sshgo is closing")
	}
	type picked struct {
		text string
		err  error
	}
	ch := make(chan picked, 1)
	a.win.Update(func() {
		rt := a.runtimes[hostID]
		if rt == nil || rt.term == nil || rt.state != connConnected || rt.ssh == nil {
			ch <- picked{err: errors.New("the terminal for this host is not connected")}
			return
		}
		ch <- picked{text: rt.term.Text()}
	})
	select {
	case p := <-ch:
		return p.text, p.err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// agentToolEnabled says whether the Agent may use one tool now.
func (a *App) agentToolEnabled(name string) bool {
	if a.state.AgentNoTools {
		return false
	}
	return !slices.Contains(a.state.AgentDisabledTools, name)
}

// agentReadToolEnabled says whether any tool that only reads is on, which
// is when the "read-only tools" switch can matter.
func (a *App) agentReadToolEnabled() bool {
	for _, name := range []string{"read_file", "list_dir", "terminal_screen", "memory"} {
		if a.agentToolEnabled(name) {
			return true
		}
	}
	return false
}

// setAgentToolEnabled turns one tool on or off without touching the others.
func (a *App) setAgentToolEnabled(name string, on bool) {
	disabled := a.state.AgentDisabledTools[:0]
	for _, tool := range a.state.AgentDisabledTools {
		if tool != name {
			disabled = append(disabled, tool)
		}
	}
	if !on {
		disabled = append(disabled, name)
	}
	a.state.AgentDisabledTools = disabled
}

// agentTools are the tools this app lets the Agent use now.
func (a *App) agentTools() []agent.Tool {
	return agent.Tools(agent.ToolOptions{
		Shell:     a.agentToolEnabled("run_shell_command"),
		ReadFile:  a.agentToolEnabled("read_file"),
		WriteFile: a.agentToolEnabled("write_file"),
		ListDir:   a.agentToolEnabled("list_dir"),
		Terminal:  a.agentToolEnabled("terminal_screen"),
		Memory:    a.agentToolEnabled("memory"),
		WebFetch:  a.agentToolEnabled("web_fetch"),
		AskUser:   a.agentToolEnabled("ask_user"),
	})
}

// agentPolicy is when a tool may run without asking: read-only tools by
// their own nature, read-only shell commands by their shape.
func (a *App) agentPolicy() agent.Policy {
	if a.state.AgentNoTools {
		return agent.Policy{}
	}
	return agent.Policy{
		ReadTools:    a.state.AgentAutoRunReadTools,
		ReadCommands: a.state.AgentAutoRunSafe && a.agentToolEnabled("run_shell_command"),
	}
}

// agentHosts are the hosts the Agent may work on, connected or not.
func (a *App) agentHosts() []agent.Host {
	out := make([]agent.Host, 0, len(a.state.Hosts))
	for _, h := range a.state.Hosts {
		out = append(out, agent.Host{ID: h.ID, Name: h.Name, Address: h.Address(), OS: h.OS})
	}
	return out
}

// agentHostByID is a host the app knows, connected or not.
func (a *App) agentHostByID(id string) (agent.Host, bool) {
	h, ok := a.state.Host(id)
	if !ok {
		return agent.Host{}, false
	}
	return agent.Host{ID: h.ID, Name: h.Name, Address: h.Address(), OS: h.OS}, true
}

func (a *App) agentHostConnected(id string) bool {
	if id == "" {
		return false
	}
	rt := a.runtimes[id]
	return rt != nil && rt.state == connConnected && rt.ssh != nil
}

// agentTargetHost is the host this conversation runs on, connected or not.
func (a *App) agentTargetHost() (agent.Host, bool) {
	return a.agentHostByID(a.agent.target)
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
