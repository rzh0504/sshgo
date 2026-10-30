package main

import (
	"strconv"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
	"golang.org/x/crypto/ssh"

	"sshgo/internal/knownhosts"
	"sshgo/internal/store"
	"sshgo/locales"
)

type editorState struct {
	open     bool
	id       string
	name     string
	host     string
	port     string
	user     string
	password string
	save     bool
	err      string
}

type passwordState struct {
	open     bool
	host     store.Host
	password string
	save     bool
	err      string
}

type hostKeyState struct {
	open    bool
	host    store.Host
	addr    string
	key     ssh.PublicKey
	known   []ssh.PublicKey
	attempt *connectAttempt
	answer  chan bool
}

type confirmState struct {
	open bool
	host store.Host
}

func (a *App) openEditor(id string) {
	ed := &editorState{open: true, port: "22", user: systemUser()}
	if id != "" {
		if h, ok := a.state.Host(id); ok {
			ed.id = h.ID
			ed.name = h.Name
			ed.host = h.Host
			ed.port = strconv.Itoa(h.Port)
			ed.user = h.User
			ed.save = h.SavePassword
		}
	}
	a.editor = ed
}

func (a *App) openPassword(h store.Host) {
	a.password = &passwordState{open: true, host: h, save: h.SavePassword}
}

func (a *App) openDelete(h store.Host) {
	a.confirm = &confirmState{open: true, host: h}
}

func (a *App) hostMenu(m *ui.Menu, h store.Host) {
	rt := a.runtimes[h.ID]
	live := rt != nil && (rt.state == connConnected || rt.state == connEnded || rt.state == connConnecting)
	if live {
		if m.Item(locales.ActionDisconnect()).Chosen() {
			a.disconnect(h.ID)
		}
		if m.Item(locales.ActionReconnect()).Chosen() {
			a.connect(h.ID)
		}
	} else if m.Item(locales.ActionConnect()).Chosen() {
		a.connect(h.ID)
	}
	m.Separator()
	if m.Item(locales.ActionEditHost()).Chosen() {
		a.openEditor(h.ID)
	}
	if m.Item(locales.ActionCopyAddress()).Chosen() {
		if err := mygo.Clipboard.Write(transfer.TextData(h.Address())); err == nil {
			a.toast = locales.ToastAddressCopied()
		}
	}
	m.Separator()
	if m.Item(locales.ActionDeleteHost()).Chosen() {
		a.openDelete(h)
	}
}

func (a *App) dialogs(c *ui.Context, pal *palette) {
	a.editorView(c, pal)
	a.passwordView(c, pal)
	a.hostKeyView(c, pal)
	a.confirmView(c, pal)
}

func dialogHead(c *ui.Context, pal *palette, title, detail string) {
	ui.Text(c, title).FontSize(15).FontWeight(650).TextColor(pal.text)
	if detail != "" {
		ui.Text(c, detail).FontSize(12.5).TextColor(pal.textMuted).LineHeight(1.35)
	}
}

func formField(c *ui.Context, key, label, placeholder string, value *string, pal *palette, password bool) ui.Element {
	ui.Text(c, label).FontSize(12).FontWeight(550).TextColor(pal.textMuted).Margin(0, 0, 2, 0)
	in := ui.TextInputBase(c.Key(key), value).Height(32).Padding(0, 10).Radius(8).FontSize(13.5).
		TextColor(pal.text).Placeholder(placeholder).Label(label)
	face(in, pal, pal.control)
	if password {
		in.Password()
	}
	return in
}

