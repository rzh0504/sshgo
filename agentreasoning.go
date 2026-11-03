package main

import (
	"time"

	"github.com/egoist/mygo/ui"

	"sshgo/internal/agent"
	"sshgo/locales"
)

const (
	reasoningDefault = "default"
	reasoningLow     = "low"
	reasoningMedium  = "medium"
	reasoningHigh    = "high"
)

var reasoningLevels = []string{reasoningDefault, reasoningLow, reasoningMedium, reasoningHigh}

func validReasoning(value string) bool {
	switch value {
	case reasoningDefault, reasoningLow, reasoningMedium, reasoningHigh:
		return true
	}
	return false
}

func normalizeReasoning(value string) string {
	if validReasoning(value) {
		return value
	}
	return reasoningDefault
}

func reasoningLabel(value string) string {
	switch value {
	case reasoningLow:
		return locales.AgentThinkLow()
	case reasoningMedium:
		return locales.AgentThinkMedium()
	case reasoningHigh:
		return locales.AgentThinkHigh()
	}
	return locales.AgentThinkDefault()
}

func (a *App) syncAgentReasoning(chat *agent.Chat) {
	a.agent.reasoning = normalizeReasoning(a.state.AgentDefaultReasoning)
	if chat != nil && chat.Reasoning != "" {
		a.agent.reasoning = normalizeReasoning(chat.Reasoning)
	}
}

func (a *App) pickAgentReasoning(value string) {
	value = normalizeReasoning(value)
	a.closeAgentMenus()
	a.agent.mu.Lock()
	if a.agent.busy {
		a.agent.mu.Unlock()
		return
	}
	chat := a.agent.chatLocked(a.agent.current)
	if chat != nil {
		chat.Reasoning = value
		chat.UpdatedAt = time.Now().UnixMilli()
	}
	a.agent.mu.Unlock()
	a.agent.reasoning = value
	a.state.AgentDefaultReasoning = value
	a.saveState()
	if chat != nil {
		a.saveAgentChat(chat.ID)
	}
	a.repaint()
}

func (a *App) agentReasoningPicker(c *ui.Context, pal *palette, snap agentSnap) {
	label := reasoningLabel(snap.reasoning)
	fg := pal.textMuted
	if normalizeReasoning(snap.reasoning) != reasoningDefault {
		fg = pal.text
	}
	b := ui.ButtonBase(c.Key("agent-reasoning")).Height(24).Padding(0, 4).Radius(6).Gap(4).
		AlignItems(ui.Center).Shrink(0).Label(label).Tooltip(locales.AgentThinking())
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
		ui.Icon(c, bulbIcon).Size(11, 11).TextColor(pal.textMuted).Shrink(0)
		ui.Text(c, label).FontSize(11.5).TextColor(fg).SingleLine()
		if !snap.busy {
			ui.Icon(c, chevronDownIcon).Size(10, 10).TextColor(pal.textFaint).Shrink(0)
		}
	})
	if snap.busy {
		a.closeAgentMenus()
		return
	}
	if b.Clicked() {
		open := !a.agent.reasoningMenuOpen
		a.closeAgentMenus()
		a.agent.reasoningMenuOpen = open
	}
	ui.PopoverBase(c, b, &a.agent.reasoningMenuOpen, func(panel ui.Element) {
		panel.AttachTo(b, ui.AnchorBottomRight, ui.AnchorTopRight).Margin(6, 0, 0, 0)
		a.agentReasoningMenu(c, pal, panel)
	})
}

func (a *App) agentReasoningMenu(c *ui.Context, pal *palette, panel ui.Element) {
	header := locales.AgentThinking()
	width := float32(0)
	if w, _ := c.MeasureText(0, ui.Span{Text: header, Size: 11, Weight: 600}); w > width {
		width = w
	}
	for _, value := range reasoningLevels {
		if w, _ := c.MeasureText(0, ui.Span{Text: reasoningLabel(value), Size: 12.5, Weight: 400}); w > width {
			width = w
		}
	}
	width = min(max(width+50, 140), 200)
	h := 8 + 26 + len(reasoningLevels)*30
	floating(panel.Width(width).Padding(4).Radius(10).Background(pal.popover).Clip(), pal)
	panel.Children(func() {
		ui.Scroll(c.Key("agent-reasoning-menu")).Height(float32(h)).FillWidth().Children(func() {
			ui.Column(c).Padding(4, 0).FillWidth().Children(func() {
				ui.Row(c).Height(26).Padding(0, 8).AlignItems(ui.Center).FillWidth().Children(func() {
					ui.Text(c, header).FontSize(11).FontWeight(600).TextColor(pal.textMuted)
				})
				for _, value := range reasoningLevels {
					selected := normalizeReasoning(a.agent.reasoning) == value
					row := ui.Row(c.Key("agent-reasoning-"+value)).Height(30).Padding(0, 8).
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
						ui.Text(c, reasoningLabel(value)).FontSize(12.5).TextColor(pal.text).SingleLine()
					})
					if row.Clicked() {
						a.pickAgentReasoning(value)
					}
				}
			})
		})
	})
}
