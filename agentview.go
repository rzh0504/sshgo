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
	"sshgo/internal/store"
	"sshgo/locales"
)

// agentSnap is the Agent's state as one frame shows it, apart from the
// engine's goroutine.
type agentSnap struct {
	chats       []*agent.Chat
	current     string
	target      string
	provider    string
	model       string
	reasoning   string
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
		target:      a.agent.target,
		provider:    a.agent.provider,
		model:       a.agent.model,
		reasoning:   a.agent.reasoning,
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
			}).Grow(1).MinHeight(0).Padding(18, 40, 12, 40).Gap(18).Justify(ui.End)
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
		ui.Icon(c, agentIcon).Size(30, 30).TextColor(pal.accent)
		ui.Text(c, locales.AgentEmptyTitle()).FontSize(17).FontWeight(650).TextColor(pal.text)
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
	hostName := ""
	if chat.HostID != "" {
		hostName = a.agentHostName(chat.HostID)
	}
	row := ui.Row(c.Key("agent-chat-"+chat.ID)).Height(40).Padding(6, 8, 6, 8).Gap(8).
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
		ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
			ui.Text(c, title).FontSize(13).TextColor(fg).SingleLine().Ellipsis("…").
				Shrink(1).MinWidth(0).Tooltip(title)
			if hostName != "" {
				ui.Text(c, hostName).FontSize(11).TextColor(pal.textFaint).SingleLine().Ellipsis("…")
			}
		})
	})
}

func (a *App) agentMessage(c *ui.Context, pal *palette, m *agent.Message, snap agentSnap) {
	if m.Role == "user" {
		ui.Row(c).Children(func() {
			ui.Spacer(c)
			ui.Column(c).Padding(9, 13).Radius(12).Background(pal.chip).MaxWidth(560).Gap(6).
				Children(func() {
					if m.Content != "" {
						ui.Text(c, m.Content).Selectable().FontSize(13.5).LineHeight(1.45).TextColor(pal.text)
					}
					if len(m.Attachments) > 0 {
						a.agentMessageAttachments(c, pal, m.Attachments)
					}
				})
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
	hostID := call.HostID
	if hostID == "" && call.Result != nil {
		hostID = call.Result.HostID
	}
	if hostID == "" {
		hostID = snap.target
	}
	hostName := a.agentHostName(hostID)
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
	ui.Column(c).Shrink(0).Padding(10, 40, 14, 40).Gap(8).Children(func() {
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
		a.agentPendingAttachments(c, pal)
		box := ui.Column(c).FillWidth().Shrink(0).MinWidth(0).Radius(10).Clip()
		var in ui.Element
		box.Children(func() {
			in = ui.TextAreaBufferBase(c.Key("agent-input"), &a.agent.draft).Lines(3, 8).
				Padding(8, 10, 4, 10).FontSize(13.5).TextColor(pal.text).Background(ui.Transparent).
				Placeholder(locales.AgentPlaceholder()).Label(locales.AgentPlaceholder()).
				FillWidth().Bind(&a.agent.composer)
			in.OnTextKey(func(k ui.TextKey) bool {
				if k.Key != ui.KeyEnter || k.Mods != 0 {
					return false
				}
				if a.sendAgent(k.Text()) {
					k.SetText("")
				}
				return true
			})
			ui.Row(c).FillWidth().Height(32).Shrink(0).Padding(0, 8, 5, 8).Gap(6).AlignItems(ui.Center).Children(func() {
				a.agentAttachButton(c, pal)
				ui.Spacer(c)
				a.agentModelPicker(c, pal, snap)
				a.agentReasoningPicker(c, pal, snap)
				a.agentSendButton(c, pal, snap)
			})
		})
		fieldFace(box, pal)
		focusEdge(box, in.Focused(), pal)
		if a.agent.focusComposer && !snap.busy && !a.anyDialogOpen() {
			a.agent.focusComposer = false
			a.agent.composer.Focus()
		}
		ui.Row(c).FillWidth().Height(24).Shrink(0).Padding(0, 0, 0, 4).AlignItems(ui.Center).Children(func() {
			a.agentHostPicker(c, pal, snap)
		})
	})
}

func (a *App) agentAttachButton(c *ui.Context, pal *palette) {
	b := ui.ButtonBase(c.Key("agent-attach")).Size(26, 26).Radius(7).Center().
		Label(locales.AgentAttach()).Tooltip(locales.AgentAttach())
	bg := pal.text.Alpha(0.06)
	switch {
	case b.Pressed():
		bg = pal.text.Alpha(0.12)
	case b.Hovered():
		bg = pal.rowHover
	}
	b.Background(bg).TextColor(pal.textMuted)
	b.Children(func() { ui.Icon(c, attachIcon).Size(14, 14) })
	if b.Clicked() {
		go a.attachAgentFiles()
	}
}

// agentHostPicker is the host the conversation runs on, in the composer's
// footer: a menu of the hosts connected now.
func (a *App) agentHostPicker(c *ui.Context, pal *palette, snap agentSnap) {
	host, known := a.agentHostByID(snap.target)
	connected := known && a.agentHostConnected(snap.target)
	label := locales.AgentSelectHost()
	tip := locales.AgentTargetHost()
	fg := pal.attention
	if known {
		label = host.Name
		tip = host.Address
		fg = pal.textMuted
	}
	if known && !connected {
		label = host.Name + " · " + locales.AgentHostOffline()
		fg = pal.attention
	}
	b := ui.ButtonBase(c.Key("agent-host")).Height(24).Padding(0, 4).Radius(6).Gap(5).
		AlignItems(ui.Center).Shrink(1).MinWidth(0).Label(label).Tooltip(tip)
	if snap.busy {
		b.Disabled(true)
	}
	bg := ui.Transparent
	switch {
	case b.Pressed() && !snap.busy:
		bg = pal.rowHover
	case b.Hovered() && !snap.busy:
		bg = pal.rowHover
	}
	b.Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 110 * time.Millisecond}))
	if !snap.busy {
		b.Cursor(ui.CursorPointer)
	}
	b.Children(func() {
		ui.Icon(c, serverIcon).Size(12, 12).TextColor(pal.textMuted).Shrink(0)
		ui.Text(c, label).FontSize(12).TextColor(fg).SingleLine().Ellipsis("…").
			Shrink(1).MinWidth(0)
		if !snap.busy {
			ui.Icon(c, chevronDownIcon).Size(10, 10).TextColor(pal.textFaint).Shrink(0)
		}
	})
	if snap.busy {
		a.closeAgentMenus()
		return
	}
	if b.Clicked() {
		open := !a.agent.hostMenuOpen
		a.closeAgentMenus()
		a.agent.hostMenuOpen = open
	}
	ui.PopoverBase(c, b, &a.agent.hostMenuOpen, func(panel ui.Element) {
		a.agentHostMenu(c, pal, panel, snap)
	})
}

