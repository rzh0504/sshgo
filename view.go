package main

import (
	"math"
	"time"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"

	"sshgo/internal/store"
	"sshgo/locales"
)

const (
	// barMid is the line the title bar's contents center on.
	barMid  = 30
	topBar  = 2 * barMid
	headerH = 2 * (barMid - cardGap)
	cardGap = 8
	rowH2   = 48
	// rowInset is where a sidebar row's text starts.
	rowInset = 12
)

// chrome is where the sidebar's animation puts things, in whole pixels.
type chrome struct {
	shown     float32
	sideW     float32
	mainPad   float32
	headerPad float32
}

// buttonsLeft places the sidebar's buttons that the header holds while
// the sidebar hides, in the header's coordinates.
func (ch chrome) buttonsLeft(bar ui.TitleBar) float32 {
	return lead(bar) - (ch.sideW + ch.mainPad)
}

// lead is where the sidebar's buttons start in the window.
func lead(bar ui.TitleBar) float32 {
	if ownControls {
		return bar.Left + 2*cardGap
	}
	return bar.Left + cardGap
}

var sidebarMotion = ui.Animation{Duration: 260 * time.Millisecond, Ease: ui.CubicBezier(0.25, 1, 0.5, 1)}

func chromeFor(progress, width float32, bar ui.TitleBar) chrome {
	round := func(v float32) float32 { return float32(math.Round(float64(v))) }
	w := round(width * min(max(progress, 0), 1))
	s := w / width
	pad := round(cardGap * (1 - s))
	hidden, shown := lead(bar)+16+60, width+16
	x := round(hidden + (shown-hidden)*s)
	return chrome{shown: s, sideW: w, mainPad: pad, headerPad: x - w - pad}
}

func b2f(b bool) float32 {
	if b {
		return 1
	}
	return 0
}

func (a *App) view(c *ui.Context) {
	pal := paletteFor(c)
	c.SetTheme(borderless(c, pal))
	if a.toast != "" {
		c.Toast(a.toast)
		a.toast = ""
	}
	a.shortcuts(c)

	bar := c.TitleBar()
	row := ui.Row(c).Fill().AlignItems(ui.Stretch)
	backdrop(c, row)
	progress := row.AnimateTo("sidebar", b2f(!a.state.SidebarHidden), sidebarMotion)
	a.chrome = chromeFor(progress, a.sidebarWidth(), bar)
	shown := a.chrome.shown
	row.Children(func() {
		w := a.chrome.sideW
		side := ui.Row(c).Width(w).Shrink(0).AlignItems(ui.Stretch).ClipX()
		side.Children(func() {
			ui.Column(c).Width(a.sidebarWidth()).Shrink(0).Margin(0, 0, 0, w-a.sidebarWidth()).
				Opacity(0.4 + 0.6*shown).Children(func() { a.sidebar(c, pal, bar) })
		})
		main := ui.Column(c).Grow(1).MinWidth(0).Padding(cardGap, cardGap, cardGap, a.chrome.mainPad).DragWindow()
		main.Children(func() {
			a.card(c, pal, bar, shown)
		})
		if shown > 0.01 {
			a.resizeHandle(c, pal, w)
		}
	})
	a.dialogs(c, pal)
}

// resizeHandle is the sidebar's edge, which a drag moves. It sits above
// the window's content, beside the sidebar, so that a drag of it is never
// a drag of the window.
func (a *App) resizeHandle(c *ui.Context, pal *palette, at float32) {
	h := ui.Box(c.Key("resize")).Absolute().Left(at - 4).Top(0).Bottom(0).Width(9).Cursor(ui.CursorResizeColumn).KeepFocus()
	if dx, _, ok := h.Dragged(); ok {
		if a.sidebarDrag == 0 {
			a.sidebarDrag = a.sidebarWidth()
		}
		a.sidebarDrag += dx
		a.state.SidebarWidth = float32(math.Round(float64(min(max(a.sidebarDrag, minSidebar), maxSidebar))))
	} else if a.sidebarDrag != 0 && !h.Pressed() {
		a.sidebarDrag = 0
		a.saveState()
	}
	if h.DoubleClicked() {
		a.state.SidebarWidth = defaultSidebar
		a.saveState()
	}
}

