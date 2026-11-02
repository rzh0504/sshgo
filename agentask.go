package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"sshgo/internal/agent"
	"sshgo/locales"
)

type agentAskState struct {
	open    bool
	request agent.AskRequest
	answers []agentAskAnswer
	answer  chan agent.AskResult
	err     string
}

type agentAskAnswer struct {
	selected []bool
	text     string
}

func newAgentAskState(request agent.AskRequest, answer chan agent.AskResult) *agentAskState {
	state := &agentAskState{
		open:    true,
		request: request,
		answers: make([]agentAskAnswer, len(request.Questions)),
		answer:  answer,
	}
	for i, question := range request.Questions {
		state.answers[i].selected = make([]bool, len(question.Options))
	}
	return state
}

// askAgentUser shows the form and waits for the user, as the engine's Ask
// hook. It runs on the engine's goroutine; the form itself is the window's.
func (a *App) askAgentUser(ctx context.Context, request agent.AskRequest) (agent.AskResult, error) {
	if a.win == nil {
		return agent.AskResult{}, errors.New("sshgo is closing")
	}
	ch := make(chan agent.AskResult, 1)
	a.update(func() {
		a.agentAsk = newAgentAskState(request, ch)
		a.repaint()
	})
	select {
	case result := <-ch:
		return result, nil
	case <-ctx.Done():
		a.update(func() {
			if a.agentAsk != nil && a.agentAsk.answer == ch {
				a.agentAsk = nil
			}
		})
		return agent.AskResult{}, ctx.Err()
	}
}

func (k *agentAskState) result(a *App) (agent.AskResult, error) {
	answers := make([]agent.AskAnswer, 0, len(k.request.Questions))
	for i, question := range k.request.Questions {
		state := k.answers[i]
		if len(question.Options) > 0 {
			var values []string
			for j, option := range question.Options {
				if state.selected[j] {
					values = append(values, option.Value)
				}
			}
			if len(values) == 0 {
				return agent.AskResult{}, fmt.Errorf("%s", locales.AgentAskRequired())
			}
			answers = append(answers, agent.AskAnswer{ID: question.ID, Values: values})
			continue
		}
		text := strings.TrimSpace(state.text)
		if text == "" {
			return agent.AskResult{}, fmt.Errorf("%s", locales.AgentAskRequired())
		}
		if question.Secret {
			text = a.newAgentSecret(text)
		}
		answers = append(answers, agent.AskAnswer{ID: question.ID, Text: text})
	}
	return agent.AskResult{Answers: answers}, nil
}

func (a *App) agentAskView(c *ui.Context, pal *palette) {
	k := a.agentAsk
	if k == nil || !k.open {
		return
	}
	submitted := false
	var result agent.AskResult
	ui.DialogBase(c.Key("agent-ask"), &k.open, func(backdrop, panel ui.Element) {
		backdrop.Background(pal.overlay)
		floating(panel.Width(540).Padding(22).Gap(14).Radius(14).Background(pal.popover), pal).
			Label(locales.AgentAskTitle())
		dialogHead(c, pal, locales.AgentAskTitle(), locales.AgentAskDetail())
		ui.Scroll(c.Key("agent-ask-scroll")).MaxHeight(420).Children(func() {
			ui.Column(c).Gap(16).Children(func() {
				for i, question := range k.request.Questions {
					state := &k.answers[i]
					ui.Column(c).Gap(6).Children(func() {
						ui.Text(c, question.Question).FontSize(13.5).FontWeight(550).TextColor(pal.text).
							LineHeight(1.35)
						if question.Detail != "" {
							ui.Text(c, question.Detail).FontSize(12).TextColor(pal.textMuted).LineHeight(1.35)
						}
						if len(question.Options) > 0 {
							hint := locales.AgentAskChooseOne()
							if question.Multiple {
								hint = locales.AgentAskChooseMany()
							}
							ui.Text(c, hint).FontSize(11.5).TextColor(pal.textFaint)
							for j, option := range question.Options {
								selected := state.selected[j]
								row := ui.Row(c.Key("ask-"+question.ID+"-"+strconv.Itoa(j))).Height(34).
									Padding(8, 10).Gap(8).Radius(7).AlignItems(ui.Center).FillWidth()
								if selected {
									row.Background(pal.rowSelected)
								} else if row.Hovered() {
									row.Background(pal.rowHover)
								}
								row.Children(func() {
									if selected {
										ui.Icon(c, checkIcon).Size(12, 12).TextColor(pal.accent).Shrink(0)
									} else {
										ui.Box(c).Size(12, 12).Radius(3).Shrink(0).
											Border(1, pal.text.Alpha(0.3))
									}
									ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
										ui.Text(c, option.Label).FontSize(12.5).TextColor(pal.text)
										if option.Description != "" {
											ui.Text(c, option.Description).FontSize(11.5).
												TextColor(pal.textMuted).LineHeight(1.3)
										}
									})
								})
								if row.Clicked() {
									if question.Multiple {
										state.selected[j] = !state.selected[j]
									} else {
										for n := range state.selected {
											state.selected[n] = false
										}
										state.selected[j] = true
									}
								}
							}
						} else {
							placeholder := question.Placeholder
							if placeholder == "" {
								placeholder = locales.AgentAskTextPlaceholder()
								if question.Secret {
									placeholder = locales.AgentAskSecretPlaceholder()
								}
							}
							in := ui.TextInputBase(c.Key("ask-input-"+question.ID), &state.text).Height(32).
								Padding(0, 10).Radius(8).FontSize(13.5).TextColor(pal.text).
								Placeholder(placeholder).Label(question.Question)
							fieldFace(in, pal)
							focusEdge(in, in.Focused(), pal)
							if question.Secret {
								in.Password()
								ui.Text(c, locales.AgentAskSecretHint()).FontSize(11.5).
									TextColor(pal.textFaint).LineHeight(1.3)
							}
						}
					})
				}
				if k.err != "" {
					ui.Text(c, k.err).FontSize(12).TextColor(pal.danger).LineHeight(1.35)
				}
			})
		})
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if button(c, "agent-ask-cancel", locales.ActionCancel(), "esc", false, pal) ||
				c.Shortcut(0, ui.KeyEscape) {
				k.open = false
			}
			if button(c, "agent-ask-submit", locales.AgentAskSubmit(), "", true, pal) {
				res, err := k.result(a)
				if err != nil {
					k.err = err.Error()
					return
				}
				result = res
				submitted = true
				k.open = false
			}
		})
	})
	if !k.open {
		a.agentAsk = nil
		res := result
		if !submitted {
			res = agent.AskResult{Cancelled: true}
		}
		select {
		case k.answer <- res:
		default:
		}
		a.repaint()
	}
}
