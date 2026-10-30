// sshgo is a small SSH client of native UI: hosts in a sidebar, their
// shells in a card beside it, with kero's look.
package main

import (
	"log"
	"runtime"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"sshgo/locales"
)

func main() {
	a := newApp()
	a.load()
	mygo.Theme.SetSource(mygo.ThemeSource(a.state.Appearance))
	mygo.App.SetLocale(localeTag(a.state.Language))
	mygo.App.WhenReady(func() {
		a.win = mygo.NewWindow(framed(mygo.WindowOptions{
			Title:     "sshgo",
			Width:     1180,
			Height:    760,
			MinWidth:  680,
			MinHeight: 520,
			StateKey:  "main",
			Content:   ui.View(a.view),
		}))
		a.updateTitle()
		mygo.App.SetMenu(a.buildMenu())
	})
	mygo.App.OnWindowAllClosed(func() { mygo.App.Quit() })
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { a.closeAll() })
	mygo.App.OnLocaleChanged(func(string) {
		a.fontPick = fontLabel(a.state.FontFamily)
		mygo.App.SetMenu(a.buildMenu())
		a.updateTitle()
		if a.win != nil {
			a.win.Invalidate()
		}
		if a.setWin != nil {
			a.setWin.SetTitle(locales.SettingsTitle())
			a.setWin.Invalidate()
		}
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

func (a *App) buildMenu() *mygo.Menu {
	file := []*mygo.MenuItem{
		{Label: locales.ActionSettings(), Accelerator: "CmdOrCtrl+,", Click: func(*mygo.MenuItem, *mygo.Window) {
			a.openSettings()
		}},
		mygo.Separator(),
		{Label: locales.ActionNewHost(), Accelerator: "CmdOrCtrl+Shift+N", Click: func(*mygo.MenuItem, *mygo.Window) {
			a.openEditor("")
		}},
		{Label: locales.ActionConnect(), Accelerator: "CmdOrCtrl+Shift+Enter", Click: func(*mygo.MenuItem, *mygo.Window) {
			if h, ok := a.selectedHost(); ok {
				a.connect(h.ID)
			}
		}},
		mygo.Separator(),
	}
	// Cmd+W closes the window on macOS; Windows keeps Ctrl+W for the
	// shell, which deletes a word.
	if runtime.GOOS == "darwin" {
		file = append(file, &mygo.MenuItem{Role: mygo.RoleClose})
	}
	m := mygo.NewMenu([]*mygo.MenuItem{
		{Role: mygo.RoleAppMenu},
		{Label: locales.MenuFile(), Submenu: file},
		{Role: mygo.RoleEditMenu},
		{Label: locales.MenuView(), Submenu: []*mygo.MenuItem{
			{ID: "sidebar", Label: a.sidebarMenuLabel(), Accelerator: "CmdOrCtrl+Shift+B", Click: func(*mygo.MenuItem, *mygo.Window) {
				a.toggleSidebar()
			}},
			mygo.Separator(),
			{Role: mygo.RoleToggleFullScreen},
		}},
		{Role: mygo.RoleWindowMenu},
	})
	a.menuBar = m
	return m
}