func (a *App) sidebar(c *ui.Context, pal *palette, bar ui.TitleBar) {
	ui.Row(c).Height(topBar).Shrink(0).AlignItems(ui.Center).Padding(0, 12, 0, lead(bar)).Gap(2).DragWindow().Children(func() {
		iconButton(c, "hide-sidebar", sidebarIcon, locales.ToggleHideSidebar(), pal, a.toggleSidebar)
		iconButton(c, "new-host", plusIcon, locales.ActionNewHost(), pal, func() { a.openEditor("") })
		ui.Spacer(c)
		ui.Text(c, "sshgo").FontSize(13.5).FontWeight(650).TextColor(pal.text).SingleLine().Padding(0, 4, 0, 2)
	})
	ui.Scroll(c.Key("hosts")).Grow(1).Padding(0, 10, 16, 10).DragWindow().Children(func() {
		if len(a.state.Hosts) == 0 {
			ui.Column(c).Padding(8, 10).Children(func() {
				ui.Text(c, locales.SidebarNoHosts()).FontSize(12.5).TextColor(pal.textFaint)
			})
			return
		}
		for _, h := range a.state.Hosts {
			a.hostRow(c, pal, h)
		}
	})
	a.sidebarFooter(c, pal)
}

func (a *App) hostRow(c *ui.Context, pal *palette, h store.Host) {
	selected := h.ID == a.selected
	rt := a.runtimes[h.ID]
	row := ui.Row(c.Key(h.ID)).Height(rowH2).Padding(0, 10, 0, rowInset).Gap(10).AlignItems(ui.Center).Radius(8).Margin(0, 0, 1, 0)
	hovered := row.Hovered()
	if pal.dark {
		row.Border(1, ui.Transparent)
	}
	bg := ui.Transparent
	switch {
	case selected:
		bg = pal.rowSelected
		chosen(row, pal)
	case hovered:
		bg = pal.rowHover
	}
	row.Background(row.AnimateColor("bg", bg, ui.Animation{Duration: 110 * time.Millisecond}))
	row.Tooltip(a.hostTip(h, rt))
	if (row.Pressed() || row.Clicked()) && !selected {
		a.selectHost(h.ID)
	}
	if row.DoubleClicked() {
		a.connect(h.ID)
	}
	row.ContextMenu(func(m *ui.Menu) { a.hostMenu(m, h) })
	row.Children(func() {
		ui.Icon(c, serverIcon).Size(15, 15).TextColor(hostColor(pal, h.Name)).Shrink(0)
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, h.Name).FontSize(13.5).SingleLine().Ellipsis("…").Shrink(1).MinWidth(0).TextColor(pal.text)
				ui.Spacer(c)
				a.hostState(c, pal, rt)
			})
			detail := h.Address()
			fg := pal.textFaint
			if rt != nil && rt.state == connFailed {
				detail = rt.err
				fg = pal.danger
			}
			ui.Text(c, detail).FontSize(12).TextColor(fg).SingleLine().Ellipsis("…").MinWidth(0)
		})
	})
}

// hostState says how a host stands, at the end of its row.
func (a *App) hostState(c *ui.Context, pal *palette, rt *hostRuntime) {
	if rt == nil {
		return
	}
	switch rt.state {
	case connConnecting:
		ui.Spinner(c).Size(12, 12)
	case connConnected:
		ui.Box(c).Size(7, 7).Radius(3.5).Background(okColor(pal)).Shrink(0).Label(locales.StatusConnected())
	case connEnded:
		ui.Box(c).Size(7, 7).Radius(3.5).Background(pal.attention).Shrink(0).Label(locales.StatusEnded())
	case connFailed:
		ui.Box(c).Size(7, 7).Radius(3.5).Background(pal.danger).Shrink(0).Label(locales.StatusDisconnected())
	}
}

