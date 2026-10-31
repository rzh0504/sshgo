package main

import (
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"sshgo/internal/store"
	"sshgo/locales"
)

// groupNames are the names of the groups, as the editor suggests them.
func (a *App) groupNames() []string {
	names := make([]string, len(a.state.Groups))
	for i, g := range a.state.Groups {
		names[i] = g.Name
	}
	return names
}

// groupName is the name of the group an id names, "" when it names none.
func (a *App) groupName(id string) string {
	if g, ok := a.state.Group(id); ok {
		return g.Name
	}
	return ""
}

// hostsIn lists the hosts of a group, "" for the hosts in none.
func (a *App) hostsIn(group string) []store.Host {
	var out []store.Host
	for _, h := range a.state.Hosts {
		if h.Group == group {
			out = append(out, h)
		}
	}
	return out
}

// groupFor is the id of the group a name names, which it makes when there
// is none: "" for a blank name, which names no group.
func (a *App) groupFor(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", nil
	}
	if g, ok := a.state.GroupByName(name); ok {
		return g.ID, nil
	}
	g, err := store.NewGroup(name)
	if err != nil {
		return "", err
	}
	a.state.Groups = append(a.state.Groups, g)
	return g.ID, nil
}

func (a *App) groupCollapsed(id string) bool { return a.collapsed[id] }

func (a *App) toggleGroup(id string) {
	if a.collapsed == nil {
		a.collapsed = map[string]bool{}
	}
	a.collapsed[id] = !a.collapsed[id]
}

func (a *App) groupMenu(m *ui.Menu, g store.Group) {
	if m.Item(locales.ActionNewHostInGroup()).Chosen() {
		a.openEditorInGroup(g.ID)
	}
	m.Separator()
	if m.Item(locales.ActionRenameGroup()).Chosen() {
		a.openGroupEditor(g.ID)
	}
	if m.Item(locales.ActionDeleteGroup()).Chosen() {
		a.openDeleteGroup(g)
	}
}

// hostGroupMenu is a group's menu, for the hosts in no group too, which
// can only get a host.
func (a *App) hostGroupMenu(m *ui.Menu, id string) {
	if id == "" {
		if m.Item(locales.ActionNewHostInGroup()).Chosen() {
			a.openEditor("")
		}
		return
	}
	if g, ok := a.state.Group(id); ok {
		a.groupMenu(m, g)
	}
}

// sidebarGroup is a group's header and, while it is open, its hosts.
func (a *App) sidebarGroup(c *ui.Context, pal *palette, g store.Group, name string, hosts []store.Host) {
	open := !a.groupCollapsed(g.ID)
	hdr := ui.Row(c.Key("group-"+g.ID)).Height(28).Margin(4, 0, 1, 0).Padding(0, 8, 0, 4).
		Gap(6).AlignItems(ui.Center).Radius(6)
	if pal.dark {
		hdr.Border(1, ui.Transparent)
	}
	bg := ui.Transparent
	if hdr.Hovered() {
		bg = pal.rowHover
	}
	hdr.Background(hdr.AnimateColor("bg", bg, ui.Animation{Duration: 110 * time.Millisecond}))
	hdr.ContextMenu(func(m *ui.Menu) { a.hostGroupMenu(m, g.ID) })
	if hdr.Clicked() {
		a.toggleGroup(g.ID)
	}
	hdr.Children(func() {
		chevron := chevronRightIcon
		if open {
			chevron = chevronDownIcon
		}
		ui.Icon(c, chevron).Size(11, 11).TextColor(pal.textFaint).Shrink(0)
		ui.Icon(c, folderIcon).Size(13, 13).TextColor(pal.textMuted).Shrink(0)
		ui.Text(c, name).FontSize(12.5).FontWeight(600).TextColor(pal.textMuted).
			SingleLine().Ellipsis("…").Shrink(1).MinWidth(0)
		ui.Spacer(c)
		ui.Text(c, strconv.Itoa(len(hosts))).FontSize(11.5).FontFeatures("tnum").TextColor(pal.textFaint)
	})
	if !open {
		return
	}
	for _, h := range hosts {
		a.hostRow(c, pal, h, 4)
	}
}
