package main

// The surfaces follow kero (MIT): https://github.com/egoist/kero

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// Surfaces stand out by their color and an edge, crisp, on whole pixels.

// raised lifts a surface off what is under it a little: the card, the
// selected row, buttons.
func raised(e ui.Element, pal *palette) ui.Element {
	if pal.dark {
		return darkShadow(litEdge(e, pal))
	}
	return e.Shadow(0, 1, 2, 0, pal.cardShadow).Shadow(0, 1, 6, 0, pal.cardShadow2)
}

// paned edges a pane's card flat on the wash: in the light a hairline and
// no shadow; in the dark, as raised.
func paned(e ui.Element, pal *palette) ui.Element {
	if pal.dark {
		return raised(e, pal)
	}
	return e.Border(0.5, ui.RGBA(0, 0, 0, 0.106)).InsetShadow(0, 0, 0, 0.5, ui.RGBA(0, 0, 0, 0.063))
}

// chosen lifts the chosen row of a list a hair.
func chosen(e ui.Element, pal *palette) ui.Element {
	if pal.dark {
		return raised(e, pal)
	}
	return e.Shadow(0, 0, 0, 0.5, ui.RGBA(0, 0, 0, 0.06)).Shadow(0, 0.5, 2, 0, ui.RGBA(0, 0, 0, 0.05))
}

// floating lifts a surface above the window: dialogs.
func floating(e ui.Element, pal *palette) ui.Element {
	if pal.dark {
		return darkShadow(litEdge(e, pal)).Shadow(0, 16, 40, 0, pal.floatShadow)
	}
	return e.Shadow(0, 1, 3, 0, pal.cardShadow).Shadow(0, 16, 40, 0, pal.floatShadow)
}

// litEdge draws a dark surface's edge as if lit from above.
func litEdge(e ui.Element, pal *palette) ui.Element {
	return e.Border(1, pal.text.Alpha(0.12)).BorderLinearGradient(ui.LinearGradient{
		Angle: 180, From: pal.text.Alpha(0.18), To: pal.text.Alpha(0.06),
	})
}

// darkShadow is a surface shadow in the dark: three tight layers.
func darkShadow(e ui.Element) ui.Element {
	return e.Shadow(0, 1, 1, 0, ui.RGBA(0, 0, 0, 0.2)).Shadow(0, 2, 2, -1, ui.RGBA(0, 0, 0, 0.2)).Shadow(0, 4, 6, -3, ui.RGBA(0, 0, 0, 0.24))
}

// face styles a control's face.
func face(e ui.Element, pal *palette, bg ui.Color) ui.Element {
	e.Background(bg)
	if pal.controlShadow {
		return e.Shadow(0, 0.5, 1.5, 0, ui.RGBA(0, 0, 0, 0.13)).Shadow(0, 0, 0, 0.5, ui.RGBA(0, 0, 0, 0.05))
	}
	return darkShadow(litEdge(e, pal))
}

// button is a raised surface, or the accent's for the primary one.
func button(c *ui.Context, key, label, keys string, primary bool, pal *palette) bool {
	return sizedButtonAt(c, key, label, keys, primary, 30, 8, pal, false)
}

// defaultButton is button, focused when its dialog first appears, so that
// Return presses it.
func defaultButton(c *ui.Context, key, label, keys string, primary bool, pal *palette) bool {
	return sizedButtonAt(c, key, label, keys, primary, 30, 8, pal, true)
}

func sizedButtonAt(c *ui.Context, key, label, keys string, primary bool, height, radius float32, pal *palette, focus bool) bool {
	b := ui.ButtonBase(c.Key(key)).Height(height).Padding(0, 14).Gap(8).Radius(radius).AlignItems(ui.Center).Label(label)
	if focus {
		b.AutoFocus()
	}
	bg, fg := pal.control, pal.text
	if primary {
		bg, fg = pal.accent, ui.Hex("#ffffff")
	}
	switch {
	case b.Pressed():
		bg = bg.Mix(ui.Hex("#000000"), 0.08)
	case b.Hovered() && primary:
		bg = bg.Mix(ui.Hex("#ffffff"), 0.08)
	case b.Hovered():
		bg = pal.controlHover
	}
	face(b, pal, b.AnimateColor("bg", bg, ui.Animation{Duration: 90 * time.Millisecond})).TextColor(fg)
	b.Children(func() {
		ui.Text(c, label).FontSize(13).FontWeight(500).SingleLine()
		if keys != "" {
			ui.Text(c, keys).FontSize(12).TextColor(fg.Alpha(0.55)).SingleLine()
		}
	})
	return b.Clicked()
}

// dangerButton is a button for what cannot be undone.
func dangerButton(c *ui.Context, key, label string, pal *palette) bool {
	b := ui.ButtonBase(c.Key(key)).Height(30).Padding(0, 14).Radius(8).AlignItems(ui.Center).Label(label)
	bg := pal.danger
	if b.Pressed() {
		bg = bg.Mix(ui.Hex("#000000"), 0.10)
	} else if b.Hovered() {
		bg = bg.Mix(ui.Hex("#ffffff"), 0.08)
	}
	face(b, pal, b.AnimateColor("bg", bg, ui.Animation{Duration: 90 * time.Millisecond})).TextColor(ui.Hex("#ffffff"))
	b.Children(func() { ui.Text(c, label).FontSize(13).FontWeight(500).SingleLine() })
	return b.Clicked()
}

// toggle is sshgo's switch: a track that shows on and off alike, the
// accent's when on, with a knob that springs across.
func toggle(c *ui.Context, on *bool, pal *palette) ui.Element {
	sw := ui.SwitchBase(c, on).Size(38, 22).Radius(11).Padding(2).Shrink(0)
	track := pal.text.Alpha(0.14)
	if pal.dark {
		track = ui.RGBA(255, 255, 255, 0.2)
	}
	if *on {
		track = pal.accent
	}
	sw.Background(sw.AnimateColor("track", track, ui.Animation{Duration: 150 * time.Millisecond}))
	x := sw.AnimateTo("knob", b2f(*on)*16, ui.Animation{Spring: ui.SpringOptions{Duration: 260 * time.Millisecond, Bounce: 0.15}})
	sw.Children(func() {
		ui.Box(c).Size(18, 18).Radius(9).Background(ui.Hex("#ffffff")).Translate(x, 0).
			Shadow(0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.22)).Shadow(0, 0, 0, 0.5, ui.RGBA(0, 0, 0, 0.06))
	})
	return sw
}

// iconButton is a quiet square button with an icon, as in a toolbar.
func iconButton(c *ui.Context, key string, svg *ui.SVG, label string, pal *palette, fn func()) {
	b := ui.ButtonBase(c.Key(key)).Size(28, 26).Radius(6).Center().TextColor(pal.textMuted).Label(label).Tooltip(label)
	if b.Pressed() {
		b.Background(pal.rowHover.Alpha(0.12))
	} else if b.Hovered() {
		b.Background(pal.rowHover)
		b.TextColor(pal.text)
	}
	b.Children(func() { ui.Icon(c, svg).Size(16, 16) })
	if b.Clicked() {
		fn()
	}
}