func (a *App) editorView(c *ui.Context, pal *palette) {
	ed := a.editor
	if ed == nil || !ed.open {
		return
	}
	submit := 0
	ui.DialogBase(c.Key("editor"), &ed.open, func(backdrop, panel ui.Element) {
		backdrop.Background(pal.overlay)
		title := locales.EditorAddTitle()
		if ed.id != "" {
			title = locales.EditorEditTitle()
		}
		floating(panel.Width(460).Padding(22).Gap(14).Radius(14).Background(pal.popover), pal).Label(title)
		dialogHead(c, pal, title, locales.EditorDetail())
		var inName, inHost, inPort, inUser, inPass ui.Element
		ui.Column(c).Gap(5).Children(func() {
			inName = formField(c, "name", locales.FieldName(), locales.PlaceholderName(), &ed.name, pal, false)
		})
		ui.Row(c).Gap(10).AlignItems(ui.End).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(5).Children(func() {
				inHost = formField(c, "host", locales.FieldHost(), locales.PlaceholderHost(), &ed.host, pal, false)
			})
			ui.Column(c).Width(92).Gap(5).Children(func() {
				inPort = formField(c, "port", locales.FieldPort(), locales.PlaceholderPort(), &ed.port, pal, false)
			})
		})
		ui.Column(c).Gap(5).Children(func() {
			inUser = formField(c, "user", locales.FieldUser(), locales.PlaceholderUser(), &ed.user, pal, false)
		})
		ui.Column(c).Gap(5).Children(func() {
			placeholder := ""
			if ed.save {
				placeholder = locales.PlaceholderPassword()
			}
			inPass = formField(c, "password", locales.FieldPassword(), placeholder, &ed.password, pal, true)
		})
		ui.Checkbox(c, &ed.save, locales.PasswordSaveLabel())
		if ed.err != "" {
			ui.Text(c, ed.err).FontSize(12.5).TextColor(pal.danger).LineHeight(1.35)
		}
		ui.Row(c).Gap(8).Justify(ui.End).Margin(6, 0, 0, 0).Children(func() {
			if button(c, "cancel", locales.ActionCancel(), "esc", false, pal) || c.Shortcut(0, ui.KeyEscape) {
				ed.open = false
			}
			if button(c, "save", locales.ActionSave(), "", false, pal) || c.Shortcut(ui.Cmd, ui.KeyS) {
				submit = 1
				ed.open = false
			}
			if button(c, "save-connect", locales.ActionSaveAndConnect(), "", true, pal) {
				submit = 2
				ed.open = false
			}
			if inName.Submitted() || inHost.Submitted() || inPort.Submitted() || inUser.Submitted() || inPass.Submitted() {
				submit = 2
				ed.open = false
			}
		})
	})
	if !ed.open {
		a.editor = nil
		if submit != 0 {
			if err := a.commitEditor(ed, submit == 2); err != "" {
				ed.err = err
				ed.open = true
				a.editor = ed
			}
		}
	}
}

// commitEditor saves the editor's host, and returns what is wrong with it,
// if anything.
func (a *App) commitEditor(ed *editorState, connect bool) string {
	host, addrUser, addrPort, err := store.ParseAddress(ed.host)
	if err != nil {
		return locales.ErrorHostRequired()
	}
	user := strings.TrimSpace(ed.user)
	if addrUser != "" {
		user = strings.TrimSpace(addrUser)
	}
	if user == "" {
		return locales.ErrorUserRequired()
	}
	port := addrPort
	if port == 0 {
		text := strings.TrimSpace(ed.port)
		if text == "" {
			port = 22
		} else {
			n, err := strconv.Atoi(text)
			if err != nil || n < 1 || n > 65535 {
				return locales.ErrorPortInvalid()
			}
			port = n
		}
	}
	name := strings.TrimSpace(ed.name)
	if name == "" {
		name = user + "@" + host
	}
	password := ed.password

	if ed.id == "" {
		h, err := store.New(name, host, port, user, ed.save)
		if err != nil {
			return err.Error()
		}
		if ed.save && password != "" {
			h.SavePassword = a.savePassword(h, password)
		}
		a.state.Hosts = append(a.state.Hosts, h)
		a.selected = h.ID
	} else {
		idx := a.state.Index(ed.id)
		if idx < 0 {
			return ""
		}
		old := a.state.Hosts[idx]
		h := old
		h.Name, h.Host, h.Port, h.User = name, host, port, user
		h.SavePassword = ed.save
		changed := old.Host != host || old.Port != port || old.User != user
		if ed.save {
			if password != "" {
				h.SavePassword = a.savePassword(h, password)
			}
		} else {
			_ = mygo.Secrets.Delete(passwordKey(h))
		}
		if changed {
			a.resetRuntime(h.ID)
			if rt := a.runtimes[h.ID]; rt != nil {
				rt.password = ""
			}
		}
		a.state.Hosts[idx] = h
		a.selected = h.ID
	}
	a.saveState()
	a.updateTitle()
	if connect {
		a.connect(a.selected)
	}
	return ""
}

