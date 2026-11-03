package main

import (
	"context"
	"errors"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"sshgo/internal/agent"
	"sshgo/internal/llm"
	"sshgo/internal/store"
	"sshgo/locales"
)

const agentDefaultProviderID = "default"

// providerSecret is where a provider's API key lives. The provider read
// from the old single-endpoint settings keeps the old entry.
func agentProviderSecret(id string) string {
	if id == "" {
		return ""
	}
	if id == agentDefaultProviderID {
		return agentSecretKey
	}
	return "agent-provider-" + id
}

// loadAgentProviders reads the provider list, turning the old single
// endpoint into the first provider without writing anything until the
// settings are saved.
func (a *App) loadAgentProviders() {
	if len(a.state.AgentProviders) == 0 {
		base := strings.TrimSpace(a.state.AgentBaseURL)
		model := strings.TrimSpace(a.state.AgentModel)
		if base != "" && model != "" {
			a.state.AgentProviders = []store.AgentProvider{{
				ID:      agentDefaultProviderID,
				Name:    providerName(base),
				BaseURL: base,
				Models:  []string{model},
			}}
			a.state.AgentDefaultProvider = agentDefaultProviderID
			a.state.AgentDefaultModel = model
		}
	}
	a.ensureAgentDefaults()
	chat := a.currentAgentChat()
	a.syncAgentModel(chat)
	a.syncAgentReasoning(chat)
}

func providerName(base string) string {
	if u, err := url.Parse(base); err == nil && u.Host != "" {
		return u.Host
	}
	return "Provider"
}

func (a *App) providerByID(id string) (store.AgentProvider, bool) {
	for _, p := range a.state.AgentProviders {
		if p.ID == id {
			return p, true
		}
	}
	return store.AgentProvider{}, false
}

// ensureAgentDefaults makes the default provider and model point at
// something the list still has.
func (a *App) ensureAgentDefaults() {
	if len(a.state.AgentProviders) == 0 {
		a.state.AgentDefaultProvider, a.state.AgentDefaultModel = "", ""
		a.state.AgentTitleProvider, a.state.AgentTitleModel = "", ""
		return
	}
	p, ok := a.providerByID(a.state.AgentDefaultProvider)
	if !ok || len(p.Models) == 0 {
		ok = false
		for _, candidate := range a.state.AgentProviders {
			if len(candidate.Models) == 0 {
				continue
			}
			p, ok = candidate, true
			a.state.AgentDefaultProvider = candidate.ID
			break
		}
		if !ok {
			a.state.AgentDefaultProvider, a.state.AgentDefaultModel = "", ""
			return
		}
	}
	if a.state.AgentDefaultModel == "" || !slices.Contains(p.Models, a.state.AgentDefaultModel) {
		a.state.AgentDefaultModel = p.Models[0]
	}
	if a.state.AgentTitleModel != "" && !a.agentModelReady(a.state.AgentTitleProvider, a.state.AgentTitleModel) {
		a.state.AgentTitleProvider, a.state.AgentTitleModel = "", ""
	}
}

// agentModelReady reports whether a provider and model can be used now.
func (a *App) agentModelReady(providerID, model string) bool {
	p, ok := a.providerByID(providerID)
	if !ok || strings.TrimSpace(p.BaseURL) == "" || strings.TrimSpace(model) == "" {
		return false
	}
	return slices.Contains(p.Models, model)
}

func (a *App) agentConfigured() bool {
	return a.agentModelReady(a.agent.provider, a.agent.model)
}

// resolveAgentModel picks the model for a turn: the conversation's own,
// then the composer's, then the default.
func (a *App) resolveAgentModel(chat *agent.Chat) (store.AgentProvider, string, bool) {
	candidates := make([][2]string, 0, 3)
	if chat != nil {
		candidates = append(candidates, [2]string{chat.ProviderID, chat.Model})
	}
	candidates = append(candidates,
		[2]string{a.agent.provider, a.agent.model},
		[2]string{a.state.AgentDefaultProvider, a.state.AgentDefaultModel},
	)
	for _, candidate := range candidates {
		if a.agentModelReady(candidate[0], candidate[1]) {
			p, _ := a.providerByID(candidate[0])
			return p, candidate[1], true
		}
	}
	return store.AgentProvider{}, "", false
}

