package main

import (
	"strconv"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"

	"sshgo/internal/store"
	"sshgo/locales"
)

// hostMenuState is a host card's context menu, at the point it opened.
type hostMenuState struct {
	open bool
	host store.Host
	x, y float32
	hi   int
	// hover is the item the pointer is on, -1 when none: the pointer
	// moves the choice only as it comes to another item.
	hover int
}

// hostMenuEntry is an item of a host card's menu; nil is a separator.
type hostMenuEntry struct {
	icon  *ui.SVG
	label string
	run   func()
}

func (a *App) openHostMenu(h store.Host, x, y float32) {
	a.hostMenu = &hostMenuState{open: true, host: h, x: x, y: y, hover: -1}
}

func (a *App) hostMenuEntries(h store.Host) []*hostMenuEntry {
	rt := a.runtimes[h.ID]
	live := rt != nil && (rt.state == connConnected || rt.state == connEnded || rt.state == connConnecting)
	var entries []*hostMenuEntry
	if live {
		entries = append(entries,
			&hostMenuEntry{disconnectIcon, locales.ActionDisconnect(), func() { a.disconnect(h.ID) }},
			&hostMenuEntry{retryIcon, locales.ActionReconnect(), func() { a.connect(h.ID) }},
		)
	} else {
		entries = append(entries, &hostMenuEntry{connectIcon, locales.ActionConnect(), func() { a.connect(h.ID) }})
	}
	entries = append(entries, nil,
		&hostMenuEntry{pencilIcon, locales.MenuEditHost(), func() { a.openEditor(h.ID) }},
		&hostMenuEntry{copyIcon, locales.ActionCopyAddress(), func() {
			if err := mygo.Clipboard.Write(transfer.TextData(h.Address())); err == nil {
				a.toast = locales.ToastAddressCopied()
			}
		}},
		nil,
		&hostMenuEntry{trashIcon, locales.MenuDeleteHost(), func() { a.openDelete(h) }},
	)
	return entries
}

// hostMenuView shows a host card's context menu where it opened, in the
// window's own style: as wide as its items, with their icons.
func (a *App) hostMenuView(c *ui.Context, pal *palette) {
	st := a.hostMenu
	if st == nil || !st.open {
		return
	}
	entries := a.hostMenuEntries(st.host)
	var choices []*hostMenuEntry
	for _, e := range entries {
		if e != nil {
			choices = append(choices, e)
		}
	}
	if len(choices) == 0 {
		a.hostMenu = nil
		return
	}
	if st.hi < 0 || st.hi >= len(choices) {
		st.hi = 0
	}
	var anchor ui.Element
	ui.Overlay(c, func() {
		anchor = ui.Box(c.Key("host-menu-anchor")).Absolute().Left(st.x).Top(st.y).Size(0, 0)
	})
	var chosen func()
	ui.PopoverBase(c, anchor, &st.open, func(panel ui.Element) {
		p := menu(panel.Radius(14).Padding(6), pal)
		p.Role(ui.RoleMenu).Focusable().FocusRing(false).AutoFocus()
		hovered := -1
		p.Children(func() {
			ui.Column(c).Gap(1).AlignItems(ui.Stretch).Children(func() {
				i := 0
				for _, e := range entries {
					if e == nil {
						ui.Box(c).Height(1).Margin(4, 4).Background(pal.divider).Shrink(0)
						continue
					}
					idx := i
					i++
					row := ui.Row(c.Key("host-menu-"+strconv.Itoa(idx))).Height(30).Padding(0, 10).Gap(10).
						Radius(8).AlignItems(ui.Center).Label(e.label).Role(ui.RoleMenuItem)
					if row.Hovered() {
						hovered = idx
						if st.hover != idx {
							st.hi = idx
						}
					}
					if st.hi == idx {
						row.Background(pal.accent)
						row.Children(func() {
							ui.Icon(c, e.icon).Size(15, 15).TextColor(ui.Hex("#ffffff")).Shrink(0)
							ui.Text(c, e.label).FontSize(13.5).TextColor(ui.Hex("#ffffff")).SingleLine()
						})
					} else {
						row.Children(func() {
							ui.Icon(c, e.icon).Size(15, 15).TextColor(pal.textMuted).Shrink(0)
							ui.Text(c, e.label).FontSize(13.5).TextColor(pal.text).SingleLine()
						})
					}
					if row.Clicked() {
						chosen = e.run
						st.open = false
					}
				}
			})
		})
		st.hover = hovered
		n := len(choices)
		if p.Shortcut(0, ui.KeyDown) {
			st.hi = (st.hi + 1) % n
		}
		if p.Shortcut(0, ui.KeyUp) {
			st.hi = (st.hi - 1 + n) % n
		}
		if p.Shortcut(0, ui.KeyHome) {
			st.hi = 0
		}
		if p.Shortcut(0, ui.KeyEnd) {
			st.hi = n - 1
		}
		if p.Shortcut(0, ui.KeyEnter) || p.Shortcut(0, ui.KeySpace) {
			chosen = choices[st.hi].run
			st.open = false
		}
	})
	if !st.open {
		a.hostMenu = nil
		if chosen != nil {
			chosen()
		}
	}
}