func (a *App) hostTip(h store.Host, rt *hostRuntime) string {
	tip := h.Address()
	if rt != nil {
		switch rt.state {
		case connConnecting:
			tip += "\n" + locales.StatusConnecting()
		case connConnected:
			tip += "\n" + locales.StatusConnected()
		case connEnded:
			tip += "\n" + locales.StatusEnded()
			if rt.err != "" {
				tip += "\n" + rt.err
			}
		case connFailed:
			tip += "\n" + rt.err
		}
	}
	return tip
}

func (a *App) sidebarFooter(c *ui.Context, pal *palette) {
	ui.Row(c).Height(36).Shrink(0).Padding(0, 12, 4, 12).Gap(2).AlignItems(ui.Center).DragWindow().Children(func() {
		icon, label, target := moonIcon, locales.ActionSwitchDark(), "dark"
		if c.Theme().Dark {
			icon, label, target = sunIcon, locales.ActionSwitchLight(), "light"
		}
		iconButton(c, "appearance-mode", icon, label, pal, func() {
			a.state.Appearance = target
			a.applySettings()
		})
		iconButton(c, "settings", gearIcon, locales.ActionSettings(), pal, a.openSettings)
		ui.Spacer(c)
		n := a.connectedCount()
		dot := pal.textFaint
		if n > 0 {
			dot = okColor(pal)
		}
		ui.Row(c).Gap(7).Padding(0, 6).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(6, 6).Radius(3).Background(dot)
			ui.Text(c, locales.SidebarConnectedCount(n)).FontSize(12).TextColor(pal.textFaint).SingleLine()
		})
	})
}

func (a *App) card(c *ui.Context, pal *palette, bar ui.TitleBar, shown float32) {
	h, hasHost := a.selectedHost()
	card := paned(ui.Column(c).Grow(1).MinHeight(0).Radius(14).Background(pal.card), pal).Clip()
	card.Children(func() {
		ui.Row(c).Height(headerH).Shrink(0).AlignItems(ui.Center).Padding(0, 10, 0, a.chrome.headerPad).DragWindow().Children(func() {
			if shown < 0.5 {
				ui.Row(c).Absolute().Left(a.chrome.buttonsLeft(bar)).Top(0).Bottom(0).AlignItems(ui.Center).Gap(2).Opacity(1 - 2*shown).Children(func() {
					iconButton(c, "show-sidebar", sidebarIcon, locales.ToggleShowSidebar(), pal, a.toggleSidebar)
					iconButton(c, "new-host-2", plusIcon, locales.ActionNewHost(), pal, func() { a.openEditor("") })
				})
			}
			if hasHost {
				ui.Icon(c, serverIcon).Size(15, 15).TextColor(hostColor(pal, h.Name)).Shrink(0)
				ui.Text(c, h.Name).FontSize(14).FontWeight(600).TextColor(pal.text).SingleLine().Ellipsis("…").
					Shrink(1).MaxWidth(280).Margin(0, 0, 0, 8)
				ui.Text(c, h.Address()).FontSize(12).TextColor(pal.textFaint).SingleLine().Ellipsis("…").
					Shrink(2).MinWidth(0).Margin(0, 0, 0, 8)
			}
			ui.Spacer(c)
			if hasHost {
				a.headerState(c, pal, h)
				if rt := a.runtimes[h.ID]; rt != nil && (rt.state == connFailed || rt.state == connEnded) {
					iconButton(c, "reconnect", retryIcon, locales.ActionReconnect(), pal, func() { a.connect(h.ID) })
				}
			}
			windowControls(c, pal, a.win)
		})
		ui.Column(c).Grow(1).MinHeight(0).Children(func() { a.cardBody(c, pal) })
	})
}

