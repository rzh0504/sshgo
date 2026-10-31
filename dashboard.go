package main

import (
	"strconv"
	"time"

	"github.com/egoist/mygo/ui"

	"sshgo/internal/store"
	"sshgo/locales"
)

const (
	hostCardW = 240
	hostCardH = 64
	dashPad   = 24
)

// hostSection is a group of hosts the dashboard lists together, its name
// "" for the hosts of an app without groups.
type hostSection struct {
	id    string
	name  string
	hosts []store.Host
}

// dashboardSections are the hosts as the dashboard lists them: by group
// once there are groups, the ones in no group first.
func (a *App) dashboardSections() []hostSection {
	if len(a.state.Groups) == 0 {
		return []hostSection{{hosts: a.state.Hosts}}
	}
	var out []hostSection
	if ungrouped := a.hostsIn(""); len(ungrouped) > 0 {
		out = append(out, hostSection{name: locales.GroupUngrouped(), hosts: ungrouped})
	}
	for _, g := range a.state.Groups {
		out = append(out, hostSection{id: g.ID, name: g.Name, hosts: a.hostsIn(g.ID)})
	}
	return out
}

// dashboard is what the card shows while no shell does: the hosts
// connected last, then the hosts to manage, by group.
func (a *App) dashboard(c *ui.Context, pal *palette) {
	ui.Scroll(c.Key("dashboard")).Fill().TrackScroll(&a.dashScroll).Children(func() {
		ui.Column(c).Padding(18, dashPad, 26, dashPad).Gap(20).Children(func() {
			if len(a.state.Hosts) == 0 && len(a.state.Groups) == 0 {
				a.dashboardEmpty(c, pal)
				return
			}
			if recent := a.state.RecentHosts(); len(recent) > 0 {
				a.dashboardRecent(c, pal, recent)
			}
			for _, s := range a.dashboardSections() {
				a.dashboardSection(c, pal, s)
			}
		})
	})
}

// dashboardRecent lists the hosts connected last, above a rule that sets
// the groups below apart.
func (a *App) dashboardRecent(c *ui.Context, pal *palette, hosts []store.Host) {
	ui.Column(c).Gap(10).Children(func() {
		ui.Row(c).Height(26).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, clockIcon).Size(13, 13).TextColor(pal.textMuted).Shrink(0)
			ui.Text(c, locales.DashboardRecent()).FontSize(12.5).FontWeight(600).TextColor(pal.textMuted)
		})
		ui.Row(c).Wrap().Gap(10).Children(func() {
			for _, h := range hosts {
				a.dashboardHost(c, pal, h)
			}
		})
		ui.Box(c).Height(1).Shrink(0).Background(pal.divider).Margin(2, 0, 0, 0)
	})
}

func (a *App) dashboardSection(c *ui.Context, pal *palette, s hostSection) {
	ui.Column(c).Gap(10).Children(func() {
		open := s.id == "" || !a.dashGroupCollapsed(s.id)
		if s.name != "" {
			tip := locales.ActionAddHost()
			if s.id != "" {
				tip = locales.ActionNewHostInGroup()
			}
			hdr := ui.Row(c.Key("dash-group-" + s.id)).Height(26).Gap(8).AlignItems(ui.Center)
			hdr.ContextMenu(func(m *ui.Menu) { a.hostGroupMenu(m, s.id) })
			if s.id != "" {
				hdr.Cursor(ui.CursorPointer)
				if hdr.Clicked() {
					a.toggleDashGroup(s.id)
				}
			}
			hdr.Children(func() {
				ui.Icon(c, folderIcon).Size(13, 13).TextColor(pal.textMuted).Shrink(0)
				ui.Text(c, s.name).FontSize(12.5).FontWeight(600).TextColor(pal.textMuted).
					SingleLine().Ellipsis("…").Shrink(1).MinWidth(0)
				ui.Text(c, strconv.Itoa(len(s.hosts))).FontSize(11).FontFeatures("tnum").
					TextColor(pal.textFaint).Background(pal.chip).Radius(4).Padding(1, 6)
				if s.id != "" {
					chevron := chevronRightIcon
					if open {
						chevron = chevronDownIcon
					}
					ui.Icon(c, chevron).Size(11, 11).TextColor(pal.textFaint).Shrink(0)
				}
				ui.Spacer(c)
				iconButton(c, "dash-add-"+s.id, plusIcon, tip, pal, func() {
					a.openEditorInGroup(s.id)
				})
			})
		}
		if !open {
			return
		}
		if len(s.hosts) == 0 {
			ui.Text(c, locales.GroupEmpty()).FontSize(12).TextColor(pal.textFaint).Padding(0, 2)
			return
		}
		ui.Row(c).Wrap().Gap(10).Children(func() {
			for _, h := range s.hosts {
				a.dashboardHost(c, pal, h)
			}
		})
	})
}