func (a *App) passwordView(c *ui.Context, pal *palette) {
	p := a.password
	if p == nil || !p.open {
		return
	}
	submit := false
	ui.DialogBase(c.Key("password"), &p.open, func(backdrop, panel ui.Element) {
		backdrop.Background(pal.overlay)
		floating(panel.Width(400).Padding(22).Gap(12).Radius(14).Background(pal.popover), pal).
			Label(locales.PasswordTitle(p.host.Name))
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(30, 30).Radius(9).Center().Background(pal.chip).Children(func() {
				ui.Icon(c, lockIcon).Size(16, 16).TextColor(pal.textMuted)
			})
			ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
				ui.Text(c, locales.PasswordTitle(p.host.Name)).FontSize(15).FontWeight(650).TextColor(pal.text)
				ui.Text(c, locales.PasswordDetail(p.host.User, p.host.Endpoint())).FontSize(12.5).TextColor(pal.textMuted).SingleLine().Ellipsis("…")
			})
		})
		in := ui.TextInputBase(c.Key("password"), &p.password).Height(32).Padding(0, 10).Radius(8).FontSize(13.5).
			TextColor(pal.text).Password().AutoFocus().Label(locales.FieldPassword()).Placeholder(locales.FieldPassword())
		face(in, pal, pal.control)
		ui.Checkbox(c, &p.save, locales.PasswordSaveLabel())
		if p.err != "" {
			ui.Text(c, p.err).FontSize(12.5).TextColor(pal.danger)
		}
		ui.Row(c).Gap(8).Justify(ui.End).Margin(4, 0, 0, 0).Children(func() {
			if button(c, "cancel", locales.ActionCancel(), "esc", false, pal) || c.Shortcut(0, ui.KeyEscape) {
				p.open = false
			}
			if button(c, "sign-in", locales.ActionConnect(), "", true, pal) || in.Submitted() {
				submit = true
				p.open = false
			}
		})
	})
	if !p.open {
		a.password = nil
		if submit {
			if err := a.submitPassword(p); err != "" {
				p.err = err
				p.open = true
				a.password = p
			}
		}
	}
}

func (a *App) submitPassword(p *passwordState) string {
	if p.password == "" {
		return locales.PasswordEmptyError()
	}
	idx := a.state.Index(p.host.ID)
	if idx < 0 {
		return ""
	}
	h := a.state.Hosts[idx]
	h.SavePassword = p.save
	if p.save {
		h.SavePassword = a.savePassword(h, p.password)
	} else {
		_ = mygo.Secrets.Delete(passwordKey(h))
	}
	a.state.Hosts[idx] = h
	a.saveState()
	a.beginConnect(h, p.password)
	return ""
}

