package main

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
	"golang.org/x/crypto/ssh"

	"sshgo/internal/knownhosts"
	"sshgo/internal/sshkey"
	"sshgo/internal/store"
	"sshgo/locales"
)

type editorState struct {
	open        bool
	id          string
	name        string
	group       string
	os          string
	osName      string
	host        string
	port        string
	user        string
	auth        string
	keyPath     string
	passphrase  string
	password    string
	save        bool
	saveKeyPass bool
	err         string
}

// groupEditState is the dialog that makes a group, or renames one.
type groupEditState struct {
	open bool
	id   string
	name string
	err  string
}

type passwordState struct {
	open     bool
	host     store.Host
	password string
	save     bool
	err      string
}

// keyPassState is the dialog that unlocks an encrypted private key.
type keyPassState struct {
	open       bool
	host       store.Host
	passphrase string
	save       bool
	err        string
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
	open  bool
	host  store.Host
	group *store.Group
	quit  bool // the app's quit, with sessions open
}

func (a *App) openEditor(id string) {
	linux := osKindByID("linux")
	ed := &editorState{open: true, port: "22", os: linux.id, osName: linux.name, auth: store.AuthPassword}
	if id != "" {
		if h, ok := a.state.Host(id); ok {
			k := osKindByID(h.OS)
			ed.id = h.ID
			ed.name = h.Name
			ed.group = a.groupName(h.Group)
			ed.os, ed.osName = k.id, k.name
			ed.host = h.Host
			ed.port = strconv.Itoa(h.Port)
			ed.user = h.User
			ed.auth = h.Auth
			ed.keyPath = h.KeyPath
			ed.save = !h.UsesKey() && h.SavePassword
			ed.saveKeyPass = h.UsesKey() && h.SavePassphrase
		}
	}
	a.editor = ed
}

// openEditorInGroup makes a host in a group: "" makes one in no group.
func (a *App) openEditorInGroup(group string) {
	a.openEditor("")
	a.editor.group = a.groupName(group)
}

func (a *App) openGroupEditor(id string) {
	ge := &groupEditState{open: true, id: id}
	if g, ok := a.state.Group(id); ok {
		ge.name = g.Name
	}
	a.groupEdit = ge
}

func (a *App) openPassword(h store.Host) {
	a.showWindow()
	a.password = &passwordState{open: true, host: h, save: h.SavePassword}
}

func (a *App) openKeyPassphrase(h store.Host) {
	a.showWindow()
	if kp := a.keyPass; kp != nil && kp.open && kp.host.ID == h.ID {
		return
	}
	a.keyPass = &keyPassState{open: true, host: h, save: h.SavePassphrase}
}

// pickPrivateKey asks the system for a private key file, "" when the user
// cancels.
func (a *App) pickPrivateKey(current string) string {
	start := sshkey.Expand(current)
	if start == "" {
		if home, err := os.UserHomeDir(); err == nil {
			start = filepath.Join(home, ".ssh")
			if st, err := os.Stat(start); err != nil || !st.IsDir() {
				start = home
			}
		}
	}
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent:          a.win,
		Title:           locales.ChoosePrivateKey(),
		DefaultPath:     start,
		ShowHiddenFiles: true,
	})
	if err != nil {
		a.toast = err.Error()
		return ""
	}
	if len(paths) == 0 {
		return ""
	}
	return paths[0]
}

func (a *App) openDelete(h store.Host) {
	a.confirm = &confirmState{open: true, host: h}
}

func (a *App) openDeleteGroup(g store.Group) {
	a.confirm = &confirmState{open: true, group: &g}
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
	if m.Item(locales.MenuEditHost()).Chosen() {
		a.openEditor(h.ID)
	}
	if m.Item(locales.ActionCopyAddress()).Chosen() {
		if err := mygo.Clipboard.Write(transfer.TextData(h.Address())); err == nil {
			a.toast = locales.ToastAddressCopied()
		}
	}
	m.Separator()
	if m.Item(locales.MenuDeleteHost()).Chosen() {
		a.openDelete(h)
	}
}

func (a *App) dialogs(c *ui.Context, pal *palette) {
	a.editorView(c, pal)
	a.groupEditorView(c, pal)
	a.passwordView(c, pal)
	a.keyPassView(c, pal)
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
	fieldFace(in, pal)
	focusEdge(in, in.Focused(), pal)
	if password {
		in.Password()
	}
	return in
}

