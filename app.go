package main

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
	"golang.org/x/crypto/ssh"

	kh "sshgo/internal/knownhosts"
	"sshgo/internal/sshx"
	"sshgo/internal/store"
	"sshgo/locales"
)

type connState int

const (
	connIdle connState = iota
	connConnecting
	connConnected
	connFailed
	connEnded
)

const (
	defaultSidebar = 268
	minSidebar     = 200
	maxSidebar     = 440
)

// connectAttempt tells whether the app still wants a connection, which a
// user may have cancelled.
type connectAttempt struct {
	cancelled atomic.Bool
}

// hostRuntime is what the app holds of a host while it connects or is
// connected: its terminal, its SSH session, and where it stands.
type hostRuntime struct {
	state    connState
	err      string
	term     *terminal.Terminal
	ssh      *sshx.Session
	password string
	gen      int
	title    string
	attempt  *connectAttempt
}

// App is the app's state. It lives on the main thread; goroutines change
// it through update.
type App struct {
	win      *mygo.Window
	state    store.State
	keys     kh.Store
	runtimes map[string]*hostRuntime
	handles  map[string]*ui.Handle
	selected string

	editor    *editorState
	groupEdit *groupEditState
	password  *passwordState
	hostKey   *hostKeyState
	confirm   *confirmState

	chrome      chrome
	sidebarDrag float32
	toast       string
	menuBar     *mygo.Menu
	focusWanted string
	collapsed   map[string]bool
	dashScroll  ui.ScrollState

	tray          *mygo.Tray // the icon in the notification area, while it shows
	quitting      bool       // a quit is on its way: let the windows close
	quitConfirmed bool       // the user answered the quit's question

	setWin  *mygo.Window
	ss      settingsState
	applied settingsValues
	// fontPick is the font the settings show: the system font's label, or
	// the family's name.
	fontPick    string
	openAtLogin bool // the login item, as last read from the system
	updates     updatePrefs

	askMu sync.Mutex // one host key question at a time
}

func newApp() *App {
	return &App{
		runtimes: map[string]*hostRuntime{},
		handles:  map[string]*ui.Handle{},
	}
}

func (a *App) load() {
	s, err := store.Load()
	if err != nil {
		a.toast = err.Error()
	}
	if s.SidebarWidth == 0 {
		s.SidebarWidth = defaultSidebar
	}
	s.SidebarWidth = min(max(s.SidebarWidth, minSidebar), maxSidebar)
	if _, ok := s.Host(s.Selected); !ok {
		s.Selected = ""
		if len(s.Hosts) > 0 {
			s.Selected = s.Hosts[0].ID
		}
	}
	a.state = s
	a.selected = s.Selected
	a.state.Theme = knownTheme(a.state.Theme)
	setLooks(a.state.Theme)
	a.applied = valuesOf(a.state)
	if dir, err := store.Dir(); err == nil {
		a.keys = kh.Store{Path: filepath.Join(dir, "known_hosts")}
	}
}

func (a *App) update(fn func()) {
	if a.win != nil {
		a.win.Update(fn)
	}
}

func (a *App) saveState() {
	a.state.Selected = a.selected
	if err := a.state.Save(); err != nil {
		a.toast = err.Error()
	}
}

// recordRecent remembers a host as connected just now.
func (a *App) recordRecent(id string) {
	a.state.RecordRecent(id)
	a.saveState()
}

func (a *App) runtime(id string) *hostRuntime {
	rt := a.runtimes[id]
	if rt == nil {
		rt = &hostRuntime{}
		a.runtimes[id] = rt
	}
	return rt
}

func (a *App) selectedHost() (store.Host, bool) {
	if a.selected == "" {
		return store.Host{}, false
	}
	return a.state.Host(a.selected)
}

func (a *App) selectHost(id string) {
	if a.selected == id {
		a.focusTerminal(id)
		return
	}
	a.selected = id
	a.focusWanted = id
	a.saveState()
	a.updateTitle()
	a.focusTerminal(id)
}

func (a *App) focusTerminal(id string) {
	if h := a.handles[id]; h != nil {
		h.Focus()
	}
}

func (a *App) handleFor(id string) *ui.Handle {
	h := a.handles[id]
	if h == nil {
		h = &ui.Handle{}
		a.handles[id] = h
	}
	return h
}

func (a *App) sidebarWidth() float32 {
	return float32(math.Round(float64(min(max(a.state.SidebarWidth, minSidebar), maxSidebar))))
}

func (a *App) toggleSidebar() {
	a.state.SidebarHidden = !a.state.SidebarHidden
	a.saveState()
	a.updateSidebarMenu()
	if !a.state.SidebarHidden {
		a.focusWanted = a.selected
		a.focusTerminal(a.selected)
	}
}

func (a *App) updateSidebarMenu() {
	if a.menuBar == nil {
		return
	}
	if item := a.menuBar.ItemByID("sidebar"); item != nil {
		item.SetLabel(a.sidebarMenuLabel())
	}
}