func (a *App) agentHostMenu(c *ui.Context, pal *palette, panel ui.Element, snap agentSnap) {
	hosts := a.agentHosts()
	if len(hosts) == 0 {
		label := locales.AgentNoConnectedHosts()
		w, _ := c.MeasureText(0, ui.Span{Text: label, Size: 12, Weight: 400})
		width := min(max(w+36, 140), 260)
		floating(panel.Width(width).Padding(4).Radius(10).Background(pal.popover).Clip(), pal)
		panel.Children(func() {
			ui.Text(c, label).FontSize(12).TextColor(pal.textMuted).Padding(8, 8)
		})
		return
	}
	h := 8 + 26 + len(hosts)*40
	h = min(h, 300)
	width := float32(0)
	for _, host := range hosts {
		if w, _ := c.MeasureText(0, ui.Span{Text: host.Name, Size: 12.5, Weight: 400}); w > width {
			width = w
		}
		if w, _ := c.MeasureText(0, ui.Span{Text: host.Address, Size: 11, Weight: 400}); w > width {
			width = w
		}
	}
	width = min(max(width+51, 180), 300)
	floating(panel.Width(width).Padding(4).Radius(10).Background(pal.popover).Clip(), pal)
	panel.Children(func() {
		ui.Scroll(c.Key("agent-host-menu")).Height(float32(h)).FillWidth().Children(func() {
			ui.Column(c).Padding(4, 0).FillWidth().Children(func() {
				ui.Row(c).Height(26).Padding(0, 8).AlignItems(ui.Center).FillWidth().Children(func() {
					ui.Text(c, locales.AgentConnectedHosts()).FontSize(11).FontWeight(600).TextColor(pal.textMuted)
				})
				for _, host := range hosts {
					selected := host.ID == snap.target
					row := ui.Row(c.Key("agent-host-"+host.ID)).Height(40).Padding(6, 8).Gap(8).
						Radius(6).AlignItems(ui.Center).FillWidth()
					if row.Hovered() {
						row.Background(pal.rowHover)
					}
					row.Children(func() {
						if selected {
							ui.Icon(c, checkIcon).Size(11, 11).TextColor(pal.accent).Shrink(0)
						} else {
							ui.Box(c).Size(11, 11).Shrink(0)
						}
						ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
							ui.Text(c, host.Name).FontSize(12.5).TextColor(pal.text).SingleLine().Ellipsis("…")
							ui.Text(c, host.Address).FontSize(11).TextColor(pal.textFaint).SingleLine().Ellipsis("…")
						})
					})
					if row.Clicked() {
						a.agent.hostMenuOpen = false
						a.pickAgentHost(host.ID)
					}
				}
			})
		})
	})
}