// groupField is a host's group: chosen among those there are, or a name
// typed in for a new one.
func groupField(c *ui.Context, key string, value *string, options []string, pal *palette) ui.Element {
	ui.Text(c, locales.FieldGroup()).FontSize(12).FontWeight(550).TextColor(pal.textMuted).Margin(0, 0, 2, 0)
	box := fieldFace(ui.Row(c).Height(32).Radius(8).AlignItems(ui.Center), pal)
	var cb ui.ComboboxParts
	box.Children(func() {
		cb = ui.ComboboxBase(c.Key(key), value)
		cb.Input.FillHeight().Grow(1).MinWidth(0).Padding(0, 28, 0, 10).Radius(8).FontSize(13.5).
			TextColor(pal.text).Placeholder(locales.PlaceholderGroup()).Label(locales.FieldGroup())
		arrow := ui.ButtonBase(c.Key(key+"-arrow")).Size(18, 18).Center().Radius(4).
			Absolute().Right(7).Top(7).Label(locales.FieldGroup())
		arrow.Children(func() {
			ui.Icon(c, chevronDownIcon).Size(11, 11).TextColor(pal.textFaint)
		})
		if arrow.Clicked() {
			cb.SetOpen(!cb.Open())
			cb.Input.Focus()
		}
	})
	focusEdge(box, cb.Input.Focused(), pal)
	q := strings.ToLower(strings.TrimSpace(*value))
	var shown []string
	for _, o := range options {
		if q == "" || strings.Contains(strings.ToLower(o), q) {
			shown = append(shown, o)
		}
	}
	cb.Popup(func(panel ui.Element) {
		floating(panel.Padding(4).Radius(10).Background(pal.popover), pal)
		panel.MaxHeight(240)
		panel.Children(func() {
			for _, name := range shown {
				item := cb.Item(name).Padding(6, 10).Radius(6)
				fg := pal.text
				if item.Highlighted() {
					item.Background(pal.accent)
					fg = ui.Hex("#ffffff")
				}
				item.Children(func() {
					ui.Text(c, name).FontSize(13).TextColor(fg).SingleLine().Ellipsis("…")
				})
			}
		})
	})
	if v, ok := cb.Chosen(); ok {
		*value = v
	}
	return cb.Input
}