// resolveAgentTitleModel picks the model that names conversations: the
// chosen one while it is ready, else the conversation's own.
func (a *App) resolveAgentTitleModel(chat *agent.Chat) (store.AgentProvider, string, bool) {
	if a.agentModelReady(a.state.AgentTitleProvider, a.state.AgentTitleModel) {
		p, _ := a.providerByID(a.state.AgentTitleProvider)
		return p, a.state.AgentTitleModel, true
	}
	return a.resolveAgentModel(chat)
}

func (a *App) syncAgentModel(chat *agent.Chat) {
	if chat != nil && a.agentModelReady(chat.ProviderID, chat.Model) {
		a.agent.provider, a.agent.model = chat.ProviderID, chat.Model
		return
	}
	a.agent.provider, a.agent.model = a.state.AgentDefaultProvider, a.state.AgentDefaultModel
}

func (a *App) currentAgentChat() *agent.Chat {
	a.agent.mu.Lock()
	defer a.agent.mu.Unlock()
	return a.agent.chatLocked(a.agent.current)
}

// pickAgentModel makes a model the conversation's and the next new
// conversation's default.
func (a *App) pickAgentModel(providerID, model string) {
	if !a.agentModelReady(providerID, model) {
		return
	}
	a.closeAgentMenus()
	a.agent.mu.Lock()
	if a.agent.busy {
		a.agent.mu.Unlock()
		return
	}
	chat := a.agent.chatLocked(a.agent.current)
	if chat != nil {
		chat.ProviderID = providerID
		chat.Model = model
		chat.UpdatedAt = time.Now().UnixMilli()
	}
	a.agent.mu.Unlock()
	a.agent.provider, a.agent.model = providerID, model
	a.state.AgentDefaultProvider, a.state.AgentDefaultModel = providerID, model
	a.state.AgentBaseURL, a.state.AgentModel = "", ""
	a.saveState()
	if chat != nil {
		a.saveAgentChat(chat.ID)
	}
	a.repaint()
	a.repaintSettings()
}

type providerEditState struct {
	open          bool
	id            string
	name          string
	endpoint      string
	models        []string
	detected      []string
	manual        ui.TextBuffer
	key           string
	err           string
	detecting     bool
	confirmDelete bool
}

func (a *App) openAgentProvider(id string) {
	pe := &providerEditState{open: true, id: id}
	if id != "" {
		p, ok := a.providerByID(id)
		if !ok {
			return
		}
		pe.name = p.Name
		pe.endpoint = p.BaseURL
		pe.models = append([]string(nil), p.Models...)
		pe.key = a.savedSecret(agentProviderSecret(id))
	}
	a.providerEdit = pe
	a.repaintSettings()
}

// detectAgentModels asks the endpoint for its models, with the key as it is
// in the form, and fills the model list.
func (a *App) detectAgentModels(pe *providerEditState) {
	if pe.detecting {
		return
	}
	endpoint := strings.TrimSpace(pe.endpoint)
	if endpoint == "" {
		pe.err = locales.AgentProviderEndpointRequired()
		a.repaintSettings()
		return
	}
	win := a.setWin
	if win == nil {
		return
	}
	key := strings.TrimSpace(pe.key)
	pe.detecting = true
	pe.err = ""
	a.repaintSettings()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		models, err := (&llm.Client{BaseURL: endpoint, APIKey: key}).Models(ctx)
		win.Update(func() {
			if a.providerEdit != pe {
				return
			}
			pe.detecting = false
			switch {
			case err != nil:
				pe.err = err.Error()
			case len(models) == 0:
				pe.err = locales.AgentProviderNoModelsFound()
			default:
				pe.detected = models
				pe.err = ""
			}
			a.repaintSettings()
		})
	}()
}