func (a *App) agentSendButton(c *ui.Context, pal *palette, snap agentSnap) {
	if snap.busy {
		b := ui.ButtonBase(c.Key("agent-stop")).Size(26, 26).Radius(7).Center().
			Label(locales.AgentStop()).Tooltip(locales.AgentStop())
		bg := pal.text.Alpha(0.08)
		if b.Pressed() {
			bg = pal.text.Alpha(0.14)
		} else if b.Hovered() {
			bg = pal.text.Alpha(0.12)
		}
		b.Background(bg).TextColor(pal.textMuted)
		b.Children(func() { ui.Icon(c, stopIcon).Size(12, 12) })
		if b.Clicked() {
			a.stopAgent()
		}
		return
	}
	active := (strings.TrimSpace(a.agent.draft.String()) != "" || len(a.agent.attachments) > 0) &&
		a.agentHostConnected(snap.target) && a.agentModelReady(snap.provider, snap.model)
	bg, fg := pal.text.Alpha(0.08), pal.textFaint
	if active {
		bg, fg = pal.accent, ui.Hex("#ffffff")
	}
	b := ui.ButtonBase(c.Key("agent-send")).Size(26, 26).Radius(7).Center().
		Label(locales.AgentSend()).Tooltip(locales.AgentSend())
	switch {
	case b.Pressed() && active:
		bg = bg.Mix(ui.Hex("#000000"), 0.08)
	case b.Hovered() && active:
		bg = bg.Mix(ui.Hex("#ffffff"), 0.08)
	case b.Hovered():
		bg = pal.text.Alpha(0.12)
	}
	b.Background(bg).TextColor(fg)
	b.Children(func() { ui.Icon(c, sendIcon).Size(14, 14) })
	if b.Clicked() && active {
		a.sendAgent(a.agent.draft.String())
	}
}

// agentModelPicker is the model this conversation uses, with the providers'
// models in a menu.
func (a *App) agentModelPicker(c *ui.Context, pal *palette, snap agentSnap) {
	label := snap.model
	fg := pal.textMuted
	tip := label
	if label == "" {
		label = locales.AgentSelectModel()
		fg = pal.attention
		tip = locales.AgentSelectModel()
	} else if p, ok := a.providerByID(snap.provider); ok {
		tip = p.Name + " · " + p.BaseURL
	}
	b := ui.ButtonBase(c.Key("agent-model")).Height(24).Padding(0, 4).Radius(6).Gap(4).
		AlignItems(ui.Center).Shrink(1).MinWidth(0).Label(label).Tooltip(tip)
	if snap.busy {
		b.Disabled(true)
	}
	bg := ui.Transparent
	switch {
	case b.Pressed() && !snap.busy:
		bg = pal.rowHover
	case b.Hovered() && !snap.busy:
		bg = pal.rowHover
	}
	b.Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 110 * time.Millisecond}))
	if !snap.busy {
		b.Cursor(ui.CursorPointer)
	}
	b.Children(func() {
		ui.Text(c, label).FontSize(11.5).TextColor(fg).SingleLine().Ellipsis("…").
			Shrink(1).MinWidth(0)
		if !snap.busy {
			ui.Icon(c, chevronDownIcon).Size(10, 10).TextColor(pal.textFaint).Shrink(0)
		}
	})
	if snap.busy {
		a.closeAgentMenus()
		return
	}
	if b.Clicked() {
		open := !a.agent.modelMenuOpen
		a.closeAgentMenus()
		a.agent.modelMenuOpen = open
	}
	ui.PopoverBase(c, b, &a.agent.modelMenuOpen, func(panel ui.Element) {
		panel.AttachTo(b, ui.AnchorBottomRight, ui.AnchorTopRight).Margin(6, 0, 0, 0)
		a.agentModelMenu(c, pal, panel, snap)
	})
}