// newGroupHint says when the group name is one no group has yet, which
// saving makes.
func (a *App) newGroupHint(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if _, ok := a.state.GroupByName(name); ok {
		return ""
	}
	return locales.GroupWillCreate(name)
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
		dialogHead(c, pal, title, "")
		var inName, inGroup, inHost, inPort, inUser, inKey, inKeyPass, inPass ui.Element
		ui.Row(c).Gap(10).AlignItems(ui.Start).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(5).Children(func() {
				inName = formField(c, "name", locales.FieldName(), locales.PlaceholderName(), &ed.name, pal, false).AutoFocus()
			})
			ui.Column(c).Width(180).Shrink(0).Gap(4).Children(func() {
				inGroup = groupField(c, "group-"+ed.id, &ed.group, a.groupNames(), pal)
				if hint := a.newGroupHint(ed.group); hint != "" {
					ui.Text(c, hint).FontSize(11).TextColor(pal.textFaint).SingleLine().Ellipsis("…")
				}
			})
		})
		ui.Row(c).Gap(10).AlignItems(ui.End).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(5).Children(func() {
				inHost = formField(c, "host", locales.FieldHost(), locales.PlaceholderHost(), &ed.host, pal, false)
			})
			ui.Column(c).Width(92).Gap(5).Children(func() {
				inPort = formField(c, "port", locales.FieldPort(), locales.PlaceholderPort(), &ed.port, pal, false)
			})
		})
		ui.Row(c).Gap(10).AlignItems(ui.End).Children(func() {
			ui.Column(c).Width(190).Shrink(0).Gap(5).Children(func() {
				ui.Text(c, locales.FieldOS()).FontSize(12).FontWeight(550).TextColor(pal.textMuted).Margin(0, 0, 2, 0)
				if ui.Combobox(c.Key("os-"+ed.id), &ed.osName, osNames()).Height(32).Width(190).
					Label(locales.FieldOS()).Changed() {
					ed.os = osKindByName(ed.osName).id
				}
			})
			ui.Column(c).Grow(1).MinWidth(0).Gap(5).Children(func() {
				inUser = formField(c, "user", locales.FieldUser(), locales.PlaceholderUser(), &ed.user, pal, false)
			})
		})
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.FieldAuth()).FontSize(12).FontWeight(550).TextColor(pal.textMuted)
			segmented(c, "auth-"+ed.id, &ed.auth,
				[]string{store.AuthPassword, store.AuthKey, store.AuthAgent},
				[]string{locales.AuthPassword(), locales.AuthKey(), locales.AuthAgent()})
		})
		if ed.auth == store.AuthKey {
			ui.Row(c).Gap(8).AlignItems(ui.End).Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Gap(5).Children(func() {
					inKey = formField(c, "key-path", locales.FieldPrivateKey(), locales.PlaceholderPrivateKey(), &ed.keyPath, pal, false)
				})
				if button(c, "browse-key", locales.ActionBrowse(), "", false, pal) {
					if path := a.pickPrivateKey(ed.keyPath); path != "" {
						ed.keyPath = path
					}
				}
			})
			ui.Text(c, locales.KeyFormatsHint()).FontSize(11).TextColor(pal.textFaint).LineHeight(1.3)
			ui.Column(c).Gap(5).Children(func() {
				inKeyPass = formField(c, "key-pass", locales.FieldPassphrase(), locales.PlaceholderKeyPassphrase(), &ed.passphrase, pal, true)
			})
			ui.Checkbox(c, &ed.saveKeyPass, locales.PassphraseSaveLabel())
		} else if ed.auth == store.AuthAgent {
			ui.Text(c, locales.AgentHint()).FontSize(12).TextColor(pal.textMuted).LineHeight(1.4)
		} else {
			ui.Column(c).Gap(5).Children(func() {
				placeholder := ""
				if ed.save {
					placeholder = locales.PlaceholderPassword()
				}
				inPass = formField(c, "password", locales.FieldPassword(), placeholder, &ed.password, pal, true)
			})
			ui.Checkbox(c, &ed.save, locales.PasswordSaveLabel())
		}
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
			if inName.Submitted() || inGroup.Submitted() || inHost.Submitted() || inPort.Submitted() ||
				inUser.Submitted() || inKey.Submitted() || inKeyPass.Submitted() || inPass.Submitted() {
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
	auth := ed.auth
	if auth != store.AuthKey && auth != store.AuthAgent {
		auth = store.AuthPassword
	}
	keyPath := strings.TrimSpace(ed.keyPath)
	if auth != store.AuthKey {
		keyPath = ""
	} else if keyPath == "" {
		return locales.ErrorKeyRequired()
	}
	if auth == store.AuthKey {
		if _, err := sshkey.Load(keyPath, ""); err != nil && !errors.Is(err, sshkey.ErrPassphrase) {
			return locales.ErrorKeyLoad(err.Error())
		}
		if ed.passphrase != "" {
			if _, err := sshkey.Load(keyPath, ed.passphrase); err != nil {
				return locales.ErrorKeyUnlock()
			}
		}
	}
	group, err := a.groupFor(ed.group)
	if err != nil {
		return err.Error()
	}

	if ed.id == "" {
		h, err := store.New(name, host, port, user, auth == store.AuthPassword && ed.save)
		if err != nil {
			return err.Error()
		}
		h.Group = group
		h.OS = ed.os
		h.Auth = auth
		switch auth {
		case store.AuthKey:
			h.KeyPath = keyPath
			h.SavePassphrase = ed.saveKeyPass && (ed.passphrase == "" || a.savePassphrase(h, ed.passphrase))
		case store.AuthPassword:
			if ed.save && password != "" {
				h.SavePassword = a.savePassword(h, password)
			}
		}
		a.state.Hosts = append(a.state.Hosts, h)
		a.selected = h.ID
		if auth == store.AuthKey && ed.passphrase != "" {
			a.runtime(h.ID).passphrase = ed.passphrase
		}
	} else {
		idx := a.state.Index(ed.id)
		if idx < 0 {
			return ""
		}
		old := a.state.Hosts[idx]
		h := old
		h.Name, h.Host, h.Port, h.User = name, host, port, user
		h.Group = group
		h.OS = ed.os
		h.Auth = auth
		switch auth {
		case store.AuthKey:
			h.KeyPath = keyPath
			h.SavePassphrase = ed.saveKeyPass
			h.SavePassword = false
			_ = mygo.Secrets.Delete(passwordKey(h))
			keyChanged := old.Auth != auth || old.KeyPath != keyPath
			switch {
			case ed.passphrase != "":
				if keyChanged || !h.SavePassphrase {
					_ = mygo.Secrets.Delete(passphraseKey(h))
				}
				if h.SavePassphrase {
					h.SavePassphrase = a.savePassphrase(h, ed.passphrase)
					if !h.SavePassphrase {
						_ = mygo.Secrets.Delete(passphraseKey(h))
					}
				}
			case keyChanged || !h.SavePassphrase:
				_ = mygo.Secrets.Delete(passphraseKey(h))
			}
		case store.AuthAgent:
			h.KeyPath = ""
			h.SavePassphrase = false
			h.SavePassword = false
			_ = mygo.Secrets.Delete(passphraseKey(h))
			_ = mygo.Secrets.Delete(passwordKey(h))
		default:
			h.KeyPath = ""
			h.SavePassphrase = false
			_ = mygo.Secrets.Delete(passphraseKey(h))
			h.SavePassword = ed.save
			if ed.save {
				if password != "" {
					h.SavePassword = a.savePassword(h, password)
				}
			} else {
				_ = mygo.Secrets.Delete(passwordKey(h))
			}
		}
		changed := old.Host != host || old.Port != port || old.User != user ||
			old.Auth != h.Auth || old.KeyPath != h.KeyPath
		if changed {
			a.resetRuntime(h.ID)
			if rt := a.runtimes[h.ID]; rt != nil {
				rt.password = ""
				rt.passphrase = ""
			}
		}
		if auth == store.AuthKey && ed.passphrase != "" {
			a.runtime(h.ID).passphrase = ed.passphrase
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

func (a *App) groupEditorView(c *ui.Context, pal *palette) {
	ge := a.groupEdit
	if ge == nil || !ge.open {
		return
	}
	submit := false
	ui.DialogBase(c.Key("group-editor"), &ge.open, func(backdrop, panel ui.Element) {
		backdrop.Background(pal.overlay)
		title := locales.GroupAddTitle()
		if ge.id != "" {
			title = locales.GroupEditTitle()
		}
		floating(panel.Width(400).Padding(22).Gap(14).Radius(14).Background(pal.popover), pal).Label(title)
		dialogHead(c, pal, title, locales.GroupEditDetail())
		var in ui.Element
		ui.Column(c).Gap(5).Children(func() {
			in = formField(c, "group-name", locales.FieldGroupName(), locales.PlaceholderGroupName(), &ge.name, pal, false).AutoFocus()
		})
		if ge.err != "" {
			ui.Text(c, ge.err).FontSize(12.5).TextColor(pal.danger).LineHeight(1.35)
		}
		ui.Row(c).Gap(8).Justify(ui.End).Margin(6, 0, 0, 0).Children(func() {
			if button(c, "cancel", locales.ActionCancel(), "esc", false, pal) || c.Shortcut(0, ui.KeyEscape) {
				ge.open = false
			}
			if button(c, "save-group", locales.ActionSave(), "", true, pal) || c.Shortcut(ui.Cmd, ui.KeyS) {
				submit = true
				ge.open = false
			}
			if in.Submitted() {
				submit = true
				ge.open = false
			}
		})
	})
	if !ge.open {
		a.groupEdit = nil
		if submit {
			if err := a.commitGroup(ge); err != "" {
				ge.err = err
				ge.open = true
				a.groupEdit = ge
			}
		}
	}
}

// commitGroup saves a new group, or the new name of one, and returns what
// is wrong with it, if anything.
func (a *App) commitGroup(ge *groupEditState) string {
	name := strings.TrimSpace(ge.name)
	if name == "" {
		return locales.ErrorGroupNameRequired()
	}
	if g, ok := a.state.GroupByName(name); ok && g.ID != ge.id {
		return locales.ErrorGroupExists()
	}
	if ge.id == "" {
		g, err := store.NewGroup(name)
		if err != nil {
			return err.Error()
		}
		a.state.Groups = append(a.state.Groups, g)
	} else if idx := a.state.GroupIndex(ge.id); idx >= 0 {
		a.state.Groups[idx].Name = name
	}
	a.saveState()
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
		fieldFace(in, pal)
		focusEdge(in, in.Focused(), pal)
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
	a.beginConnect(h, connectAuth{password: p.password})
	return ""
}

func (a *App) keyPassView(c *ui.Context, pal *palette) {
	p := a.keyPass
	if p == nil || !p.open {
		return
	}
	submit := false
	ui.DialogBase(c.Key("key-passphrase"), &p.open, func(backdrop, panel ui.Element) {
		backdrop.Background(pal.overlay)
		title := locales.KeyPassphraseTitle(p.host.Name)
		floating(panel.Width(420).Padding(22).Gap(12).Radius(14).Background(pal.popover), pal).Label(title)
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(30, 30).Radius(9).Center().Background(pal.chip).Children(func() {
				ui.Icon(c, keyIcon).Size(16, 16).TextColor(pal.textMuted)
			})
			ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
				ui.Text(c, title).FontSize(15).FontWeight(650).TextColor(pal.text)
				ui.Text(c, locales.KeyPassphraseDetail(p.host.User, p.host.Endpoint())).FontSize(12.5).
					TextColor(pal.textMuted).SingleLine().Ellipsis("…")
			})
		})
		in := ui.TextInputBase(c.Key("key-passphrase"), &p.passphrase).Height(32).Padding(0, 10).Radius(8).FontSize(13.5).
			TextColor(pal.text).Password().AutoFocus().Label(locales.FieldPassphrase()).Placeholder(locales.FieldPassphrase())
		fieldFace(in, pal)
		focusEdge(in, in.Focused(), pal)
		ui.Checkbox(c, &p.save, locales.PassphraseSaveLabel())
		if p.err != "" {
			ui.Text(c, p.err).FontSize(12.5).TextColor(pal.danger).LineHeight(1.35)
		}
		ui.Row(c).Gap(8).Justify(ui.End).Margin(4, 0, 0, 0).Children(func() {
			if button(c, "cancel", locales.ActionCancel(), "esc", false, pal) || c.Shortcut(0, ui.KeyEscape) {
				p.open = false
			}
			if button(c, "unlock", locales.ActionUnlock(), "", true, pal) || in.Submitted() {
				submit = true
				p.open = false
			}
		})
	})
	if !p.open {
		a.keyPass = nil
		if submit {
			if err := a.submitKeyPassphrase(p); err != "" {
				p.err = err
				p.open = true
				a.keyPass = p
			}
		}
	}
}

