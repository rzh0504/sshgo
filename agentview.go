package main

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"

	"sshgo/internal/agent"
	"sshgo/locales"
)

// agentSnap is the Agent's state as one frame shows it, apart from the
// engine's goroutine.
type agentSnap struct {
	chats       []*agent.Chat
	current     string
	messages    []*agent.Message
	busy        bool
	err         string
	streamingID string
	pending     *agent.Proposal
	configured  bool
}

func (a *App) agentSnapshot() agentSnap {
	a.agent.mu.Lock()
	defer a.agent.mu.Unlock()
	snap := agentSnap{
		current:     a.agent.current,
		busy:        a.agent.busy,
		err:         a.agent.err,
		streamingID: a.agent.streamingID,
		configured:  a.agentConfigured(),
	}
	for _, c := range a.agent.chats {
		snap.chats = append(snap.chats, &agent.Chat{
			ID: c.ID, Title: c.Title, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
		})
	}
	sort.SliceStable(snap.chats, func(i, j int) bool {
		return snap.chats[i].UpdatedAt > snap.chats[j].UpdatedAt
	})
	if chat := a.agent.chatLocked(a.agent.current); chat != nil {
		snap.messages = agent.CloneMessages(chat.Messages)
	}
	if a.agent.pending != nil {
		p := *a.agent.pending
		snap.pending = &p
	}
	return snap
}

// agentView is the Agent's page in the main card.
func (a *App) agentView(c *ui.Context, pal *palette) {
	snap := a.agentSnapshot()
	row := ui.Row(c).Fill().AlignItems(ui.Stretch)
	row.Children(func() {
		if a.agent.showHistory {
			a.agentHistory(c, pal, snap)
			ui.Box(c).Width(1).Shrink(0).Background(pal.divider)
		}
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			a.agentConversation(c, pal, snap)
		})
	})
}

func (a *App) agentConversation(c *ui.Context, pal *palette, snap agentSnap) {
	rows := agentRows(snap.messages)
	ui.Column(c).Fill().Children(func() {
		if len(rows) == 0 {
			a.agentEmpty(c, pal, snap)
		} else {
			a.agent.list.Key = func(i int) any { return rows[i].ID }
			a.agent.list.FollowEnd = true
			a.agent.list.FollowAnimation = &ui.Smooth
			ui.List(c, &a.agent.list, len(rows), func(i int) {
				a.agentMessage(c, pal, rows[i], snap)
			}).Grow(1).MinHeight(0).Padding(18, 24, 12, 24).Gap(18).Justify(ui.End)
		}
		if snap.err != "" {
			a.agentError(c, pal, snap.err)
		}
		a.agentComposer(c, pal, snap)
	})
}

// agentRows are the messages the view draws: tool answers are part of the
// call cards, not messages of their own.
func agentRows(msgs []*agent.Message) []*agent.Message {
	out := make([]*agent.Message, 0, len(msgs))
	for _, m := range msgs {
		switch m.Role {
		case "user":
			out = append(out, m)
		case "assistant":
			if m.Content != "" || len(m.ToolCalls) > 0 {
				out = append(out, m)
			}
		}
	}
	return out
}

func (a *App) agentEmpty(c *ui.Context, pal *palette, snap agentSnap) {
	ui.Column(c).Grow(1).MinHeight(0).FillWidth().Center().Gap(10).Padding(24).Children(func() {
		ui.Icon(c, sparklesIcon).Size(30, 30).TextColor(pal.accent)
		ui.Text(c, locales.AgentEmptyTitle()).FontSize(17).FontWeight(650).TextColor(pal.text)
		ui.Text(c, locales.AgentEmptyDetail()).FontSize(13).TextColor(pal.textMuted).
			MaxWidth(440).TextAlign(ui.Center).LineHeight(1.5)
		switch {
		case !snap.configured:
			ui.Text(c, locales.AgentNotConfigured()).FontSize(12.5).TextColor(pal.attention).
				MaxWidth(440).TextAlign(ui.Center).LineHeight(1.4)
			if button(c, "agent-open-settings", locales.AgentOpenSettings(), "", true, pal) {
				a.openAgentSettings()
			}
		case len(a.agentHosts()) == 0:
			ui.Text(c, locales.AgentNoHosts()).FontSize(12.5).TextColor(pal.textFaint).
				MaxWidth(440).TextAlign(ui.Center).LineHeight(1.4)
		}
	})
}