func (a *App) saveAgentProvider(pe *providerEditState) string {
	name := strings.TrimSpace(pe.name)
	endpoint := strings.TrimSpace(pe.endpoint)
	models := append([]string(nil), pe.models...)
	switch {
	case name == "":
		return locales.AgentProviderNameRequired()
	case endpoint == "":
		return locales.AgentProviderEndpointRequired()
	case len(models) == 0:
		return locales.AgentProviderModelsRequired()
	}
	provider := store.AgentProvider{ID: pe.id, Name: name, BaseURL: endpoint, Models: models}
	if provider.ID == "" {
		provider.ID = agent.NewID("prov")
	}
	replaced := false
	for i := range a.state.AgentProviders {
		if a.state.AgentProviders[i].ID == provider.ID {
			a.state.AgentProviders[i] = provider
			replaced = true
			break
		}
	}
	if !replaced {
		a.state.AgentProviders = append(a.state.AgentProviders, provider)
	}
	key := strings.TrimSpace(pe.key)
	var err error
	if key == "" {
		err = mygo.Secrets.Delete(agentProviderSecret(provider.ID))
		if errors.Is(err, mygo.ErrSecretNotFound) || errors.Is(err, errors.ErrUnsupported) {
			err = nil
		}
	} else {
		err = mygo.Secrets.Set(agentProviderSecret(provider.ID), []byte(key))
	}
	if err != nil {
		return err.Error()
	}
	a.state.AgentBaseURL, a.state.AgentModel = "", ""
	a.ensureAgentDefaults()
	a.syncAgentModel(a.currentAgentChat())
	a.saveState()
	a.repaint()
	a.repaintSettings()
	return ""
}

func (a *App) deleteAgentProvider(pe *providerEditState) {
	if pe.id == "" {
		return
	}
	out := a.state.AgentProviders[:0]
	for _, p := range a.state.AgentProviders {
		if p.ID != pe.id {
			out = append(out, p)
		}
	}
	a.state.AgentProviders = out
	_ = mygo.Secrets.Delete(agentProviderSecret(pe.id))
	a.ensureAgentDefaults()
	a.syncAgentModel(a.currentAgentChat())
	a.saveState()
	a.repaint()
	a.repaintSettings()
}

func (a *App) repaintSettings() {
	if a.setWin != nil {
		a.setWin.Invalidate()
	}
}

func (a *App) agentProviderSettings(c *ui.Context, pal *palette) {
	settingsGroup(c, pal, locales.AgentProviderGroup(), "", func() {
		if len(a.state.AgentProviders) == 0 {
			ui.Text(c, locales.AgentNoProviders()).FontSize(12.5).TextColor(pal.textMuted).Padding(11, 0)
		}
		for _, p := range a.state.AgentProviders {
			a.agentProviderRow(c, pal, p)
		}
		ui.Row(c).Padding(11, 0).Children(func() {
			if button(c, "agent-provider-add", locales.AgentAddProvider(), "", false, pal) {
				a.openAgentProvider("")
			}
		})
	})
}

func (a *App) agentProviderRow(c *ui.Context, pal *palette, p store.AgentProvider) {
	detail := providerName(p.BaseURL)
	if len(p.Models) > 0 {
		detail += " · " + locales.AgentModelsCount(len(p.Models))
	}
	if p.ID == a.state.AgentDefaultProvider {
		detail = locales.AgentDefaultProvider() + " · " + detail
	}
	settingRow(c, pal, p.Name, detail, func() {
		if button(c, "agent-provider-edit-"+p.ID, locales.AgentEditProvider(), "", false, pal) {
			a.openAgentProvider(p.ID)
		}
	})
}