func (a *App) submitKeyPassphrase(p *keyPassState) string {
	if p.passphrase == "" {
		return locales.PassphraseEmptyError()
	}
	idx := a.state.Index(p.host.ID)
	if idx < 0 {
		return ""
	}
	h := a.state.Hosts[idx]
	signer, err := sshkey.Load(h.KeyPath, p.passphrase)
	if err != nil {
		return locales.ErrorKeyUnlock()
	}
	h.SavePassphrase = p.save && a.savePassphrase(h, p.passphrase)
	if !h.SavePassphrase {
		_ = mygo.Secrets.Delete(passphraseKey(h))
	}
	a.state.Hosts[idx] = h
	a.saveState()
	a.runtime(h.ID).passphrase = p.passphrase
	a.beginConnect(h, connectAuth{signer: signer})
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
	title, detail, action, key := locales.DeleteTitle(), locales.DeleteDetail(cf.host.Name), locales.ActionDelete(), "delete"
	switch {
	case cf.quit:
		title, detail, action, key = locales.QuitTitle(), locales.QuitDetail(a.connectedCount()), locales.ActionQuit(), "quit"
	case cf.group != nil:
		title, detail = locales.DeleteGroupTitle(), locales.DeleteGroupDetail(cf.group.Name)
	}
	run := false
	ui.DialogBase(c.Key("confirm-"+key), &cf.open, func(backdrop, panel ui.Element) {
		backdrop.Background(pal.overlay)
		floating(panel.Width(400).Padding(22).Gap(8).Radius(14).Background(pal.popover), pal).
			Label(title)
		dialogHead(c, pal, title, detail)
		ui.Row(c).Gap(8).Justify(ui.End).Margin(14, 0, 0, 0).Children(func() {
			if button(c, "cancel", locales.ActionCancel(), "esc", false, pal) || c.Shortcut(0, ui.KeyEscape) {
				cf.open = false
			}
			if dangerButton(c, key, action, pal) {
				run = true
				cf.open = false
			}
		})
	})
	if !cf.open {
		a.confirm = nil
		if run {
			switch {
			case cf.quit:
				a.quitNow()
			case cf.group != nil:
				a.deleteGroup(*cf.group)
			default:
				a.deleteHost(cf.host)
			}
		}
	}
}

func (a *App) deleteHost(h store.Host) {
	idx := a.state.Index(h.ID)
	a.resetRuntime(h.ID)
	delete(a.runtimes, h.ID)
	delete(a.handles, h.ID)
	_ = mygo.Secrets.Delete(passwordKey(h))
	_ = mygo.Secrets.Delete(passphraseKey(h))
	a.state.ForgetRecent(h.ID)
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

// deleteGroup removes a group, and leaves its hosts in no group.
func (a *App) deleteGroup(g store.Group) {
	idx := a.state.GroupIndex(g.ID)
	if idx < 0 {
		return
	}
	a.state.Groups = append(a.state.Groups[:idx], a.state.Groups[idx+1:]...)
	for i := range a.state.Hosts {
		if a.state.Hosts[i].Group == g.ID {
			a.state.Hosts[i].Group = ""
		}
	}
	delete(a.collapsed, g.ID)
	delete(a.dashCollapsed, g.ID)
	a.saveState()
}