func (a *App) agentHistory(c *ui.Context, pal *palette, snap agentSnap) {
	ui.Column(c).Width(216).Shrink(0).Padding(10, 10, 8, 10).Gap(2).Children(func() {
		ui.Row(c).Height(28).Padding(0, 8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.AgentChats()).FontSize(12).FontWeight(600).TextColor(pal.textMuted)
			ui.Spacer(c)
			iconButton(c, "agent-rail-new", plusIcon, locales.AgentNewChat(), pal, a.newAgentChat)
		})
		ui.Scroll(c.Key("agent-chats")).Grow(1).Children(func() {
			if len(snap.chats) == 0 {
				ui.Text(c, locales.AgentNoChats()).FontSize(12).TextColor(pal.textFaint).Padding(6, 8)
				return
			}
			for _, chat := range snap.chats {
				a.agentHistoryRow(c, pal, chat, snap)
			}
		})
	})
}

func (a *App) agentHistoryRow(c *ui.Context, pal *palette, chat *agent.Chat, snap agentSnap) {
	sel := chat.ID == snap.current
	title := chat.Title
	if title == "" {
		title = locales.AgentUntitledChat()
	}
	row := ui.Row(c.Key("agent-chat-"+chat.ID)).Height(32).Padding(0, 8, 0, 8).Gap(8).
		AlignItems(ui.Center).Radius(7)
	if pal.dark {
		row.Border(1, ui.Transparent)
	}
	bg := ui.Transparent
	fg := pal.text
	switch {
	case sel:
		bg = pal.rowSelected
		chosen(row, pal)
	case row.Hovered():
		bg = pal.rowHover
	}
	row.Background(row.AnimateColor("bg", bg, ui.Animation{Duration: 110 * time.Millisecond}))
	if row.Clicked() && !sel && !snap.busy {
		a.selectAgentChat(chat.ID)
	}
	row.ContextMenu(func(m *ui.Menu) {
		if m.Item(locales.AgentDeleteChat()).Chosen() {
			a.openAgentDelete(chat.ID)
		}
	})
	row.Children(func() {
		ui.Icon(c, chatIcon).Size(13, 13).TextColor(pal.textMuted).Shrink(0)
		ui.Text(c, title).FontSize(13).TextColor(fg).SingleLine().Ellipsis("…").
			Shrink(1).MinWidth(0).Tooltip(title)
	})
}

func (a *App) agentMessage(c *ui.Context, pal *palette, m *agent.Message, snap agentSnap) {
	if m.Role == "user" {
		ui.Row(c).Children(func() {
			ui.Spacer(c)
			ui.Text(c, m.Content).Selectable().FontSize(13.5).LineHeight(1.45).TextColor(pal.text).
				Padding(9, 13).Radius(12).Background(pal.chip).MaxWidth(560)
		})
		return
	}
	ui.Column(c).MaxWidth(760).Gap(10).Children(func() {
		if m.Content != "" {
			if snap.busy && snap.streamingID == m.ID {
				markdown.Stream(c, m.Content, &a.agent.md)
			} else {
				markdown.View(c, m.Content, &a.agent.md)
			}
		}
		for _, call := range m.ToolCalls {
			a.agentCallCard(c, pal, call, snap)
		}
	})
}