func (a *App) headerState(c *ui.Context, pal *palette, h store.Host) {
	rt := a.runtimes[h.ID]
	if rt == nil {
		return
	}
	var dot ui.Color
	var label string
	switch rt.state {
	case connConnecting:
		ui.Spinner(c).Size(12, 12).Margin(0, 6, 0, 6)
		return
	case connConnected:
		dot, label = okColor(pal), locales.StatusConnected()
	case connEnded:
		dot, label = pal.attention, locales.StatusEnded()
	case connFailed:
		dot, label = pal.danger, locales.StatusDisconnected()
	default:
		return
	}
	ui.Row(c).Height(22).Padding(0, 8, 0, 7).Gap(6).Radius(11).AlignItems(ui.Center).Background(pal.chip).
		Margin(0, 0, 0, 4).Children(func() {
		ui.Box(c).Size(6, 6).Radius(3).Background(dot).Shrink(0)
		ui.Text(c, label).FontSize(11.5).FontWeight(500).TextColor(pal.textMuted).SingleLine()
	})
}

func (a *App) cardBody(c *ui.Context, pal *palette) {
	h, ok := a.selectedHost()
	if !ok {
		a.emptyState(c, pal)
		return
	}
	rt := a.runtimes[h.ID]
	if rt != nil && rt.term != nil && (rt.state == connConnected || rt.state == connEnded) {
		handle := a.handleFor(h.ID)
		terminal.View(c.Key("term-"+h.ID), rt.term).Fill().Bind(handle)
		if a.focusWanted == h.ID && !a.anyDialogOpen() {
			a.focusWanted = ""
			handle.Focus()
		}
		return
	}
	if rt != nil && rt.state == connConnecting {
		ui.Column(c).Fill().Center().Gap(10).Padding(24).Children(func() {
			ui.Spinner(c).Size(18, 18)
			ui.Text(c, locales.ConnectingTitle(h.Name)).FontSize(17).FontWeight(600).TextColor(pal.text)
			ui.Box(c).Height(4)
			if button(c, "cancel-connect", locales.ActionCancel(), "", false, pal) {
				a.disconnect(h.ID)
			}
		})
		return
	}
	if rt != nil && rt.state == connFailed {
		a.message(c, pal, locales.FailedTitle(), rt.err, func() {
			if button(c, "retry", locales.ActionRetry(), "", true, pal) {
				a.connect(h.ID)
			}
			if button(c, "edit-failed", locales.ActionEditHost(), "", false, pal) {
				a.openEditor(h.ID)
			}
		})
		return
	}
	a.message(c, pal, locales.HostIdleTitle(h.Name), "", func() {
		if button(c, "connect", locales.ActionConnect(), "", true, pal) {
			a.connect(h.ID)
		}
		if button(c, "edit-host", locales.ActionEditHost(), "", false, pal) {
			a.openEditor(h.ID)
		}
	})
}

func (a *App) message(c *ui.Context, pal *palette, title, detail string, actions func()) {
	ui.Column(c).Fill().Center().Gap(10).Padding(24).Children(func() {
		ui.Text(c, title).FontSize(17).FontWeight(600).TextColor(pal.text)
		if detail != "" {
			ui.Text(c, detail).FontSize(13).TextColor(pal.textMuted).MaxWidth(460).TextAlign(ui.Center).LineHeight(1.4).Selectable()
		}
		if actions != nil {
			ui.Row(c).Gap(10).Margin(10, 0, 0, 0).Children(actions)
		}
	})
}

func (a *App) emptyState(c *ui.Context, pal *palette) {
	ui.Column(c).Fill().Center().Gap(0).Padding(24).Children(func() {
		ui.Box(c).Size(52, 52).Radius(14).Center().Background(pal.chip).TextColor(pal.textMuted).Margin(0, 0, 18, 0).Children(func() {
			ui.Icon(c, terminalIcon).Size(26, 26)
		})
		ui.Text(c, locales.EmptyTitle()).FontSize(19).FontWeight(650).TextColor(pal.text)
		ui.Row(c).Gap(10).Margin(22, 0, 0, 0).Children(func() {
			if button(c, "empty-add", locales.ActionAddHost(), "", true, pal) {
				a.openEditor("")
			}
		})
	})
}

func (a *App) shortcuts(c *ui.Context) {
	if a.anyDialogOpen() {
		return
	}
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyB) {
		a.toggleSidebar()
	}
}