func (a *App) agentModelMenu(c *ui.Context, pal *palette, panel ui.Element, snap agentSnap) {
	groups := make([]store.AgentProvider, 0, len(a.state.AgentProviders))
	for _, p := range a.state.AgentProviders {
		if len(p.Models) > 0 {
			groups = append(groups, p)
		}
	}
	if len(groups) == 0 {
		noModels := locales.AgentNoModels()
		openSettings := locales.AgentOpenSettings()
		w1, _ := c.MeasureText(0, ui.Span{Text: noModels, Size: 12, Weight: 400})
		w2, _ := c.MeasureText(0, ui.Span{Text: openSettings, Size: 12.5, Weight: 400})
		width := min(max(max(w1+32, w2+51), 160), 260)
		floating(panel.Width(width).Padding(4).Radius(10).Background(pal.popover).Clip(), pal)
		panel.Children(func() {
			ui.Column(c).Padding(4, 0).FillWidth().Children(func() {
				ui.Text(c, noModels).FontSize(12).TextColor(pal.textMuted).Padding(8, 8)
				row := ui.Row(c.Key("agent-model-settings")).Height(30).Padding(0, 8).Gap(7).
					Radius(6).AlignItems(ui.Center).FillWidth()
				if row.Hovered() {
					row.Background(pal.rowHover)
				}
				row.Children(func() {
					ui.Icon(c, gearIcon).Size(12, 12).TextColor(pal.accent)
					ui.Text(c, openSettings).FontSize(12.5).TextColor(pal.accent)
				})
				if row.Clicked() {
					a.closeAgentMenus()
					a.openAgentSettings()
				}
			})
		})
		return
	}
	h := 8
	width := float32(0)
	for i, p := range groups {
		if i > 0 {
			h += 9
		}
		h += 26 + len(p.Models)*30
		if w, _ := c.MeasureText(0, ui.Span{Text: p.Name, Size: 11, Weight: 600}); w > width {
			width = w
		}
		for _, model := range p.Models {
			if w, _ := c.MeasureText(0, ui.Span{Text: model, Size: 12.5, Weight: 400}); w > width {
				width = w
			}
		}
	}
	h = min(h, 320)
	width = min(max(width+50, 160), 320)
	floating(panel.Width(width).Padding(4).Radius(10).Background(pal.popover).Clip(), pal)
	panel.Children(func() {
		ui.Scroll(c.Key("agent-model-menu")).Height(float32(h)).FillWidth().Children(func() {
			ui.Column(c).Padding(4, 0).FillWidth().Children(func() {
				for gi, p := range groups {
					if gi > 0 {
						ui.Box(c).Height(1).Shrink(0).FillWidth().Background(pal.divider).Margin(4, 8, 4, 8)
					}
					ui.Row(c).Height(26).Padding(0, 8).AlignItems(ui.Center).FillWidth().Children(func() {
						ui.Text(c, p.Name).FontSize(11).FontWeight(600).TextColor(pal.textMuted).
							SingleLine().Ellipsis("…").Shrink(1).MinWidth(0)
					})
					for _, model := range p.Models {
						selected := p.ID == snap.provider && model == snap.model
						row := ui.Row(c.Key("agent-model-"+p.ID+"-"+model)).Height(30).Padding(0, 8).
							Gap(7).Radius(6).AlignItems(ui.Center).FillWidth()
						if row.Hovered() {
							row.Background(pal.rowHover)
						}
						row.Children(func() {
							if selected {
								ui.Icon(c, checkIcon).Size(11, 11).TextColor(pal.accent).Shrink(0)
							} else {
								ui.Box(c).Size(11, 11).Shrink(0)
							}
							ui.Text(c, model).FontSize(12.5).TextColor(pal.text).SingleLine().
								Ellipsis("…").Shrink(1).MinWidth(0)
						})
						if row.Clicked() {
							a.closeAgentMenus()
							a.pickAgentModel(p.ID, model)
						}
					}
				}
			})
		})
	})
}

func (a *App) agentError(c *ui.Context, pal *palette, message string) {
	ui.Row(c).Shrink(0).Gap(8).AlignItems(ui.Center).Padding(0, 40).Children(func() {
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