func (a *App) agentCallCard(c *ui.Context, pal *palette, call *agent.ToolCall, snap agentSnap) {
	args, _ := agent.ParseShellArgs(call.Arguments)
	command := args.Command
	if command == "" {
		command = strings.TrimSpace(call.Arguments)
	}
	risk := agent.Classify(command)
	if args.Destructive {
		risk = agent.RiskDestructive
	}
	hostName := a.agentHostName(args.HostID)
	badge, badgeColor := agentRiskBadge(pal, risk)

	card := ui.Column(c).Radius(10).Padding(10, 12).Gap(7).Background(pal.chip)
	if pal.dark {
		card.Border(1, pal.text.Alpha(0.10))
	} else {
		card.Border(1, pal.text.Alpha(0.07))
	}
	card.Children(func() {
		ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, terminalIcon).Size(13, 13).TextColor(pal.textMuted).Shrink(0)
			ui.Text(c, locales.AgentRunTool()).FontSize(12.5).FontWeight(600).TextColor(pal.text).SingleLine()
			if hostName != "" {
				ui.Text(c, hostName).FontSize(11.5).TextColor(pal.textMuted).SingleLine().Shrink(1).MinWidth(0)
			}
			ui.Spacer(c)
			ui.Text(c, badge).FontSize(10.5).FontWeight(600).TextColor(badgeColor).
				Background(badgeColor.Alpha(0.12)).Padding(1, 6).Radius(4)
		})
		if command != "" {
			box := ui.Box(c).Padding(8, 10).Radius(7).Background(pal.control)
			box.Border(1, pal.text.Alpha(0.08))
			box.Children(func() {
				ui.Text(c, command).Font("monospace").FontSize(12.5).TextColor(pal.text).
					LineHeight(1.4).Selectable().Wrap()
			})
		}
		if args.Description != "" && args.Description != command {
			ui.Text(c, args.Description).FontSize(12).TextColor(pal.textMuted).LineHeight(1.4)
		}
		switch call.Status {
		case agent.CallPending:
			ui.Row(c).Gap(8).AlignItems(ui.Center).Margin(2, 0, 0, 0).Children(func() {
				if snap.pending != nil && snap.pending.CallID == call.ID {
					if button(c, "agent-run-"+call.ID, locales.AgentRun(), "", true, pal) {
						a.answerAgentCall(call.ID, agent.DecisionRun)
					}
					if button(c, "agent-skip-"+call.ID, locales.AgentSkip(), "", false, pal) {
						a.answerAgentCall(call.ID, agent.DecisionDeny)
					}
					ui.Spacer(c)
					ui.Text(c, locales.AgentReviewHint()).FontSize(11.5).TextColor(pal.textFaint).
						SingleLine().Ellipsis("…").Shrink(1).MinWidth(0)
				} else {
					ui.Spinner(c).Size(11, 11)
					ui.Text(c, locales.AgentWaiting()).FontSize(12).TextColor(pal.textMuted)
				}
			})
		case agent.CallRunning:
			ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
				ui.Spinner(c).Size(11, 11)
				ui.Text(c, locales.AgentRunning()).FontSize(12).TextColor(pal.textMuted)
			})
		case agent.CallDenied:
			ui.Text(c, locales.AgentDenied()).FontSize(12).TextColor(pal.textFaint)
		default:
			a.agentCallResult(c, pal, call)
		}
	})
}

func (a *App) agentCallResult(c *ui.Context, pal *palette, call *agent.ToolCall) {
	res := call.Result
	if res == nil {
		return
	}
	out := agentToolOutput(res)
	meta := ""
	if call.Status == agent.CallError && res.Error != "" {
		meta = res.Error
	} else if res.DurationMS > 0 {
		meta = fmt.Sprintf("%s · %.1fs", locales.AgentExitPrefix()+strconv.Itoa(res.ExitCode), float64(res.DurationMS)/1000)
	} else {
		meta = locales.AgentExitPrefix() + strconv.Itoa(res.ExitCode)
	}
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, meta).FontSize(11.5).TextColor(pal.textFaint).SingleLine().Ellipsis("…").Shrink(1).MinWidth(0)
	})
	if out == "" {
		return
	}
	long := len([]rune(out)) > agentPreviewChars
	expanded := a.agent.expanded[call.ID]
	shown := out
	if long && !expanded {
		shown = string([]rune(out)[:agentPreviewChars]) + "\n…"
	}
	box := ui.Box(c).Padding(8, 10).Radius(7).Background(pal.control)
	box.Border(1, pal.text.Alpha(0.08))
	box.Children(func() {
		ui.Text(c, shown).Font("monospace").FontSize(12).TextColor(pal.text).
			LineHeight(1.4).Selectable().Wrap()
	})
	if long {
		label := locales.AgentShowAll()
		if expanded {
			label = locales.AgentShowLess()
		}
		b := ui.ButtonBase(c.Key("agent-output-"+call.ID)).Height(22).Padding(0, 7).Radius(5).Center().Label(label)
		b.TextColor(pal.accent)
		b.Children(func() {
			ui.Text(c, label).FontSize(11.5).TextColor(pal.accent)
		})
		if b.Clicked() {
			if a.agent.expanded == nil {
				a.agent.expanded = map[string]bool{}
			}
			a.agent.expanded[call.ID] = !expanded
		}
	}
}

const agentPreviewChars = 1600