// agentTitleModelRow chooses the model that names conversations, the
// conversation's own by default.
func (a *App) agentTitleModelRow(c *ui.Context, pal *palette) {
	settingRow(c, pal, locales.SetAgentTitleModel(), locales.SetAgentTitleModelDetail(), func() {
		label := locales.AgentTitleModelSame()
		if a.state.AgentTitleModel != "" {
			label = a.state.AgentTitleModel
		}
		b := ui.ButtonBase(c.Key("agent-title-model")).Height(26).Padding(0, 8).Radius(6).Gap(5).
			AlignItems(ui.Center).MaxWidth(260).Label(label).Cursor(ui.CursorPointer)
		bg := pal.control
		switch {
		case b.Pressed():
			bg = bg.Mix(ui.Hex("#000000"), 0.08)
		case b.Hovered():
			bg = pal.controlHover
		}
		b.Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 90 * time.Millisecond}))
		b.Children(func() {
			ui.Text(c, label).FontSize(12.5).TextColor(pal.text).SingleLine().Ellipsis("…").Shrink(1).MinWidth(0)
			ui.Icon(c, chevronDownIcon).Size(10, 10).TextColor(pal.textFaint).Shrink(0)
		})
		if b.Clicked() {
			a.ss.titleModelOpen = !a.ss.titleModelOpen
		}
		ui.PopoverBase(c, b, &a.ss.titleModelOpen, func(panel ui.Element) {
			panel.AttachTo(b, ui.AnchorBottomRight, ui.AnchorTopRight).Margin(6, 0, 0, 0)
			a.agentTitleModelMenu(c, pal, panel)
		})
	})
}

func (a *App) agentTitleModelMenu(c *ui.Context, pal *palette, panel ui.Element) {
	same := locales.AgentTitleModelSame()
	width, _ := c.MeasureText(0, ui.Span{Text: same, Size: 12.5, Weight: 400})
	h := float32(38)
	for _, p := range a.state.AgentProviders {
		if len(p.Models) == 0 {
			continue
		}
		if w, _ := c.MeasureText(0, ui.Span{Text: p.Name, Size: 11, Weight: 600}); w > width {
			width = w
		}
		h += 9 + 26 + float32(len(p.Models))*30
		for _, model := range p.Models {
			if w, _ := c.MeasureText(0, ui.Span{Text: model, Size: 12.5, Weight: 400}); w > width {
				width = w
			}
		}
	}
	width = min(max(width+50, 140), 320)
	h = min(h, 360)
	choose := func(providerID, model string) {
		a.ss.titleModelOpen = false
		a.state.AgentTitleProvider, a.state.AgentTitleModel = providerID, model
		a.saveState()
	}
	floating(panel.Width(width).Padding(4).Radius(10).Background(pal.popover).Clip(), pal)
	panel.Children(func() {
		ui.Scroll(c.Key("agent-title-model-menu")).Height(h).FillWidth().Children(func() {
			ui.Column(c).Padding(4, 0).FillWidth().Children(func() {
				sameRow := ui.Row(c.Key("agent-title-model-same")).Height(30).Padding(0, 8).
					Gap(7).Radius(6).AlignItems(ui.Center).FillWidth().Cursor(ui.CursorPointer)
				if sameRow.Hovered() {
					sameRow.Background(pal.rowHover)
				}
				sameRow.Children(func() {
					if a.state.AgentTitleModel == "" {
						ui.Icon(c, checkIcon).Size(11, 11).TextColor(pal.accent).Shrink(0)
					} else {
						ui.Box(c).Size(11, 11).Shrink(0)
					}
					ui.Text(c, same).FontSize(12.5).TextColor(pal.text).SingleLine().
						Ellipsis("…").Shrink(1).MinWidth(0)
				})
				if sameRow.Clicked() {
					choose("", "")
				}
				for _, p := range a.state.AgentProviders {
					if len(p.Models) == 0 {
						continue
					}
					ui.Box(c).Height(1).Shrink(0).FillWidth().Background(pal.divider).Margin(4, 8, 4, 8)
					ui.Row(c).Height(26).Padding(0, 8).AlignItems(ui.Center).FillWidth().Children(func() {
						ui.Text(c, p.Name).FontSize(11).FontWeight(600).TextColor(pal.textMuted).
							SingleLine().Ellipsis("…").Shrink(1).MinWidth(0)
					})
					for _, model := range p.Models {
						selected := a.state.AgentTitleProvider == p.ID && a.state.AgentTitleModel == model
						row := ui.Row(c.Key("agent-title-model-"+p.ID+"-"+model)).Height(30).Padding(0, 8).
							Gap(7).Radius(6).AlignItems(ui.Center).FillWidth().Cursor(ui.CursorPointer)
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
							choose(p.ID, model)
						}
					}
				}
			})
		})
	})
}