func (a *App) hostKeyView(c *ui.Context, pal *palette) {
	hk := a.hostKey
	if hk == nil || !hk.open {
		return
	}
	trust := false
	ui.DialogBase(c.Key("host-key"), &hk.open, func(backdrop, panel ui.Element) {
		backdrop.Background(pal.overlay)
		changed := len(hk.known) > 0
		title := locales.HostKeyTitle()
		if changed {
			title = locales.HostKeyChangedTitle()
		}
		floating(panel.Width(480).Padding(22).Gap(12).Radius(14).Background(pal.popover), pal).Label(title)
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(30, 30).Radius(9).Center().Background(pal.chip).Children(func() {
				ui.Icon(c, shieldIcon).Size(16, 16).TextColor(pal.textMuted)
			})
			titleColor := pal.text
			if changed {
				titleColor = pal.danger
			}
			ui.Text(c, title).FontSize(15).FontWeight(650).TextColor(titleColor)
		})
		detail := locales.HostKeyDetail()
		if changed {
			detail = locales.HostKeyChangedDetail(hk.addr)
		}
		ui.Text(c, detail).FontSize(13).TextColor(pal.textMuted).LineHeight(1.45).Selectable()
		if changed {
			keys := make([]string, len(hk.known))
			for i, key := range hk.known {
				keys[i] = knownhosts.Fingerprint(key)
			}
			ui.Column(c).Padding(10, 12).Gap(4).Radius(8).Background(pal.attnSoft).Children(func() {
				ui.Text(c, locales.HostKeyOldKeys(strings.Join(keys, "\n"))).FontSize(12).Font("monospace").
					TextColor(pal.text).LineHeight(1.4).Selectable()
			})
		} else {
			ui.Column(c).Padding(10, 12).Radius(8).Background(pal.chip).Children(func() {
				ui.Text(c, knownhosts.Fingerprint(hk.key)).FontSize(12).Font("monospace").TextColor(pal.text).
					LineHeight(1.4).Selectable()
			})
		}
		ui.Text(c, locales.HostKeyTrustHint()).FontSize(12).TextColor(pal.textFaint).LineHeight(1.35)
		ui.Row(c).Gap(8).Justify(ui.End).Margin(4, 0, 0, 0).Children(func() {
			if button(c, "cancel", locales.ActionCancel(), "esc", false, pal) || c.Shortcut(0, ui.KeyEscape) {
				hk.open = false
			}
			label := locales.ActionTrustConnect()
			ok := false
			if changed {
				label = locales.ActionUpdateConnect()
				ok = dangerButton(c, "update-connect", label, pal)
			} else {
				ok = defaultButton(c, "trust", label, "", true, pal)
			}
			if ok {
				trust = true
				hk.open = false
			}
		})
	})
	if !hk.open {
		a.answerHostKey(trust)
	}
}

func (a *App) confirmView(c *ui.Context, pal *palette) {
	cf := a.confirm
	if cf == nil || !cf.open {
		return
	}
	run := false
	ui.DialogBase(c.Key("confirm"), &cf.open, func(backdrop, panel ui.Element) {
		backdrop.Background(pal.overlay)
		floating(panel.Width(400).Padding(22).Gap(8).Radius(14).Background(pal.popover), pal).
			Label(locales.DeleteTitle())
		dialogHead(c, pal, locales.DeleteTitle(), locales.DeleteDetail(cf.host.Name))
		ui.Row(c).Gap(8).Justify(ui.End).Margin(14, 0, 0, 0).Children(func() {
			if button(c, "cancel", locales.ActionCancel(), "esc", false, pal) || c.Shortcut(0, ui.KeyEscape) {
				cf.open = false
			}
			if dangerButton(c, "delete", locales.ActionDelete(), pal) {
				run = true
				cf.open = false
			}
		})
	})
	if !cf.open {
		a.confirm = nil
		if run {
			a.deleteHost(cf.host)
		}
	}
}

func (a *App) deleteHost(h store.Host) {
	idx := a.state.Index(h.ID)
	a.resetRuntime(h.ID)
	delete(a.runtimes, h.ID)
	delete(a.handles, h.ID)
	_ = mygo.Secrets.Delete(passwordKey(h))
	if idx >= 0 {
		a.state.Hosts = append(a.state.Hosts[:idx], a.state.Hosts[idx+1:]...)
	}
	if a.selected == h.ID {
		a.selected = ""
		if len(a.state.Hosts) > 0 {
			next := idx
			if next >= len(a.state.Hosts) {
				next = len(a.state.Hosts) - 1
			}
			a.selected = a.state.Hosts[max(next, 0)].ID
			a.focusWanted = a.selected
		}
	}
	a.saveState()
	a.updateTitle()
	a.focusTerminal(a.selected)
}