func (a *App) sidebarMenuLabel() string {
	if a.state.SidebarHidden {
		return locales.ToggleShowSidebar()
	}
	return locales.ToggleHideSidebar()
}

func (a *App) updateTitle() {
	if a.win == nil {
		return
	}
	if a.showsDashboard() {
		a.win.SetTitle("sshgo")
		return
	}
	title := "sshgo"
	if h, ok := a.selectedHost(); ok {
		title = h.Name + " — sshgo"
		if rt := a.runtimes[h.ID]; rt != nil && rt.state == connConnected && rt.title != "" {
			title = rt.title + " — " + h.Name
		}
	}
	a.win.SetTitle(title)
}

// showsDashboard reports whether the card holds the dashboard: no shell,
// no connection on its way, and no failure to report.
func (a *App) showsDashboard() bool {
	h, ok := a.selectedHost()
	if !ok {
		return true
	}
	rt := a.runtimes[h.ID]
	if rt == nil {
		return true
	}
	switch rt.state {
	case connConnecting, connFailed:
		return false
	}
	return rt.term == nil || (rt.state != connConnected && rt.state != connEnded)
}

func (a *App) connectedCount() int {
	n := 0
	for _, rt := range a.runtimes {
		if rt.state == connConnected {
			n++
		}
	}
	return n
}

func (a *App) anyDialogOpen() bool {
	return (a.editor != nil && a.editor.open) || (a.groupEdit != nil && a.groupEdit.open) ||
		(a.password != nil && a.password.open) || (a.hostKey != nil && a.hostKey.open) ||
		(a.confirm != nil && a.confirm.open)
}

func (a *App) connect(id string) {
	h, ok := a.state.Host(id)
	if !ok {
		return
	}
	rt := a.runtime(id)
	switch rt.state {
	case connConnecting, connConnected:
		a.selectHost(id)
		return
	}
	pw := rt.password
	if pw == "" && h.SavePassword {
		b, err := mygo.Secrets.Get(passwordKey(h))
		switch {
		case err == nil:
			pw = string(b)
		case errors.Is(err, mygo.ErrSecretNotFound), errors.Is(err, errors.ErrUnsupported):
		default:
			a.toast = err.Error()
		}
	}
	if pw == "" {
		a.openPassword(h)
		return
	}
	a.beginConnect(h, pw)
}

func (a *App) beginConnect(h store.Host, password string) {
	rt := a.runtime(h.ID)
	a.resetRuntime(h.ID)
	rt.gen++
	gen := rt.gen
	attempt := &connectAttempt{}
	rt.attempt = attempt
	rt.state = connConnecting
	rt.err = ""
	rt.password = password
	a.selected = h.ID
	a.focusWanted = h.ID
	a.saveState()
	a.updateTitle()
	go func() {
		sess, err := sshx.Connect(context.Background(), sshx.Config{
			Address:         h.Endpoint(),
			User:            h.User,
			Password:        password,
			HostKeyCallback: a.hostKeyCallback(h, attempt),
		})
		if err != nil {
			a.update(func() {
				if rt.gen != gen {
					return
				}
				rt.state = connFailed
				rt.err = a.connectError(h, err)
				if errors.Is(err, sshx.ErrAuth) {
					rt.password = ""
					a.forgetSavedPassword(h)
				}
			})
			return
		}
		var term *terminal.Terminal
		light, dark := termThemes()
		term, err = terminal.New(terminal.Options{
			Conn:         sess,
			Font:         a.termFont(),
			Theme:        light,
			DarkTheme:    dark,
			Cursor:       a.termCursor(),
			NoBlink:      a.state.NoBlink,
			Transparent:  true,
			Padding:      terminal.Padding{X: 16, Y: 6},
			PaddingColor: terminal.PaddingExtend,
			OnTitle: func(title string) {
				a.update(func() {
					if term == nil || rt.term != term {
						return
					}
					rt.title = title
					a.updateTitle()
				})
			},
			OnExit: func(code int) {
				a.update(func() {
					if term == nil || rt.term != term {
						return
					}
					rt.state = connEnded
					rt.err = locales.ErrorSessionEnded(strconv.Itoa(code))
					rt.ssh = nil
				})
			},
		})
		if err != nil {
			sess.Close()
			a.update(func() {
				if rt.gen != gen {
					return
				}
				rt.state = connFailed
				rt.err = locales.ErrorTerminal(err.Error())
			})
			return
		}
		a.update(func() {
			if rt.gen != gen {
				term.Close()
				sess.Close()
				return
			}
			rt.term = term
			rt.ssh = sess
			rt.err = ""
			select {
			case <-term.Done():
				// The shell ended before its terminal reached the view.
				rt.state = connEnded
				rt.err = locales.ErrorSessionEnded(strconv.Itoa(term.ExitCode()))
				rt.ssh = nil
			default:
				rt.state = connConnected
			}
			a.recordRecent(h.ID)
			a.updateTitle()
			a.focusTerminal(h.ID)
		})
	}()
}