func (a *App) agentProviderView(c *ui.Context, pal *palette) {
	pe := a.providerEdit
	if pe == nil || !pe.open {
		return
	}
	submitted, deleting := false, false
	ui.DialogBase(c.Key("agent-provider"), &pe.open, func(backdrop, panel ui.Element) {
		backdrop.Background(pal.overlay)
		title := locales.AgentProviderNew()
		if pe.id != "" {
			title = locales.AgentProviderEditTitle()
		}
		floating(panel.Width(460).Padding(22).Gap(12).Radius(14).Background(pal.popover), pal).Label(title)
		if pe.confirmDelete {
			dialogHead(c, pal, title, locales.AgentProviderDeleteConfirm(pe.name))
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if button(c, "agent-provider-delete-cancel", locales.ActionCancel(), "esc", false, pal) ||
					c.Shortcut(0, ui.KeyEscape) {
					pe.confirmDelete = false
				}
				if dangerButton(c, "agent-provider-delete-confirm", locales.AgentProviderDelete(), pal) {
					deleting = true
					pe.open = false
				}
			})
			return
		}
		dialogHead(c, pal, title, locales.AgentProviderDetail())
		ui.Column(c).Gap(5).Children(func() {
			formField(c, "agent-provider-name", locales.AgentProviderName(),
				locales.AgentProviderNamePlaceholder(), &pe.name, pal, false)
		})
		ui.Column(c).Gap(5).Children(func() {
			formField(c, "agent-provider-endpoint", locales.AgentProviderEndpoint(),
				locales.AgentProviderEndpointPlaceholder(), &pe.endpoint, pal, false)
		})
		ui.Column(c).Gap(5).Children(func() {
			formField(c, "agent-provider-key", locales.AgentProviderKey(),
				locales.AgentProviderKeyPlaceholder(), &pe.key, pal, true)
		})
		ui.Column(c).Gap(5).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Text(c, locales.AgentProviderModels()).FontSize(12).FontWeight(550).TextColor(pal.textMuted)
				ui.Spacer(c)
				if pe.detecting {
					ui.Spinner(c).Size(11, 11)
					ui.Text(c, locales.AgentProviderDetecting()).FontSize(11.5).TextColor(pal.textMuted)
				} else {
					b := ui.ButtonBase(c.Key("agent-provider-detect")).Height(22).Padding(0, 6).Radius(5).
						Gap(4).AlignItems(ui.Center).Label(locales.AgentProviderDetect()).
						Tooltip(locales.AgentProviderDetect()).Background(ui.Transparent).Cursor(ui.CursorPointer)
					if b.Hovered() {
						b.Background(pal.rowHover)
					}
					b.Children(func() {
						ui.Icon(c, retryIcon).Size(11, 11).TextColor(pal.accent)
						ui.Text(c, locales.AgentProviderDetect()).FontSize(11.5).TextColor(pal.accent)
					})
					if b.Clicked() {
						a.detectAgentModels(pe)
					}
				}
			})
			candidates := append([]string(nil), pe.detected...)
			for _, m := range pe.models {
				if !slices.Contains(candidates, m) {
					candidates = append(candidates, m)
				}
			}
			addModel := func() {
				m := strings.TrimSpace(pe.manual.String())
				if m == "" {
					return
				}
				if !slices.Contains(pe.models, m) {
					pe.models = append(pe.models, m)
				}
				if !slices.Contains(pe.detected, m) {
					pe.detected = append(pe.detected, m)
				}
				pe.manual.Set("")
			}
			if len(candidates) > 0 {
				h := float32(min(len(candidates)*30+8, 140))
				list := ui.Scroll(c.Key("agent-provider-models")).Height(h).Shrink(0).Radius(8).
					Background(pal.chip).Border(1, pal.text.Alpha(0.08))
				list.Children(func() {
					ui.Column(c).Padding(4, 0).Children(func() {
						for _, model := range candidates {
							on := slices.Contains(pe.models, model)
							row := ui.CheckboxBase(c, &on).Height(30).Padding(0, 8).Gap(8).
								Radius(6).AlignItems(ui.Center).Cursor(ui.CursorPointer)
							if row.Hovered() {
								row.Background(pal.rowHover)
							}
							row.Children(func() {
								box := ui.Box(c).Size(16, 16).Radius(4).Shrink(0).Center()
								if on {
									box.Background(pal.accent)
									box.Children(func() {
										ui.Icon(c, checkIcon).Size(10, 10).TextColor(ui.Hex("#ffffff"))
									})
								} else {
									box.Border(1, pal.text.Alpha(0.22))
								}
								ui.Text(c, model).FontSize(12.5).TextColor(pal.text).SingleLine().
									Ellipsis("…").Shrink(1).MinWidth(0)
							})
							if row.Changed() {
								if on {
									if !slices.Contains(pe.models, model) {
										pe.models = append(pe.models, model)
									}
								} else {
									pe.models = slices.DeleteFunc(pe.models, func(v string) bool { return v == model })
								}
							}
						}
					})
				})
			}
			ui.Row(c).Gap(8).AlignItems(ui.End).Children(func() {
				in := ui.TextInputBufferBase(c.Key("agent-provider-model-input"), &pe.manual).
					Grow(1).MinWidth(0).Height(30).Padding(0, 10).Radius(8).FontSize(13).
					TextColor(pal.text).Placeholder(locales.AgentProviderModelPlaceholder()).
					Label(locales.AgentProviderModelPlaceholder())
				fieldFace(in, pal)
				focusEdge(in, in.Focused(), pal)
				if in.Submitted() {
					addModel()
				}
				if button(c, "agent-provider-model-add", locales.AgentProviderAddModel(), "", false, pal) {
					addModel()
				}
			})
		})
		if pe.err != "" {
			ui.Text(c, pe.err).FontSize(12.5).TextColor(pal.danger).LineHeight(1.35)
		}
		ui.Row(c).Gap(8).Justify(ui.SpaceBetween).Margin(6, 0, 0, 0).Children(func() {
			if pe.id != "" {
				if dangerButton(c, "agent-provider-delete", locales.AgentProviderDelete(), pal) {
					pe.confirmDelete = true
				}
			} else {
				ui.Spacer(c)
			}
			ui.Row(c).Gap(8).Children(func() {
				if button(c, "agent-provider-cancel", locales.ActionCancel(), "esc", false, pal) ||
					c.Shortcut(0, ui.KeyEscape) {
					pe.open = false
				}
				if button(c, "agent-provider-save", locales.ActionSave(), "", true, pal) ||
					c.Shortcut(ui.Cmd, ui.KeyS) {
					submitted = true
					pe.open = false
				}
			})
		})
	})
	if pe.open {
		return
	}
	a.providerEdit = nil
	switch {
	case deleting:
		a.deleteAgentProvider(pe)
	case submitted:
		if err := a.saveAgentProvider(pe); err != "" {
			pe.err = err
			pe.open = true
			a.providerEdit = pe
		}
	}
	a.repaintSettings()
}
