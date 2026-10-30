package main

import (
	"runtime"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"sshgo/locales"
)

// ownControls tells that sshgo draws the window controls itself: on
// Windows and Linux the card's header holds minimize, maximize and close.
// macOS keeps its traffic lights.
var ownControls = runtime.GOOS != "darwin"

// framed sets a window's frame: the title bar hidden, under the traffic
// lights on macOS and without its buttons elsewhere, and the sidebar's
// material behind the window's wash. The material is always created, as
// Windows shows one only in a window created with one; a setting removes
// it when the window should stay opaque.
func framed(o mygo.WindowOptions) mygo.WindowOptions {
	o.Vibrancy = mygo.VibrancySidebar
	if ownControls {
		o.TitleBarStyle = mygo.TitleBarHidden
		o.HideWindowButtons = true
		o.AutoHideMenuBar = true
		return o
	}
	o.TitleBarStyle = mygo.TitleBarHiddenInset
	o.TitleBarHeight = topBar
	o.TrafficLightPosition = &mygo.Point{X: 18, Y: barMid - 7}
	return o
}

// vibrancy is the window material the settings choose.
func (a *App) vibrancy() mygo.Vibrancy {
	if a.state.NoVibrancy {
		return mygo.VibrancyNone
	}
	return mygo.VibrancySidebar
}

// applyVibrancy puts the settings' material behind the windows.
func (a *App) applyVibrancy() {
	v := a.vibrancy()
	if a.win != nil {
		a.win.SetVibrancy(v)
	}
	if a.setWin != nil {
		a.setWin.SetVibrancy(v)
	}
}

// windowControls are a window's minimize, maximize and close buttons, at
// the right of a header.
func windowControls(c *ui.Context, pal *palette, w *mygo.Window) {
	if !ownControls || w == nil {
		return
	}
	ui.Row(c.Key("window-controls")).Gap(2).AlignItems(ui.Center).Shrink(0).Margin(0, 0, 0, 6).Children(func() {
		iconButton(c, "minimize", minimizeIcon, locales.WindowMinimize(), pal, w.Minimize)
		maximized := w.IsMaximized()
		label, svg := locales.WindowMaximize(), maximizeIcon
		if maximized {
			label, svg = locales.WindowRestore(), restoreIcon
		}
		m := ui.ButtonBase(c.Key("maximize")).Size(28, 26).Radius(6).Center().TextColor(pal.textMuted).Label(label).MaximizeButton()
		if m.Pressed() {
			m.Background(pal.rowHover.Alpha(0.12))
		} else if m.Hovered() {
			m.Background(pal.rowHover).TextColor(pal.text)
		}
		m.Children(func() { ui.Icon(c, svg).Size(16, 16) })
		if m.Clicked() {
			if maximized {
				w.Unmaximize()
			} else {
				w.Maximize()
			}
		}
		b := ui.ButtonBase(c.Key("close-window")).Size(28, 26).Radius(6).Center().TextColor(pal.textMuted).Label(locales.WindowClose()).Tooltip(locales.WindowClose())
		if b.Hovered() || b.Pressed() {
			b.Background(pal.danger).TextColor(ui.RGB(255, 255, 255))
		}
		b.Children(func() { ui.Icon(c, closeIcon).Size(16, 16) })
		if b.Clicked() {
			w.Close()
		}
	})
}