// resetRuntime lets go of a host's terminal and session, if it has them,
// and makes any connection still on its way stale.
func (a *App) resetRuntime(id string) {
	rt := a.runtimes[id]
	if rt == nil {
		return
	}
	rt.gen++
	attempt := rt.attempt
	if attempt != nil {
		attempt.cancelled.Store(true)
	}
	term, sess := rt.term, rt.ssh
	rt.term, rt.ssh = nil, nil
	rt.state = connIdle
	rt.err = ""
	rt.title = ""
	if hk := a.hostKey; hk != nil && hk.attempt == attempt {
		a.answerHostKey(false)
	}
	if term != nil {
		term.Close()
	}
	if sess != nil {
		sess.Close()
	}
}

func (a *App) disconnect(id string) {
	a.resetRuntime(id)
	a.updateTitle()
}

func (a *App) closeAll() {
	for id := range a.runtimes {
		a.resetRuntime(id)
	}
}

// quitNeedsConfirm reports whether a quit should ask first: the settings
// say so, sessions are open, and this is not the quit the user answered.
func (a *App) quitNeedsConfirm() bool {
	if !a.state.ConfirmQuit || a.quitConfirmed || a.connectedCount() == 0 {
		return false
	}
	return a.win != nil && !a.win.IsDestroyed()
}

// askQuit shows the window and asks before the quit disconnects the
// sessions that are open.
func (a *App) askQuit() {
	a.showWindow()
	a.confirm = &confirmState{open: true, quit: true}
	if a.win != nil {
		a.win.Invalidate()
	}
}

// quitNow quits as the user confirmed, without asking again.
func (a *App) quitNow() {
	a.quitConfirmed = true
	mygo.App.Quit()
}

// connectLast connects the host selected last when sshgo starts, as the
// settings say.
func (a *App) connectLast() {
	if h, ok := a.selectedHost(); ok {
		a.connect(h.ID)
	}
}

func (a *App) connectError(h store.Host, err error) string {
	switch {
	case errors.Is(err, sshx.ErrAuth):
		return locales.ErrorAuth()
	case errors.Is(err, sshx.ErrHostKeyRejected), errors.Is(err, kh.ErrUnknown), errors.Is(err, kh.ErrChanged):
		return locales.ErrorHostKey()
	default:
		return locales.ErrorNetwork(h.Address(), err.Error())
	}
}

// hostKeyCallback checks the host's key against the keys trusted before,
// and asks the user about one it does not know or that changed.
func (a *App) hostKeyCallback(h store.Host, attempt *connectAttempt) ssh.HostKeyCallback {
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		if attempt.cancelled.Load() {
			return sshx.ErrHostKeyRejected
		}
		if a.keys.Path == "" {
			return fmt.Errorf("sshgo: no known_hosts file")
		}
		err := a.keys.Verify(hostname, key)
		if err == nil {
			return nil
		}
		if !errors.Is(err, kh.ErrUnknown) && !errors.Is(err, kh.ErrChanged) {
			return err
		}
		var known []ssh.PublicKey
		var changed *kh.ChangedError
		if errors.As(err, &changed) {
			known = changed.Known
		}
		if !a.askHostKey(h, hostname, key, known, attempt) {
			return sshx.ErrHostKeyRejected
		}
		if attempt.cancelled.Load() {
			return sshx.ErrHostKeyRejected
		}
		return a.keys.Trust(hostname, key, errors.Is(err, kh.ErrChanged))
	}
}

func (a *App) askHostKey(h store.Host, addr string, key ssh.PublicKey, known []ssh.PublicKey, attempt *connectAttempt) bool {
	a.askMu.Lock()
	defer a.askMu.Unlock()
	answer := make(chan bool, 1)
	a.update(func() {
		a.showWindow()
		a.hostKey = &hostKeyState{open: true, host: h, addr: addr, key: key, known: known, attempt: attempt, answer: answer}
	})
	return <-answer
}

func (a *App) answerHostKey(trust bool) {
	hk := a.hostKey
	if hk == nil {
		return
	}
	a.hostKey = nil
	select {
	case hk.answer <- trust:
	default:
	}
}

func passwordKey(h store.Host) string { return "host-" + h.ID }

// termFont is the terminal's text, as the settings choose it.
func (a *App) termFont() terminal.Font {
	return terminal.Font{Family: a.state.FontFamily, Size: float32(a.state.FontSize)}
}

// termCursor is the cursor shape the settings choose.
func (a *App) termCursor() terminal.CursorStyle {
	switch a.state.Cursor {
	case "bar":
		return terminal.CursorBar
	case "underline":
		return terminal.CursorUnderline
	}
	return terminal.CursorBlock
}

func (a *App) forgetSavedPassword(h store.Host) {
	_ = mygo.Secrets.Delete(passwordKey(h))
	if idx := a.state.Index(h.ID); idx >= 0 && a.state.Hosts[idx].SavePassword {
		a.state.Hosts[idx].SavePassword = false
		a.saveState()
	}
}

func (a *App) savePassword(h store.Host, password string) bool {
	if err := mygo.Secrets.Set(passwordKey(h), []byte(password)); err != nil {
		a.toast = locales.PasswordSaveFailed(err.Error())
		return false
	}
	return true
}