func (a *App) agentComposer(c *ui.Context, pal *palette, snap agentSnap) {
	ui.Column(c).Shrink(0).Padding(10, 18, 14, 18).Gap(8).Children(func() {
		if snap.busy {
			ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
				ui.Spinner(c).Size(11, 11)
				label := locales.AgentWorking()
				if snap.pending != nil {
					label = locales.AgentWaitingApproval()
				}
				ui.Text(c, label).FontSize(12).TextColor(pal.textMuted)
			})
		}
		ui.Row(c).Gap(8).AlignItems(ui.End).Children(func() {
			in := ui.TextAreaBuffer(c.Key("agent-input"), &a.agent.draft).Lines(1, 6).
				Padding(8, 10).Radius(10).FontSize(13.5).TextColor(pal.text).
				Placeholder(locales.AgentPlaceholder()).Label(locales.AgentPlaceholder()).Grow(1).MinWidth(0).
				Bind(&a.agent.composer)
			fieldFace(in, pal)
			focusEdge(in, in.Focused(), pal)
			if a.agent.focusComposer && !snap.busy && !a.anyDialogOpen() {
				a.agent.focusComposer = false
				a.agent.composer.Focus()
			}
			in.OnTextKey(func(k ui.TextKey) bool {
				if k.Key != ui.KeyEnter || k.Mods != 0 {
					return false
				}
				if a.sendAgent(k.Text()) {
					k.SetText("")
				}
				return true
			})
			if snap.busy {
				if button(c, "agent-stop", locales.AgentStop(), "", false, pal) {
					a.stopAgent()
				}
				return
			}
			if button(c, "agent-send", locales.AgentSend(), "↵", true, pal) {
				a.sendAgent(a.agent.draft.String())
			}
		})
	})
}

func (a *App) agentError(c *ui.Context, pal *palette, message string) {
	ui.Row(c).Shrink(0).Gap(8).AlignItems(ui.Center).Padding(0, 18).Children(func() {
		ui.Icon(c, infoIcon).Size(13, 13).TextColor(pal.danger).Shrink(0)
		ui.Text(c, message).FontSize(12).TextColor(pal.danger).LineHeight(1.35).
			Selectable().Grow(1).MinWidth(0)
		iconButton(c, "agent-clear-error", closeIcon, locales.AgentDismiss(), pal, func() {
			a.agent.mu.Lock()
			a.agent.err = ""
			a.agent.mu.Unlock()
			a.repaint()
		})
	})
}

func (a *App) agentHostName(id string) string {
	if id == "" {
		id = a.agentDefaultHost()
	}
	for _, h := range a.state.Hosts {
		if h.ID == id {
			return h.Name
		}
	}
	return ""
}

func agentRiskBadge(pal *palette, risk agent.Risk) (string, ui.Color) {
	switch risk {
	case agent.RiskReadOnly:
		return locales.AgentRiskReadOnly(), pal.textMuted
	case agent.RiskCaution:
		return locales.AgentRiskCaution(), pal.attention
	case agent.RiskDestructive:
		return locales.AgentRiskDestructive(), pal.danger
	}
	return locales.AgentRiskUnknown(), pal.attention
}

func agentToolOutput(res *agent.ToolResult) string {
	if res == nil {
		return ""
	}
	var b strings.Builder
	if res.Stdout != "" {
		b.WriteString(res.Stdout)
		if !strings.HasSuffix(res.Stdout, "\n") {
			b.WriteString("\n")
		}
	}
	if res.Stderr != "" {
		b.WriteString(res.Stderr)
		if !strings.HasSuffix(res.Stderr, "\n") {
			b.WriteString("\n")
		}
	}
	if res.Error != "" {
		b.WriteString(res.Error)
		b.WriteString("\n")
	}
	return strings.TrimSpace(stripANSI(b.String()))
}

var ansiPattern = regexp.MustCompile("\x1b\\[[0-9;?]*[ -/]*[@-~]")

func stripANSI(s string) string { return ansiPattern.ReplaceAllString(s, "") }

func (a *App) agentDeleteView(c *ui.Context, pal *palette) {
	d := a.agentDelete
	if d == nil || !d.open {
		return
	}
	confirmed := false
	ui.DialogBase(c.Key("agent-delete"), &d.open, func(backdrop, panel ui.Element) {
		backdrop.Background(pal.overlay)
		floating(panel.Width(420).Padding(22).Gap(14).Radius(14).Background(pal.popover), pal).
			Label(locales.AgentDeleteChat())
		dialogHead(c, pal, locales.AgentDeleteChat(), locales.AgentDeleteDetail(d.title))
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if button(c, "agent-delete-cancel", locales.ActionCancel(), "esc", false, pal) || c.Shortcut(0, ui.KeyEscape) {
				d.open = false
			}
			if dangerButton(c, "agent-delete-confirm", locales.ActionDelete(), pal) {
				confirmed = true
				d.open = false
			}
		})
	})
	if !d.open {
		a.agentDelete = nil
		if confirmed {
			a.deleteAgentChat(d.id)
		}
	}
}
