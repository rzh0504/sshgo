package main

import (
	_ "embed"
	"log"

	"github.com/egoist/mygo"

	"sshgo/locales"
)

// trayIcon is sshgo's icon in the notification area, while it runs there.
//
//go:embed assets/icons/tray.png
var trayIcon []byte

// closeToTray reports whether the main window's close button leaves sshgo
// running in the notification area.
func (a *App) closeToTray() bool {
	return a.state.CloseBehavior == "tray"
}

// windowClosing is the main window's close button: in the tray, the window
// hides with its terminals, and the app keeps running until the tray icon
// opens it again. Otherwise the window's close ends the app, which asks
// first when sessions are open. A quit closes the windows too, which
// passes through.
func (a *App) windowClosing(e *mygo.CloseEvent) {
	if a.quitting {
		return
	}
	if a.closeToTray() && a.tray != nil {
		e.PreventDefault()
		e.Window.Hide()
		return
	}
	if a.quitNeedsConfirm() {
		e.PreventDefault()
		a.askQuit()
	}
}

// applyTray shows or removes the icon, as the settings say.
func (a *App) applyTray() {
	show := a.closeToTray() && !a.quitting
	switch {
	case show && a.tray == nil:
		t, err := mygo.NewTray(mygo.TrayOptions{
			ID:      "sshgo",
			Icon:    trayIcon,
			ToolTip: mygo.App.Name(),
			Menu:    a.trayMenu(),
		})
		if err != nil {
			log.Println("sshgo: tray:", err)
			return
		}
		// Windows' left click, where the right one opens the menu.
		t.OnClick(a.showWindow)
		a.tray = t
	case !show && a.tray != nil:
		a.tray.Destroy()
		a.tray = nil
	}
}

// applyCloseBehavior follows a change of the close button's behavior: the
// window comes back when sshgo stops keeping an icon to reopen it from.
func (a *App) applyCloseBehavior() {
	if !a.closeToTray() && a.win != nil && !a.win.IsVisible() {
		a.showWindow()
	}
	a.applyTray()
}

// trayMenu is the icon's menu: the window, Settings, and quitting.
func (a *App) trayMenu() *mygo.Menu {
	return mygo.NewMenu([]*mygo.MenuItem{
		{Label: locales.TrayOpen(mygo.App.Name()), Click: func(*mygo.MenuItem, *mygo.Window) { a.showWindow() }},
		{Label: locales.ActionSettings(), Click: func(*mygo.MenuItem, *mygo.Window) {
			mygo.App.Focus()
			a.openSettings()
		}},
		mygo.Separator(),
		{Label: locales.TrayQuit(mygo.App.Name()), Click: func(*mygo.MenuItem, *mygo.Window) { mygo.App.Quit() }},
	})
}

// showWindow brings the main window back from the tray, or the taskbar.
func (a *App) showWindow() {
	if a.win == nil || a.win.IsDestroyed() {
		return
	}
	if a.win.IsMinimized() {
		a.win.Restore()
	}
	a.win.Show()
	a.win.Focus()
	mygo.App.Focus()
}