func (a *App) dashboardHost(c *ui.Context, pal *palette, h store.Host) {
	rt := a.runtimes[h.ID]
	selected := h.ID == a.selected
	card := ui.ButtonBase(c.Key("dash-"+h.ID)).Row().Width(hostCardW).Height(hostCardH).Shrink(0).
		Padding(12, 12).Gap(10).Radius(12).AlignItems(ui.Center).Label(h.Name)
	bg := pal.chip
	switch {
	case selected:
		bg = pal.rowSelected
		if pal.dark {
			chosen(card, pal)
		} else {
			// A crisp edge, as a border would give, and a shadow under it
			// that makes the card read as raised, not blurred.
			card.Shadow(0, 0, 0, 1, pal.cardBorder).Shadow(0, 1, 3, 0, pal.rowShadow)
		}
	case card.Hovered():
		bg = pal.chip.Mix(pal.text, 0.05)
	}
	card.Background(card.AnimateColor("bg", bg, ui.Animation{Duration: 110 * time.Millisecond}))
	card.ContextMenu(func(m *ui.Menu) { a.hostMenu(m, h) })
	if card.Clicked() {
		a.selectHost(h.ID)
	}
	if card.DoubleClicked() {
		a.connect(h.ID)
	}
	card.Children(func() {
		ui.Box(c).Size(30, 30).Radius(9).Center().Background(pal.chip).Shrink(0).Children(func() {
			a.hostGlyph(c, pal, h, 20)
		})
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			a.maskedText(c, h.Name, 13, 600, pal.text).SingleLine().Ellipsis("…")
			detail := h.Address()
			fg := pal.textFaint
			if rt != nil && rt.state == connFailed {
				detail, fg = rt.err, pal.danger
			}
			a.maskedText(c, detail, 11.5, 400, fg).SingleLine().Ellipsis("…")
		})
		if card.Hovered() {
			b := ui.ButtonBase(c.Key("dash-connect-"+h.ID)).Size(24, 24).Radius(6).Center().
				TextColor(pal.textMuted).Label(locales.ActionConnect()).Tooltip(locales.ActionConnect())
			if b.Hovered() {
				b.Background(pal.rowHover).TextColor(pal.text)
			}
			b.Children(func() { ui.Icon(c, terminalIcon).Size(13, 13) })
			if b.Clicked() {
				a.connect(h.ID)
			}
		}
		a.hostState(c, pal, rt)
	})
}

func (a *App) dashboardEmpty(c *ui.Context, pal *palette) {
	ui.Column(c).AlignItems(ui.Center).Padding(40, 0).Gap(0).Children(func() {
		ui.Box(c).Size(52, 52).Radius(14).Center().Background(pal.chip).TextColor(pal.textMuted).
			Margin(0, 0, 16, 0).Children(func() {
			ui.Icon(c, serverIcon).Size(24, 24)
		})
		ui.Text(c, locales.SidebarNoHosts()).FontSize(17).FontWeight(600).TextColor(pal.text)
		ui.Row(c).Gap(8).Margin(18, 0, 0, 0).Children(func() {
			if button(c, "dash-empty-group", locales.ActionNewGroup(), "", false, pal) {
				a.openGroupEditor("")
			}
			if button(c, "dash-empty-add", locales.ActionAddHost(), "", true, pal) {
				a.openEditor("")
			}
		})
	})
}
